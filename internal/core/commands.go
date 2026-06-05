/*
Basic static commands that don't require additional helpers
*/

package core

import (
	"encoding/base64"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/disk"

	"autofilterbot/internal/autofilter"
	"autofilterbot/internal/button"
	"autofilterbot/internal/fsub"
	"autofilterbot/internal/functions"
	"autofilterbot/internal/limiter"
	"autofilterbot/internal/middleware"
	"autofilterbot/internal/model"
	"autofilterbot/internal/model/message"
	"autofilterbot/pkg/callbackdata"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
)

// StaticCommands handles all static text commands like about, help, privacy etc.
// Also handles callback queries in the format cmd:<command_name>
func StaticCommands(bot *gotgbot.Bot, ctx *ext.Context) error {
	var (
		commandName string
		isMedia     bool
	)

	isCallback := ctx.CallbackQuery != nil
	if c := ctx.CallbackQuery; isCallback {
		data := callbackdata.FromString(c.Data)
		if len(data.Path) > 1 {
			commandName = strings.ToLower(data.Path[1])
		}

		if m, ok := c.Message.(gotgbot.Message); ok {
			isMedia = functions.HasMedia(&m)
		}
	} else {
		m := ctx.EffectiveMessage

		text := ctx.EffectiveMessage.GetText()
		if text == "" {
			commandName = "start"
		} else {
			fields := strings.Fields(text)
			if len(fields) > 0 {
				firstWord := fields[0]
				cmd, _, _ := strings.Cut(firstWord, "@")
				commandName = strings.ToLower(strings.TrimPrefix(cmd, "/"))
			} else {
				commandName = "start"
			}
		}
		isMedia = functions.HasMedia(m)
	}

	var (
		msg         *message.Message
		err         error
		extraValues map[string]any
	)

	switch commandName {
	case "start":
		msg = _app.Config.GetStartMessage(bot.Username)
	case "about":
		msg = _app.Config.GetAboutMessage()
		lat := time.Since(time.Unix(ctx.EffectiveMessage.Date, 0))
		extraValues = map[string]any{
			"os":         runtime.GOOS,
			"database":   _app.DB.GetName(),
			"latency":    fmt.Sprintf("%.2fms", float64(lat.Microseconds())/1000.0),
			"go_version": runtime.Version(),
			"cpu":        func() string { if p, err := cpu.Percent(0, false); err == nil && len(p) > 0 { return fmt.Sprintf("%.2f%%", p[0]) } else { return "N/A" } }(),
			"ram":        func() string { if v, err := mem.VirtualMemory(); err == nil { return fmt.Sprintf("%.2f%%", v.UsedPercent) } else { return "N/A" } }(),
			"free":       func() string { if d, err := disk.Usage("."); err == nil { return fmt.Sprintf("%.2fGB", float64(d.Free)/(1<<30)) } else { return "N/A" } }(),
			"uptime":    time.Since(_app.GetStartTime()).Truncate(time.Second).String(),
		}
	case "help":
		msg = _app.Config.GetHelpMessage()
	case "ghelp", "grouphelp":
		msg = _app.Config.GetGroupHelpMessage()
	case "movies":
		msg = _app.Config.GetMoviesMessage()
	case "series":
		msg = _app.Config.GetSeriesMessage()
	case "privacy":
		msg = _app.Config.GetPrivacyMessage()
	case "copyright":
		msg = _app.Config.GetCopyrightMessage()
	case "stats": // failsafe
		return Stats(bot, ctx)
	case "fstats":
		return FStats(bot, ctx)
	case "top":
		return TopSearching(bot, ctx)
	default:
		msg = &message.Message{
			Text: fmt.Sprintf("Command %v Was Not Found!", commandName),
		}
	}

	var sendOpts *gotgbot.SendMessageOpts
	if commandName == "start" && !isCallback {
		sendOpts = &gotgbot.SendMessageOpts{
			MessageEffectId: "5046509860340391262", // Confetti Effect
		}
	}

	var sentMsg *gotgbot.Message
	msg.Format(_app.BasicMessageValues(ctx, extraValues))

	if isCallback {
		if isMedia {
			limiter.Wait()
			sentMsg, _, err = ctx.EffectiveMessage.EditCaption(bot, &gotgbot.EditMessageCaptionOpts{
				Caption:     msg.Text,
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: button.UnwrapKeyboard(msg.Keyboard)},
				ParseMode:   gotgbot.ParseModeHTML,
			})
		} else {
			limiter.Wait()
			sentMsg, _, err = ctx.EffectiveMessage.EditText(bot, msg.Text, &gotgbot.EditMessageTextOpts{
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: button.UnwrapKeyboard(msg.Keyboard)},
				ParseMode:   gotgbot.ParseModeHTML,
				LinkPreviewOptions: &gotgbot.LinkPreviewOptions{
					IsDisabled: true,
				},
			})
		}
	} else {
		limiter.Wait()
		if sendOpts != nil {
			sentMsg, err = msg.Send(bot, ctx.EffectiveChat.Id, sendOpts)
		} else {
			sentMsg, err = msg.Send(bot, ctx.EffectiveChat.Id)
		}
	}

	if err == nil && sentMsg != nil {
		middleware.ReactWithRandomEmoji(bot, sentMsg.Chat.Id, sentMsg.MessageId, _app.Config, _app.Log)
	}

	if err != nil {
		_app.Log.Warn(err.Error(), zap.String("command", commandName))
	}

	return nil
}

