package core

import (
	"fmt"
	"strconv"
	"strings"

	"autofilterbot/internal/database"
	"autofilterbot/internal/functions"
	"autofilterbot/internal/index"
	"autofilterbot/internal/model"
	"autofilterbot/pkg/callbackdata"
	"autofilterbot/pkg/conversation"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
)

// CmdIndex handles the /index command.
func CmdIndex(bot *gotgbot.Bot, ctx *ext.Context) error {
	if !_app.AuthAdmin(ctx) {
		return nil
	}

	m := ctx.EffectiveMessage
	chatId := m.Chat.Id

	// copied msg link parsing from batch
	//TODO: refactor parsing mechanism

	var (
		channelId, startId, endId int64
	)

	if replyM := m.ReplyToMessage; replyM != nil {
		if origin, ok := replyM.ForwardOrigin.(gotgbot.MessageOriginChannel); ok {
			channelId = origin.Chat.Id
			startId = origin.MessageId
		} else if link, err := functions.ParseMessageLink(replyM.Text); err == nil {
			if c, err := link.GetChat(bot); err == nil {
				channelId = c.Id
				startId = link.MessageId
			} else {
				sendChatErr(bot, chatId, err)
				return nil
			}
		}
	}

	if ctx.CallbackQuery == nil {
		split := strings.Fields(m.Text)
		if len(split) > 1 {
			if link, err := functions.ParseMessageLink(split[1]); err == nil {
				if startId != 0 {
					endId = link.MessageId
				} else {
					if c, err := link.GetChat(bot); err == nil {
						channelId = c.Id
						startId = link.MessageId
					} else {
						sendChatErr(bot, chatId, err)
						return nil
					}
				}
			}

			if len(split) > 2 && endId == 0 {
				if link, err := functions.ParseMessageLink(split[2]); err == nil {
					if startId != 0 {
						endId = link.MessageId
					} else {
						if c, err := link.GetChat(bot); err == nil {
							channelId = c.Id
							startId = link.MessageId
						} else {
							sendChatErr(bot, chatId, err)
							return nil
						}
					}
				}
			}
		}
	}

	if startId == 0 {
		conv := conversation.NewConversatorFromUpdate(bot, ctx.Update)

		askM, err := conv.Ask(_app.Ctx, "Please forward or send the post link of the first message in the batch:", &gotgbot.SendMessageOpts{
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{Text: "❌ Cancel", CallbackData: "admin:cancel"}}},
			},
		})
		if err != nil {
			_app.Log.Debug("batch: conv exited with error", zap.Error(err))
			return nil
		}

		if origin, ok := askM.ForwardOrigin.(gotgbot.MessageOriginChannel); ok {
			channelId = origin.Chat.Id
			startId = origin.MessageId
		} else if link, err := functions.ParseMessageLink(askM.Text); err == nil {
			if c, err := link.GetChat(bot); err == nil {
				channelId = c.Id
				startId = link.MessageId
			} else {
				sendChatErr(bot, chatId, err)
				return nil
			}
		} else {
			askM.Reply(bot, "Message Is Not a Forwarded Channel Post or Message Link!", nil)
			return nil
		}
	}

	if endId == 0 {
		conv := conversation.NewConversatorFromUpdate(bot, ctx.Update)

		askM, err := conv.Ask(_app.Ctx, "Please forward or send the post link of the last message in the batch:", &gotgbot.SendMessageOpts{
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{Text: "❌ Cancel", CallbackData: "admin:cancel"}}},
			},
		})
		if err != nil {
			_app.Log.Debug("batch: conv exited with error", zap.Error(err))
			return nil
		}

		if origin, ok := askM.ForwardOrigin.(gotgbot.MessageOriginChannel); ok {
			endId = origin.MessageId
		} else if link, err := functions.ParseMessageLink(askM.Text); err == nil {
			endId = link.MessageId
		} else {
			askM.Reply(bot, "Message Is Not a Forwarded Channel Post or Message Link!", nil)
			return nil
		}
	}

	if startId >= endId && endId != 0 {
		m.Reply(bot, "First Message Cannot be After The Last :/", nil)
		return nil
	}

	// Check if an operation already exists for this channel to prevent duplicate active operations
	existingOps, err := _app.DB.GetIndexOperationByChannel(channelId)
	if err == nil && existingOps != nil {
		plainChannelID := index.TDLibChannelIDToPlain(channelId)
		text := fmt.Sprintf(`
<b><u>Index Operation Overview</u></b>

An indexing operation is already active or configured for this channel.

<b>Process ID</b>: <code>%s</code>
<b>Channel</b>: <code>%d</code>
<b>Start</b>: <a href='https://t.me/c/%d/%d'>%d</a>
<b>End</b>: `, existingOps.ID, existingOps.ChannelID, plainChannelID, existingOps.StartMessageID, existingOps.StartMessageID)

		if existingOps.EndMessageID != 0 {
			text += fmt.Sprintf("<a href='https://t.me/c/%d/%d'>%d</a>\n<b>Total Messages</b>: %d", plainChannelID, existingOps.EndMessageID, existingOps.EndMessageID, existingOps.EndMessageID-existingOps.StartMessageID)
		} else {
			text += "<i>Until Latest</i>"
		}

		text += fmt.Sprintf("\n<b>Current Progress</b>: <a href='https://t.me/c/%d/%d'>%d</a>", plainChannelID, existingOps.CurrentMessageID, existingOps.CurrentMessageID)

		keyboard := [][]gotgbot.InlineKeyboardButton{{existingOps.CancelButton(), existingOps.ModifyButton()}}
		if existingOps.IsPaused {
			keyboard[0] = append(keyboard[0], existingOps.StartButton())
		} else {
			keyboard[0] = append(keyboard[0], existingOps.PauseButton())
		}

		_, err = bot.SendMessage(chatId, text, &gotgbot.SendMessageOpts{
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard},
		})
		return err
	}

	// Check if the channel was previously indexed to offer resume options
	lastIndexedID, err := _app.DB.GetIndexedChannel(channelId)
	if err == nil && lastIndexedID > 0 {
		text := fmt.Sprintf(`
<b>🔄 Channel Index Choice</b>

This channel was previously indexed. The last indexed message ID is <code>%d</code>.

Do you want to start indexing from the beginning or resume from the last indexed message?`, lastIndexedID)

		keyboard := [][]gotgbot.InlineKeyboardButton{
			{
				{
					Text:         "From Start 🔄",
					CallbackData: fmt.Sprintf("index_choice|%d_%d_%d_start", channelId, startId, endId),
				},
				{
					Text:         "From Last Indexed ⏩",
					CallbackData: fmt.Sprintf("index_choice|%d_%d_%d_resume", channelId, startId, endId),
				},
			},
		}

		_, err = bot.SendMessage(chatId, text, &gotgbot.SendMessageOpts{
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard},
		})
		return err
	}

	progressMessage, err := bot.SendMessage(chatId, "<code>Setting Up Index Operation ...</code>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	if err != nil {
		_app.Log.Warn(fmt.Sprintf("cmdindex: failed to send progress message: %v", err), zap.Int64("chat_id", chatId))
		return nil
	}

	indexModel := model.Index{
		ID:                    functions.RandString(6),
		StartMessageID:        startId,
		EndMessageID:          endId,
		CurrentMessageID:      startId,
		ChannelID:             channelId,
		ProgressMessageChatID: progressMessage.Chat.Id,
		ProgressMessageID:     progressMessage.MessageId,
		IsPaused:              true, // incase app restarts before user finishes setup
	}

	err = _app.DB.NewIndexOperation(&indexModel)
	if err != nil {
		_app.Log.Error(fmt.Sprintf("cmdindex: failed to insert index to db: %v", err))
		m.Reply(bot, "Failed to create db entry: "+err.Error(), nil)

		return nil
	}

	// Automatically save index channel to monitored channels
	_ = _app.AddFileChannel(channelId)

	plainChannelID := index.TDLibChannelIDToPlain(channelId)

	text := fmt.Sprintf(`
<b><u>Index Operation Overview</u></b>

<b>Channel</b>: <code>%d</code>
<b>Start</b>: <a href='https://t.me/c/%d/%d'>%d</a>
<b>End</b>: <a href='https://t.me/c/%d/%d'>%d</a>`, indexModel.ChannelID, plainChannelID, indexModel.StartMessageID, indexModel.StartMessageID, plainChannelID, indexModel.EndMessageID, indexModel.EndMessageID,
	)

	if indexModel.EndMessageID != 0 {
		text += fmt.Sprintf("\n<b>Total Messages</b>: %d", indexModel.EndMessageID-indexModel.StartMessageID)
	} else {
		text += "\n<b>End</b>: <i>Until Latest</i>"
	}

	keyboard := [][]gotgbot.InlineKeyboardButton{{indexModel.CancelButton(), indexModel.ModifyButton(), indexModel.StartButton()}}

	_, _, err = progressMessage.EditText(bot, text, &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML, ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard}})
	if err != nil {
		_app.Log.Warn(fmt.Sprintf("cmdindex: failed to update progress message to start: %v", err))
		return nil
	}

	return nil
}

