package core

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"time"

	"autofilterbot/internal/app"
	"autofilterbot/internal/cache"
	"autofilterbot/internal/config"
	"autofilterbot/internal/configpanel"
	"autofilterbot/internal/database/mongo"
	"autofilterbot/internal/fsub"
	"autofilterbot/internal/index"
	"autofilterbot/internal/middleware"
	"autofilterbot/internal/ott"
	"autofilterbot/pkg/autodelete"
	"autofilterbot/pkg/env"
	"autofilterbot/pkg/log"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

var _app *Core

// Core wraps various individual components of the app to orchestrate application processes.
type Core struct {
	app.App
	Ctx context.Context

	additionalURLsCount int
}

// extendedHandler returns a handlers.Response that calls

// RunAppOptions wraps command-line arguments for app startup.
type RunAppOptions struct {
	MongodbURI         string
	LogLevel           string
	BotToken           string
	DisableConsoleLogs bool
}

// Run starts the application and initializes core components.
func Run(opts RunAppOptions) {
	err := godotenv.Load(".env") // config.env is supported bcuz other repos use it for some reason
	if err != nil {
		fmt.Println("ERROR: load variables from .env file failed", err)
	}

	logLevel := opts.LogLevel
	if s := os.Getenv("LOG_LEVEL"); s != "" {
		logLevel = s
	}

	log.Initialize(logLevel, opts.DisableConsoleLogs)
	logger := log.Logger()

	botToken := opts.BotToken
	if s := os.Getenv("BOT_TOKEN"); s != "" {
		botToken = s
	}

	if botToken == "" {
		logger.Fatal("bot token not provided")
	}

	bot, err := gotgbot.NewBot(botToken, &gotgbot.BotOpts{})
	if err != nil {
		logger.Fatal("create bot failed", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background()) // all background jobs (tickers) must use this ctx

	mongodbUri := opts.MongodbURI
	if s := os.Getenv("MONGODB_URI"); s != "" {
		mongodbUri = s
	}

	if mongodbUri == "" {
		logger.Fatal("MONGODB_URI not provided. Set it as an environment variable or provide as a command line argument.")
	}

	databaseName := os.Getenv("DATABASE_NAME")
	collectionName := os.Getenv("COLLECTION_NAME")

	var additionalUri []string

	for i := 1; i <= 5; i++ { // attempts to fetch MONGODB_URI1 to MONGODB_URI5. //TODO: remove hardcoded limit after testing
		if s := os.Getenv(fmt.Sprintf("MONGODB_URI%d", i)); s != "" {
			additionalUri = append(additionalUri, s)
		}
	}

	mongoOpts := mongo.NewClientOpts{
		DatabaseName:        databaseName,
		FilesCollectionName: collectionName,
		AdditionalURLs:      additionalUri,
	}

	db, err := mongo.NewClient(ctx, mongodbUri, bot.Id, logger, mongoOpts)

	if err != nil {
		logger.Fatal("database setup failed", zap.Error(err))
	}

	appConfig, err := db.GetConfig(bot.Id)
	if err != nil {
		logger.Error("failed to load configs from db", zap.Error(err))
	}

	if appConfig.FileCollectionIndex != 0 {
		err = db.UpdateStorageCollection(appConfig.FileCollectionIndex)
		if err != nil {
			logger.Warn("setting custom storage collection failed, using default database", zap.Error(err))
		}
	}

	autodeleteManager, err := autodelete.NewManager(bot, logger)
	if err != nil {
		logger.Error("autodelete module setup failed", zap.Error(err))
	}

	go autodeleteManager.Run(ctx)

	_app = &Core{
		App: app.App{
			DB:           db,
			Config:       appConfig,
			Bot:          bot,
			Log:          logger,
			AutoDelete:   autodeleteManager,
			StartTime:    time.Now(),
			Cache:        cache.NewCache(),
			Admins:       env.Int64s("ADMINS"),
			IndexManager: index.NewManager(),
			Analytics:    NewAnalyticsService(),
			WorkerPool:   NewWorkerPool(8, 1000), // 8 workers, 1000 queue size
			LogChannel:   env.Int64("LOG_CHANNEL"),
		},
		Ctx: ctx,
	}

	_app.Analytics.(*AnalyticsService).SetApp(_app) // Ensure analytics has access to db
	_app.WorkerPool.(*WorkerPool).Start(ctx, logger)

	_app.additionalURLsCount = len(additionalUri)
	_app.ConfigPanel = configpanel.CreatePanel(_app)
	_app.MigrateFsubLinks()

	dispatcher := SetupDispatcher(logger)
	updater := ext.NewUpdater(dispatcher, &ext.UpdaterOpts{
		UnhandledErrFunc: func(err error) {
			logger.Debug("updater: unhandled error", zap.Error(err))
		},
	})

	err = updater.StartPolling(bot, &ext.PollingOpts{
		DropPendingUpdates: true,
		GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
			AllowedUpdates: []string{"message", "channel_post", "inline_query", "chosen_inline_result", "callback_query", "chat_join_request", "chat_member"},
		},
	})
	if err != nil {
		logger.Fatal(
			"failed to start polling updates",
			zap.Error(err),
		)
	}

	// Render Optimizations: 512MB RAM management
	debug.SetGCPercent(50) // more frequent GC to keep heap small
	go Recycler(logger)
	go middleware.ClearRateLimitOldEntries()

	logger.Info(fmt.Sprintf("@%s started successfully !", bot.Username))
	logger.Info("reaction feature status", zap.Bool("enabled", appConfig.GetEmojiMode()))
	go func() {
		port := os.Getenv("PORT")
		if port == "" {
			port = "10001"
		}
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "Bot is alive!")
		})
		logger.Info(fmt.Sprintf("starting health check server on port %s", port))
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			logger.Error("failed to start health check server", zap.Error(err))
		}
	}()

	go _app.RestartActiveIndexOperations(ctx)

	if appConfig.FileCollectionUpdater {
		_app.DB.RunCollectionUpdater(ctx, logger)
	}

	ott.RunScheduler(ctx, &_app.App)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	s := <-c // wait until an interrupt signal is received
	logger.Info("stopping app: interrupt signal received", zap.Any("signal", s))

	updater.Stop()

	cancel() // autodelete & mongo updater should stop with this
	_app.DB.Shutdown()
}

