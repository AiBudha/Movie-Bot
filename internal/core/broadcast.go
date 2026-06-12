package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"autofilterbot/internal/button"
	"autofilterbot/internal/fsub"
	"autofilterbot/internal/functions"
	"autofilterbot/internal/limiter"
	"autofilterbot/internal/model"
	"autofilterbot/pkg/conversation"
	"autofilterbot/pkg/send"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
)

// Broadcast handles the /broadcast command to show a preview and confirm before sending.
func Broadcast(bot *gotgbot.Bot, ctx *ext.Context) error {
	ok, _ := fsub.CheckFsub(_app, bot, ctx)
	if !ok {
		return nil
	}
	if !_app.AuthAdmin(ctx) {
		return nil
	}

	m := ctx.EffectiveMessage
	var (
		opts      send.SendOpts
		method    send.SendMethod
		methodStr string
	)

	if replyM := m.ReplyToMessage; replyM != nil {
		methodName, sendMethod, text, fileId, err := sendOptsFromMessage(replyM)
		if err != nil {
			m.Reply(bot, "<b>⛔ 𝖱𝖾𝗉𝗅𝗂𝖾𝖽 𝖬𝖾𝗌𝗌𝖺𝗀𝖾 𝖢𝗈𝗇𝗍𝖺𝗂𝗇𝗌 𝖴𝗇𝗌𝗎𝗉𝗉𝗈𝗋𝗍𝖾𝖽 𝖬𝖾𝖽𝗂𝖺!</b>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
			return nil
		}

		method = sendMethod
		methodStr = methodName
		opts.Text += text
		opts.FileId = fileId

		if replyM.ReplyMarkup != nil && len(replyM.ReplyMarkup.InlineKeyboard) != 0 {
			opts.Keyboard = append(opts.Keyboard, replyM.ReplyMarkup.InlineKeyboard...)
		}
	}

	if ctx.CallbackQuery == nil {
		split := strings.SplitN(m.OriginalHTML(), " ", 2)
		if len(split) > 1 {
			opts.Text += " " + split[1]
			if method == nil {
				method = send.SendMessage
				methodStr = "text"
			}
		}
	}

	if method == nil {
		promptMsg, err := conversation.NewConversatorFromUpdate(bot, ctx.Update).Ask(_app.Ctx, "<b>𝖯𝗅𝖾𝖺𝗌𝖾 𝖲𝖾𝗇𝖽 𝗍𝗁𝖾 𝖬𝖾𝗌𝗌𝖺𝗀𝖾 𝗍𝗈 𝖻𝖾 𝖡𝗋𝗈𝖺𝖽𝖼𝖺𝗌𝗍𝖾𝖽:</b>", &gotgbot.SendMessageOpts{
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{Text: "❌ Cancel", CallbackData: "admin:cancel"}}},
			},
			ParseMode: gotgbot.ParseModeHTML,
		})
		if err != nil {
			return nil
		}

		methodName, sendMethod, text, fileId, err := sendOptsFromMessage(promptMsg)
		if err != nil {
			promptMsg.Reply(bot, "<b>⛔ 𝖬𝖾𝗌𝗌𝖺𝗀𝖾 𝖢𝗈𝗇𝗍𝖺𝗂𝗇𝗌 𝖴𝗇𝗌𝗎𝗉𝗉𝗈𝗋𝗍𝖾𝖽 𝖬𝖾𝖽𝗂𝖺!</b>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
			return nil
		}

		method = sendMethod
		methodStr = methodName
		opts.Text += text
		opts.FileId = fileId

		if promptMsg.ReplyMarkup != nil && len(promptMsg.ReplyMarkup.InlineKeyboard) != 0 {
			opts.Keyboard = append(opts.Keyboard, promptMsg.ReplyMarkup.InlineKeyboard...)
		}
	}

	parsedText, keyboard, err := button.ParseFromText(opts.Text)
	if err != nil {
		m.Reply(bot, fmt.Sprintf("<b>𝖯𝖺𝗋𝗌𝗂𝗇𝗀 𝖡𝗎𝗍𝗍𝗈𝗇𝗌 𝖥𝖺𝗂𝗅𝖾𝖽 🙁</b>\nError: <code>%s</code>", err.Error()), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		_app.Log.Debug("broadcast: parse buttons failed", zap.Error(err), zap.String("text", opts.Text))
		return nil
	}
	opts.Text = parsedText
	opts.Keyboard = append(opts.Keyboard, button.UnwrapKeyboard(keyboard)...)

	// Fetch user count for confirmation message info
	userIds, err := _app.DB.GetAllUserIDs()
	if err != nil {
		m.Reply(bot, "Fetch Users From Database Failed :/", nil)
		return nil
	}
	totalUsers := len(userIds)

	// Create pending broadcast in DB
	bId := fmt.Sprintf("bc_%d", time.Now().UnixNano())
	bRecord := &model.Broadcast{
		ID:             bId,
		Text:           opts.Text,
		FileId:         opts.FileId,
		Method:         methodStr,
		InlineKeyboard: opts.Keyboard,
		CreatedAt:      time.Now(),
		Status:         "pending",
		Total:          totalUsers,
	}

	err = _app.DB.SaveBroadcast(bRecord)
	if err != nil {
		m.Reply(bot, fmt.Sprintf("Failed to save pending broadcast: %v", err), nil)
		return nil
	}

	// Send Preview
	previewMsg, err := method(bot, m.Chat.Id, &opts)
	if err != nil {
		m.Reply(bot, fmt.Sprintf("<b>⛔ Failed to generate preview:</b> <code>%s</code>", err.Error()), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		_app.DB.DeleteBroadcast(bId)
		return nil
	}

	// Send Confirm Panel
	confirmText := fmt.Sprintf(`<b>☝️ ABOVE IS THE PREVIEW OF THE BROADCAST</b>

<b>📢 Broadcast ID:</b> <code>%s</code>
<b>📝 Message Type:</b> <code>%s</code>
<b>👥 Target Users:</b> <code>%d</code>

<i>Click confirm below to start the broadcast:</i>`, bId, methodStr, totalUsers)

	_, err = bot.SendMessage(m.Chat.Id, confirmText, &gotgbot.SendMessageOpts{
		ParseMode: gotgbot.ParseModeHTML,
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{
			InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
				{
					{Text: "⚡ Confirm & Send", CallbackData: "admin:bc_send:" + bId},
					{Text: "❌ Cancel", CallbackData: "admin:bc_cancel:" + bId},
				},
			},
		},
		ReplyParameters: &gotgbot.ReplyParameters{MessageId: previewMsg.MessageId},
	})
	if err != nil {
		_app.Log.Warn("broadcast: send confirm panel failed", zap.Error(err))
	}

	return nil
}

