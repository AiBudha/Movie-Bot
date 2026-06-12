package core

import (
	"autofilterbot/internal/fsub"
	"fmt"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
)

// HandleJoinRequest handles join requests to force subsribe channels.
func HandleJoinRequest(bot *gotgbot.Bot, ctx *ext.Context) error {
	update := ctx.ChatJoinRequest

	// saves all join requests, not just from fsub channels
	err := _app.DB.SaveUserJoinRequest(update.From.Id, update.Chat.Id)
	if err != nil {
		_app.Log.Warn("handlejoinrequest: failed to save join request to db", zap.Int64("user_id", update.From.Id), zap.Int64("chat_id", update.Chat.Id), zap.Error(err))
	}
	fsub.SetMembershipCache(update.From.Id, update.Chat.Id, true)

	// Sequential Fsub Transition Logic
	go func() {
		userId := update.From.Id
		msgId, _ := _app.DB.GetUserFsubMessage(userId)
		if msgId == 0 {
			return // No active fsub prompt
		}

		channels := _app.Config.GetFsubChannels()
		missing := fsub.GetMissingChannels(bot, _app.GetDB(), _app.Log, userId, channels)

		if len(missing) > 0 {
			// Still missing some channels, update the prompt to the next one
			ch := missing[0]
			link := ch.InviteLink
			if link == "" {
				link = "https://t.me/telegram"
			}
			text := fmt.Sprintf("<b>✅ 𝖱𝖾𝗊𝗎𝖾𝗌𝗍 𝖱𝖾𝖼𝖾𝗂𝗏𝖾𝖽 𝖿𝗈𝗋 𝖢𝗁𝖺𝗇𝗇𝖾𝗅 %d!</b>\n\n<i>𝖭𝗈𝗐, 𝖯𝗅𝖾𝖺𝗌𝖾 𝖲𝖾𝗇𝖽 𝖠 𝖩𝗈𝗂𝗇 𝖱𝖾𝗊𝗎𝖾𝗌𝗍 𝖳𝗈 𝖳𝗁𝖾 𝖭𝖾𝗑𝗍 𝖢𝗁𝖺𝗇𝗇𝖾𝗅 𝖡𝖾𝗅𝗈𝗐.</i>\n\n<b>Channel [%d/%d]</b>", len(channels)-len(missing), len(channels)-len(missing)+1, len(channels))
			btns := [][]gotgbot.InlineKeyboardButton{
				{{Text: "ᴊᴏɪɴ " + ch.Title, Url: link}},
				{{Text: "ᴛʀʏ ᴀɢᴀɪɴ 🔄", CallbackData: "fsub_verify"}},
			}

			_, _, err := bot.EditMessageText(text, &gotgbot.EditMessageTextOpts{
				ChatId:      userId,
				MessageId:   msgId,
				ParseMode:   "HTML",
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: btns},
			})
			if err != nil {
				_app.Log.Debug("handlejoinrequest: failed to update fsub message", zap.Error(err))
			}
		} else {
			// All channels requested! Auto-verify and send success
			_app.Log.Info("handlejoinrequest: user finished sequential fsub", zap.Int64("user_id", userId))

			// Delete the prompt
			_, _ = bot.DeleteMessage(userId, msgId, nil)
			_app.DB.SetUserFsubMessage(userId, 0)

			// Trigger success logic instantly
			_ = ResumeUserAction(bot, ctx, userId)
		}
	}()

	return nil
}

