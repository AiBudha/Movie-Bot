package core

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"autofilterbot/internal/model"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
)

// Helper to html escape strings
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	return strings.ReplaceAll(s, ">", "&gt;")
}

func ptrBool(b bool) *bool {
	return &b
}

func getCustomTitle(member gotgbot.ChatMember) string {
	switch m := member.(type) {
	case *gotgbot.ChatMemberAdministrator:
		return m.CustomTitle
	case gotgbot.ChatMemberAdministrator:
		return m.CustomTitle
	case *gotgbot.ChatMemberOwner:
		return m.CustomTitle
	case gotgbot.ChatMemberOwner:
		return m.CustomTitle
	}
	return ""
}


// Helper to prevent group command usage in private chats
func checkPrivateChat(bot *gotgbot.Bot, m *gotgbot.Message) bool {
	if m.Chat.Type == "private" {
		_, _ = m.Reply(bot, "<b>⚠️ This command can only be used inside group chats!</b>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return true
	}
	return false
}

// Helper to resolve chat ID and verify if user is an admin of that chat.
// Works for both group chats and private chats (via connection).
// If in private chat, verifies the user is connected to a group and is admin there.
// If it fails or user is not admin, it replies/alerts and returns (0, false).
func resolveChatIDAndVerifyAdmin(bot *gotgbot.Bot, m *gotgbot.Message) (int64, bool) {
	if m.Chat.Type == "private" {
		connChatID, err := _app.DB.GetUserConnection(m.From.Id)
		if err != nil || connChatID == 0 {
			_, _ = m.Reply(bot, "❌ You are not connected to any group. Use <code>/connect &lt;group_id&gt;</code> to connect first.", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
			return 0, false
		}
		isAdmin, err := IsUserAdmin(bot, connChatID, m.From.Id)
		if err != nil || !isAdmin {
			_, _ = m.Reply(bot, "❌ Only group administrators can use this command.", nil)
			return 0, false
		}
		return connChatID, true
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return 0, false
	}
	return m.Chat.Id, true
}


// Check if a user is admin in the chat
func IsUserAdmin(bot *gotgbot.Bot, chatID, userID int64) (bool, error) {
	for _, adminID := range _app.Admins {
		if adminID == userID {
			return true, nil
		}
	}

	member, err := bot.GetChatMember(chatID, userID, nil)
	if err != nil {
		return false, err
	}
	status := member.GetStatus()
	return status == "creator" || status == "administrator", nil
}

// Parses target user from reply or command arguments
func parseTargetUser(bot *gotgbot.Bot, ctx *ext.Context) (*gotgbot.User, string, error) {
	m := ctx.EffectiveMessage
	if m.ReplyToMessage != nil {
		return m.ReplyToMessage.From, "", nil
	}
	args := ctx.Args()
	if len(args) == 0 {
		return nil, "Please reply to a user or specify a user ID/username.", nil
	}

	userID, err := strconv.ParseInt(args[0], 10, 64)
	if err == nil {
		member, err := bot.GetChatMember(m.Chat.Id, userID, nil)
		if err == nil {
			u := member.GetUser()
			return &u, "", nil
		}
	}

	return nil, fmt.Sprintf("Could not resolve user '%s'. Please reply to one of their messages.", args[0]), nil
}

// /ban command
func BanUser(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}

	target, errMsg, err := parseTargetUser(bot, ctx)
	if err != nil {
		return err
	}
	if errMsg != "" {
		_, _ = m.Reply(bot, errMsg, nil)
		return nil
	}

	if target.Id == bot.Id {
		_, _ = m.Reply(bot, "I cannot ban myself!", nil)
		return nil
	}

	isTargetAdmin, _ := IsUserAdmin(bot, m.Chat.Id, target.Id)
	if isTargetAdmin {
		_, _ = m.Reply(bot, "I cannot ban an administrator!", nil)
		return nil
	}

	_, err = bot.BanChatMember(m.Chat.Id, target.Id, nil)
	if err != nil {
		_, _ = m.Reply(bot, fmt.Sprintf("Failed to ban user: %s", err.Error()), nil)
		return nil
	}

	markup := gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{{Text: "Unban 🔓", CallbackData: fmt.Sprintf("gunban:%d", target.Id)}},
		},
	}

	_, err = m.Reply(bot, fmt.Sprintf("💥 <b>%s</b> has been banned from the group!", htmlEscape(target.FirstName)), &gotgbot.SendMessageOpts{
		ParseMode:   gotgbot.ParseModeHTML,
		ReplyMarkup: markup,
	})
	return err
}

// /unban command
func UnbanUser(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}

	target, errMsg, err := parseTargetUser(bot, ctx)
	if err != nil {
		return err
	}
	if errMsg != "" {
		_, _ = m.Reply(bot, errMsg, nil)
		return nil
	}

	_, err = bot.UnbanChatMember(m.Chat.Id, target.Id, &gotgbot.UnbanChatMemberOpts{OnlyIfBanned: true})
	if err != nil {
		_, _ = m.Reply(bot, fmt.Sprintf("Failed to unban user: %s", err.Error()), nil)
		return nil
	}

	_, err = m.Reply(bot, fmt.Sprintf("🔓 <b>%s</b> has been unbanned!", htmlEscape(target.FirstName)), &gotgbot.SendMessageOpts{
		ParseMode: gotgbot.ParseModeHTML,
	})
	return err
}

// Handles inline unban button click
func HandleInlineUnban(bot *gotgbot.Bot, ctx *ext.Context) error {
	c := ctx.CallbackQuery
	isAdmin, err := IsUserAdmin(bot, c.Message.GetChat().Id, c.From.Id)
	if err != nil || !isAdmin {
		_, _ = c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "Only administrators can click this!",
			ShowAlert: true,
		})
		return nil
	}

	parts := strings.Split(c.Data, ":")
	if len(parts) < 2 {
		return nil
	}
	userID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil
	}

	_, err = bot.UnbanChatMember(c.Message.GetChat().Id, userID, &gotgbot.UnbanChatMemberOpts{OnlyIfBanned: true})
	if err != nil {
		_, _ = c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "Failed to unban: " + err.Error(),
			ShowAlert: true,
		})
		return nil
	}

	_, _ = c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
		Text: "User has been unbanned!",
	})
	_, _, _ = c.Message.EditText(bot, "🔓 User unbanned by administrator.", nil)
	return nil
}

// /kick command
func KickUser(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}

	target, errMsg, err := parseTargetUser(bot, ctx)
	if err != nil {
		return err
	}
	if errMsg != "" {
		_, _ = m.Reply(bot, errMsg, nil)
		return nil
	}

	if target.Id == bot.Id {
		_, _ = m.Reply(bot, "I cannot kick myself!", nil)
		return nil
	}

	isTargetAdmin, _ := IsUserAdmin(bot, m.Chat.Id, target.Id)
	if isTargetAdmin {
		_, _ = m.Reply(bot, "I cannot kick an administrator!", nil)
		return nil
	}

	// Ban and immediately unban to perform a kick/remove
	_, err = bot.BanChatMember(m.Chat.Id, target.Id, nil)
	if err != nil {
		_, _ = m.Reply(bot, fmt.Sprintf("Failed to kick user: %s", err.Error()), nil)
		return nil
	}
	_, _ = bot.UnbanChatMember(m.Chat.Id, target.Id, &gotgbot.UnbanChatMemberOpts{OnlyIfBanned: true})

	_, err = m.Reply(bot, fmt.Sprintf("🚪 <b>%s</b> has been kicked from the group!", htmlEscape(target.FirstName)), &gotgbot.SendMessageOpts{
		ParseMode: gotgbot.ParseModeHTML,
	})
	return err
}