// HandleConfirmBroadcast handles confirmed broadcast and runs it in a goroutine.
func HandleConfirmBroadcast(bot *gotgbot.Bot, ctx *ext.Context, bId string) error {
	c := ctx.CallbackQuery
	b, err := _app.DB.GetBroadcast(bId)
	if err != nil {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Broadcast Not Found ❌", ShowAlert: true})
		return nil
	}

	if b.Status != "pending" {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Broadcast Already Processed ⚠️", ShowAlert: true})
		return nil
	}

	// Edit confirmation message to "Starting broadcast..."
	_, _, err = c.Message.EditText(bot, "<b>Sᴛᴀʀᴛɪɴɢ Bʀᴏᴀᴅᴄᴀsᴛ... 🚀</b>\n<i>Please wait...</i>", &gotgbot.EditMessageTextOpts{
		ParseMode: gotgbot.ParseModeHTML,
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{
			InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{Text: "❌ Cancel Broadcast", CallbackData: "admin:bc_cancel:" + bId}}},
		},
	})
	if err != nil {
		_app.Log.Warn("broadcast: edit status to starting failed", zap.Error(err))
	}

	// Update status in DB
	err = _app.DB.UpdateBroadcast(bId, map[string]interface{}{"status": "sending"})
	if err != nil {
		_app.Log.Warn("broadcast: update status failed", zap.Error(err))
	}

	// Start goroutine to do the broadcast!
	go RunBroadcast(bot, bId, c.Message.GetChat().Id, c.Message.GetMessageId())

	return nil
}