// AuthAdmin reports whether the user who sent the message is an admin or otherwise sends a warn message.
func (core *Core) AuthAdmin(ctx *ext.Context) bool {
	switch {
	case ctx.Message != nil:
		if !containsI64(_app.Admins, ctx.Message.From.Id) {
			_app.Log.Warn("authadmin: unauthorized access attempt", zap.Int64("user_id", ctx.Message.From.Id), zap.Int64s("admins", _app.Admins))
			ctx.Message.Reply(core.Bot, "<b>𝖮𝗇𝗅𝗒 𝖺𝗇 𝖺𝖽𝗆𝗂𝗇 𝖼𝖺𝗇 𝗎𝗌𝖾 𝗍𝗁𝖺𝗍 𝖼𝗈𝗆𝗆𝖺𝗇𝖽, 𝖯𝖾𝖺𝗌𝖺𝗇𝗍❗</b>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
			return false
		}
	case ctx.CallbackQuery != nil:
		if !containsI64(_app.Admins, ctx.CallbackQuery.From.Id) {
			_app.Log.Warn("authadmin: unauthorized callback attempt", zap.Int64("user_id", ctx.CallbackQuery.From.Id), zap.Int64s("admins", _app.Admins))
			ctx.CallbackQuery.Answer(_app.Bot, &gotgbot.AnswerCallbackQueryOpts{Text: "𝖮𝗇𝗅𝗒 𝖺𝗇 𝖺𝖽𝗆𝗂𝗇 𝖼𝖺𝗇 𝗎𝗌𝖾 𝗍𝗁𝖺𝗍 𝖼𝗈𝗆𝗆𝖺𝗇𝖽, 𝖯𝖾𝖺𝗌𝖺𝗇𝗍❗", ShowAlert: true})
			return false
		}
	default:
		_app.Log.Warn("authadmin: unsupported update received", zap.Int64("update_id", ctx.UpdateId))
		return false
	}

	return true
}

// RefreshConfig refetches the bot configs from db.
func (core *Core) RefreshConfig() {
	c, err := core.DB.GetConfig(core.Bot.Id)
	if err != nil {
		core.Log.Error("failed to refresh configs", zap.Error(err))
	}

	core.Config = c

	// Flush the membership cache so users are re-checked against the
	// new set of Fsub channels. Without this, stale cached entries
	// cause the bot to silently skip enforcement on newly-added channels
	// or keep enforcing removed channels.
	fsub.ClearMembershipCache()
}

// RestartActiveIndexOperations restarts all active index operations.
func (c *Core) RestartActiveIndexOperations(ctx context.Context) {
	ops, err := c.DB.GetActiveIndexOperations()
	if err != nil {
		_app.Log.Debug("core: failed to fetch active index operations", zap.Error(err))
		return
	}

	if len(ops) == 0 {
		return
	}

	c.Log.Debug("core: restarting active index operations", zap.Int("num", len(ops)))

	for _, i := range ops {
		ctx, o := c.IndexManager.NewOperation(ctx, i, c.DB, c.Log, c.Bot)
		c.IndexManager.RunOperation(ctx, o)
	}
}

// GetAdditionalCollectionCount returns the number of additional db urls provided.
func (c *Core) GetAdditionalCollectionCount() int {
	return c.additionalURLsCount
}

func (c *Core) SetCollectionIndex(index int) {
	err := c.DB.UpdateStorageCollection(index)
	if err != nil {
		c.Log.Warn("core: failed to update collection index", zap.Error(err))
	}
}

func (c *Core) GetContext() context.Context {
	return c.Ctx
}

// App returns the initialized global app instance.
func Application() *Core {
	return _app
}

// LogUpdate prints update information to debug log and tracks active users and groups.
func LogUpdate(bot *gotgbot.Bot, ctx *ext.Context) error {
	_app.Log.Debug(fmt.Sprintf("received %s update (%d)", ctx.GetType(), ctx.UpdateId))

	// Track user and group asynchronously to keep processing lag-free
	go func() {
		if ctx.EffectiveUser != nil && !ctx.EffectiveUser.IsBot {
			// Save/update user statistics
			_ = _app.DB.SaveUserExtended(ctx.EffectiveUser.Id, "active", 0, ctx.EffectiveUser.LanguageCode)
		}

		if ctx.EffectiveChat != nil && (ctx.EffectiveChat.Type == "group" || ctx.EffectiveChat.Type == "supergroup") {
			// Ensure group is registered in the database config/collection
			_, _ = _app.DB.GetGroupConfig(ctx.EffectiveChat.Id)
		}
	}()

	return nil
}

// Recycler periodically frees OS memory to keep the footprint low on Render's free tier.
func Recycler(log *zap.Logger) {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		debug.FreeOSMemory()
		log.Debug("memory recycler: FreeOSMemory() called")
	}
}