// /mute command
func MuteUser(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}

	target, errMsg, err := parseTargetUser(bot, ctx)
	if err != nil {
		return err
	}
	if errMsg != "" {
		_, _ = m.Reply(bot, errMsg, nil)
		return nil
	}

	if target.Id == bot.Id {
		_, _ = m.Reply(bot, "I cannot mute myself!", nil)
		return nil
	}

	isTargetAdmin, _ := IsUserAdmin(bot, m.Chat.Id, target.Id)
	if isTargetAdmin {
		_, _ = m.Reply(bot, "I cannot mute an administrator!", nil)
		return nil
	}

	_, err = bot.RestrictChatMember(m.Chat.Id, target.Id, gotgbot.ChatPermissions{
		CanSendMessages:       false,
		CanSendAudios:         false,
		CanSendDocuments:      false,
		CanSendPhotos:         false,
		CanSendVideos:         false,
		CanSendVideoNotes:     false,
		CanSendVoiceNotes:     false,
		CanSendOtherMessages:  false,
		CanAddWebPagePreviews: false,
	}, nil)
	if err != nil {
		_, _ = m.Reply(bot, fmt.Sprintf("Failed to mute user: %s", err.Error()), nil)
		return nil
	}

	_, err = m.Reply(bot, fmt.Sprintf("🔇 <b>%s</b> has been muted in this group!", htmlEscape(target.FirstName)), &gotgbot.SendMessageOpts{
		ParseMode: gotgbot.ParseModeHTML,
	})
	return err
}

// /unmute command
func UnmuteUser(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}

	target, errMsg, err := parseTargetUser(bot, ctx)
	if err != nil {
		return err
	}
	if errMsg != "" {
		_, _ = m.Reply(bot, errMsg, nil)
		return nil
	}

	_, err = bot.RestrictChatMember(m.Chat.Id, target.Id, gotgbot.ChatPermissions{
		CanSendMessages:       true,
		CanSendAudios:         true,
		CanSendDocuments:      true,
		CanSendPhotos:         true,
		CanSendVideos:         true,
		CanSendVideoNotes:     true,
		CanSendVoiceNotes:     true,
		CanSendOtherMessages:  true,
		CanAddWebPagePreviews: true,
	}, nil)
	if err != nil {
		_, _ = m.Reply(bot, fmt.Sprintf("Failed to unmute user: %s", err.Error()), nil)
		return nil
	}

	_, err = m.Reply(bot, fmt.Sprintf("🔊 <b>%s</b> has been unmuted!", htmlEscape(target.FirstName)), &gotgbot.SendMessageOpts{
		ParseMode: gotgbot.ParseModeHTML,
	})
	return err
}

// /warn command
func WarnUser(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}

	target, errMsg, err := parseTargetUser(bot, ctx)
	if err != nil {
		return err
	}
	if errMsg != "" {
		_, _ = m.Reply(bot, errMsg, nil)
		return nil
	}

	if target.Id == bot.Id {
		_, _ = m.Reply(bot, "I cannot warn myself!", nil)
		return nil
	}

	isTargetAdmin, _ := IsUserAdmin(bot, m.Chat.Id, target.Id)
	if isTargetAdmin {
		_, _ = m.Reply(bot, "I cannot warn an administrator!", nil)
		return nil
	}

	count, err := _app.DB.AddUserWarning(m.Chat.Id, target.Id)
	if err != nil {
		_, _ = m.Reply(bot, fmt.Sprintf("Database error: %s", err.Error()), nil)
		return nil
	}

	reason := "No reason specified."
	args := ctx.Args()
	if len(args) > 0 {
		if m.ReplyToMessage != nil {
			reason = strings.Join(args, " ")
		} else if len(args) > 1 {
			reason = strings.Join(args[1:], " ")
		}
	}

	if count >= 3 {
		_, _ = bot.BanChatMember(m.Chat.Id, target.Id, nil)
		_ = _app.DB.ResetUserWarnings(m.Chat.Id, target.Id)
		_, err = m.Reply(bot, fmt.Sprintf("🚨 <b>%s</b> reached the warning limit (3/3) and has been banned!", htmlEscape(target.FirstName)), &gotgbot.SendMessageOpts{
			ParseMode: gotgbot.ParseModeHTML,
		})
		return err
	}

	_, err = m.Reply(bot, fmt.Sprintf("⚠️ <b>%s</b> has been warned (%d/3)!\n\n<b>Reason:</b> %s", htmlEscape(target.FirstName), count, htmlEscape(reason)), &gotgbot.SendMessageOpts{
		ParseMode: gotgbot.ParseModeHTML,
	})
	return err
}

// /unwarn command
func UnwarnUser(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}

	target, errMsg, err := parseTargetUser(bot, ctx)
	if err != nil {
		return err
	}
	if errMsg != "" {
		_, _ = m.Reply(bot, errMsg, nil)
		return nil
	}

	err = _app.DB.ResetUserWarnings(m.Chat.Id, target.Id)
	if err != nil {
		_, _ = m.Reply(bot, "Failed to reset warnings.", nil)
		return nil
	}

	_, err = m.Reply(bot, fmt.Sprintf("✅ Warnings reset for <b>%s</b>.", htmlEscape(target.FirstName)), &gotgbot.SendMessageOpts{
		ParseMode: gotgbot.ParseModeHTML,
	})
	return err
}

// /warns command
func ShowWarns(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}

	target, errMsg, err := parseTargetUser(bot, ctx)
	if err != nil {
		return err
	}
	if errMsg != "" {
		_, _ = m.Reply(bot, errMsg, nil)
		return nil
	}

	count, err := _app.DB.GetUserWarning(m.Chat.Id, target.Id)
	if err != nil {
		count = 0
	}

	_, err = m.Reply(bot, fmt.Sprintf("ℹ️ <b>%s</b> has <b>%d/3</b> warnings.", htmlEscape(target.FirstName), count), &gotgbot.SendMessageOpts{
		ParseMode: gotgbot.ParseModeHTML,
	})
	return err
}

// /pin command
func PinMessage(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}

	if m.ReplyToMessage == nil {
		_, _ = m.Reply(bot, "Please reply to the message you want to pin.", nil)
		return nil
	}

	_, err = bot.PinChatMessage(m.Chat.Id, m.ReplyToMessage.MessageId, &gotgbot.PinChatMessageOpts{
		DisableNotification: true,
	})
	if err != nil {
		_, _ = m.Reply(bot, fmt.Sprintf("Failed to pin message: %s", err.Error()), nil)
		return nil
	}

	_, err = m.Reply(bot, "📌 Message pinned successfully!", nil)
	return err
}

// /unpin command
func UnpinMessage(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}

	_, err = bot.UnpinChatMessage(m.Chat.Id, &gotgbot.UnpinChatMessageOpts{})
	if err != nil {
		_, _ = m.Reply(bot, fmt.Sprintf("Failed to unpin message: %s", err.Error()), nil)
		return nil
	}

	_, err = m.Reply(bot, "📌 Last pinned message unpinned!", nil)
	return err
}

// /rules command
func ShowRules(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}

	cfg, err := _app.DB.GetGroupConfig(m.Chat.Id)
	if err != nil || cfg == nil || cfg.Rules == "" {
		_, err = m.Reply(bot, "No rules set for this group.", nil)
		return err
	}

	_, err = m.Reply(bot, fmt.Sprintf("📋 <b>Group Rules:</b>\n\n%s", cfg.Rules), &gotgbot.SendMessageOpts{
		ParseMode: gotgbot.ParseModeHTML,
	})
	return err
}