// HandleCancelBroadcast cancels the broadcast (deletes pending or updates status to cancelled).
func HandleCancelBroadcast(bot *gotgbot.Bot, ctx *ext.Context, bId string) error {
	c := ctx.CallbackQuery
	b, err := _app.DB.GetBroadcast(bId)
	if err == nil && b != nil {
		if b.Status == "pending" {
			_app.DB.DeleteBroadcast(bId)
			c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Broadcast Cancelled ❌"})
			_, err = c.Message.Delete(bot, nil)
			return err
		} else if b.Status == "sending" {
			_app.DB.UpdateBroadcast(bId, map[string]interface{}{"status": "cancelled"})
			c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Cancelling Broadcast... ⏳"})
			_, _, err = c.Message.EditText(bot, "<b>Cancelling Broadcast... ⏳</b>\n<i>Please wait for the current send loop to finish...</i>", &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML})
			return err
		}
	}

	c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Broadcast Not Found ❌"})
	return nil
}

type broadcastLimiter struct {
	ch chan struct{}
}

func newBroadcastLimiter(rate int) *broadcastLimiter {
	l := &broadcastLimiter{
		ch: make(chan struct{}, rate),
	}
	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(rate))
		defer ticker.Stop()
		for range ticker.C {
			select {
			case l.ch <- struct{}{}:
			default:
			}
		}
	}()
	return l
}

func (l *broadcastLimiter) Wait() {
	<-l.ch
}