// CbIndex handles the callback from index management buttons including, start, pause, modify, cancel etc.
// Strucuture: index|<pid>_<operation>
func CbIndex(bot *gotgbot.Bot, ctx *ext.Context) error {
	if !_app.AuthAdmin(ctx) {
		return nil
	}

	c := ctx.CallbackQuery

	d := callbackdata.FromString(c.Data)
	if d.LenArgs() < 2 {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Not enough arguments in callback button", ShowAlert: true})
		_app.Log.Warn("cbindex: no arguments in callback", zap.String("data", c.Data), zap.Strings("args", d.Args))

		return nil
	}

	pid := d.Args[0]

	switch d.Args[1] {
	case model.IndexCharCancel:
		conv := conversation.NewConversatorFromUpdate(bot, ctx.Update)

		confirmMessage, err := conv.Ask(
			_app.Ctx,
			fmt.Sprintf("⚠️ Are you sure you want to permanently cancel this index function? Plase send the process id <code>%s</code> to confirm: ", pid),
			&gotgbot.SendMessageOpts{
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{
					InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{Text: "❌ Cancel", CallbackData: "admin:cancel"}}},
				},
				ParseMode: gotgbot.ParseModeHTML,
			},
		)
		if err != nil {
			_app.Log.Warn(fmt.Sprintf("cbindex: cancel: failed to send confirmation query message: %v", err))
			return nil
		}

		if strings.TrimSpace(confirmMessage.Text) != pid {
			confirmMessage.Reply(bot, "❗ Operation pid does not match. Cancel Failed.", nil)
			return nil
		}

		ok := _app.IndexManager.CancelOperation(pid)
		if !ok {
			_app.Log.Debug("cbindex: cancel: operation is not currently active", zap.String("pid", pid))
		}

		err = _app.DB.DeleteOperation(pid)
		if err != nil {
			_app.Log.Warn(fmt.Sprintf("cbindex: cancel: failed to delete operation from db: %v", err), zap.String("pid", pid))
			confirmMessage.Reply(bot, fmt.Sprintf("An error occurred while trying to delete operation: %v", err), nil)

			return nil
		}

		_, err = confirmMessage.Reply(bot, "✅ Operation Cancelled Successfully!", nil)
		if err != nil {
			_app.Log.Warn(fmt.Sprintf("cbindex: cancel: failed to send cancellation success message: %v", err))
			return nil
		}
	case model.IndexCharPause:
		ok := _app.IndexManager.CancelOperation(pid)
		if !ok {
			_app.Log.Debug("cbindex: pause: operation is not currently active", zap.String("pid", pid))
		}

		ok, err := _app.DB.UpdateIndexOperation(pid, map[string]any{"is_paused": true})
		if !ok {
			c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Operation not found in database!\nMay have ended or been cancelled.", ShowAlert: true})
			return nil
		} else if err != nil {
			_app.Log.Error(fmt.Sprintf("cbindex: pause: failed to set db paused status: %v", err), zap.String("pid", pid))
			c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Setting DB status to paused failed, please check logs!", ShowAlert: true})
			return nil
		}

		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Operation Will Pause Shortly 🎉"})
	case model.IndexCharModify:
		conv := conversation.NewConversatorFromUpdate(bot, ctx.Update)

		{
			ans, err := conv.Ask(_app.Ctx, "Would you like to change the end of the index? (Send <b>Yes</b> or <b>No</b>)", &gotgbot.SendMessageOpts{
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{
					InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
						{{Text: "❌ Cancel", CallbackData: "admin:cancel"}},
					},
				},
				ParseMode: gotgbot.ParseModeHTML,
			})
			if err != nil {
				_app.Log.Error(fmt.Sprintf("cbindex: modify: failed to send confirmation query message: %v", err))
				return nil
			}

			if strings.ToLower(strings.TrimSpace(ans.Text)) != "yes" {
				_app.Log.Debug("cbindex: modify: no modifications made", zap.String("pid", pid))
				ans.Reply(bot, "Index not modified.", &gotgbot.SendMessageOpts{ReplyMarkup: gotgbot.ReplyKeyboardRemove{RemoveKeyboard: true}})

				return nil
			}
		}

		{
			ans, err := conv.Ask(_app.Ctx, "Please send the link or forward(with quotes) the new end message: ", &gotgbot.SendMessageOpts{
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{
					InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{Text: "❌ Cancel", CallbackData: "admin:cancel"}}},
				},
			})
			if err != nil {
				_app.Log.Error(fmt.Sprintf("cbindex: modify: failed to send message request message: %v", err))
				return nil
			}

			var (
				channelID int64
				messageID int64
			)

			// parse msg link or find forward origin
			if origin, ok := ans.ForwardOrigin.(gotgbot.MessageOriginChannel); ok {
				channelID = origin.Chat.Id
				messageID = origin.MessageId
			} else if link, err := functions.ParseMessageLink(ans.Text); err == nil {
				if c, err := link.GetChat(bot); err == nil {
					channelID = c.Id
					messageID = link.MessageId
				} else {
					sendChatErr(bot, ctx.EffectiveChat.Id, err)
					return nil
				}
			}

			if messageID == 0 {
				_app.Log.Debug("cbindex: modify: received msg is not link or forwarded", zap.String("pid", pid), zap.String("msg", ans.Text))
				ans.Reply(bot, "This is not a message link or a forwarded message!", &gotgbot.SendMessageOpts{ReplyMarkup: gotgbot.ReplyKeyboardRemove{RemoveKeyboard: true}})

				return nil
			}

			o, err := _app.DB.GetIndexOperation(pid)
			if err != nil {
				_app.Log.Error(fmt.Sprintf("cbindex: modify: failed to fetch operation: %v", err), zap.String("pid", pid))
				return nil
			}

			if channelID != o.ChannelID {
				_app.Log.Debug("cbindex: modify: channel ids do not match", zap.String("pid", pid), zap.Int64("received_id", channelID), zap.Int64("expected_id", o.ChannelID))
				ans.Reply(bot, "This message is not from the same channel as the index operation!", &gotgbot.SendMessageOpts{ReplyMarkup: gotgbot.ReplyKeyboardRemove{RemoveKeyboard: true}})

				return nil
			}

			if messageID <= o.CurrentMessageID {
				_app.Log.Debug("cbindex: modify: new end is lower that current message id", zap.String("pid", pid), zap.Int64("rexeived_id", messageID), zap.Int64("current_id", o.CurrentMessageID))
				ans.Reply(bot, "New message comes before the current index location!", &gotgbot.SendMessageOpts{ReplyMarkup: gotgbot.ReplyKeyboardRemove{RemoveKeyboard: true}})

				return nil
			}

			_app.IndexManager.CancelOperation(pid) // pauses the operation if active, user must unpause to resume

			ok, err := _app.DB.UpdateIndexOperation(pid, map[string]interface{}{"end": messageID})
			if !ok {
				c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Operation not found!\nMay have ended or been cancelled.", ShowAlert: true})
				return nil
			} else if err != nil {
				_app.Log.Error(fmt.Sprintf("cbindex: modify: failed to set end in db: %v", err), zap.String("pid", pid))
				ans.Reply(bot, "Failed to set new end, a db error occurred. Please check logs for more.", &gotgbot.SendMessageOpts{ReplyMarkup: gotgbot.ReplyKeyboardRemove{RemoveKeyboard: true}})

				return nil
			}

			_, err = ans.Reply(bot, "New end location has been set successfully 🎉\n\nOperation has been paused, please resume to continue indexing files.", &gotgbot.SendMessageOpts{ReplyMarkup: gotgbot.ReplyKeyboardRemove{RemoveKeyboard: true}})
			if err != nil {
				_app.Log.Warn(fmt.Sprintf("cbindex: modify: failed to send success message: %v", err), zap.String("pid", pid))
			}
		}
	case model.IndexCharStart:
		_app.IndexManager.CancelOperation(pid) // cancel active operation if applicable

		o, err := _app.DB.GetIndexOperation(pid)
		if database.IsNoDocumentsError(err) {
			c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Operation Not Found!\nOperation may be completed or cancelled.", ShowAlert: true})
			return nil
		} else if err != nil {
			_app.Log.Error(fmt.Sprintf("cbindex: start: failed to fetch operation: %v", err), zap.String("pid", pid))
			return nil
		}

		operationCtx, operation := _app.IndexManager.NewOperation(_app.Ctx, o, _app.DB, _app.Log, bot)

		_app.IndexManager.RunOperation(operationCtx, operation)

		ok, err := _app.DB.UpdateIndexOperation(pid, map[string]interface{}{"is_paused": false}) // ensure operation resumes at restart
		if !ok {
			c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Operation Not Found!\nOperation may be completed or cancelled.", ShowAlert: true})
			return nil
		} else if err != nil {
			_app.Log.Error(fmt.Sprintf("cbindex: start: failed to set db paused status: %v", err), zap.String("pid", pid))
		}

		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Starting Index Operation..."})
	}

	return nil
}