// Logs handles the /logs command.
func Logs(bot *gotgbot.Bot, ctx *ext.Context) error {
	ok, _ := fsub.CheckFsub(_app, bot, ctx)
	if !ok {
		return nil
	}
	if !_app.AuthAdmin(ctx) {
		return nil
	}

	chatId := ctx.EffectiveChat.Id
	var prg *gotgbot.Message
	var err error

	limiter.Wait()
	if ctx.CallbackQuery != nil {
		ctx.CallbackQuery.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "⏳ Downloading logs..."})
		prg, err = bot.SendMessage(chatId, "⏳ 𝖴𝗉𝗅𝗈𝖺𝖽𝗂𝗇𝗀 . . .", nil)
	} else {
		m := ctx.EffectiveMessage
		prg, err = m.Reply(bot, "⏳ 𝖴𝗉𝗅𝗈𝖺𝖽𝗂𝗇𝗀 . . .", nil)
	}
	if err != nil {
		_app.Log.Warn("logs: failed to send progress message", zap.Error(err))
	}

	f, err := os.Open("logs/app.log")
	if err != nil {
		_app.Log.Warn("open log file failed", zap.Error(err))
		if prg != nil {
			prg.Delete(bot, nil)
		}
		return nil
	}
	defer f.Close()

	sendOpts := &gotgbot.SendDocumentOpts{
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{button.Close(ctx.EffectiveUser.Id)}}},
	}
	if ctx.CallbackQuery == nil && ctx.EffectiveMessage != nil {
		sendOpts.ReplyParameters = &gotgbot.ReplyParameters{
			MessageId: ctx.EffectiveMessage.MessageId,
		}
	}

	limiter.Wait()
	_, err = bot.SendDocument(
		chatId,
		gotgbot.InputFileByReader("app-log.json", f),
		sendOpts,
	)
	if err != nil {
		_app.Log.Warn("send log file failed", zap.Error(err))
	}

	if prg != nil {
		prg.Delete(bot, nil)
	}

	return nil
}