func drawProgressBar(percent float64) string {
	const barWidth = 10
	filled := int(percent * barWidth)
	if filled > barWidth {
		filled = barWidth
	}
	if filled < 0 {
		filled = 0
	}
	bar := ""
	for i := 0; i < filled; i++ {
		bar += "█"
	}
	for i := filled; i < barWidth; i++ {
		bar += "░"
	}
	return bar
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// RunBroadcast executes the broadcast in the background and reports progress.
func RunBroadcast(bot *gotgbot.Bot, bId string, adminChatId int64, progressMessageId int64) {
	b, err := _app.DB.GetBroadcast(bId)
	if err != nil {
		_app.Log.Error("RunBroadcast: failed to get broadcast", zap.String("id", bId), zap.Error(err))
		return
	}

	userIds, err := _app.DB.GetAllUserIDs()
	if err != nil {
		_app.Log.Error("RunBroadcast: failed to get all user IDs", zap.Error(err))
		return
	}

	total := len(userIds)
	if total == 0 {
		_app.DB.UpdateBroadcast(bId, map[string]interface{}{"status": "completed"})
		bot.EditMessageText("<b>Broadcast Completed: No target users found.</b>", &gotgbot.EditMessageTextOpts{
			ChatId: adminChatId, MessageId: progressMessageId, ParseMode: gotgbot.ParseModeHTML,
		})
		return
	}

	// Create a cancellable context for workers
	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()

	// Initialize progress variables (thread-safe using mutex)
	var mu sync.Mutex
	progress := &broadcastProgress{
		total: total,
	}

	startTime := time.Now()
	
	// Create a separate rate limiter for the broadcast (e.g. 20 msgs/sec)
	// This leaves 10 reqs/sec of the Telegram 30 reqs/sec limit for regular users!
	lim := newBroadcastLimiter(20)

	// Channel for user IDs
	jobsChan := make(chan int64, total)
	for _, uId := range userIds {
		jobsChan <- uId
	}
	close(jobsChan)

	var sentMsgs []model.BroadcastMessage
	var sentMsgsMu sync.Mutex

	// Spawn workers
	numWorkers := 15
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	method := MethodFromString(b.Method)
	opts := &send.SendOpts{
		Text:     b.Text,
		FileId:   b.FileId,
		Keyboard: b.InlineKeyboard,
	}

	for w := 0; w < numWorkers; w++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-workerCtx.Done():
					return
				case uId, ok := <-jobsChan:
					if !ok {
						return
					}

					// Dynamic zero-width space padding for anti-ban
					userOpts := *opts
					paddingCount := int((time.Now().UnixNano() / 1000) % 10) + 1
					userOpts.Text = b.Text + strings.Repeat("\u200B", paddingCount)

					var success bool
					var lastErr error
					var sentM *gotgbot.Message

					for attempt := 0; attempt < 3; attempt++ {
						select {
						case <-workerCtx.Done():
							return
						default:
						}

						// Wait for the broadcast rate limiter (does NOT block main global limiter!)
						lim.Wait()

						sentM, lastErr = method(bot, uId, &userOpts)
						if lastErr == nil {
							success = true
							break
						}

						if floodErr, ok := functions.AsFloodWait(lastErr); ok {
							_app.Log.Info("broadcast: flood wait detected, sleeping", zap.Int64("duration_seconds", floodErr.Duration), zap.Int64("user_id", uId))
							// Sleep on flood wait, respecting workerCtx cancel
							select {
							case <-workerCtx.Done():
								return
							case <-time.After(time.Second * time.Duration(floodErr.Duration)):
							}
							attempt--
							continue
						}

						if functions.IsChatNotFoundErr(lastErr) || strings.Contains(lastErr.Error(), "deactivated") {
							break
						}

						select {
						case <-workerCtx.Done():
							return
						case <-time.After(time.Second * time.Duration(attempt+1)):
						}
					}

					mu.Lock()
					if !success {
						progress.failed++
						errStr := lastErr.Error()
						switch {
						case strings.Contains(errStr, "blocked"):
							_app.DB.DeleteUser(uId)
							progress.blocked++
						case strings.Contains(errStr, "deactivated") || strings.Contains(errStr, "deleted"):
							_app.DB.DeleteUser(uId)
							progress.deleted++
						case strings.Contains(errStr, "chat not found"):
							_app.DB.DeleteUser(uId)
							progress.blocked++
						default:
							progress.otherErr++
						}
					} else {
						progress.success++
						if sentM != nil {
							sentMsgsMu.Lock()
							sentMsgs = append(sentMsgs, model.BroadcastMessage{
								UserId:    uId,
								MessageId: sentM.MessageId,
							})
							sentMsgsMu.Unlock()
						}
					}
					mu.Unlock()
				}
			}
		}()
	}

	// Progress reporter loop
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	// Wait group to know when workers finish
	workersDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(workersDone)
	}()

	isCancelled := false
	isStopped := false

	for {
		select {
		case <-workersDone:
			goto finish
		case <-_app.Ctx.Done():
			isStopped = true
			cancelWorkers()
			goto finish
		case <-ticker.C:
			// Fetch broadcast from DB to check for admin cancellation
			currB, err := _app.DB.GetBroadcast(bId)
			if err == nil && currB.Status == "cancelled" {
				isCancelled = true
				cancelWorkers()
				goto finish
			}

			// Get progress snapshot
			mu.Lock()
			snapshot := *progress
			mu.Unlock()

			// Update progress in admin chat
			percent := float64(snapshot.success+snapshot.failed) / float64(snapshot.total)
			pBar := drawProgressBar(percent)

			processed := snapshot.success + snapshot.failed
			var etaStr string
			if processed > 0 {
				elapsed := time.Since(startTime)
				speed := float64(processed) / elapsed.Seconds()
				if speed > 0 {
					remaining := snapshot.total - processed
					eta := time.Duration(float64(remaining)/speed) * time.Second
					etaStr = formatDuration(eta)
				} else {
					etaStr = "Calculating..."
				}
			} else {
				etaStr = "Calculating..."
			}

			msgText := fmt.Sprintf(`<b>📢 Broadcast Progress [%s]</b>
%d / %d processed (%d%%)

<blockquote><b>Success:</b> <code>%d</code>
<b>Failed:</b> <code>%d</code>
   • <i>Blocked:</i> <code>%d</code>
   • <i>Deleted:</i> <code>%d</code>
   • <i>Other:</i> <code>%d</code></blockquote>

<b>⏱️ ETA:</b> <code>%s</code>`, 
				pBar, processed, snapshot.total, int(percent*100),
				snapshot.success, snapshot.failed, snapshot.blocked, snapshot.deleted, snapshot.otherErr,
				etaStr)

			bot.EditMessageText(
				msgText,
				&gotgbot.EditMessageTextOpts{
					ChatId:    adminChatId,
					MessageId: progressMessageId,
					ParseMode: gotgbot.ParseModeHTML,
					ReplyMarkup: gotgbot.InlineKeyboardMarkup{
						InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{Text: "❌ Cancel Broadcast", CallbackData: "admin:bc_cancel:" + bId}}},
					},
				},
			)

			// Save stats to DB
			sentMsgsMu.Lock()
			msgsCopy := make([]model.BroadcastMessage, len(sentMsgs))
			copy(msgsCopy, sentMsgs)
			sentMsgsMu.Unlock()

			_app.DB.UpdateBroadcast(bId, map[string]interface{}{
				"success":       snapshot.success,
				"failed":        snapshot.failed,
				"blocked":       snapshot.blocked,
				"deleted":       snapshot.deleted,
				"other_err":     snapshot.otherErr,
				"sent_messages": msgsCopy,
			})
		}
	}

