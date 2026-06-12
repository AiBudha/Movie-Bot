package core

import (
	"strings"

	"autofilterbot/pkg/conversation"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
)

// AdminPanel sends the admin dashboard with inline buttons.
func AdminPanel(bot *gotgbot.Bot, ctx *ext.Context) error {
	if !_app.AuthAdmin(ctx) {
		return nil
	}

	text := "<b>🛠️ ADMIN PANEL</b>\n\n<i>Select a category below to manage the bot:</i>"
	markup := gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			// Row 1: Stats & Overview
			{{Text: "Stats 📊", CallbackData: "admin:stats"}, {Text: "Users 👥", CallbackData: "admin:users"}, {Text: "FSub Stats 📈", CallbackData: "admin:fstats"}},
			// Row 2: Messages & Advertising
			{{Text: "Broadcast 📢", CallbackData: "admin:broadcast"}, {Text: "History 📂", CallbackData: "admin:bchistory"}, {Text: "Create Post 📮", CallbackData: "admin:post"}},
			// Row 3: File Syncing & Indexing
			{{Text: "Index 🗂️", CallbackData: "admin:index"}, {Text: "Batch 📦", CallbackData: "admin:batch"}, {Text: "GenLink 🔗", CallbackData: "admin:genlink"}},
			// Row 4: Delete Operations
			{{Text: "Delete 🗑️", CallbackData: "admin:delete"}, {Text: "Delete All 🚯", CallbackData: "admin:deleteall"}, {Text: "Clean Quality 🧹", CallbackData: "admin:clean"}, {Text: "Clean Dups 👥", CallbackData: "admin:cleandups"}},
			// Row 5: Skipping & Logs
			{{Text: "Set Skip ⏭️", CallbackData: "admin:setskip"}, {Text: "Logs 📄", CallbackData: "admin:logs"}},
			
			// === CONFIG SETTINGS ===
			// Row 6: Size Button, Poster, Results Channel
			{{Text: "Size Button 📏", CallbackData: "config:sizebtn"}, {Text: "Poster 🖼️", CallbackData: "config:poster"}, {Text: "Results Chan 📢", CallbackData: "config:reschan"}},
			// Row 7: Auto Delete, File AutoDelete, Monitored Chans
			{{Text: "Auto Delete ⏱️", CallbackData: "config:autodel"}, {Text: "File AutoDelete 📁", CallbackData: "config:filedel"}, {Text: "Monitored Chans 🎧", CallbackData: "config:moniterd"}},
			// Row 8: Force Sub 🔒, Group Defaults, Database Config
			{{Text: "Force Sub 🔒", CallbackData: "config:fsub"}, {Text: "Group Defaults 👥", CallbackData: "config:group"}, {Text: "Database 📂", CallbackData: "config:db"}},
			// Row 9: Result Button, Footer Buttons
			{{Text: "Result Button 🔗", CallbackData: "config:resbtn"}, {Text: "Footer Buttons 🎛️", CallbackData: "config:footer"}},
			
			// Row 10: Close
			{{Text: "Close ❌", CallbackData: "admin:close"}},
		},
	}

	var err error
	if ctx.CallbackQuery != nil {
		_, _, err = ctx.CallbackQuery.Message.EditText(bot, text, &gotgbot.EditMessageTextOpts{
			ReplyMarkup: markup,
			ParseMode:   gotgbot.ParseModeHTML,
		})
	} else {
		_, err = bot.SendMessage(ctx.EffectiveChat.Id, text, &gotgbot.SendMessageOpts{
			ReplyMarkup: markup,
			ParseMode:   gotgbot.ParseModeHTML,
		})
	}

	if err != nil {
		_app.Log.Warn("admin_panel: send/edit failed", zap.Error(err))
	}

	return nil
}

// AdminCallbackHandler handles all callbacks starting with "admin:".
func AdminCallbackHandler(bot *gotgbot.Bot, ctx *ext.Context) error {
	if !_app.AuthAdmin(ctx) {
		return nil
	}

	c := ctx.CallbackQuery
	data := c.Data
	action := strings.TrimPrefix(data, "admin:")

	// Answer callback to remove loading state for standard actions
	if !strings.HasPrefix(action, "bc_") && !strings.HasPrefix(action, "bchist_") {
		c.Answer(bot, nil)
	}

	// Handle prefix actions
	switch {
	case action == "bchistory":
		return ListBroadcastHistory(bot, ctx)
	case strings.HasPrefix(action, "bchist_view:"):
		return ViewBroadcastDetails(bot, ctx, strings.TrimPrefix(action, "bchist_view:"))
	case strings.HasPrefix(action, "bchist_delmsg:"):
		return HandleDeleteBroadcastMessages(bot, ctx, strings.TrimPrefix(action, "bchist_delmsg:"))
	case strings.HasPrefix(action, "bchist_delrec:"):
		return HandleDeleteBroadcastRecord(bot, ctx, strings.TrimPrefix(action, "bchist_delrec:"))
	case strings.HasPrefix(action, "bc_send:"):
		return HandleConfirmBroadcast(bot, ctx, strings.TrimPrefix(action, "bc_send:"))
	case strings.HasPrefix(action, "bc_cancel:"):
		return HandleCancelBroadcast(bot, ctx, strings.TrimPrefix(action, "bc_cancel:"))
	}

	switch action {
	case "back":
		return AdminPanel(bot, ctx)
	case "cancel":
		conversation.Cancel(ctx.EffectiveChat.Id, c.From.Id)
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Operation Cancelled ❌"})
		_, err := c.Message.Delete(bot, nil)
		return err
	case "stats", "users":
		return Stats(bot, ctx)
	case "fstats":
		return FStats(bot, ctx)
	case "broadcast":
		return Broadcast(bot, ctx)
	case "index":
		return CmdIndex(bot, ctx)
	case "batch":
		return NewBatch(bot, ctx)
	case "genlink":
		return GenLink(bot, ctx)
	case "delete":
		return DeleteFile(bot, ctx)
	case "deleteall":
		return DeleteAllFiles(bot, ctx)
	case "clean":
		return CleanQuality(bot, ctx)
	case "cleandups":
		return CleanDuplicatesCommand(bot, ctx)
	case "fsub":
		return SetFsub(bot, ctx)
	case "setskip":
		return SetSkip(bot, ctx)
	case "settings":
		return Settings(bot, ctx)
	case "logs":
		return Logs(bot, ctx)
	case "post":
		return PostCommand(bot, ctx)
	case "close":
		return Close(bot, ctx)
	default:
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Unknown Admin Action", ShowAlert: true})
	}

	return nil
}