// Stats handles the stats command and callback query.
func Stats(bot *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat != nil && ctx.EffectiveChat.Type != "private" {
		return GroupStats(bot, ctx)
	}

	if !_app.AuthAdmin(ctx) {
		return nil
	}

	analytics := _app.Analytics.(*AnalyticsService)
	s, err := analytics.GetStats()
	if err != nil {
		_app.Log.Warn("stats: get stats failed", zap.Error(err))
		return nil
	}

	dbStats, _ := _app.DB.Stats()
	var filesCount string
	var groupsCount int64
	if dbStats != nil {
		filesCount = fmt.Sprint(dbStats.Files)
		groupsCount = dbStats.Groups
	} else {
		filesCount = "0"
	}

	var mostSearched strings.Builder
	if len(s.TopSearches) > 0 {
		for i, st := range s.TopSearches {
			mostSearched.WriteString(fmt.Sprintf("%d. %s (%d)\n", i+1, st.Query, st.Count))
			if i >= 9 {
				break
			}
		}
	} else {
		mostSearched.WriteString("<i>No searches recorded</i>")
	}

	var topLangs strings.Builder
	if len(s.Languages) > 0 {
		count := 0
		type langEntry struct {
			lang string
			val  int64
		}
		var entries []langEntry
		for lang, val := range s.Languages {
			if lang == "Unknown" {
				continue
			}
			entries = append(entries, langEntry{lang, val})
		}
		
		// Sort descending
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[i].val < entries[j].val {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}

		for _, entry := range entries {
			topLangs.WriteString(fmt.Sprintf("├ %s: %d\n", entry.lang, entry.val))
			count++
			if count >= 5 {
				break
			}
		}
		if count == 0 {
			topLangs.WriteString("<i>No language data available</i>")
		}
	} else {
		topLangs.WriteString("<i>No language data available</i>")
	}

	var topCountries strings.Builder
	if len(s.Countries) > 0 {
		count := 0
		type countryEntry struct {
			country string
			val     int64
		}
		var entries []countryEntry
		for country, val := range s.Countries {
			entries = append(entries, countryEntry{country, val})
		}
		// Sort descending
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[i].val < entries[j].val {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}

		for _, entry := range entries {
			topCountries.WriteString(fmt.Sprintf("├ %s: %d\n", entry.country, entry.val))
			count++
			if count >= 8 {
				break
			}
		}
	} else {
		topCountries.WriteString("<i>No country data available</i>")
	}

	text := fmt.Sprintf(`<b>━━ SYSTEM METRICS ━━</b>

<b>Overview</b>

Files     <code>%s</code>  
Users     <code>%d</code>  
Groups    <code>%d</code>  

━━━━━━━━━━━━━━━━━━  

<b>Most Searched</b>

%s
━━━━━━━━━━━━━━━━━━  

<b>Activity</b>

├ Daily: <code>%d</code>
├ Weekly: <code>%d</code>
├ Monthly: <code>%d</code>

━━━━━━━━━━━━━━━━━━  

<b>Languages</b>

%s
━━━━━━━━━━━━━━━━━━  

<b>Users From</b>

%s`,
		filesCount,
		s.TotalUsers,
		groupsCount,
		mostSearched.String(),
		s.ActiveUsers,
		s.ActiveUsersWeekly,
		s.ActiveUsersMonthly,
		topLangs.String(),
		topCountries.String(),
	)

	markup := gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{
			Text: "🔄 Refresh", CallbackData: "cmd:stats", Style: "primary",
		}, {
			Text: "🔙 Back", CallbackData: "admin:back", Style: "primary",
		}, {
			Text: "🗑️ Close", CallbackData: "close", Style: "danger",
		}}},
	}

	switch {
	case ctx.Message != nil:
		_, err = bot.SendMessage(ctx.EffectiveChat.Id, text, &gotgbot.SendMessageOpts{
			ReplyMarkup: markup,
			ParseMode:   gotgbot.ParseModeHTML,
		})
	case ctx.CallbackQuery != nil:
		c := ctx.CallbackQuery
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Stats Refreshed!"})
		_, _, err = c.Message.EditText(bot, text, &gotgbot.EditMessageTextOpts{
			ReplyMarkup: markup,
			ParseMode:   gotgbot.ParseModeHTML,
		})
	}

	if err != nil {
		_app.Log.Warn("stats: send result failed", zap.Error(err))
	}

	return nil
}