// /setrules command
func SetRules(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}

	rulesText := strings.TrimSpace(m.Text[len("/setrules"):])
	if rulesText == "" {
		_, _ = m.Reply(bot, "Please provide the rules text. Example: <code>/setrules 1. Be nice</code>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}

	cfg, err := _app.DB.GetGroupConfig(m.Chat.Id)
	if err != nil {
		cfg = &model.GroupConfig{ChatID: m.Chat.Id}
	}
	cfg.Rules = rulesText

	err = _app.DB.SaveGroupConfig(cfg)
	if err != nil {
		_, _ = m.Reply(bot, "Failed to save rules.", nil)
		return nil
	}

	_, err = m.Reply(bot, "✅ Rules saved successfully!", nil)
	return err
}

// /clearrules command
func ClearRules(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}

	cfg, err := _app.DB.GetGroupConfig(m.Chat.Id)
	if err == nil && cfg != nil {
		cfg.Rules = ""
		_ = _app.DB.SaveGroupConfig(cfg)
	}

	_, err = m.Reply(bot, "🗑️ Group rules cleared.", nil)
	return err
}

// Welcome message handler
func HandleWelcomeMessage(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil || len(m.NewChatMembers) == 0 {
		return nil
	}

	cfg, err := _app.DB.GetGroupConfig(m.Chat.Id)
	if err != nil || cfg == nil {
		return nil
	}

	welcomeEnabled := cfg.WelcomeEnabled
	welcomeText := cfg.WelcomeText

	// If welcome settings are not initialized/customized locally, use global defaults
	if !welcomeEnabled && welcomeText == "" {
		welcomeEnabled = _app.Config.GetDefaultWelcomeEnabled()
		welcomeText = _app.Config.GetDefaultWelcomeText()
	}

	if !welcomeEnabled || welcomeText == "" {
		return nil
	}

	for _, user := range m.NewChatMembers {
		if user.Id == bot.Id {
			continue
		}

		if cfg.AntiRaidEnabled {
			_, _ = bot.BanChatMember(m.Chat.Id, user.Id, nil)
			_, _ = bot.UnbanChatMember(m.Chat.Id, user.Id, &gotgbot.UnbanChatMemberOpts{OnlyIfBanned: true})
			_app.Log.Info("kicked user due to antiraid", zap.Int64("chat_id", m.Chat.Id), zap.Int64("user_id", user.Id))
			continue
		}

		mention := fmt.Sprintf("<a href=\"tg://user?id=%d\">%s</a>", user.Id, htmlEscape(user.FirstName))

		if cfg.CaptchaEnabled {
			// Restrict new member from sending messages
			_, _ = bot.RestrictChatMember(m.Chat.Id, user.Id, gotgbot.ChatPermissions{
				CanSendMessages:       false,
				CanSendAudios:         false,
				CanSendDocuments:      false,
				CanSendPhotos:         false,
				CanSendVideos:         false,
				CanSendVideoNotes:     false,
				CanSendVoiceNotes:     false,
				CanSendPolls:          false,
				CanSendOtherMessages:  false,
				CanAddWebPagePreviews: false,
			}, nil)

			verifyMarkup := gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
					{{Text: "Verify 🤖", CallbackData: fmt.Sprintf("gverify:%d", user.Id)}},
				},
			}

			captchaTime := cfg.CaptchaTime
			if captchaTime < 30 {
				captchaTime = 300 // default 5 minutes
			}

			captchaMsg, err := bot.SendMessage(m.Chat.Id, fmt.Sprintf("Welcome %s! Please click the button below to verify you are human in %d seconds.", mention, captchaTime), &gotgbot.SendMessageOpts{
				ParseMode:   gotgbot.ParseModeHTML,
				ReplyMarkup: verifyMarkup,
			})

			if err == nil && captchaMsg != nil {
				go func(chatID, userID, msgID int64, duration int) {
					time.Sleep(time.Duration(duration) * time.Second)
					member, err := bot.GetChatMember(chatID, userID, nil)
					if err == nil && member.GetStatus() == "restricted" {
						_, _ = bot.BanChatMember(chatID, userID, nil)
						_, _ = bot.UnbanChatMember(chatID, userID, &gotgbot.UnbanChatMemberOpts{OnlyIfBanned: true})
						_, _ = bot.DeleteMessage(chatID, msgID, nil)
					}
				}(m.Chat.Id, user.Id, captchaMsg.MessageId, captchaTime)
			}
			continue
		}

		text := welcomeText
		text = strings.ReplaceAll(text, "{mention}", mention)
		text = strings.ReplaceAll(text, "{title}", htmlEscape(m.Chat.Title))
		text = strings.ReplaceAll(text, "{first_name}", htmlEscape(user.FirstName))
		text = strings.ReplaceAll(text, "{last_name}", htmlEscape(user.LastName))
		text = strings.ReplaceAll(text, "{username}", htmlEscape(user.Username))
		text = strings.ReplaceAll(text, "{id}", fmt.Sprint(user.Id))

		_, err = bot.SendMessage(m.Chat.Id, text, &gotgbot.SendMessageOpts{
			ParseMode: gotgbot.ParseModeHTML,
		})
		if err != nil {
			_app.Log.Warn("failed to send welcome message", zap.Error(err))
		}
	}

	return nil
}

// Group locks message handler
func HandleGroupLocks(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil || m.Chat.Type == "private" || m.Chat.Type == "channel" {
		return nil
	}

	// Fetch config
	cfg, err := _app.DB.GetGroupConfig(m.Chat.Id)
	if err != nil || cfg == nil {
		return nil
	}

	// Merge with global defaults if locks are empty
	locks := cfg.Locks
	if len(locks) == 0 {
		locks = _app.Config.GetDefaultLocks()
	}

	if len(locks) == 0 {
		return nil
	}

	// Admin check: admins are exempt from locks
	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err == nil && isAdmin {
		return nil
	}

	deleteMessage := false
	reason := ""

	if locks["stickers"] && m.Sticker != nil {
		deleteMessage = true
		reason = "stickers"
	}
	if locks["gifs"] && m.Animation != nil {
		deleteMessage = true
		reason = "gifs"
	}
	if locks["media"] && (m.Photo != nil || m.Video != nil || m.Audio != nil || m.Document != nil || m.Voice != nil || m.VideoNote != nil) {
		deleteMessage = true
		reason = "media"
	}
	if locks["forwards"] && m.ForwardOrigin != nil {
		deleteMessage = true
		reason = "forwards"
	}
	if locks["links"] {
		hasLink := false
		for _, entity := range m.Entities {
			if entity.Type == "url" || entity.Type == "text_link" {
				hasLink = true
				break
			}
		}
		for _, entity := range m.CaptionEntities {
			if entity.Type == "url" || entity.Type == "text_link" {
				hasLink = true
				break
			}
		}
		if hasLink {
			deleteMessage = true
			reason = "links"
		}
	}

	if deleteMessage {
		_, err := m.Delete(bot, nil)
		if err == nil {
			_app.Log.Info("deleted message due to group lock", zap.Int64("chat_id", m.Chat.Id), zap.String("lock_type", reason), zap.Int64("user_id", m.From.Id))
		}
		return ext.EndGroups
	}

	return nil
}