// containsI64 reports whether the given value is in a slice.
func containsI64(s []int64, val int64) bool {
	for _, i := range s {
		if i == val {
			return true
		}
	}
	return false
}

// AddFileChannel adds a channel ID to the list of monitored channels in the database config.
func (core *Core) AddFileChannel(channelId int64) error {
	// Check if already in DB config
	for _, id := range core.Config.FileChannels {
		if id == channelId {
			return nil
		}
	}
	// Check if already in env FILE_CHANNELS
	for _, id := range env.Int64s("FILE_CHANNELS") {
		if id == channelId {
			return nil
		}
	}

	newChannels := append(core.Config.FileChannels, channelId)
	err := core.DB.UpdateConfig(core.Bot.Id, "file_channels", newChannels)
	if err != nil {
		core.Log.Error("AddFileChannel: failed to update config in DB", zap.Error(err))
		return err
	}

	core.RefreshConfig()
	core.Log.Info("automatically added channel to monitored channels", zap.Int64("channel_id", channelId))
	return nil
}

// MigrateFsubLinks scans all ForceSub channels and updates them to request invite links if they aren't already.
func (core *Core) MigrateFsubLinks() {
	channels := core.Config.GetFsubChannels()
	if len(channels) == 0 {
		return
	}

	updated := false
	for i, ch := range channels {
		// If CreatesJoinRequest is false, or if invite link is empty, or doesn't look like a request link (contains no '+')
		if !ch.CreatesJoinRequest || ch.InviteLink == "" || !strings.Contains(ch.InviteLink, "+") {
			chat, err := core.Bot.GetChat(ch.ID, nil)
			if err != nil {
				core.Log.Warn("MigrateFsubLinks: failed to get chat info", zap.Int64("chat_id", ch.ID), zap.Error(err))
				continue
			}

			// Generate a request invite link (creates join request)
			link, err := core.Bot.CreateChatInviteLink(ch.ID, &gotgbot.CreateChatInviteLinkOpts{
				Name:               "ForceSub",
				CreatesJoinRequest: true,
			})
			if err != nil {
				core.Log.Warn("MigrateFsubLinks: failed to create request invite link. Make sure bot is Admin!", zap.Int64("chat_id", ch.ID), zap.Error(err))
				continue
			}

			channels[i].InviteLink = link.InviteLink
			channels[i].CreatesJoinRequest = true
			channels[i].Title = chat.Title
			updated = true
			core.Log.Info("MigrateFsubLinks: successfully migrated channel to request invite link", zap.Int64("chat_id", ch.ID), zap.String("link", link.InviteLink))
		}
	}

	if updated {
		err := core.DB.UpdateConfig(core.Bot.Id, config.FieldNameFsub, channels)
		if err != nil {
			core.Log.Error("MigrateFsubLinks: failed to save migrated config to DB", zap.Error(err))
		} else {
			core.RefreshConfig()
		}
	}
}