// CbIndexChoice handles the user choice (From Start or From Last Indexed).
func CbIndexChoice(bot *gotgbot.Bot, ctx *ext.Context) error {
	if !_app.AuthAdmin(ctx) {
		return nil
	}

	c := ctx.CallbackQuery
	d := callbackdata.FromString(c.Data)
	if d.LenArgs() < 4 {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Not enough arguments in callback", ShowAlert: true})
		return nil
	}

	chatId := c.Message.GetChat().Id
	channelId, _ := strconv.ParseInt(d.Args[0], 10, 64)
	startId, _ := strconv.ParseInt(d.Args[1], 10, 64)
	endId, _ := strconv.ParseInt(d.Args[2], 10, 64)
	choice := d.Args[3]

	if choice == "resume" {
		lastID, err := _app.DB.GetIndexedChannel(channelId)
		if err == nil && lastID > 0 {
			startId = lastID + 1
		}
	}

	if endId != 0 && startId >= endId {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "⚠️ Channel is already fully indexed up to the requested end ID!",
			ShowAlert: true,
		})
		return nil
	}

	// Double check if an operation already exists for this channel to prevent duplicate active operations
	existingOps, err := _app.DB.GetIndexOperationByChannel(channelId)
	if err == nil && existingOps != nil {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "An active/paused operation already exists for this channel!", ShowAlert: true})
		return nil
	}

	progressMessage, _, err := c.Message.EditText(bot, "<code>Setting Up Index Operation ...</code>", &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML})
	if err != nil {
		_app.Log.Warn(fmt.Sprintf("cbindexchoice: failed to edit progress message: %v", err))
		return nil
	}

	indexModel := model.Index{
		ID:                    functions.RandString(6),
		StartMessageID:        startId,
		EndMessageID:          endId,
		CurrentMessageID:      startId,
		ChannelID:             channelId,
		ProgressMessageChatID: progressMessage.Chat.Id,
		ProgressMessageID:     progressMessage.MessageId,
		IsPaused:              true,
	}

	err = _app.DB.NewIndexOperation(&indexModel)
	if err != nil {
		_app.Log.Error(fmt.Sprintf("cbindexchoice: failed to insert index to db: %v", err))
		_, _ = bot.SendMessage(chatId, "Failed to create db entry: "+err.Error(), nil)
		return nil
	}

	_ = _app.AddFileChannel(channelId)

	plainChannelID := index.TDLibChannelIDToPlain(channelId)
	text := fmt.Sprintf(`
<b><u>Index Operation Overview</u></b>

<b>Channel</b>: <code>%d</code>
<b>Start</b>: <a href='https://t.me/c/%d/%d'>%d</a>
<b>End</b>: `, indexModel.ChannelID, plainChannelID, indexModel.StartMessageID, indexModel.StartMessageID)

	if indexModel.EndMessageID != 0 {
		text += fmt.Sprintf("<a href='https://t.me/c/%d/%d'>%d</a>\n<b>Total Messages</b>: %d", plainChannelID, indexModel.EndMessageID, indexModel.EndMessageID, indexModel.EndMessageID-indexModel.StartMessageID)
	} else {
		text += "<i>Until Latest</i>"
	}

	keyboard := [][]gotgbot.InlineKeyboardButton{{indexModel.CancelButton(), indexModel.ModifyButton(), indexModel.StartButton()}}

	_, _, err = progressMessage.EditText(bot, text, &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML, ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard}})
	if err != nil {
		_app.Log.Warn(fmt.Sprintf("cbindexchoice: failed to update progress message to start: %v", err))
	}

	c.Answer(bot, nil)
	return nil
}