finish:
	// Wait for workers to cleanly shutdown if cancelled/stopped
	<-workersDone

	// Final status update
	status := "completed"
	if isCancelled {
		status = "cancelled"
	} else if isStopped {
		status = "stopped"
	}

	mu.Lock()
	finalProgress := *progress
	mu.Unlock()

	sentMsgsMu.Lock()
	finalSentMsgs := sentMsgs
	sentMsgsMu.Unlock()

	_app.DB.UpdateBroadcast(bId, map[string]interface{}{
		"status":        status,
		"success":       finalProgress.success,
		"failed":        finalProgress.failed,
		"blocked":       finalProgress.blocked,
		"deleted":       finalProgress.deleted,
		"other_err":     finalProgress.otherErr,
		"sent_messages": finalSentMsgs,
	})

	var kb [][]gotgbot.InlineKeyboardButton
	if finalProgress.success > 0 {
		kb = append(kb, []gotgbot.InlineKeyboardButton{{Text: "🗑️ Delete Messages", CallbackData: "admin:bchist_delmsg:" + bId}})
	}
	kb = append(kb, []gotgbot.InlineKeyboardButton{{Text: "🔙 Back to Panel", CallbackData: "admin:back"}})

	var finalStatusMsg string
	if isCancelled {
		finalStatusMsg = "<b>Broadcast Cancelled By Admin ❌</b>"
	} else if isStopped {
		finalStatusMsg = "<b>Broadcast Stopped Due to Application Stop ⚠️</b>"
	} else {
		finalStatusMsg = "<b>Broadcast Completed Successfully ✅</b>"
	}

	percent := float64(finalProgress.success+finalProgress.failed) / float64(finalProgress.total)
	pBar := drawProgressBar(percent)

	msgText := fmt.Sprintf(`<b>📢 Broadcast Finished [%s]</b>
%d / %d processed (%d%%)

<blockquote><b>Success:</b> <code>%d</code>
<b>Failed:</b> <code>%d</code>
   • <i>Blocked:</i> <code>%d</code>
   • <i>Deleted:</i> <code>%d</code>
   • <i>Other:</i> <code>%d</code></blockquote>

%s`, 
		pBar, finalProgress.success+finalProgress.failed, finalProgress.total, int(percent*100),
		finalProgress.success, finalProgress.failed, finalProgress.blocked, finalProgress.deleted, finalProgress.otherErr,
		finalStatusMsg)

	bot.EditMessageText(
		msgText,
		&gotgbot.EditMessageTextOpts{
			ChatId:      adminChatId,
			MessageId:   progressMessageId,
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: kb},
		},
	)
}

