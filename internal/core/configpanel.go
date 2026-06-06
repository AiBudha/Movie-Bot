package core

import (
	"autofilterbot/internal/button"
	"autofilterbot/pkg/panel"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
)

// Settings handles the /settings command which acts as the entrypoint into the config panel.
func Settings(bot *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat != nil && ctx.EffectiveChat.Type != "private" {
		return GroupSettings(bot, ctx)
	}

	if !_app.AuthAdmin(ctx) {
		return nil
	}

	// Mock or set the CallbackQuery data to "config" to get the root panel page
	if ctx.CallbackQuery == nil {
		ctx.CallbackQuery = &gotgbot.CallbackQuery{
			Data: "config",
		}
	} else {
		ctx.CallbackQuery.Data = "config"
	}

	content, markup, err := panel.ProcessUpdate(_app.ConfigPanel, ctx, bot)
	if err != nil {
		_app.Log.Error("failed to process config panel update", zap.Error(err))
		return err
	}

	if len(markup) == 0 {
		markup = [][]gotgbot.InlineKeyboardButton{{button.Close()}}
	}

	// Customize root page back row with Back to Admin and Close buttons
	if len(markup) > 0 {
		lastRowIdx := len(markup) - 1
		if len(markup[lastRowIdx]) == 1 && (markup[lastRowIdx][0].CallbackData == "close" || markup[lastRowIdx][0].Text == "🗑️ Close") {
			markup[lastRowIdx] = []gotgbot.InlineKeyboardButton{
				{Text: "🔙 Back", CallbackData: "admin:back"},
				{Text: "Close ❌", CallbackData: "admin:close"},
			}
		}
	}

	var sendErr error
	if ctx.CallbackQuery != nil {
		_, _, sendErr = ctx.CallbackQuery.Message.EditText(bot, content, &gotgbot.EditMessageTextOpts{
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: markup},
			ParseMode:   gotgbot.ParseModeHTML,
		})
	} else if ctx.Message != nil {
		_, sendErr = ctx.Message.Reply(bot, content, &gotgbot.SendMessageOpts{
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: markup},
			ParseMode:   gotgbot.ParseModeHTML,
		})
	} else {
		_, sendErr = bot.SendMessage(ctx.EffectiveChat.Id, content, &gotgbot.SendMessageOpts{
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: markup},
			ParseMode:   gotgbot.ParseModeHTML,
		})
	}

	if sendErr != nil {
		_app.Log.Warn("send/edit settings msg failed", zap.Error(sendErr))
	}

	return nil
}

// ConfigPanel handles callback queries for the config panel.
func ConfigPanel(bot *gotgbot.Bot, ctx *ext.Context) error {
	if !_app.AuthAdmin(ctx) {
		return nil
	}

	err := _app.ConfigPanel.HandleUpdate(ctx, bot)
	if err != nil {
		_app.Log.Warn("handle config panel failed", zap.Error(err))
	}

	return nil
}