// Generates Group Settings Inline Keyboard Markup
func getGroupSettingsMarkup(cfg *model.GroupConfig) gotgbot.InlineKeyboardMarkup {
	welcomeStatus := "❌ Disabled"
	if cfg.WelcomeEnabled {
		welcomeStatus = "✅ Enabled"
	}

	return gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{{Text: "Welcome Message: " + welcomeStatus, CallbackData: "gcfg:welcome_toggle"}},
			{{Text: "Edit Welcome Text 📝", CallbackData: "gcfg:welcome_edit"}},
			{{Text: "Content Locks 🔒", CallbackData: "gcfg:locks_menu"}},
			{{Text: "Close 🗑️", CallbackData: "gcfg:close"}},
		},
	}
}

// Generates Group Locks Inline Keyboard Markup
func getGroupLocksMarkup(cfg *model.GroupConfig) gotgbot.InlineKeyboardMarkup {
	status := func(lock string) string {
		if cfg.Locks[lock] {
			return "🔒 Locked"
		}
		return "🔓 Unlocked"
	}

	return gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{
				{Text: "Stickers: " + status("stickers"), CallbackData: "gcfg:lock_toggle:stickers"},
				{Text: "Gifs: " + status("gifs"), CallbackData: "gcfg:lock_toggle:gifs"},
			},
			{
				{Text: "Media: " + status("media"), CallbackData: "gcfg:lock_toggle:media"},
				{Text: "Forwards: " + status("forwards"), CallbackData: "gcfg:lock_toggle:forwards"},
			},
			{
				{Text: "Links: " + status("links"), CallbackData: "gcfg:lock_toggle:links"},
			},
			{
				{Text: "Back 🔙", CallbackData: "gcfg:home"},
			},
		},
	}
}

// /gsettings command
func GroupSettings(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}

	chatID, ok := resolveChatIDAndVerifyAdmin(bot, m)
	if !ok {
		return nil
	}

	cfg, err := _app.DB.GetGroupConfig(chatID)
	if err != nil || cfg == nil {
		cfg = &model.GroupConfig{ChatID: chatID}
	}

	text := "<b>⚙️ Group Settings Panel</b>\n\nConfigure welcome messages and content locks below."
	_, err = m.Reply(bot, text, &gotgbot.SendMessageOpts{
		ParseMode:   gotgbot.ParseModeHTML,
		ReplyMarkup: getGroupSettingsMarkup(cfg),
	})
	return err
}

// Callback handler for gcfg:
func GroupSettingsCallback(bot *gotgbot.Bot, ctx *ext.Context) error {
	c := ctx.CallbackQuery
	mMsg, ok := c.Message.(gotgbot.Message)
	if !ok {
		return nil
	}
	m := &mMsg

	var chatID int64
	if m.Chat.Type == "private" {
		connChatID, err := _app.DB.GetUserConnection(c.From.Id)
		if err != nil || connChatID == 0 {
			_, _ = c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
				Text:      "You are not connected to any group!",
				ShowAlert: true,
			})
			return nil
		}
		chatID = connChatID
	} else {
		chatID = m.Chat.Id
	}

	isAdmin, err := IsUserAdmin(bot, chatID, c.From.Id)
	if err != nil || !isAdmin {
		_, _ = c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "Only administrators can click this!",
			ShowAlert: true,
		})
		return nil
	}

	cfg, err := _app.DB.GetGroupConfig(chatID)
	if err != nil || cfg == nil {
		cfg = &model.GroupConfig{ChatID: chatID}
	}

	data := c.Data
	if data == "gcfg:home" {
		_, _ = c.Answer(bot, nil)
		_, _, _ = m.EditText(bot, "<b>⚙️ Group Settings Panel</b>\n\nConfigure welcome messages and content locks below.", &gotgbot.EditMessageTextOpts{
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: getGroupSettingsMarkup(cfg),
		})
	} else if data == "gcfg:welcome_toggle" {
		cfg.WelcomeEnabled = !cfg.WelcomeEnabled
		_ = _app.DB.SaveGroupConfig(cfg)
		_, _ = c.Answer(bot, nil)
		_, _, _ = m.EditText(bot, "<b>⚙️ Group Settings Panel</b>\n\nConfigure welcome messages and content locks below.", &gotgbot.EditMessageTextOpts{
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: getGroupSettingsMarkup(cfg),
		})
	} else if data == "gcfg:welcome_edit" {
		_, _ = c.Answer(bot, nil)
		groupTitle := "the group"
		groupChat, err := bot.GetChat(chatID, nil)
		if err == nil && groupChat != nil {
			groupTitle = groupChat.Title
		}
		_, _ = bot.SendMessage(c.From.Id, fmt.Sprintf("Please send the welcome message you want to set for the group: <b>%s</b>\n\nUse {mention}, {title}, {first_name}, {id} as placeholders.", htmlEscape(groupTitle)), &gotgbot.SendMessageOpts{
			ParseMode: gotgbot.ParseModeHTML,
		})
		
		var instr string
		if m.Chat.Type == "private" {
			instr = "📝 To update welcome message, run <code>/setwelcome &lt;text&gt;</code> here in PM."
		} else {
			instr = "📝 To update welcome message, run <code>/setwelcome &lt;text&gt;</code> in the group chat."
		}
		_, _, _ = m.EditText(bot, instr, &gotgbot.EditMessageTextOpts{
			ParseMode: gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{Text: "Back 🔙", CallbackData: "gcfg:home"}}},
			},
		})
	} else if data == "gcfg:locks_menu" {
		_, _ = c.Answer(bot, nil)
		_, _, _ = m.EditText(bot, "<b>🔒 Content Locks Panel</b>\n\nLock/Unlock message types for non-admin members.", &gotgbot.EditMessageTextOpts{
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: getGroupLocksMarkup(cfg),
		})
	} else if strings.HasPrefix(data, "gcfg:lock_toggle:") {
		lock := data[len("gcfg:lock_toggle:"):]
		if cfg.Locks == nil {
			cfg.Locks = make(map[string]bool)
		}
		cfg.Locks[lock] = !cfg.Locks[lock]
		_ = _app.DB.SaveGroupConfig(cfg)
		_, _ = c.Answer(bot, nil)
		_, _, _ = m.EditText(bot, "<b>🔒 Content Locks Panel</b>\n\nLock/Unlock message types for non-admin members.", &gotgbot.EditMessageTextOpts{
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: getGroupLocksMarkup(cfg),
		})
	} else if data == "gcfg:close" {
		_, _ = c.Answer(bot, nil)
		_, _ = m.Delete(bot, nil)
	}

	return nil
}

// /setwelcome command
func SetWelcomeText(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}
	chatID, ok := resolveChatIDAndVerifyAdmin(bot, m)
	if !ok {
		return nil
	}

	welcomeText := strings.TrimSpace(m.Text[len("/setwelcome"):])
	if welcomeText == "" {
		_, _ = m.Reply(bot, "Please provide the welcome message text. Example: <code>/setwelcome Welcome {mention}!</code>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}

	cfg, err := _app.DB.GetGroupConfig(chatID)
	if err != nil {
		cfg = &model.GroupConfig{ChatID: chatID}
	}
	cfg.WelcomeText = welcomeText
	cfg.WelcomeEnabled = true

	err = _app.DB.SaveGroupConfig(cfg)
	if err != nil {
		_, _ = m.Reply(bot, "Failed to save welcome message.", nil)
		return nil
	}

	_, err = m.Reply(bot, "✅ Welcome message set and enabled successfully!", nil)
	return err
}

// /clearwelcome command
func ClearWelcomeText(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}
	chatID, ok := resolveChatIDAndVerifyAdmin(bot, m)
	if !ok {
		return nil
	}

	cfg, err := _app.DB.GetGroupConfig(chatID)
	if err == nil && cfg != nil {
		cfg.WelcomeText = ""
		cfg.WelcomeEnabled = false
		_ = _app.DB.SaveGroupConfig(cfg)
	}

	_, err = m.Reply(bot, "🗑️ Welcome message text cleared and disabled.", nil)
	return err
}