// HandleDeleteBroadcastMessages recall-deletes the broadcasted messages from the users' chats.
func HandleDeleteBroadcastMessages(bot *gotgbot.Bot, ctx *ext.Context, bId string) error {
	c := ctx.CallbackQuery
	b, err := _app.DB.GetBroadcast(bId)
	if err != nil {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Broadcast Not Found ❌", ShowAlert: true})
		return nil
	}

	if len(b.SentMessages) == 0 {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "No Messages to Delete! ⚠️", ShowAlert: true})
		return nil
	}

	c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Starting Deletion... 🗑️"})
	
	// Update status
	_app.DB.UpdateBroadcast(bId, map[string]interface{}{"status": "deleting"})

	// Edit details message to show deleting status
	_, _, err = c.Message.EditText(bot, "<b>🗑️ Starting message deletion for broadcast...</b>\n<i>Please wait...</i>", &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML})
	if err != nil {
		_app.Log.Warn("broadcast: edit status to deleting failed", zap.Error(err))
	}

	// Start deletion in a goroutine
	go RunDeleteBroadcastMessages(bot, bId, c.Message.GetChat().Id, c.Message.GetMessageId())

	return nil
}

// RunDeleteBroadcastMessages does the actual message deletion sequentially with rate limiters.
func RunDeleteBroadcastMessages(bot *gotgbot.Bot, bId string, adminChatId int64, progressMessageId int64) {
	b, err := _app.DB.GetBroadcast(bId)
	if err != nil {
		return
	}

	total := len(b.SentMessages)
	deleted := 0
	failed := 0

	for i, sm := range b.SentMessages {
		limiter.Wait()
		_, err := bot.DeleteMessage(sm.UserId, sm.MessageId, nil)
		if err != nil {
			failed++
		} else {
			deleted++
		}

		if (i+1)%50 == 0 || i+1 == total {
			bot.EditMessageText(
				fmt.Sprintf("<b>🗑️ Deleting Broadcast Messages...</b>\n\nTotal: %d\nDeleted: %d\nFailed: %d", total, deleted, failed),
				&gotgbot.EditMessageTextOpts{
					ChatId:    adminChatId,
					MessageId: progressMessageId,
					ParseMode: gotgbot.ParseModeHTML,
				},
			)
		}
	}

	// Update status and clear sent_messages to release DB memory/storage space
	_app.DB.UpdateBroadcast(bId, map[string]interface{}{
		"status":        "messages_deleted",
		"sent_messages": []model.BroadcastMessage{},
	})

	bot.EditMessageText(
		fmt.Sprintf("<b>✅ Message Deletion Completed !</b>\n\nSuccessfully deleted %d of %d messages.", deleted, total),
		&gotgbot.EditMessageTextOpts{
			ChatId:      adminChatId,
			MessageId:   progressMessageId,
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{Text: "🔙 Back", CallbackData: "admin:bchist_view:" + bId}}}},
		},
	)
}

type broadcastProgress struct {
	total    int
	success  int
	failed   int
	blocked  int
	deleted  int
	otherErr int
}

