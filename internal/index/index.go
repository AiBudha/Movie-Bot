package index

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"autofilterbot/internal/database"
	"autofilterbot/internal/limiter"
	"autofilterbot/internal/model"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/amarnathcjd/gogram/telegram"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// Default API Credentials from tgx, generating your own and setting as env vars is recommended
const (
	DefaultAppID   = 21724
	DefaultAppHash = "3e0cb5efcd52300aec5994fdfc5bdc16"
)

// Operation handles and manages the index operation.
type Operation struct {
	mu sync.Mutex

	*model.Index

	db      database.Database
	log     *zap.Logger
	bot     *gotgbot.Bot
	manager *Manager

	// for accurate calculation of ETA

	startTime        time.Time // time at which this intance of operation was started/resumed
	startMessageID   int64     // msg id at which this intance of operation started/resumed
	mtprotoChannelID int64

	cancelFunc      context.CancelFunc
	completedSignal chan byte // notifies goroutines linked to the operation of completion

	pacingDelayMs int64 // dynamic pacing control

	floodMu        sync.Mutex
	floodWaitUntil time.Time
}

// NewOperation creates a new index operation and context to pass to *Operation.Run.
func (m *Manager) NewOperation(ctx context.Context, i *model.Index, db database.Database, log *zap.Logger, b *gotgbot.Bot) (context.Context, *Operation) {
	ctx2, cancel := context.WithCancel(ctx)
	return ctx2, &Operation{
		Index:           i,
		db:              db,
		log:             log,
		bot:             b,
		manager:         m,
		cancelFunc:      cancel,
		completedSignal: make(chan byte),
		pacingDelayMs:   100, // starts at 100ms pacing delay
	}
}

const (
	defaultBatchSize = 200

	progressUpdateSeconds = 10 // number of seconds after which progress msg should be updated
)