// /locks command to show lock statuses
func ShowLocks(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}
	chatID, ok := resolveChatIDAndVerifyAdmin(bot, m)
	if !ok {
		return nil
	}

	cfg, err := _app.DB.GetGroupConfig(chatID)
	if err != nil || cfg == nil {
		cfg = &model.GroupConfig{ChatID: chatID}
	}

	locks := cfg.Locks
	if len(locks) == 0 {
		locks = _app.Config.GetDefaultLocks()
	}

	status := func(lock string) string {
		if locks[lock] {
			return "🔒 Locked"
		}
		return "🔓 Unlocked"
	}

	text := fmt.Sprintf(
		"<b>🔒 Group Content Locks:</b>\n\n"+
			"• Stickers: %s\n"+
			"• Gifs: %s\n"+
			"• Media: %s\n"+
			"• Forwards: %s\n"+
			"• Links: %s\n",
		status("stickers"), status("gifs"), status("media"), status("forwards"), status("links"),
	)

	_, err = m.Reply(bot, text, &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

// /lock command
func LockCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}
	chatID, ok := resolveChatIDAndVerifyAdmin(bot, m)
	if !ok {
		return nil
	}

	args := ctx.Args()
	if len(args) == 0 {
		_, _ = m.Reply(bot, "Please specify a lock type. Available locks: <code>stickers</code>, <code>gifs</code>, <code>media</code>, <code>forwards</code>, <code>links</code>.", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}

	lockType := strings.ToLower(args[0])
	validLocks := map[string]bool{"stickers": true, "gifs": true, "media": true, "forwards": true, "links": true}
	if !validLocks[lockType] {
		_, _ = m.Reply(bot, "Invalid lock type. Choose from: <code>stickers</code>, <code>gifs</code>, <code>media</code>, <code>forwards</code>, <code>links</code>.", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}

	cfg, err := _app.DB.GetGroupConfig(chatID)
	if err != nil {
		cfg = &model.GroupConfig{ChatID: chatID}
	}
	if cfg.Locks == nil {
		cfg.Locks = make(map[string]bool)
	}

	cfg.Locks[lockType] = true
	err = _app.DB.SaveGroupConfig(cfg)
	if err != nil {
		_, _ = m.Reply(bot, "Failed to save lock settings.", nil)
		return nil
	}

	_, err = m.Reply(bot, fmt.Sprintf("🔒 Lock enabled for <b>%s</b>.", lockType), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

// /unlock command
func UnlockCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}
	chatID, ok := resolveChatIDAndVerifyAdmin(bot, m)
	if !ok {
		return nil
	}

	args := ctx.Args()
	if len(args) == 0 {
		_, _ = m.Reply(bot, "Please specify a lock type to unlock. Available locks: <code>stickers</code>, <code>gifs</code>, <code>media</code>, <code>forwards</code>, <code>links</code>.", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}

	lockType := strings.ToLower(args[0])
	validLocks := map[string]bool{"stickers": true, "gifs": true, "media": true, "forwards": true, "links": true}
	if !validLocks[lockType] {
		_, _ = m.Reply(bot, "Invalid lock type. Choose from: <code>stickers</code>, <code>gifs</code>, <code>media</code>, <code>forwards</code>, <code>links</code>.", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}

	cfg, err := _app.DB.GetGroupConfig(chatID)
	if err != nil {
		cfg = &model.GroupConfig{ChatID: chatID}
	}
	if cfg.Locks == nil {
		cfg.Locks = make(map[string]bool)
	}

	cfg.Locks[lockType] = false
	err = _app.DB.SaveGroupConfig(cfg)
	if err != nil {
		_, _ = m.Reply(bot, "Failed to save lock settings.", nil)
		return nil
	}

	_, err = m.Reply(bot, fmt.Sprintf("🔓 Lock disabled for <b>%s</b>.", lockType), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

// /promote command
func PromoteUser(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}
	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}
	target, errMsg, err := parseTargetUser(bot, ctx)
	if err != nil {
		return err
	}
	if errMsg != "" {
		_, _ = m.Reply(bot, errMsg, nil)
		return nil
	}
	_, err = bot.PromoteChatMember(m.Chat.Id, target.Id, &gotgbot.PromoteChatMemberOpts{
		CanChangeInfo:      true,
		CanDeleteMessages:  true,
		CanInviteUsers:     true,
		CanRestrictMembers: ptrBool(true),
		CanPinMessages:     true,
		CanPromoteMembers:  false,
	})
	if err != nil {
		_, _ = m.Reply(bot, fmt.Sprintf("Failed to promote: %s", err.Error()), nil)
		return nil
	}
	_, err = m.Reply(bot, fmt.Sprintf("✅ <b>%s</b> has been promoted to administrator!", htmlEscape(target.FirstName)), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

// /demote command
func DemoteUser(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}
	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}
	target, errMsg, err := parseTargetUser(bot, ctx)
	if err != nil {
		return err
	}
	if errMsg != "" {
		_, _ = m.Reply(bot, errMsg, nil)
		return nil
	}
	_, err = bot.PromoteChatMember(m.Chat.Id, target.Id, &gotgbot.PromoteChatMemberOpts{
		CanChangeInfo:      false,
		CanPostMessages:    false,
		CanEditMessages:    false,
		CanDeleteMessages:  false,
		CanInviteUsers:     false,
		CanRestrictMembers: ptrBool(false),
		CanPinMessages:     false,
		CanPromoteMembers:  false,
	})
	if err != nil {
		_, _ = m.Reply(bot, fmt.Sprintf("Failed to demote: %s", err.Error()), nil)
		return nil
	}
	_, err = m.Reply(bot, fmt.Sprintf("❌ <b>%s</b> has been demoted to normal member.", htmlEscape(target.FirstName)), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

// /title command
func SetAdminTitle(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}
	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}
	target, errMsg, err := parseTargetUser(bot, ctx)
	if err != nil {
		return err
	}
	if errMsg != "" {
		_, _ = m.Reply(bot, errMsg, nil)
		return nil
	}
	args := ctx.Args()
	title := ""
	if m.ReplyToMessage != nil && len(args) > 0 {
		title = strings.Join(args, " ")
	} else if len(args) > 1 {
		title = strings.Join(args[1:], " ")
	}
	if title == "" {
		_, _ = m.Reply(bot, "Please specify a custom title.", nil)
		return nil
	}
	_, err = bot.SetChatAdministratorCustomTitle(m.Chat.Id, target.Id, title, nil)
	if err != nil {
		_, _ = m.Reply(bot, fmt.Sprintf("Failed to set title: %s", err.Error()), nil)
		return nil
	}
	_, err = m.Reply(bot, fmt.Sprintf("✅ Custom title for <b>%s</b> set to: <code>%s</code>", htmlEscape(target.FirstName), htmlEscape(title)), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

// /adminlist command
func ListAdmins(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}
	admins, err := bot.GetChatAdministrators(m.Chat.Id, nil)
	if err != nil {
		return err
	}
	var list []string
	list = append(list, fmt.Sprintf("<b>🕵️ Administrators in %s:</b>\n", htmlEscape(m.Chat.Title)))
	for _, admin := range admins {
		u := admin.GetUser()
		customTitle := getCustomTitle(admin)
		role := "Admin"
		if admin.GetStatus() == "creator" {
			role = "Creator 👑"
		}
		if customTitle != "" {
			role = fmt.Sprintf("%s (%s)", role, customTitle)
		}
		list = append(list, fmt.Sprintf("• <a href=\"tg://user?id=%d\">%s</a> — <i>%s</i>", u.Id, htmlEscape(u.FirstName), htmlEscape(role)))
	}
	_, err = m.Reply(bot, strings.Join(list, "\n"), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

// Helper to parse duration string (e.g. 30m, 2h, 1d)
func parseDuration(s string) (time.Duration, error) {
	s = strings.ToLower(s)
	var unit time.Duration
	var valStr string
	if strings.HasSuffix(s, "m") {
		unit = time.Minute
		valStr = s[:len(s)-1]
	} else if strings.HasSuffix(s, "h") {
		unit = time.Hour
		valStr = s[:len(s)-1]
	} else if strings.HasSuffix(s, "d") {
		unit = 24 * time.Hour
		valStr = s[:len(s)-1]
	} else {
		return 0, fmt.Errorf("invalid time unit (use m, h, or d)")
	}
	val, err := strconv.ParseInt(valStr, 10, 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(val) * unit, nil
}

// Ban variant helper
func BanUserVariant(bot *gotgbot.Bot, ctx *ext.Context, silent bool, deleteReplied bool, temp bool) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}
	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}
	target, errMsg, err := parseTargetUser(bot, ctx)
	if err != nil {
		return err
	}
	if errMsg != "" {
		_, _ = m.Reply(bot, errMsg, nil)
		return nil
	}
	if target.Id == bot.Id {
		_, _ = m.Reply(bot, "I cannot ban myself!", nil)
		return nil
	}
	isTargetAdmin, _ := IsUserAdmin(bot, m.Chat.Id, target.Id)
	if isTargetAdmin {
		_, _ = m.Reply(bot, "I cannot ban an administrator!", nil)
		return nil
	}

	var untilDate int64
	durationText := ""
	if temp {
		args := ctx.Args()
		var timeArg string
		if m.ReplyToMessage != nil && len(args) > 0 {
			timeArg = args[0]
		} else if len(args) > 1 {
			timeArg = args[1]
		}
		if timeArg == "" {
			_, _ = m.Reply(bot, "Please specify a duration (e.g. 30m, 2h, 1d).", nil)
			return nil
		}
		dur, err := parseDuration(timeArg)
		if err != nil {
			_, _ = m.Reply(bot, "Invalid duration format. Example: 30m, 2h, 1d", nil)
			return nil
		}
		untilDate = time.Now().Add(dur).Unix()
		durationText = fmt.Sprintf(" for %s", timeArg)
	}

	if deleteReplied && m.ReplyToMessage != nil {
		_, _ = m.ReplyToMessage.Delete(bot, nil)
	}

	opts := &gotgbot.BanChatMemberOpts{}
	if temp {
		opts.UntilDate = untilDate
	}
	_, err = bot.BanChatMember(m.Chat.Id, target.Id, opts)
	if err != nil {
		_, _ = m.Reply(bot, fmt.Sprintf("Failed to ban: %s", err.Error()), nil)
		return nil
	}

	if silent {
		_, _ = m.Delete(bot, nil)
		return nil
	}

	_, err = m.Reply(bot, fmt.Sprintf("💥 <b>%s</b> has been banned%s from the group!", htmlEscape(target.FirstName), durationText), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

func SBanCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	return BanUserVariant(bot, ctx, true, false, false)
}
func DBanCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	return BanUserVariant(bot, ctx, false, true, false)
}
func TBanCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	return BanUserVariant(bot, ctx, false, false, true)
}

// Mute variant helper
func MuteUserVariant(bot *gotgbot.Bot, ctx *ext.Context, silent bool, deleteReplied bool, temp bool) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}
	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}
	target, errMsg, err := parseTargetUser(bot, ctx)
	if err != nil {
		return err
	}
	if errMsg != "" {
		_, _ = m.Reply(bot, errMsg, nil)
		return nil
	}
	if target.Id == bot.Id {
		_, _ = m.Reply(bot, "I cannot mute myself!", nil)
		return nil
	}
	isTargetAdmin, _ := IsUserAdmin(bot, m.Chat.Id, target.Id)
	if isTargetAdmin {
		_, _ = m.Reply(bot, "I cannot mute an administrator!", nil)
		return nil
	}

	var untilDate int64
	durationText := ""
	if temp {
		args := ctx.Args()
		var timeArg string
		if m.ReplyToMessage != nil && len(args) > 0 {
			timeArg = args[0]
		} else if len(args) > 1 {
			timeArg = args[1]
		}
		if timeArg == "" {
			_, _ = m.Reply(bot, "Please specify a duration (e.g. 30m, 2h, 1d).", nil)
			return nil
		}
		dur, err := parseDuration(timeArg)
		if err != nil {
			_, _ = m.Reply(bot, "Invalid duration format. Example: 30m, 2h, 1d", nil)
			return nil
		}
		untilDate = time.Now().Add(dur).Unix()
		durationText = fmt.Sprintf(" for %s", timeArg)
	}

	if deleteReplied && m.ReplyToMessage != nil {
		_, _ = m.ReplyToMessage.Delete(bot, nil)
	}

	permissions := gotgbot.ChatPermissions{
		CanSendMessages:       false,
		CanSendAudios:         false,
		CanSendDocuments:      false,
		CanSendPhotos:         false,
		CanSendVideos:         false,
		CanSendVideoNotes:     false,
		CanSendVoiceNotes:     false,
		CanSendPolls:          false,
		CanSendOtherMessages:  false,
		CanAddWebPagePreviews: false,
	}

	opts := &gotgbot.RestrictChatMemberOpts{}
	if temp {
		opts.UntilDate = untilDate
	}

	_, err = bot.RestrictChatMember(m.Chat.Id, target.Id, permissions, opts)
	if err != nil {
		_, _ = m.Reply(bot, fmt.Sprintf("Failed to mute: %s", err.Error()), nil)
		return nil
	}

	if silent {
		_, _ = m.Delete(bot, nil)
		return nil
	}

	_, err = m.Reply(bot, fmt.Sprintf("🔇 <b>%s</b> has been muted%s in this chat.", htmlEscape(target.FirstName), durationText), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

func SMuteCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	return MuteUserVariant(bot, ctx, true, false, false)
}
func DMuteCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	return MuteUserVariant(bot, ctx, false, true, false)
}
func TMuteCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	return MuteUserVariant(bot, ctx, false, false, true)
}

