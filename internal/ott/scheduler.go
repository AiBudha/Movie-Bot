package ott

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"autofilterbot/internal/app"
	"autofilterbot/internal/database"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"go.uber.org/zap"
)

func getOTTTargets() []int64 {
	// 1. Check OTT_CHANNEL_ID
	if s := os.Getenv("OTT_CHANNEL_ID"); s != "" {
		var list []int64
		for _, part := range strings.Split(s, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if val, err := strconv.ParseInt(part, 10, 64); err == nil {
				list = append(list, val)
			}
		}
		if len(list) > 0 {
			return list
		}
	}
	// 2. Check CHAT_ID
	if s := os.Getenv("CHAT_ID"); s != "" {
		var list []int64
		for _, part := range strings.Split(s, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if val, err := strconv.ParseInt(part, 10, 64); err == nil {
				list = append(list, val)
			}
		}
		if len(list) > 0 {
			return list
		}
	}
	// 3. Fallback to LOG_CHANNEL
	if s := os.Getenv("LOG_CHANNEL"); s != "" {
		if val, err := strconv.ParseInt(s, 10, 64); err == nil {
			return []int64{val}
		}
	}
	return nil
}

func getUpdateInterval() time.Duration {
	s := os.Getenv("UPDATE_INTERVAL_HOURS")
	if s != "" {
		if val, err := strconv.Atoi(s); err == nil && val > 0 {
			return time.Duration(val) * time.Hour
		}
	}
	return 2 * time.Hour
}

// SendUpdates fetches new releases and posts them to target channels.
func SendUpdates(ctx context.Context, bot *gotgbot.Bot, db database.Database, log *zap.Logger) {
	log.Info("⏰ Scheduled OTT job: checking for new releases ...")
	targets := getOTTTargets()
	if len(targets) == 0 {
		log.Info("No OTT target channels configured - skipping send.")
		return
	}

	// Fetch releases from the last 7 days, with dedup = true
	releases, err := GetNewReleases(ctx, db, 7, true)
	if err != nil {
		log.Error("Failed to fetch new releases", zap.Error(err))
		return
	}

	if len(releases) == 0 {
		log.Info("No new OTT releases found this cycle.")
		return
	}

	log.Info("Found new OTT items to post", zap.Int("count", len(releases)))

	// Limit to first 10 items per cycle to avoid flooding
	limit := 10
	if len(releases) < limit {
		limit = len(releases)
	}

	for _, item := range releases[:limit] {
		text := FormatItemMessage(item)
		kb := FormatItemKeyboard(item)

		for _, target := range targets {
			var err error
			if item.Poster != "" {
				// Send photo
				_, err = bot.SendPhoto(target, gotgbot.InputFileByURL(item.Poster), &gotgbot.SendPhotoOpts{
					Caption:     text,
					ParseMode:   gotgbot.ParseModeHTML,
					ReplyMarkup: kb,
				})
				if err != nil {
					// Fallback to text message if photo fails
					log.Warn("Failed to send OTT photo, falling back to text", zap.Int64("chat_id", target), zap.Error(err))
					_, err = bot.SendMessage(target, text, &gotgbot.SendMessageOpts{
						ParseMode:          gotgbot.ParseModeHTML,
						LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: false},
						ReplyMarkup:        kb,
					})
				}
			} else {
				// Send text message
				_, err = bot.SendMessage(target, text, &gotgbot.SendMessageOpts{
					ParseMode:          gotgbot.ParseModeHTML,
					LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: false},
					ReplyMarkup:        kb,
				})
			}

			if err != nil {
				log.Error("Failed to post OTT release to channel", zap.Int64("chat_id", target), zap.Error(err))
			}
			time.Sleep(500 * time.Millisecond) // stay inside rate limits
		}
	}
}