// Run starts the index operation from the given CurrentMessageID until EndMessageID.
func (o *Operation) run(ctx context.Context) {
	// updates the progress msg to a basic start msg, also doubles as a check if the msg exists and can be edited
	startText := fmt.Sprintf("index from %d at %d to %d ...", o.StartMessageID, o.CurrentMessageID, o.EndMessageID)

	if o.CurrentMessageID == o.StartMessageID {
		startText = "Starting " + startText
	} else {
		startText = "Resuming " + startText
	}

	// Use the existing progress message if available, otherwise send a new one
	var progressM *gotgbot.Message
	if o.ProgressMessageID != 0 {
		progressM = &gotgbot.Message{
			MessageId: o.ProgressMessageID,
			Chat:      gotgbot.Chat{Id: o.ProgressMessageChatID},
		}
		
		// Initial status update
		o.pushToDB()
		progressBuilder := o.buildProgressMessage()
		progressBuilder.WriteString("\n<b>⚡️ Indexing in Progress</b>")
		
		limiter.Wait()
		_, _, err := progressM.EditText(o.bot, progressBuilder.String(), &gotgbot.EditMessageTextOpts{
			ParseMode: gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
					{o.PauseButton(), o.CancelButton()},
				},
			},
		})
		if err != nil {
			o.log.Warn("index: failed to edit existing progress message, sending new one", zap.Error(err))
			o.ProgressMessageID = 0 // Trigger fallback
		}
	}

	if o.ProgressMessageID == 0 {
		limiter.Wait()
		var err error
		progressM, err = o.bot.SendMessage(o.ProgressMessageChatID, startText, &gotgbot.SendMessageOpts{})
		if err != nil {
			o.log.Error(err.Error(), zap.String("pid", o.ID), zap.Int64("chat_id", o.ProgressMessageChatID))
			o.bot.SendMessage(o.ProgressMessageChatID, fmt.Sprintf("🛑 Index Stopped: Unable to Update Progress Message: <code>%s</code>", err.Error()), &gotgbot.SendMessageOpts{
				ParseMode:   gotgbot.ParseModeHTML,
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{o.ResumeButton()}}},
			})
			return
		}
	}


	var (
		appID   = DefaultAppID
		appHash = DefaultAppHash
	)

	if s := os.Getenv("APP_ID"); s != "" {
		id, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			o.log.Debug("index: failed to parse app id from environment", zap.Error(err), zap.String("val", s))
		} else {
			if s := os.Getenv("APP_HASH"); s != "" {
				appID = int(id)
				appHash = s
			} else {
				o.log.Warn("index: app id is set but app hash is empty, operation starting using deafult credentials")
			}
		}
	}

	c, err := telegram.NewClient(telegram.ClientConfig{
		AppID:         int32(appID),
		AppHash:       appHash,
		NoUpdates:     true,
		MemorySession: true,
		LogLevel:      telegram.LogError,
		DisableCache:  true,
	})
	if err != nil {
		o.log.Error(fmt.Sprintf("index: create client failed: %v", err), zap.String("pid", o.ID))
		o.bot.SendMessage(o.ProgressMessageChatID, fmt.Sprintf("🛑 Index Stopped: Unable to Create Client: <code>%s</code>", err.Error()), &gotgbot.SendMessageOpts{
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{o.ResumeButton()}}},
		})

		return
	}

	err = c.LoginBot(o.bot.Token)
	if err != nil {
		o.log.Error(fmt.Sprintf("index: login bot failed: %v", err), zap.String("pid", o.ID))
		o.bot.SendMessage(o.ProgressMessageChatID, fmt.Sprintf("🛑 Index Stopped: Unable to Login Bot: <code>%s</code>", err.Error()), &gotgbot.SendMessageOpts{
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{o.ResumeButton()}}},
		})

		return
	}

	_, err = c.GetMe()
	if err != nil {
		o.log.Error(fmt.Sprintf("index: getme failed: %v", err), zap.String("pid", o.ID))
		o.bot.SendMessage(o.ProgressMessageChatID, fmt.Sprintf("🛑 Index Stopped: Unable to Invoke Method: <code>%s</code>", err.Error()), &gotgbot.SendMessageOpts{
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{o.ResumeButton()}}},
		})

		return
	}

	inputChannel, err := getChat(c, o.ChannelID)
	if err != nil {
		o.log.Error(fmt.Sprintf("index: getchat failed: %v", err), zap.String("pid", o.ID), zap.Int64("tdlib_id", o.ChannelID))
		o.bot.SendMessage(o.ProgressMessageChatID, fmt.Sprintf("🛑 Index Stopped: Unable to Get Chat: <code>%s</code>", err.Error()), &gotgbot.SendMessageOpts{
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{o.ResumeButton()}}},
		})

		return
	}

	msgChan := make(chan []telegram.Message, 50) // buffer 50 batches
	var processorWG sync.WaitGroup

	// Launch multiple workers for processing
	for i := 0; i < 10; i++ {
		processorWG.Add(1)
		go o.MessageProcessor(ctx, msgChan, &processorWG)
	}

	updateTicker := time.NewTicker(time.Second * time.Duration(progressUpdateSeconds))

	o.startTime = time.Now()
	o.startMessageID = o.CurrentMessageID
	o.mtprotoChannelID = inputChannel.ChannelID

	// updates progress msg and sync db, dettatched from index operation for real time updates
	// ticker may need to be adjusted in case of msg edit floods
	if o.EndMessageID == 0 {
		// Resolve end message id using mtproto client
		history, err := c.MessagesGetHistory(&telegram.MessagesGetHistoryParams{
			Peer:  &telegram.InputPeerChannel{ChannelID: inputChannel.ChannelID, AccessHash: inputChannel.AccessHash},
			Limit: 1,
		})
		if err != nil {
			o.log.Error(fmt.Sprintf("index: resolve end id failed: %v", err), zap.String("pid", o.ID))
		} else {
			switch h := history.(type) {
			case *telegram.MessagesChannelMessages:
				if len(h.Messages) > 0 {
					o.EndMessageID = int64(h.Messages[0].(*telegram.MessageObj).ID)
				}
			case *telegram.MessagesMessagesObj:
				if len(h.Messages) > 0 {
					o.EndMessageID = int64(h.Messages[0].(*telegram.MessageObj).ID)
				}
			case *telegram.MessagesMessagesSlice:
				if len(h.Messages) > 0 {
					o.EndMessageID = int64(h.Messages[0].(*telegram.MessageObj).ID)
				}
			}
		}
	}

	go func() {
		for {
			select {
			case <-o.completedSignal: // operation complete
				return
			case <-ctx.Done(): // user cancel
				return
			case <-updateTicker.C:
				o.pushToDB()

				progressBuilder := o.buildProgressMessage()

				progressBuilder.WriteString("\n<b>⚡️ Indexing in Progress</b>")

				limiter.Wait()
				_, _, err := progressM.EditText(o.bot, progressBuilder.String(), &gotgbot.EditMessageTextOpts{
					ParseMode: gotgbot.ParseModeHTML,
					ReplyMarkup: gotgbot.InlineKeyboardMarkup{
						InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
							{o.PauseButton(), o.CancelButton()},
						},
					},
					ChatId:    progressM.Chat.Id,
					MessageId: progressM.MessageId,
				})
				if err != nil {
					o.log.Debug(fmt.Sprintf("index: failed to update progress message: %v", err), zap.String("pid", o.ID), zap.Int64("message_id", progressM.MessageId))
				}
			}
		}
	}()

	var fetchWG sync.WaitGroup
	for i := 0; i < 3; i++ { // 3 parallel fetcher workers
		fetchWG.Add(1)
		go func(workerID int) {
			defer fetchWG.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case <-o.completedSignal:
					return
				default:
					messageChunk := o.inputMessageSlice()
					if len(messageChunk) == 0 {
						return
					}

					var rawMsgs interface{}
					var err error
					retries := 0
					for {
						select {
						case <-ctx.Done():
							return
						case <-o.completedSignal:
							return
						default:
						}

						// Check if we are currently in a global floodwait block
						o.floodMu.Lock()
						until := o.floodWaitUntil
						o.floodMu.Unlock()
						if time.Now().Before(until) {
							sleepDur := time.Until(until)
							o.log.Info("index: worker waiting for active global floodwait", zap.Int("worker", workerID), zap.Duration("duration", sleepDur))
							time.Sleep(sleepDur)
						}

						rawMsgs, err = c.ChannelsGetMessages(inputChannel, messageChunk)
						if err == nil {
							break
						}

						s, ok, _ := ParseMtProtoFloodwait(err)
						if ok {
							if s == 0 {
								s = 30 // Safe fallback if parsing fails but error is FLOOD_WAIT
							}
							o.log.Warn("index: floodwait detected, retrying chunk", zap.Int64("seconds", s), zap.Int("worker", workerID))

							// Set global floodwait block
							o.floodMu.Lock()
							wakeTime := time.Now().Add(time.Second * time.Duration(s+1))
							if wakeTime.After(o.floodWaitUntil) {
								o.floodWaitUntil = wakeTime
							}
							o.floodMu.Unlock()

							// Dynamic backoff: Increase pacing delay on floodwait
							o.mu.Lock()
							if o.pacingDelayMs < 2000 {
								o.pacingDelayMs += 250
							}
							o.mu.Unlock()

							time.Sleep(time.Second * time.Duration(s+1))
							continue
						}

						// Non-floodwait error: retry up to 10 times
						retries++
						if retries >= 10 {
							break
						}
						o.log.Error("index: getmessages failed, retrying...", zap.Error(err), zap.Int("worker", workerID))
						time.Sleep(time.Second * 3)
					}

					if err != nil {
						o.log.Error("index: skipping chunk after maximum retries", zap.Error(err), zap.Int("worker", workerID))
						continue
					}

					// Dynamic throttle reduction: Decrease pacing delay slowly on success to find maximum safe rate
					o.mu.Lock()
					if o.pacingDelayMs > 80 {
						o.pacingDelayMs -= 5
					}
					o.mu.Unlock()

					msgs := make([]telegram.Message, 0)
					switch m := rawMsgs.(type) {
					case *telegram.MessagesChannelMessages:
						msgs = m.Messages
					case *telegram.MessagesMessagesObj:
						msgs = m.Messages
					case *telegram.MessagesMessagesSlice:
						msgs = m.Messages
					}

					select {
					case msgChan <- msgs:
						// Successfully pushed to processor
					case <-ctx.Done():
						return
					}

					// Adaptive sleep duration
					o.mu.Lock()
					delay := time.Duration(o.pacingDelayMs) * time.Millisecond
					o.mu.Unlock()
					time.Sleep(delay)
				}
			}
		}(i)
	}

	fetchWG.Wait()
	close(msgChan)
	processorWG.Wait()

	// Notify completion
	close(o.completedSignal)
	if o.manager != nil {
		o.manager.RemoveOperationIfSame(o.ID, o)
	}

	o.pushToDB()

	var (
		text   string
		markup gotgbot.InlineKeyboardMarkup
	)
	select {
	case <-ctx.Done():
		// Paused
		text = o.buildProgressMessage().String() + "\n<b>⏸ Index Operation Paused</b>"
		markup = gotgbot.InlineKeyboardMarkup{
			InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
				{o.ResumeButton(), o.ModifyButton(), o.CancelButton()},
			},
		}
		_ = o.db.SaveIndexedChannel(o.ChannelID, o.CurrentMessageID)
	default:
		// Completed
		text = o.buildProgressMessage().String() + "\n<b>✅ Index Operation Completed</b>"
		o.db.DeleteOperation(o.ID)
		_ = o.db.SaveIndexedChannel(o.ChannelID, o.EndMessageID)
	}

	_, _, err = progressM.EditText(o.bot, text, &gotgbot.EditMessageTextOpts{
		ParseMode:   gotgbot.ParseModeHTML,
		ReplyMarkup: markup,
	})
	if err != nil {
		o.log.Debug("index: failed to update final progress message", zap.Error(err), zap.String("pid", o.ID))
	}
}

