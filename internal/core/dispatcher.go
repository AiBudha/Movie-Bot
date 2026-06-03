package core

import (
	"fmt"
	"runtime/debug"
	"strings"

	"time"
	"autofilterbot/internal/middleware"
	"autofilterbot/pkg/conversation"
	"autofilterbot/pkg/env"
	exthandlers "autofilterbot/pkg/filters"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	"go.uber.org/zap"
)

const (
	autofilterHandlerGroup = iota + 1
	commandHandlerGroup
	callbackQueryGroup
	miscHandlerGroup
	middleWareGroup
	joinRequestGroup
	antiSpamGroup     = -10
	autoReactionGroup = -5
)

// SetupDispatcher creates a new empty dispatcher with error and panic recovery setup.
func SetupDispatcher(log *zap.Logger) *ext.Dispatcher {
	d := ext.NewDispatcher(&ext.DispatcherOpts{
		// If an error is returned by a handler, log it and continue going.
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			logFields := []zap.Field{zap.Error(err)}
			logFields = addLogFieldsFromContext(ctx, logFields)

			log.Error("error occurred while handling update", logFields...)

			return ext.DispatcherActionNoop
		},

		UnhandledErrFunc: func(err error) {
			log.Debug("dispatcher: unhandled error", zap.Error(err))
		},

		Panic: func(b *gotgbot.Bot, ctx *ext.Context, r interface{}) {
			logFields := []zap.Field{zap.String("panic", fmt.Sprintf("%v\n%s", r, cleanedStack()))}
			logFields = addLogFieldsFromContext(ctx, logFields)

			log.Error("panic recovered", logFields...)
		},
	})

	// 0. Global Middlewares
	d.AddHandlerToGroup(handlers.NewMessage(message.All, middleware.AntiSpam(5*time.Second)), antiSpamGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.All, middleware.AntiSpam(2*time.Second)), antiSpamGroup)

	d.AddHandlerToGroup(handlers.NewMessage(func(msg *gotgbot.Message) bool {
		return !message.Command(msg)
	}, func(bot *gotgbot.Bot, ctx *ext.Context) error {
		// Dynamically fetch config to ensure we use the latest from settings panel
		return middleware.AutoReaction(_app.Config, log)(bot, ctx)
	}), autoReactionGroup)

	d.AddHandlerToGroup(handlers.NewMessage(message.All, conversation.MessageHandler), 0)

	d.AddHandlerToGroup(handlers.NewMessage(func(msg *gotgbot.Message) bool {
		return !message.Command(msg)
	}, Autofilter), autofilterHandlerGroup)

	d.AddHandlerToGroup(handlers.NewCommand("start", StartCommand), commandHandlerGroup)
	d.AddHandlerToGroup(handlers.NewCommand("admin", AdminPanel), commandHandlerGroup)
	d.AddHandlerToGroup(exthandlers.NewCommands([]string{"about", "help", "privacy", "movies", "series"}, StaticCommands), commandHandlerGroup)
	d.AddHandlerToGroup(handlers.NewCommand("delete", DeleteFile).SetAllowChannel(true), commandHandlerGroup)
	d.AddHandlerToGroup(exthandlers.NewCommands([]string{"deleteall", "delall"}, DeleteAllFiles), commandHandlerGroup)
	d.AddHandlerToGroup(exthandlers.NewCommands([]string{"settings", "configs", "configpanel"}, Settings), commandHandlerGroup)
	d.AddHandlerToGroup(handlers.NewCommand("logs", Logs), commandHandlerGroup)
	d.AddHandlerToGroup(handlers.NewCommand("stats", Stats), commandHandlerGroup)
	d.AddHandlerToGroup(handlers.NewCommand("fstats", FStats), commandHandlerGroup)
	d.AddHandlerToGroup(handlers.NewCommand("batch", NewBatch), commandHandlerGroup)
	d.AddHandlerToGroup(handlers.NewCommand("genlink", GenLink), commandHandlerGroup)
	d.AddHandlerToGroup(handlers.NewCommand("broadcast", Broadcast), commandHandlerGroup)
	d.AddHandlerToGroup(handlers.NewCommand("index", CmdIndex), commandHandlerGroup)
	d.AddHandlerToGroup(handlers.NewCommand("setskip", SetSkip), commandHandlerGroup)
	d.AddHandlerToGroup(handlers.NewCommand("fsub", SetFsub), commandHandlerGroup)
	d.AddHandlerToGroup(handlers.NewCommand("post", PostCommand), commandHandlerGroup)
	d.AddHandlerToGroup(handlers.NewCommand("id", IdCommand).SetAllowChannel(true), commandHandlerGroup)
	d.AddHandlerToGroup(handlers.NewMessage(message.All, AutoDetectIndex), miscHandlerGroup)

	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("cmd"), StaticCommands), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("admin"), AdminCallbackHandler), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("post_"), PostCallbackHandler), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("close"), Close), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("navg"), Navigate), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("fdetails"), FileDetails), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("sel"), Select), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("sendsel"), SendSelected), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("all"), All), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("ignore"), Ignore), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("config"), ConfigPanel), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Equal("stats"), Stats), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Equal("fstats"), FStats), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("index"), CbIndex), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("index_choice"), CbIndexChoice), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("fsub_add"), AddToFsub), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Equal("fsub_verify"), FsubJoined), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("index_prompt"), PromptIndexCallback), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("sn"), SeasonCallback), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("lang"), LanguageCallback), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("af"), SeasonListCallback), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("suggest"), Autofilter), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("reset"), Autofilter), callbackQueryGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("trend"), Autofilter), callbackQueryGroup)

	d.AddHandlerToGroup(handlers.NewMessage(IsMonitoredChannel, NewFile).SetAllowChannel(true).SetAllowEdited(true), miscHandlerGroup)
	d.AddHandlerToGroup(handlers.NewChatJoinRequest(func(cjr *gotgbot.ChatJoinRequest) bool { return true }, HandleJoinRequest), joinRequestGroup)
	d.AddHandlerToGroup(handlers.NewChatMember(func(cm *gotgbot.ChatMemberUpdated) bool { return true }, HandleChatMember), joinRequestGroup)

	d.AddHandlerToGroup(handlers.NewInlineQuery(func(iq *gotgbot.InlineQuery) bool { return true }, InlineSearch), miscHandlerGroup)
	d.AddHandlerToGroup(handlers.NewCallback(callbackquery.Prefix("sendfile"), SendFileCallback), callbackQueryGroup)

	d.AddHandlerToGroup(exthandlers.NewAllUpdates(LogUpdate), miscHandlerGroup)

	return d
}

