package core

import (
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


	text := "<b>⚙️ Cʟɪᴄᴋ Tʜᴇ Bᴜᴛᴛᴏɴ Bᴇʟᴏᴡ Tᴏ Oᴘᴇɴ Tʜᴇ Cᴏɴғɪɢ Pᴀɴᴇʟ 👇</b>"
	markup := gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{{Text: "𝖮𝖯𝖤𝖭", CallbackData: "config"}},
			{{Text: "🔙 Back", CallbackData: "admin:back"}},
		},
	}

	var err error
	if ctx.CallbackQuery != nil {
		_, _, err = ctx.CallbackQuery.Message.EditText(bot, text, &gotgbot.EditMessageTextOpts{
			ReplyMarkup: markup,
			ParseMode:   gotgbot.ParseModeHTML,
		})
	} else if ctx.Message != nil {
		_, err = ctx.Message.Reply(bot, text, &gotgbot.SendMessageOpts{
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
		_app.Log.Warn("send/edit settings msg failed", zap.Error(err))
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