// /dkick command
func DKickCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}
	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}
	target, errMsg, err := parseTargetUser(bot, ctx)
	if err != nil {
		return err
	}
	if errMsg != "" {
		_, _ = m.Reply(bot, errMsg, nil)
		return nil
	}
	if target.Id == bot.Id {
		_, _ = m.Reply(bot, "I cannot kick myself!", nil)
		return nil
	}
	isTargetAdmin, _ := IsUserAdmin(bot, m.Chat.Id, target.Id)
	if isTargetAdmin {
		_, _ = m.Reply(bot, "I cannot kick an administrator!", nil)
		return nil
	}

	if m.ReplyToMessage != nil {
		_, _ = m.ReplyToMessage.Delete(bot, nil)
	}

	_, err = bot.BanChatMember(m.Chat.Id, target.Id, nil)
	if err != nil {
		_, _ = m.Reply(bot, fmt.Sprintf("Failed to kick: %s", err.Error()), nil)
		return nil
	}
	_, _ = bot.UnbanChatMember(m.Chat.Id, target.Id, &gotgbot.UnbanChatMemberOpts{OnlyIfBanned: true})

	_, err = m.Reply(bot, fmt.Sprintf("👢 <b>%s</b> has been kicked from the group!", htmlEscape(target.FirstName)), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

// /kickme command
func KickMeCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}
	isAdmin, _ := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if isAdmin {
		_, _ = m.Reply(bot, "You are an administrator, I cannot kick you!", nil)
		return nil
	}
	_, err := bot.BanChatMember(m.Chat.Id, m.From.Id, nil)
	if err != nil {
		_, _ = m.Reply(bot, fmt.Sprintf("Failed to kick you: %s", err.Error()), nil)
		return nil
	}
	_, _ = bot.UnbanChatMember(m.Chat.Id, m.From.Id, &gotgbot.UnbanChatMemberOpts{OnlyIfBanned: true})
	_, err = m.Reply(bot, "👢 You have kicked yourself!", nil)
	return err
}