// FStats handles the fstats command and callback query.
func FStats(bot *gotgbot.Bot, ctx *ext.Context) error {
	if !_app.AuthAdmin(ctx) {
		return nil
	}

	var fsubStatus strings.Builder
	fsubChannels := _app.Config.GetFsubChannels()
	if len(fsubChannels) == 0 {
		fsubStatus.WriteString("<i>Disabled / No Channels</i>")
	} else {
		for i, ch := range fsubChannels {
			analytics, _ := _app.DB.GetFsubAnalytics(ch.ID)
			memberCount, err := bot.GetChatMemberCount(ch.ID, nil)
			if err != nil {
				memberCount = 0
			} else if memberCount > 0 {
				memberCount -= 1 // Exclude the bot
			}

			fsubStatus.WriteString(fmt.Sprintf(`➲ <b>ForceSub %02d</b> | 𝖱𝖾𝗊𝗎𝖾𝗌𝗍 𝖥𝗈𝗋𝖼𝖾𝖲𝗎𝖻 ✅
○ <b>Title:</b> %s
○ <b>ID:</b> <code>%d</code>
○ <b>Link:</b> %s

• <b>Joined Members (TG):</b> %d
• <b>Pending Requests (DB):</b> %d <i>(Bot Users: %d)</i>
  ├ Daily: <code>%d</code>
  ├ Weekly: <code>%d</code>
  ├ Monthly: <code>%d</code>
• <b>Total FSub Reach:</b> %d

`, i+1, ch.Title, ch.ID, ch.InviteLink, memberCount, analytics.TotalRequests, analytics.Requested, analytics.DailyRequests, analytics.WeeklyRequests, analytics.MonthlyRequests, memberCount+analytics.TotalRequests))
		}
	}

	text := fmt.Sprintf(`<b>━━ FORCE SUBSCRIBE METRICS ━━</b>

<b>Configured Channels:</b> %d

━━━━━━━━━━━━━━━━━━  

%s`, len(fsubChannels), fsubStatus.String())

	markup := gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{
			Text: "🔄 Refresh", CallbackData: "fstats", Style: "primary",
		}, {
			Text: "🔙 Back", CallbackData: "admin:back", Style: "primary",
		}, {
			Text: "🗑️ Close", CallbackData: "close", Style: "danger",
		}}},
	}

	var err error
	switch {
	case ctx.Message != nil:
		_, err = bot.SendMessage(ctx.EffectiveChat.Id, text, &gotgbot.SendMessageOpts{
			ReplyMarkup: markup,
			ParseMode:   gotgbot.ParseModeHTML,
		})
	case ctx.CallbackQuery != nil:
		c := ctx.CallbackQuery
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "FSub Stats Refreshed!"})
		_, _, err = c.Message.EditText(bot, text, &gotgbot.EditMessageTextOpts{
			ReplyMarkup: markup,
			ParseMode:   gotgbot.ParseModeHTML,
		})
	}

	if err != nil {
		_app.Log.Warn("fstats: send result failed", zap.Error(err))
	}

	return nil
}

