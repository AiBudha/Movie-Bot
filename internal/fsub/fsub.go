package fsub

import (
	"fmt"
	"html"
	"strings"
	"time"

	"autofilterbot/internal/config"
	"autofilterbot/internal/database/mongo"
	"autofilterbot/internal/model"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
)

var (
	membershipCache = NewFsubCache()
	antiSpamTTL     = 60 * time.Second
	antiSpamCache   = NewAntiSpamCache()
)

type appPreview interface {
	GetDB() *mongo.Client
	GetConfig() *config.Config
	GetLog() *zap.Logger
	BasicMessageValues(ctx *ext.Context, extraValues ...map[string]any) map[string]string
	GetAdmins() []int64
}

// GetMissingChannels returns channels the user hasn't joined.
// Uses 60s TTL cache to minimize API calls.
func GetMissingChannels(bot *gotgbot.Bot, db *mongo.Client, logger *zap.Logger, userId int64, channels []model.Channel) []model.Channel {
	var missing []model.Channel
	var userJoinReqs *model.User

	for _, ch := range channels {
		// 1. Check Cache
		if isMember, ok := membershipCache.Get(userId, ch.ID); ok && isMember {
			continue
		}

		// 2. Check getChatMember
		member, err := bot.GetChatMember(ch.ID, userId, nil)
		isMember := false
		if err != nil {
			// Fail-open for this channel: if check fails due to bot permission, chat not found,
			// or network issues, we do not block the user.
			logger.Warn("GetMissingChannels: FSub check failed, failing open for this channel",
				zap.Int64("channel_id", ch.ID),
				zap.Int64("user_id", userId),
				zap.Error(err),
			)
			isMember = true
		} else {
			status := member.GetStatus()
			if status == "creator" || status == "administrator" || status == "member" {
				isMember = true
			} else if status == "restricted" {
				if m, ok := member.(*gotgbot.ChatMemberRestricted); ok {
					isMember = m.IsMember
				} else if m, ok := member.(gotgbot.ChatMemberRestricted); ok {
					isMember = m.IsMember
				}
			}
		}

		if isMember {
			membershipCache.Set(userId, ch.ID, true, 60*time.Second)
			continue
		}

		// 3. Check Join Requests (Consider as Fsub if requested)
		if userJoinReqs == nil {
			userJoinReqs, _ = db.GetUserJoinRequests(userId)
		}
		hasReq := false
		if userJoinReqs != nil {
			for _, rid := range userJoinReqs.JoinRequests {
				if rid == ch.ID {
					hasReq = true
					break
				}
			}
		}

		if hasReq {
			membershipCache.Set(userId, ch.ID, true, 60*time.Second)
			continue
		}

		missing = append(missing, ch)
	}
	return missing
}