// Warn variant helper
func WarnUserVariant(bot *gotgbot.Bot, ctx *ext.Context, silent bool, deleteReplied bool) error {
	m := ctx.EffectiveMessage
	if checkPrivateChat(bot, m) {
		return nil
	}
	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		return nil
	}
	target, errMsg, err := parseTargetUser(bot, ctx)
	if err != nil {
		return err
	}
	if errMsg != "" {
		_, _ = m.Reply(bot, errMsg, nil)
		return nil
	}
	if target.Id == bot.Id {
		_, _ = m.Reply(bot, "I cannot warn myself!", nil)
		return nil
	}
	isTargetAdmin, _ := IsUserAdmin(bot, m.Chat.Id, target.Id)
	if isTargetAdmin {
		_, _ = m.Reply(bot, "I cannot warn an administrator!", nil)
		return nil
	}

	if deleteReplied && m.ReplyToMessage != nil {
		_, _ = m.ReplyToMessage.Delete(bot, nil)
	}

	warnCount, err := _app.DB.AddUserWarning(m.Chat.Id, target.Id)
	if err != nil {
		_, _ = m.Reply(bot, "Failed to record warning.", nil)
		return nil
	}

	cfg, _ := _app.DB.GetGroupConfig(m.Chat.Id)
	limit := 3
	mode := "ban"
	if cfg != nil {
		if cfg.WarnLimit > 0 {
			limit = cfg.WarnLimit
		}
		if cfg.WarnMode != "" {
			mode = cfg.WarnMode
		}
	}

	if warnCount >= limit {
		_ = _app.DB.ResetUserWarnings(m.Chat.Id, target.Id)
		if mode == "kick" {
			_, _ = bot.BanChatMember(m.Chat.Id, target.Id, nil)
			_, _ = bot.UnbanChatMember(m.Chat.Id, target.Id, &gotgbot.UnbanChatMemberOpts{OnlyIfBanned: true})
			if !silent {
				_, _ = m.Reply(bot, fmt.Sprintf("⚠️ <b>%s</b> reached warn limit (%d/%d) and has been kicked!", htmlEscape(target.FirstName), warnCount, limit), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
			}
		} else if mode == "mute" {
			permissions := gotgbot.ChatPermissions{
				CanSendMessages:       false,
				CanSendAudios:         false,
				CanSendDocuments:      false,
				CanSendPhotos:         false,
				CanSendVideos:         false,
				CanSendVideoNotes:     false,
				CanSendVoiceNotes:     false,
				CanSendPolls:          false,
				CanSendOtherMessages:  false,
				CanAddWebPagePreviews: false,
			}
			_, _ = bot.RestrictChatMember(m.Chat.Id, target.Id, permissions, nil)
			if !silent {
				_, _ = m.Reply(bot, fmt.Sprintf("⚠️ <b>%s</b> reached warn limit (%d/%d) and has been muted!", htmlEscape(target.FirstName), warnCount, limit), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
			}
		} else { // default "ban"
			_, _ = bot.BanChatMember(m.Chat.Id, target.Id, nil)
			if !silent {
				_, _ = m.Reply(bot, fmt.Sprintf("⚠️ <b>%s</b> reached warn limit (%d/%d) and has been banned!", htmlEscape(target.FirstName), warnCount, limit), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
			}
		}
		if silent {
			_, _ = m.Delete(bot, nil)
		}
		return nil
	}

	if silent {
		_, _ = m.Delete(bot, nil)
		return nil
	}

	reason := "No reason specified."
	args := ctx.Args()
	if m.ReplyToMessage != nil && len(args) > 0 {
		reason = strings.Join(args, " ")
	} else if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}

	_, err = m.Reply(bot, fmt.Sprintf("⚠️ <b>%s</b> has been warned (%d/%d).\n\n<b>Reason:</b> %s", htmlEscape(target.FirstName), warnCount, limit, htmlEscape(reason)), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

func SWarnCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	return WarnUserVariant(bot, ctx, true, false)
}
func DWarnCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	return WarnUserVariant(bot, ctx, false, true)
}

// /setwarnlimit command
func SetWarnLimit(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}
	chatID, ok := resolveChatIDAndVerifyAdmin(bot, m)
	if !ok {
		return nil
	}
	args := ctx.Args()
	if len(args) == 0 {
		_, _ = m.Reply(bot, "Please specify warning limit. Example: <code>/setwarnlimit 5</code>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}
	limit, err := strconv.Atoi(args[0])
	if err != nil || limit < 1 {
		_, _ = m.Reply(bot, "Please provide a valid number greater than 0.", nil)
		return nil
	}
	cfg, err := _app.DB.GetGroupConfig(chatID)
	if err != nil {
		cfg = &model.GroupConfig{ChatID: chatID}
	}
	cfg.WarnLimit = limit
	_ = _app.DB.SaveGroupConfig(cfg)

	_, err = m.Reply(bot, fmt.Sprintf("✅ Warning limit set to <b>%d</b>.", limit), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

// /setwarnmode command
func SetWarnMode(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}
	chatID, ok := resolveChatIDAndVerifyAdmin(bot, m)
	if !ok {
		return nil
	}
	args := ctx.Args()
	if len(args) == 0 {
		_, _ = m.Reply(bot, "Please specify warn action mode: <code>ban</code>, <code>kick</code>, or <code>mute</code>.", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}
	mode := strings.ToLower(args[0])
	if mode != "ban" && mode != "kick" && mode != "mute" {
		_, _ = m.Reply(bot, "Invalid mode. Choose from: <code>ban</code>, <code>kick</code>, <code>mute</code>.", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}
	cfg, err := _app.DB.GetGroupConfig(chatID)
	if err != nil {
		cfg = &model.GroupConfig{ChatID: chatID}
	}
	cfg.WarnMode = mode
	_ = _app.DB.SaveGroupConfig(cfg)

	_, err = m.Reply(bot, fmt.Sprintf("✅ Warn action mode set to <b>%s</b>.", mode), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

type userFloodState struct {
	timestamps []time.Time
}

var (
	floodStateMap = make(map[string]*userFloodState)
	floodStateMu  sync.Mutex
)

// Antiflood handler registered as a priority interceptor middleware
func HandleAntiFlood(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil || m.Chat.Type == "private" || m.Chat.Type == "channel" || m.From == nil {
		return nil
	}

	_ = _app.DB.IncrementGroupMsgCount(m.Chat.Id)

	cfg, err := _app.DB.GetGroupConfig(m.Chat.Id)
	if err != nil || cfg == nil || cfg.FloodLimit <= 0 {
		return nil
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err == nil && isAdmin {
		return nil
	}

	key := fmt.Sprintf("%d_%d", m.Chat.Id, m.From.Id)
	now := time.Now()

	floodStateMu.Lock()
	state, exists := floodStateMap[key]
	if !exists {
		state = &userFloodState{}
		floodStateMap[key] = state
	}

	var recent []time.Time
	for _, t := range state.timestamps {
		if now.Sub(t) <= 5*time.Second {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	state.timestamps = recent
	count := len(recent)
	floodStateMu.Unlock()

	if count > cfg.FloodLimit {
		_, err = bot.RestrictChatMember(m.Chat.Id, m.From.Id, gotgbot.ChatPermissions{
			CanSendMessages:       false,
			CanSendAudios:         false,
			CanSendDocuments:      false,
			CanSendPhotos:         false,
			CanSendVideos:         false,
			CanSendVideoNotes:     false,
			CanSendVoiceNotes:     false,
			CanSendPolls:          false,
			CanSendOtherMessages:  false,
			CanAddWebPagePreviews: false,
		}, &gotgbot.RestrictChatMemberOpts{
			UntilDate: now.Add(24 * time.Hour).Unix(),
		})
		if err == nil {
			_, _ = m.Reply(bot, fmt.Sprintf("⚠️ <b>%s</b> has been muted for 24 hours due to flooding!", htmlEscape(m.From.FirstName)), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
			_, _ = m.Delete(bot, nil)
		}
		return ext.EndGroups
	}

	return nil
}

// /setflood command
func SetFloodLimit(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}
	chatID, ok := resolveChatIDAndVerifyAdmin(bot, m)
	if !ok {
		return nil
	}
	args := ctx.Args()
	if len(args) == 0 {
		_, _ = m.Reply(bot, "Please specify a flood limit (messages per 5 seconds). Example: <code>/setflood 5</code> (0 to disable)", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}
	limit, err := strconv.Atoi(args[0])
	if err != nil || limit < 0 {
		_, _ = m.Reply(bot, "Please provide a valid number greater than or equal to 0.", nil)
		return nil
	}
	cfg, err := _app.DB.GetGroupConfig(chatID)
	if err != nil {
		cfg = &model.GroupConfig{ChatID: chatID}
	}
	cfg.FloodLimit = limit
	_ = _app.DB.SaveGroupConfig(cfg)

	if limit == 0 {
		_, err = m.Reply(bot, "✅ Flood control has been disabled.", nil)
	} else {
		_, err = m.Reply(bot, fmt.Sprintf("✅ Flood limit set to <b>%d</b> messages per 5 seconds.", limit), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	}
	return err
}

// /captcha command
func SetCaptcha(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}
	chatID, ok := resolveChatIDAndVerifyAdmin(bot, m)
	if !ok {
		return nil
	}
	args := ctx.Args()
	if len(args) == 0 {
		_, _ = m.Reply(bot, "Please specify captcha state: <code>on</code> or <code>off</code>.", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}
	state := strings.ToLower(args[0])
	enabled := false
	if state == "on" || state == "yes" || state == "true" {
		enabled = true
	} else if state != "off" && state != "no" && state != "false" {
		_, _ = m.Reply(bot, "Invalid choice. Use: <code>on</code> or <code>off</code>.", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}
	cfg, err := _app.DB.GetGroupConfig(chatID)
	if err != nil {
		cfg = &model.GroupConfig{ChatID: chatID}
	}
	cfg.CaptchaEnabled = enabled
	_ = _app.DB.SaveGroupConfig(cfg)

	if enabled {
		_, err = m.Reply(bot, "✅ Captcha verification enabled for new members.", nil)
	} else {
		_, err = m.Reply(bot, "✅ Captcha verification disabled.", nil)
	}
	return err
}

// /captchatime command
func SetCaptchaTime(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}
	chatID, ok := resolveChatIDAndVerifyAdmin(bot, m)
	if !ok {
		return nil
	}
	args := ctx.Args()
	if len(args) == 0 {
		_, _ = m.Reply(bot, "Please specify captcha timeout in seconds. Example: <code>/captchatime 300</code>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}
	secs, err := strconv.Atoi(args[0])
	if err != nil || secs < 30 {
		_, _ = m.Reply(bot, "Please specify a valid time limit in seconds (minimum 30).", nil)
		return nil
	}
	cfg, err := _app.DB.GetGroupConfig(chatID)
	if err != nil {
		cfg = &model.GroupConfig{ChatID: chatID}
	}
	cfg.CaptchaTime = secs
	_ = _app.DB.SaveGroupConfig(cfg)

	_, err = m.Reply(bot, fmt.Sprintf("✅ Captcha verification timeout set to <b>%d</b> seconds.", secs), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

// Callback handler for gverify:
func HandleCaptchaCallback(bot *gotgbot.Bot, ctx *ext.Context) error {
	c := ctx.CallbackQuery
	mMsg, ok := c.Message.(gotgbot.Message)
	if !ok {
		return nil
	}
	m := &mMsg
	data := c.Data

	targetUserIDStr := data[len("gverify:"):]
	targetUserID, err := strconv.ParseInt(targetUserIDStr, 10, 64)
	if err != nil {
		return nil
	}

	if c.From.Id != targetUserID {
		_, _ = c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "This verification button is not for you! 🤖",
			ShowAlert: true,
		})
		return nil
	}

	_, err = bot.RestrictChatMember(m.Chat.Id, targetUserID, gotgbot.ChatPermissions{
		CanSendMessages:       true,
		CanSendAudios:         true,
		CanSendDocuments:      true,
		CanSendPhotos:         true,
		CanSendVideos:         true,
		CanSendVideoNotes:     true,
		CanSendVoiceNotes:     true,
		CanSendPolls:          true,
		CanSendOtherMessages:  true,
		CanAddWebPagePreviews: true,
	}, nil)
	if err != nil {
		_, _ = c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "Failed to verify. Please contact administrators.",
			ShowAlert: true,
		})
		return nil
	}

	_, _ = c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
		Text: "Verification successful! Welcome to the group. 🎉",
	})
	_, _ = m.Delete(bot, nil)
	return nil
}

// /antiraid command
func SetAntiRaid(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}
	chatID, ok := resolveChatIDAndVerifyAdmin(bot, m)
	if !ok {
		return nil
	}
	args := ctx.Args()
	if len(args) == 0 {
		_, _ = m.Reply(bot, "Please specify antiraid state: <code>on</code> or <code>off</code>.", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}
	state := strings.ToLower(args[0])
	enabled := false
	if state == "on" || state == "yes" || state == "true" {
		enabled = true
	} else if state != "off" && state != "no" && state != "false" {
		_, _ = m.Reply(bot, "Invalid choice. Use: <code>on</code> or <code>off</code>.", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}
	cfg, err := _app.DB.GetGroupConfig(chatID)
	if err != nil {
		cfg = &model.GroupConfig{ChatID: chatID}
	}
	cfg.AntiRaidEnabled = enabled
	_ = _app.DB.SaveGroupConfig(cfg)

	if enabled {
		_, err = m.Reply(bot, "🚨 Raid protection enabled. All new joins will be immediately kicked!", nil)
	} else {
		_, err = m.Reply(bot, "✅ Raid protection disabled.", nil)
	}
	return err
}