// IdCommand handles the /id command, replying with the chat ID or user ID
func IdCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}

	var text string

	// Handle forwarded messages
	if m.ForwardOrigin != nil {
		mo := m.ForwardOrigin.MergeMessageOrigin()
		if mo.Chat != nil {
			text = fmt.Sprintf("<b>Forwarded Channel ID:</b> <code>%d</code>", mo.Chat.Id)
		} else if mo.SenderChat != nil {
			text = fmt.Sprintf("<b>Forwarded Chat ID:</b> <code>%d</code>", mo.SenderChat.Id)
		} else if mo.SenderUser != nil {
			text = fmt.Sprintf("<b>Forwarded User ID:</b> <code>%d</code>", mo.SenderUser.Id)
		}
	}

	// Handle normal messages
	if text == "" {
		if m.Chat.Type == "channel" {
			text = fmt.Sprintf("<b>Channel ID:</b> <code>%d</code>", m.Chat.Id)
		} else if m.Chat.Type == "private" {
			text = fmt.Sprintf("<b>User ID:</b> <code>%d</code>", m.From.Id)
		} else {
			text = fmt.Sprintf("<b>Chat ID:</b> <code>%d</code>\n<b>User ID:</b> <code>%d</code>", m.Chat.Id, m.From.Id)
		}
	}

	_, err := m.Reply(bot, text, &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

func GroupStats(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		_, _ = m.Reply(bot, "❌ You must be an administrator to view group statistics.", nil)
		return nil
	}

	cfg, err := _app.DB.GetGroupConfig(m.Chat.Id)
	if err != nil {
		cfg = &model.GroupConfig{ChatID: m.Chat.Id}
	}

	memberCount, _ := bot.GetChatMemberCount(m.Chat.Id, nil)

	locksStr := "None"
	var activeLocks []string
	for k, v := range cfg.Locks {
		if v {
			activeLocks = append(activeLocks, k)
		}
	}
	if len(activeLocks) > 0 {
		locksStr = strings.Join(activeLocks, ", ")
	}

	antiraidStatus := "Disabled"
	if cfg.AntiRaidEnabled {
		antiraidStatus = "Enabled"
	}

	captchaStatus := "Disabled"
	if cfg.CaptchaEnabled {
		captchaStatus = fmt.Sprintf("Enabled (%ds timeout)", cfg.CaptchaTime)
	}

	floodStatus := "Disabled"
	if cfg.FloodLimit > 0 {
		floodStatus = fmt.Sprintf("%d messages/5s", cfg.FloodLimit)
	}

	welcomeStatus := "Disabled"
	if cfg.WelcomeEnabled {
		welcomeStatus = "Enabled"
	}

	text := fmt.Sprintf(`<b>📊 GROUP STATISTICS: %s</b>

👥 <b>Total Members:</b> <code>%d</code>
💬 <b>Messages Processed:</b> <code>%d</code>
🔍 <b>Searches Performed:</b> <code>%d</code>

🛡️ <b>Moderation Config:</b>
├ <b>Warn Limit:</b> <code>%d</code> (Action: <code>%s</code>)
├ <b>Antiflood:</b> <code>%s</code>
├ <b>Anti-Raid:</b> <code>%s</code>
├ <b>Captcha:</b> <code>%s</code>
├ <b>Welcome Text:</b> <code>%s</code>
└ <b>Active Locks:</b> <code>%s</code>`,
		htmlEscape(m.Chat.Title),
		memberCount,
		cfg.MessageCount,
		cfg.SearchCount,
		cfg.WarnLimit,
		cfg.WarnMode,
		floodStatus,
		antiraidStatus,
		captchaStatus,
		welcomeStatus,
		locksStr,
	)

	_, err = m.Reply(bot, text, &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

func MsgStatsCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat == nil || ctx.EffectiveChat.Type == "private" {
		_, _ = ctx.EffectiveMessage.Reply(bot, "❌ This command is only available in group chats.", nil)
		return nil
	}
	return GroupStats(bot, ctx)
}

func ConnectCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}

	if m.Chat.Type == "private" {
		args := ctx.Args()
		if len(args) > 0 {
			chatIDStr := args[0]
			chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
			if err != nil {
				_, _ = m.Reply(bot, "❌ Invalid Group ID format. Please use a numeric ID (e.g. <code>/connect -100123456789</code>).", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
				return nil
			}

			// Verify membership (must be a member of the group)
			member, err := bot.GetChatMember(chatID, m.From.Id, nil)
			if err != nil {
				_, _ = m.Reply(bot, "❌ Make sure the bot is added to the group first, and that you are a member of it.", nil)
				return nil
			}
			status := member.GetStatus()
			isMember := false
			if status == "creator" || status == "administrator" || status == "member" {
				isMember = true
			} else if status == "restricted" {
				if rMsg, ok := member.(*gotgbot.ChatMemberRestricted); ok {
					isMember = rMsg.IsMember
				} else if rMsg, ok := member.(gotgbot.ChatMemberRestricted); ok {
					isMember = rMsg.IsMember
				}
			}
			if !isMember {
				_, _ = m.Reply(bot, "❌ You must be a member of the group to connect to it.", nil)
				return nil
			}

			// Save connection
			err = _app.DB.SetUserConnection(m.From.Id, chatID)
			if err != nil {
				_, _ = m.Reply(bot, "❌ Failed to connect: DB error.", nil)
				return nil
			}

			chat, err := bot.GetChat(chatID, nil)
			title := fmt.Sprintf("%d", chatID)
			if err == nil && chat != nil {
				title = chat.Title
			}
			_, err = m.Reply(bot, fmt.Sprintf("✅ Successfully connected to <b>%s</b>!\nNow you can search this group's files and configure settings directly in PM.", htmlEscape(title)), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
			return err
		}

		connChatID, err := _app.DB.GetUserConnection(m.From.Id)
		if err == nil && connChatID != 0 {
			chat, err := bot.GetChat(connChatID, nil)
			title := fmt.Sprintf("%d", connChatID)
			if err == nil && chat != nil {
				title = chat.Title
			}
			_, err = m.Reply(bot, fmt.Sprintf("✅ You are currently connected to: <b>%s</b>\nUse /disconnect to disconnect.", htmlEscape(title)), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
			return err
		}

		_, err = m.Reply(bot, "ℹ️ To connect to a group, send <code>/connect &lt;group_id&gt;</code> in PM, or send /connect inside that group and authorize.", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return err
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		_, _ = m.Reply(bot, "❌ Only group administrators can use this command.", nil)
		return nil
	}

	link := fmt.Sprintf("https://t.me/%s?start=connect_%d", bot.Username, m.Chat.Id)
	markup := gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{
			Text: "🔌 Authorize Connection",
			Url:  link,
		}}},
	}

	_, err = m.Reply(bot, "<b>🔌 Connect to PM</b>\n\nClick the button below to connect this group chat to your private messages. This will allow you to search this group's files directly in the bot's PM.", &gotgbot.SendMessageOpts{
		ParseMode:   gotgbot.ParseModeHTML,
		ReplyMarkup: markup,
	})
	return err
}

func DisconnectCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}

	if m.Chat.Type == "private" {
		connChatID, err := _app.DB.GetUserConnection(m.From.Id)
		if err != nil || connChatID == 0 {
			_, err = m.Reply(bot, "❌ You are not connected to any chat.", nil)
			return err
		}

		err = _app.DB.SetUserConnection(m.From.Id, 0)
		if err != nil {
			_, err = m.Reply(bot, "❌ Failed to disconnect: DB error.", nil)
			return err
		}

		_, err = m.Reply(bot, "🔌 Disconnected successfully!", nil)
		return err
	}

	isAdmin, err := IsUserAdmin(bot, m.Chat.Id, m.From.Id)
	if err != nil || !isAdmin {
		_, _ = m.Reply(bot, "❌ Only group administrators can use this command.", nil)
		return nil
	}

	err = _app.DB.SetUserConnection(m.From.Id, 0)
	if err != nil {
		_, err = m.Reply(bot, "❌ Failed to disconnect: DB error.", nil)
		return err
	}

	_, err = m.Reply(bot, "🔌 Disconnected this group from your private messages.", nil)
	return err
}

func FormattingCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}

	guide := `<b>📝 TELEGRAM FORMATTING GUIDE</b>

You can format your welcome messages, file captions, and other templates using standard HTML tags:

• <b>Bold Text</b>:
  <code>&lt;b&gt;bold&lt;/b&gt;</code> or <code>&lt;strong&gt;bold&lt;/strong&gt;</code>

• <i>Italic Text</i>:
  <code>&lt;i&gt;italic&lt;/i&gt;</code> or <code>&lt;em&gt;italic&lt;/em&gt;</code>

• <u>Underline Text</u>:
  <code>&lt;u&gt;underline&lt;/u&gt;</code> or <code>&lt;ins&gt;underline&lt;/ins&gt;</code>

• <s>Strikethrough</s>:
  <code>&lt;s&gt;strikethrough&lt;/s&gt;</code> or <code>&lt;strike&gt;strikethrough&lt;/strike&gt;</code> or <code>&lt;del&gt;strikethrough&lt;/del&gt;</code>

• <span class="tg-spoiler">Spoiler Text</span>:
  <code>&lt;span class="tg-spoiler"&gt;spoiler&lt;/span&gt;</code> or <code>&lt;tg-spoiler&gt;spoiler&lt;/tg-spoiler&gt;</code>

• <code>Inline Code</code>:
  <code>&lt;code&gt;code&lt;/code&gt;</code>

• Block Code:
  <code>&lt;pre&gt;code block&lt;/pre&gt;</code>

• <a href="https://example.com">Inline Link</a>:
  <code>&lt;a href="https://example.com"&gt;link text&lt;/a&gt;</code>

• Blockquote:
  <code>&lt;blockquote&gt;quoted text&lt;/blockquote&gt;</code>

<b>⚠️ IMPORTANT:</b>
All tags must be properly opened and closed, and special characters like <code>&lt;</code>, <code>&gt;</code>, and <code>&amp;</code> must be escaped as <code>&amp;lt;</code>, <code>&amp;gt;</code>, and <code>&amp;amp;</code>.`

	_, err := m.Reply(bot, guide, &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}

// NewMoviesCommand handles the /new command.
func NewMoviesCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage
	if m == nil {
		return nil
	}

	statusMsg, err := m.Reply(bot, "🔍 <i>Fetching recently added 2026 movies...</i>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	if err != nil {
		return err
	}

	files, err := _app.DB.GetRecent2026Files(200)
	if err != nil {
		_app.Log.Error("NewMoviesCommand database error", zap.Error(err))
		_, _, _ = statusMsg.EditText(bot, "❌ Could not complete request. Please try again later.", nil)
		return nil
	}

	if len(files) == 0 {
		_, _, _ = statusMsg.EditText(bot, "😔 <b>No recent 2026 movies found in the database.</b>", &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}

	text, markup := buildRecentMoviesResponse(bot.Username, files)

	_, _, err = statusMsg.EditText(bot, text, &gotgbot.EditMessageTextOpts{
		ParseMode:          gotgbot.ParseModeHTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		ReplyMarkup:        markup,
	})
	return err
}

