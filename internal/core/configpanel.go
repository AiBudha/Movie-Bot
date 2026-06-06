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

	return AdminPanel(bot, ctx)
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