func newBroadcastProgress() *broadcastProgress {
	return &broadcastProgress{}
}

type broadcastProgressBuilder struct {
	strings.Builder
}

func (b *broadcastProgressBuilder) WriteLn(s string) *broadcastProgressBuilder {
	b.WriteString("\n" + s)
	return b
}

func (p *broadcastProgress) BuildMessage() *broadcastProgressBuilder {
	var b broadcastProgressBuilder

	b.WriteString(fmt.Sprintf(`<b>𝖡𝗋𝗈𝖺𝖽𝖼𝖺𝗌𝗍 𝖯𝗋𝗈𝗀𝗋𝖾𝗌𝗌</b>
𝖳𝗈𝗍𝖺𝗅: %d
𝖲𝗎𝖼𝖼𝖾𝗌𝗌: %d
<blockquote>𝖥𝖺𝗂𝗅𝖾𝖽: %d
	𝖡𝗅𝗈𝖼𝗄𝖾𝖽: %d
	𝖣𝖾𝗅𝖾𝗍𝖾𝖽: %d
	𝖮𝗍𝗁𝖾𝗋: %d</blockquote>`, p.total, p.success, p.failed, p.blocked, p.deleted, p.otherErr))

	return &b
}

// MethodFromString converts a method name string back to SendMethod.
func MethodFromString(s string) send.SendMethod {
	switch s {
	case "document":
		return send.SendDocument
	case "video":
		return send.SendVideo
	case "audio":
		return send.SendAudio
	case "photo":
		return send.SendPhoto
	case "animation":
		return send.SendAnimation
	case "text":
		return send.SendMessage
	default:
		return send.SendMessage
	}
}

// sendOptsFromMessage gets message send message opts from given message.
//
// Error is only returned if message has no supported media or text.
func sendOptsFromMessage(m *gotgbot.Message) (methodName string, method send.SendMethod, text, fileId string, err error) {
	switch {
	case m.Document != nil:
		methodName = "document"
		method = send.SendDocument
		fileId = m.Document.FileId
	case m.Video != nil:
		methodName = "video"
		method = send.SendVideo
		fileId = m.Video.FileId
	case m.Audio != nil:
		methodName = "audio"
		method = send.SendAudio
		fileId = m.Audio.FileId
	case m.Photo != nil:
		methodName = "photo"
		method = send.SendPhoto
		fileId = m.Photo[len(m.Photo)-1].FileId
	case m.Animation != nil:
		methodName = "animation"
		method = send.SendAnimation
		fileId = m.Animation.FileId
	case m.Text != "":
		methodName = "text"
		method = send.SendMessage
		text = m.OriginalHTML()
	default:
		err = errors.New("unsupported media type")
	}

	if m.Caption != "" {
		text = m.OriginalCaptionHTML()
	}

	return
}