// pushToDB updates the progress of the operation in the database. Errors are output to logger.
func (o *Operation) pushToDB() {
	update := map[string]interface{}{
		"current": o.CurrentMessageID,
		"saved":   o.Saved,
		"failed":  o.Failed,
	}

	_, err := o.db.UpdateIndexOperation(o.ID, update)
	if err != nil {
		o.log.Error(fmt.Sprintf("index: failed to update db values %v", err), zap.String("pid", o.ID))
	}
	_ = o.db.SaveIndexedChannel(o.ChannelID, o.CurrentMessageID)
}

// getChat fetches a channel and it's access hash from it's botapi/tdlib id.
func getChat(client *telegram.Client, id int64) (*telegram.InputChannelObj, error) {
	rawChats, err := client.ChannelsGetChannels([]telegram.InputChannel{&telegram.InputChannelObj{ChannelID: TDLibChannelIDToPlain(id), AccessHash: 0}})
	if err != nil {
		return nil, errors.Wrap(err, "request failed")
	}

	var chats []telegram.Chat

	switch c := rawChats.(type) {
	case *telegram.MessagesChatsObj:
		chats = c.Chats
	case *telegram.MessagesChatsSlice:
		chats = c.Chats
	default:
		return nil, errors.New("unknown chats type")
	}

	if len(chats) == 0 {
		return nil, errors.New("chats list is empty")
	}

	switch c := chats[0].(type) {
	case *telegram.Channel:
		return &telegram.InputChannelObj{ChannelID: c.ID, AccessHash: c.AccessHash}, nil
	case *telegram.ChatObj:
		return &telegram.InputChannelObj{ChannelID: c.ID, AccessHash: 0}, nil // probably should just skip regular chats
	case *telegram.ChannelForbidden:
		return nil, errors.New("channel forbidden")
	case *telegram.ChatEmpty:
		return nil, errors.New("chat empty")
	case *telegram.ChatForbidden:
		return nil, errors.New("chat forbidden")
	default:
		return nil, errors.New(fmt.Sprintf("unknown chat type: %T", c))
	}
}