// CheckFsub is the main entry point for Fsub enforcement.
func CheckFsub(app appPreview, bot *gotgbot.Bot, ctx *ext.Context) (bool, error) {
	if ctx.EffectiveUser == nil || ctx.EffectiveUser.IsBot {
		return true, nil
	}

	userId := ctx.EffectiveUser.Id

	// Bypass Fsub check for Admins
	for _, adminId := range app.GetAdmins() {
		if userId == adminId {
			return true, nil
		}
	}

	channels := app.GetConfig().GetFsubChannels()
	if len(channels) == 0 {
		return true, nil
	}

	missing := GetMissingChannels(bot, app.GetDB(), app.GetLog(), userId, channels)
	if len(missing) == 0 {
		// User has joined all channels. Clean up any leftover fsub prompt
		// that might still exist (e.g., from a previous session or race condition).
		if oldMsgId, _ := app.GetDB().GetUserFsubMessage(userId); oldMsgId != 0 {
			bot.DeleteMessage(userId, oldMsgId, nil)
			app.GetDB().SetUserFsubMessage(userId, 0)
		}
		return true, nil
	}

	// Sequential Fsub: Only show the FIRST channel from missing
	ch := missing[0]
	var btns [][]gotgbot.InlineKeyboardButton
	
	link := ch.InviteLink
	if link == "" {
		link = "https://t.me/telegram"
	}
	btns = append(btns, []gotgbot.InlineKeyboardButton{{Text: "ᴊᴏɪɴ " + ch.Title, Url: link}})
	btns = append(btns, []gotgbot.InlineKeyboardButton{{Text: "ᴛʀʏ ᴀɢᴀɪɴ 🔄", CallbackData: "fsub_verify"}})

	// Format text using template values from config
	fsubText := app.GetConfig().GetFsubText()
	
	mention := fmt.Sprintf("<a href=\"tg://user?id=%d\">%s</a>", userId, html.EscapeString(ctx.EffectiveUser.FirstName))
	firstName := html.EscapeString(ctx.EffectiveUser.FirstName)
	lastName := html.EscapeString(ctx.EffectiveUser.LastName)
	username := ctx.EffectiveUser.Username
	if username != "" {
		username = "@" + username
	} else {
		username = mention
	}
	
	replacer := strings.NewReplacer(
		"{mention}", mention,
		"{first_name}", firstName,
		"{last_name}", lastName,
		"{username}", username,
		"{id}", fmt.Sprintf("%d", userId),
		"{channel_title}", ch.Title,
		"{index}", fmt.Sprintf("%d", len(channels)-len(missing)+1),
		"{total}", fmt.Sprintf("%d", len(channels)),
	)
	text := replacer.Replace(fsubText)
	
	// Append progress context if not already present
	if !strings.Contains(fsubText, "{channel_title}") && !strings.Contains(fsubText, ch.Title) {
		text += fmt.Sprintf("\n\n📌 <b>𝖢𝗁𝖺𝗇𝗇𝖾𝗅:</b> <code>%s</code> <b>[%d/%d]</b>", ch.Title, len(channels)-len(missing)+1, len(channels))
	}

	// Private Chat Logic
	if ctx.EffectiveChat != nil && ctx.EffectiveChat.Type == "private" {
		// Store last action for resume
		action := ""
		if ctx.Message != nil {
			action = ctx.Message.Text
		} else if ctx.CallbackQuery != nil {
			action = "cb:" + ctx.CallbackQuery.Data
		}
		if action != "" {
			app.GetDB().SetUserLastAction(userId, action)
		}

		// Try to reuse existing message
		oldMsgId, _ := app.GetDB().GetUserFsubMessage(userId)
		if oldMsgId != 0 {
			_, _, err := bot.EditMessageText(text, &gotgbot.EditMessageTextOpts{
				ChatId:      userId,
				MessageId:   oldMsgId,
				ParseMode:   "HTML",
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: btns},
			})
			if err == nil {
				return false, nil
			}
		}

		// Fallback to new message
		msg, err := bot.SendMessage(userId, text, &gotgbot.SendMessageOpts{
			ParseMode:   "HTML",
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: btns},
		})
		if err == nil {
			app.GetDB().SetUserFsubMessage(userId, msg.MessageId)
		}
		return false, err
	}

	// Non-private chat logic (group, supergroup, channel, or callback from outside private)
	if ctx.EffectiveChat != nil && ctx.EffectiveChat.Type != "private" {
		// Store last action so they can resume when they verify
		action := ""
		if ctx.CallbackQuery != nil {
			action = "cb:" + ctx.CallbackQuery.Data
		} else if ctx.Message != nil {
			action = ctx.Message.Text
		}
		if action != "" {
			app.GetDB().SetUserLastAction(userId, action)
		}

		// Attempt to DM the user
		msg, err := bot.SendMessage(userId, text, &gotgbot.SendMessageOpts{
			ParseMode:   "HTML",
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: btns},
		})

		if err != nil {
			// DM failed (usually because they haven't started the bot in PM)
			if ctx.CallbackQuery != nil {
				// Show popup with redirect URL to PM
				_, _ = ctx.CallbackQuery.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
					Text:      "⚠️ Please start the bot first in PM and join the channel!",
					ShowAlert: true,
					Url:       fmt.Sprintf("https://t.me/%s?start=fsub", bot.Username),
				})
			} else if ctx.Message != nil {
				if antiSpamCache.ShouldWarn(userId, 5*time.Minute) {
					groupText := fmt.Sprintf("<b>Hey %s, Please Join Our Channels To Use The Bot!</b>\n\n<i>⚠️ I Couldn't DM You. Please Start The Bot First!</i>", ctx.EffectiveUser.FirstName)
					startBtn := gotgbot.InlineKeyboardButton{Text: "🚀 Sᴛᴀʀᴛ Bᴏᴛ", Url: "https://t.me/" + bot.Username + "?start=fsub"}
					_, _ = bot.SendMessage(ctx.EffectiveChat.Id, groupText, &gotgbot.SendMessageOpts{
						ParseMode: "HTML",
						ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{startBtn}}},
					})
				}
			}
		} else {
			// DM Succeeded!
			if msg != nil {
				app.GetDB().SetUserFsubMessage(userId, msg.MessageId)
			}
			if ctx.CallbackQuery != nil {
				_, _ = ctx.CallbackQuery.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
					Text:      "📩 I have sent the Join Requests / links to your PM! Please join/request and click this file button again.",
					ShowAlert: true,
				})
			} else if ctx.Message != nil {
				if antiSpamCache.ShouldWarn(userId, 5*time.Minute) {
					groupText := fmt.Sprintf("<b>Hey %s, Please Join Our Channels To Use The Bot!</b>\n\n<i>I've Sent The Join Links To Your DMs.</i>", ctx.EffectiveUser.FirstName)
					_, _ = bot.SendMessage(ctx.EffectiveChat.Id, groupText, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
				}
			}
		}
		return false, nil
	}

	return false, nil
}

// SetMembershipCache sets the cached membership status for a user and channel.
func SetMembershipCache(userId, channelId int64, isMember bool) {
	membershipCache.Set(userId, channelId, isMember, 60*time.Second)
}

// ClearMembershipCache flushes the entire membership cache.
// Call this whenever Fsub channels are added, removed, or changed.
func ClearMembershipCache() {
	membershipCache.Clear()
}

// InvalidateChannelCache removes all cached entries for a specific channel.
func InvalidateChannelCache(channelId int64) {
	membershipCache.InvalidateChannel(channelId)
}