// HandleChatMember handles chat member updates to cleanup join requests.
func HandleChatMember(bot *gotgbot.Bot, ctx *ext.Context) error {
	update := ctx.ChatMember
	newStatus := update.NewChatMember.GetStatus()
	userId := update.NewChatMember.GetUser().Id

	// If the update is about the bot itself
	if userId == bot.Id {
		if update.Chat.Type == "group" || update.Chat.Type == "supergroup" {
			if newStatus == "administrator" || newStatus == "member" {
				// Bot was added to the group
				_app.Log.Info("Bot added to group", zap.String("title", update.Chat.Title), zap.Int64("chat_id", update.Chat.Id))
				_, err := _app.DB.GetGroupConfig(update.Chat.Id)
				if err != nil {
					_app.Log.Error("HandleChatMember: failed to save group config on addition", zap.Error(err))
				}

				// Alert in LogChannel
				if _app.LogChannel != 0 {
					var actorMention string
					if update.From.Id != 0 {
						actorMention = fmt.Sprintf("<a href=\"tg://user?id=%d\">%s</a>", update.From.Id, update.From.FirstName)
					} else {
						actorMention = "Unknown User"
					}
					alertMsg := fmt.Sprintf(
						"➕ <b>BOT ADDED TO GROUP</b> ➕\n\n"+
							"📍 <b>Group:</b> %s\n"+
							"🆔 <b>Group ID:</b> <code>%d</code>\n"+
							"👤 <b>Added By:</b> %s",
						update.Chat.Title, update.Chat.Id, actorMention,
					)
					_, _ = bot.SendMessage(_app.LogChannel, alertMsg, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
				}
			} else if newStatus == "left" || newStatus == "kicked" {
				// Bot was removed from the group
				_app.Log.Info("Bot removed from group", zap.String("title", update.Chat.Title), zap.Int64("chat_id", update.Chat.Id))
				err := _app.DB.DeleteGroupConfig(update.Chat.Id)
				if err != nil {
					_app.Log.Error("HandleChatMember: failed to delete group config on removal", zap.Error(err))
				}

				// Alert in LogChannel
				if _app.LogChannel != 0 {
					var actorMention string
					if update.From.Id != 0 {
						actorMention = fmt.Sprintf("<a href=\"tg://user?id=%d\">%s</a>", update.From.Id, update.From.FirstName)
					} else {
						actorMention = "Unknown User"
					}
					alertMsg := fmt.Sprintf(
						"➖ <b>BOT REMOVED FROM GROUP</b> ➖\n\n"+
							"📍 <b>Group:</b> %s\n"+
							"🆔 <b>Group ID:</b> <code>%d</code>\n"+
							"👤 <b>Action By:</b> %s",
						update.Chat.Title, update.Chat.Id, actorMention,
					)
					_, _ = bot.SendMessage(_app.LogChannel, alertMsg, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
				}
			}
		}
		return nil
	}

	// If they became a member, save to JoinRequests collection as persistent proof of membership
	if newStatus == "member" || newStatus == "administrator" || newStatus == "creator" {
		fsub.SetMembershipCache(userId, update.Chat.Id, true)
		err := _app.DB.SaveUserJoinRequest(userId, update.Chat.Id)
		if err == nil {
			_app.Log.Debug("handlechatmember: saved user to join requests collection (joined)", zap.Int64("user_id", userId), zap.Int64("chat_id", update.Chat.Id))
		}

		go func() {
			msgId, _ := _app.DB.GetUserFsubMessage(userId)
			if msgId == 0 {
				return // No active fsub prompt
			}

			channels := _app.Config.GetFsubChannels()
			missing := fsub.GetMissingChannels(bot, _app.GetDB(), _app.Log, userId, channels)

			if len(missing) > 0 {
				// Still missing some channels, update the prompt to the next one
				ch := missing[0]
				link := ch.InviteLink
				if link == "" {
					link = "https://t.me/telegram"
				}
				text := fmt.Sprintf("<b>✅ 𝖱𝖾𝗊𝗎𝖾𝗌𝗍 𝖱𝖾𝖼𝖾𝗂𝗏𝖾𝖽 𝖿𝗈𝗋 𝖢𝗁𝖺𝗇𝗇𝖾𝗅 %d!</b>\n\n<i>𝖭𝗈𝗐, 𝖯𝗅𝖾𝖺𝗌𝖾 𝖲𝖾𝗇𝖽 𝖠 𝖩𝗈𝗂𝗇 𝖱𝖾𝗊𝗎𝖾𝗌𝗍 𝖳𝗈 𝖳𝗁𝖾 𝖭𝖾𝗑𝗍 𝖢𝗁𝖺𝗇𝗇𝖾𝗅 𝖡𝖾𝗅𝗈𝗐.</i>\n\n<b>Channel [%d/%d]</b>", len(channels)-len(missing), len(channels)-len(missing)+1, len(channels))
				btns := [][]gotgbot.InlineKeyboardButton{
					{{Text: "ᴊᴏɪɴ " + ch.Title, Url: link}},
					{{Text: "ᴛʀʏ ᴀɢᴀɪɴ 🔄", CallbackData: "fsub_verify"}},
				}

				_, _, err := bot.EditMessageText(text, &gotgbot.EditMessageTextOpts{
					ChatId:      userId,
					MessageId:   msgId,
					ParseMode:   "HTML",
					ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: btns},
				})
				if err != nil {
					_app.Log.Debug("handlechatmember: failed to update fsub message", zap.Error(err))
				}
			} else {
				// All channels requested/joined! Auto-verify and send success
				_app.Log.Info("handlechatmember: user finished sequential fsub", zap.Int64("user_id", userId))

				// Delete the prompt
				_, _ = bot.DeleteMessage(userId, msgId, nil)
				_app.DB.SetUserFsubMessage(userId, 0)

				// Trigger success logic instantly
				_ = ResumeUserAction(bot, ctx, userId)
			}
		}()
	} else if newStatus == "left" || newStatus == "kicked" {
		fsub.SetMembershipCache(userId, update.Chat.Id, false)
		err := _app.DB.DeleteUserJoinRequest(userId, update.Chat.Id)
		if err == nil {
			_app.Log.Debug("handlechatmember: removed user from join requests (left/kicked)", zap.Int64("user_id", userId), zap.Int64("chat_id", update.Chat.Id))
		}
	}

	return nil
}