// NewMoviesCallback handles the btn_new callback query.
func NewMoviesCallback(bot *gotgbot.Bot, ctx *ext.Context) error {
	c := ctx.CallbackQuery
	m, ok := c.Message.(gotgbot.Message)
	if !ok {
		return nil
	}

	_, _ = c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Loading recently added 2026 movies..."})

	files, err := _app.DB.GetRecent2026Files(200)
	if err != nil {
		_app.Log.Error("NewMoviesCallback database error", zap.Error(err))
		return nil
	}

	if len(files) == 0 {
		markup := gotgbot.InlineKeyboardMarkup{
			InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{
				Text: "« Back to Search", CallbackData: "cmd:start",
			}, {
				Text: "🗑️ Close", CallbackData: "close",
			}}},
		}
		_, _, _ = m.EditText(bot, "😔 <b>No recent 2026 movies found in the database.</b>", &gotgbot.EditMessageTextOpts{
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: markup,
		})
		return nil
	}

	text, markup := buildRecentMoviesResponse(bot.Username, files)

	_, _, err = m.EditText(bot, text, &gotgbot.EditMessageTextOpts{
		ParseMode:          gotgbot.ParseModeHTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		ReplyMarkup:        markup,
	})
	return err
}

type recentMovieGroup struct {
	Title     string
	Year      string
	Languages string
	DeepLink  string
}

func buildRecentMoviesResponse(botUsername string, files []*model.File) (string, gotgbot.InlineKeyboardMarkup) {
	var movieGroups []recentMovieGroup
	seenGroups := make(map[string]bool)

	for _, f := range files {
		title := autofilter.ExtractBaseTitle(f.FileName)
		if title == "" {
			continue
		}

		cleanTitle := cleanCompareString(title)
		if cleanTitle == "" {
			continue
		}

		year := "2026"
		groupKey := cleanTitle + "_" + year
		if seenGroups[groupKey] {
			continue
		}
		seenGroups[groupKey] = true

		langs := extractRecentLanguages(f.FileName)
		encodedQuery := base64.RawURLEncoding.EncodeToString([]byte("s" + title + " " + year))
		deepLink := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, encodedQuery)

		movieGroups = append(movieGroups, recentMovieGroup{
			Title:     title,
			Year:      year,
			Languages: langs,
			DeepLink:  deepLink,
		})

		if len(movieGroups) >= 30 {
			break
		}
	}

	var sb strings.Builder
	sb.WriteString("🎬 <b>Here are the recently uploaded movies</b>\n")
	sb.WriteString("<i>Click on the title to get the movie</i>\n\n")

	for i, mg := range movieGroups {
		sb.WriteString(fmt.Sprintf("<b>%d. <a href=\"%s\">%s %s</a></b>\n", i+1, mg.DeepLink, htmlEscape(mg.Title), mg.Year))
		sb.WriteString(fmt.Sprintf("🎙 %s\n", mg.Languages))
	}

	markup := gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{
			Text: "« Back to Search", CallbackData: "cmd:start",
		}, {
			Text: "🗑️ Close", CallbackData: "close",
		}}},
	}

	return sb.String(), markup
}

func extractRecentLanguages(fileName string) string {
	lowerName := strings.ToLower(fileName)
	langMap := map[string][]string{
		"English":   {"english", "eng"},
		"Hindi":     {"hindi", "hin"},
		"Tamil":     {"tamil", "tam"},
		"Telugu":    {"telugu", "tel"},
		"Malayalam": {"malayalam", "mal"},
		"Kannada":   {"kannada", "kan"},
		"Spanish":   {"spanish", "spa"},
		"Korean":    {"korean", "kor"},
		"Italian":   {"italian", "ita"},
	}

	var langs []string
	order := []string{"English", "Hindi", "Tamil", "Telugu", "Malayalam", "Kannada", "Spanish", "Korean", "Italian"}
	for _, name := range order {
		tags := langMap[name]
		for _, tag := range tags {
			if strings.Contains(lowerName, tag) {
				langs = append(langs, name)
				break
			}
		}
	}

	if len(langs) == 0 {
		if strings.Contains(lowerName, "unknown") {
			return "Unknown"
		}
		return "Original Audio"
	}
	return strings.Join(langs, " \u2022 ")
}