// ListBroadcastHistory shows the list of past broadcasts.
func ListBroadcastHistory(bot *gotgbot.Bot, ctx *ext.Context) error {
	c := ctx.CallbackQuery
	list, err := _app.DB.GetAllBroadcasts()
	if err != nil {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Failed to fetch history ❌", ShowAlert: true})
		return nil
	}

	var text string
	var keyboard [][]gotgbot.InlineKeyboardButton

	if len(list) == 0 {
		text = "<b>📂 BROADCAST HISTORY</b>\n\n<i>No past broadcasts found.</i>"
	} else {
		text = "<b>📂 BROADCAST HISTORY</b>\n\n<i>Select a past broadcast to view detailed statistics and management options:</i>"
		
		// Limit to 10 most recent broadcasts for simplicity and speed
		limit := 10
		if len(list) < limit {
			limit = len(list)
		}
		
		for _, b := range list[:limit] {
			timeStr := b.CreatedAt.Format("02 Jan 15:04")
			
			// Try to make a friendly title
			title := b.Text
			if title == "" {
				title = "[" + b.Method + "]"
			}
			if len(title) > 25 {
				title = title[:22] + "..."
			}
			// sanitize newlines and HTML tags from title
			title = strings.ReplaceAll(title, "\n", " ")
			title = strings.ReplaceAll(title, "<b>", "")
			title = strings.ReplaceAll(title, "</b>", "")
			title = strings.ReplaceAll(title, "<i>", "")
			title = strings.ReplaceAll(title, "</i>", "")
			
			statusIcon := "⏳"
			switch b.Status {
			case "completed":
				statusIcon = "✅"
			case "cancelled":
				statusIcon = "❌"
			case "sending":
				statusIcon = "🚀"
			case "deleting":
				statusIcon = "🗑️"
			case "messages_deleted":
				statusIcon = "🧹"
			}
			
			btnText := fmt.Sprintf("%s %s (%s)", statusIcon, title, timeStr)
			keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{{
				Text:         btnText,
				CallbackData: "admin:bchist_view:" + b.ID,
			}})
		}
	}

	keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{{
		Text:         "🔙 Back to Admin",
		CallbackData: "admin:back",
	}})

	_, _, err = c.Message.EditText(bot, text, &gotgbot.EditMessageTextOpts{
		ParseMode:   gotgbot.ParseModeHTML,
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard},
	})
	return err
}

// ViewBroadcastDetails shows details of a specific broadcast.
func ViewBroadcastDetails(bot *gotgbot.Bot, ctx *ext.Context, bId string) error {
	c := ctx.CallbackQuery
	b, err := _app.DB.GetBroadcast(bId)
	if err != nil {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Broadcast Not Found ❌", ShowAlert: true})
		return nil
	}

	timeStr := b.CreatedAt.Format("2006-01-02 15:04:05 MST")
	
	text := fmt.Sprintf(`<b>📊 BROADCAST DETAILS</b>

<b>📢 ID:</b> <code>%s</code>
<b>📅 Sent:</b> <code>%s</code>
<b>📝 Type:</b> <code>%s</code>
<b>⚡ Status:</b> <code>%s</code>

<b>👥 Statistics:</b>
<blockquote>• Target Users: %d
• Success: %d
• Failed: %d
• Blocked: %d
• Deleted: %d
• Other: %d</blockquote>`, 
		b.ID, timeStr, b.Method, b.Status, 
		b.Total, b.Success, b.Failed, b.Blocked, b.Deleted, b.OtherErr)

	var keyboard [][]gotgbot.InlineKeyboardButton

	// Add action buttons based on status
	if b.Status == "sending" {
		keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{
			{Text: "❌ Cancel Broadcast", CallbackData: "admin:bc_cancel:" + b.ID},
		})
	} else if (b.Status == "completed" || b.Status == "cancelled") && len(b.SentMessages) > 0 {
		keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{
			{Text: "🗑️ Delete Messages", CallbackData: "admin:bchist_delmsg:" + b.ID},
		})
	}
	
	// Delete log record option
	keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{
		{Text: "❌ Delete Record", CallbackData: "admin:bchist_delrec:" + b.ID},
	})

	keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{
		{Text: "🔙 Back to History", CallbackData: "admin:bchistory"},
	})

	_, _, err = c.Message.EditText(bot, text, &gotgbot.EditMessageTextOpts{
		ParseMode:   gotgbot.ParseModeHTML,
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard},
	})
	return err
}

// HandleDeleteBroadcastRecord deletes the broadcast document from history database.
func HandleDeleteBroadcastRecord(bot *gotgbot.Bot, ctx *ext.Context, bId string) error {
	c := ctx.CallbackQuery
	err := _app.DB.DeleteBroadcast(bId)
	if err != nil {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Failed to delete log ❌", ShowAlert: true})
		return nil
	}

	c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Broadcast Log Deleted ✅"})
	return ListBroadcastHistory(bot, ctx)
}