// IsMonitoredChannel reports whether the message is from one of the statically or dynamically monitored channels.
func IsMonitoredChannel(m *gotgbot.Message) bool {
	chatId := m.Chat.Id
	// 1. Check environment variables
	for _, id := range env.Int64s("FILE_CHANNELS") {
		if id == chatId {
			return true
		}
	}
	// 2. Check database config
	if _app != nil && _app.Config != nil {
		for _, id := range _app.Config.FileChannels {
			if id == chatId {
				return true
			}
		}
	}
	return false
}

// logFieldsFromContext adds zap fields to logFields about specific update from ctx to help troubleshooting.
func addLogFieldsFromContext(ctx *ext.Context, logFields []zap.Field) []zap.Field {
	switch {
	case ctx.Message != nil:
		logFields = append(logFields,
			zap.Int64("chat_id", ctx.Message.Chat.Id),
			zap.Int64("message_id", ctx.Message.MessageId),
			zap.String("text", ctx.Message.Text),
		)
	case ctx.CallbackQuery != nil:
		logFields = append(logFields,
			zap.String("callback_query_id", ctx.CallbackQuery.Id),
			zap.String("data", ctx.CallbackQuery.Data),
		)
		if ctx.CallbackQuery.Message != nil {
			logFields = append(logFields,
				zap.Int64("chat_id", ctx.CallbackQuery.Message.GetChat().Id),
				zap.Int64("message_id", ctx.CallbackQuery.Message.GetMessageId()),
			)
		} else if ctx.CallbackQuery.InlineMessageId != "" {
			logFields = append(logFields,
				zap.String("inline_message_id", ctx.CallbackQuery.InlineMessageId),
			)
		}
	case ctx.InlineQuery != nil:
		logFields = append(logFields,
			zap.String("inline_query_id", ctx.InlineQuery.Id),
			zap.String("query", ctx.InlineQuery.Query),
			zap.Int64("from", ctx.InlineQuery.From.Id),
		)
	}

	return logFields
}

// cleanedStack returns stack trace with gotgbot library parts removed to prevent confusion.
// Copied from https://github.com/PaulSonOfLars/gotgbot/blob/v2/ext/dispatcher.go.
func cleanedStack() string {
	lines := strings.Split(string(debug.Stack()), "\n")
	if len(lines) > 4 {
		return strings.Join(lines[4:], "\n")
	}
	return strings.Join(lines, "\n")
}