// SendDailyDigest posts a daily summary of new releases.
func SendDailyDigest(ctx context.Context, bot *gotgbot.Bot, db database.Database, log *zap.Logger) {
	log.Info("🌙 Daily OTT digest: preparing today's release list ...")
	targets := getOTTTargets()
	if len(targets) == 0 {
		log.Info("No OTT target channels configured - skipping daily digest.")
		return
	}

	// Fetch releases from the last 1 day, with dedup = false
	releases, err := GetNewReleases(ctx, db, 1, false)
	if err != nil {
		log.Error("Failed to fetch daily releases", zap.Error(err))
		return
	}

	if len(releases) == 0 {
		log.Info("Daily OTT digest: no new releases today.")
		return
	}

	var moviesCount, seriesCount int
	for _, it := range releases {
		if it.Type == "movie" {
			moviesCount++
		} else {
			seriesCount++
		}
	}

	var lines []string
	lines = append(lines, "🍿 <b>Daily Latest Releases</b>")
	lines = append(lines, fmt.Sprintf("📅 <b>Date (UTC):</b> %s", time.Now().UTC().Format("2006-01-02")))
	lines = append(lines, fmt.Sprintf("🎬 Movies: <b>%d</b>  |  📺 Series: <b>%d</b>", moviesCount, seriesCount))
	lines = append(lines, "")

	limit := 40
	if len(releases) < limit {
		limit = len(releases)
	}

	for idx, item := range releases[:limit] {
		icon := "🎬"
		if item.Type != "movie" {
			icon = "📺"
		}
		lines = append(lines, fmt.Sprintf("%d. %s %s", idx+1, icon, item.Title))
	}

	if len(releases) > 40 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("...and %d more", len(releases)-40))
	}

	text := strings.Join(lines, "\n")

	for _, target := range targets {
		_, err := bot.SendMessage(target, text, &gotgbot.SendMessageOpts{
			ParseMode:          gotgbot.ParseModeHTML,
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		})
		if err != nil {
			log.Error("Failed to send daily digest to channel", zap.Int64("chat_id", target), zap.Error(err))
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// RunScheduler starts the background checkers.
func RunScheduler(ctx context.Context, app *app.App) {
	interval := getUpdateInterval()
	app.Log.Info("Starting OTT Scheduler background tasks", zap.Duration("interval", interval))

	// Run first check immediately on start
	go SendUpdates(ctx, app.Bot, app.DB, app.Log)

	// Create update ticker
	updateTicker := time.NewTicker(interval)

	// Create daily digest timer (every 24 hours, aligned to 20:00 UTC)
	now := time.Now().UTC()
	targetTime := time.Date(now.Year(), now.Month(), now.Day(), 20, 0, 0, 0, time.UTC)
	if now.After(targetTime) {
		targetTime = targetTime.AddDate(0, 0, 1)
	}
	initialDelay := targetTime.Sub(now)
	app.Log.Info("Daily OTT digest scheduled", zap.Time("next_run", targetTime), zap.Duration("delay", initialDelay))

	go func() {
		defer updateTicker.Stop()

		// Setup daily digest timer
		digestTimer := time.NewTimer(initialDelay)
		defer digestTimer.Stop()

		var digestTicker *time.Ticker

		for {
			select {
			case <-ctx.Done():
				if digestTicker != nil {
					digestTicker.Stop()
				}
				return
			case <-updateTicker.C:
				SendUpdates(ctx, app.Bot, app.DB, app.Log)
			case <-digestTimer.C:
				SendDailyDigest(ctx, app.Bot, app.DB, app.Log)
				// Switch to 24 hour ticker
				digestTicker = time.NewTicker(24 * time.Hour)
			}

			if digestTicker != nil {
				select {
				case <-ctx.Done():
					digestTicker.Stop()
					return
				case <-updateTicker.C:
					SendUpdates(ctx, app.Bot, app.DB, app.Log)
				case <-digestTicker.C:
					SendDailyDigest(ctx, app.Bot, app.DB, app.Log)
				}
			}
		}
	}()
}
