package core

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"autofilterbot/internal/autofilter"
	"autofilterbot/internal/ott"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
)

// PlatformsCommand shows the supported streaming platforms in the current region.
func PlatformsCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	country := os.Getenv("JUSTWATCH_COUNTRY")
	if country == "" {
		country = "IN"
	}
	country = strings.ToUpper(country)

	text := fmt.Sprintf("📡 <b>Supported OTT Platforms</b>\nRegion: <code>%s</code>\n\nNetflix, Amazon Prime, Disney+, Hotstar, Apple TV+, Max, Hulu, Peacock, Paramount+, SonyLIV, ZEE5, MX Player, Crunchyroll, MUBI, Starz", country)
	_, err := ctx.EffectiveMessage.Reply(bot, text, &gotgbot.SendMessageOpts{
		ParseMode: gotgbot.ParseModeHTML,
	})
	return err
}

// SubscribeCommand registers a user/chat for auto updates.
func SubscribeCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	chatID := ctx.EffectiveChat.Id
	username := ctx.EffectiveUser.Username

	inserted, err := _app.DB.AddOTTSubscriber(chatID, username)
	if err != nil {
		_app.Log.Error("SubscribeCommand database error", zap.Error(err))
		_, err = ctx.EffectiveMessage.Reply(bot, "❌ Could not complete subscription. Please try again later.", nil)
		return err
	}

	var text string
	if inserted {
		text = "✅ <b>Subscribed!</b>\n\nYou'll be notified whenever new content lands on OTT platforms.\n\nUse /unsubscribe to stop at any time."
	} else {
		text = "✅ You're <b>already subscribed!</b>\n\nYou'll receive updates automatically.\n\nUse /unsubscribe to stop."
	}

	_, err = ctx.EffectiveMessage.Reply(bot, text, &gotgbot.SendMessageOpts{
		ParseMode: gotgbot.ParseModeHTML,
	})
	return err
}

// UnsubscribeCommand unsubscribes a user/chat from updates.
func UnsubscribeCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	chatID := ctx.EffectiveChat.Id

	removed, err := _app.DB.RemoveOTTSubscriber(chatID)
	if err != nil {
		_app.Log.Error("UnsubscribeCommand database error", zap.Error(err))
		_, err = ctx.EffectiveMessage.Reply(bot, "❌ Could not complete unsubscription. Please try again later.", nil)
		return err
	}

	var text string
	if removed {
		text = "❌ <b>Unsubscribed!</b>\n\nYou will no longer receive OTT release updates."
	} else {
		text = "❌ You are not subscribed.\n\nUse /subscribe to enable updates."
	}

	_, err = ctx.EffectiveMessage.Reply(bot, text, &gotgbot.SendMessageOpts{
		ParseMode: gotgbot.ParseModeHTML,
	})
	return err
}

var yearRegex = regexp.MustCompile(`\b(19\d\d|20\d\d)\b`)

func extractYear(name string) string {
	matches := yearRegex.FindStringSubmatch(name)
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// LatestCommand fetches and displays recent releases with inline keyboard pagination.
func LatestCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	statusMsg, err := ctx.EffectiveMessage.Reply(bot, "🔍 Fetching latest uploaded movies...", nil)
	if err != nil {
		return err
	}

	// Fetch files from the database
	files, err := _app.DB.GetRecentFiles(150)
	if err != nil {
		_app.Log.Error("/latest command error", zap.Error(err))
		_, _, err = statusMsg.EditText(bot, "❌ Could not fetch latest uploads. Please try again later.", nil)
		return err
	}

	// Group and convert to ReleaseItems
	type movieGroup struct {
		Title string
		Year  string
	}
	var groups []movieGroup
	seen := make(map[string]bool)
	for _, f := range files {
		title := autofilter.ExtractBaseTitle(f.FileName)
		if title == "" {
			continue
		}
		cleanTitle := cleanCompareString(title)
		if cleanTitle == "" {
			continue
		}
		year := extractYear(f.FileName)
		key := cleanTitle
		if year != "" {
			key += "_" + year
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		groups = append(groups, movieGroup{Title: title, Year: year})
	}

	if len(groups) == 0 {
		text := "😔 <b>No recent uploads found in the database.</b>"
		_, _, err = statusMsg.EditText(bot, text, &gotgbot.EditMessageTextOpts{
			ParseMode:          gotgbot.ParseModeHTML,
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		})
		return err
	}

	var items []ott.ReleaseItem
	for _, g := range groups {
		items = append(items, ott.ReleaseItem{
			Title:       g.Title,
			ReleaseDate: g.Year,
			Type:        "movie",
			Source:      "database",
		})
	}

	sessionKey := ott.CreateLatestSession(items)
	text := buildLatestText(items, 0)
	kb := buildLatestKeyboard(sessionKey, 0, items)

	_, _, err = statusMsg.EditText(bot, text, &gotgbot.EditMessageTextOpts{
		ParseMode:          gotgbot.ParseModeHTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		ReplyMarkup:        kb,
	})
	return err
}

// SendNowCommand triggers the background release checker immediately (Admin only).
func SendNowCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	if !_app.AuthAdmin(ctx) {
		_, err := ctx.EffectiveMessage.Reply(bot, "🔒 You do not have permission to run this command.", nil)
		return err
	}

	_, err := ctx.EffectiveMessage.Reply(bot, "⚡ Triggering OTT release check in background ...", nil)
	if err != nil {
		return err
	}

	go ott.SendUpdates(context.Background(), bot, _app.DB, _app.Log)
	return nil
}

// LatestPageCallback handles the pagination buttons for /latest.
func LatestPageCallback(bot *gotgbot.Bot, ctx *ext.Context) error {
	cb := ctx.CallbackQuery
	parts := strings.Split(cb.Data, "|")
	if len(parts) < 3 {
		_, err := cb.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid callback data.", ShowAlert: true})
		return err
	}

	sessionKey := parts[1]
	page, err := strconv.Atoi(parts[2])
	if err != nil {
		_, err = cb.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid page index.", ShowAlert: true})
		return err
	}

	items, ok := ott.GetLatestSession(sessionKey)
	if !ok {
		_, err = cb.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Session expired. Please send /latest again.", ShowAlert: true})
		return err
	}

	text := buildLatestText(items, page)
	kb := buildLatestKeyboard(sessionKey, page, items)

	_, _, err = cb.Message.EditText(bot, text, &gotgbot.EditMessageTextOpts{
		ParseMode:          gotgbot.ParseModeHTML,
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		ReplyMarkup:        kb,
	})
	if err != nil {
		_app.Log.Error("LatestPageCallback edit failed", zap.Error(err))
	}

	_, _ = cb.Answer(bot, nil)
	return nil
}

// LatestItemCallback handles detail view click of an item.
func LatestItemCallback(bot *gotgbot.Bot, ctx *ext.Context) error {
	cb := ctx.CallbackQuery
	parts := strings.Split(cb.Data, "|")
	if len(parts) < 3 {
		_, err := cb.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid callback data.", ShowAlert: true})
		return err
	}

	sessionKey := parts[1]
	idx, err := strconv.Atoi(parts[2])
	if err != nil {
		_, err = cb.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid item index.", ShowAlert: true})
		return err
	}

	items, ok := ott.GetLatestSession(sessionKey)
	if !ok {
		_, err = cb.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Session expired. Please send /latest again.", ShowAlert: true})
		return err
	}

	if idx < 0 || idx >= len(items) {
		_, err = cb.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid item index.", ShowAlert: true})
		return err
	}

	item := items[idx]
	text := ott.FormatItemMessage(item)
	kb := ott.FormatItemKeyboard(item)

	// Append Close button restricted to the user who clicked it
	kb.InlineKeyboard = append(kb.InlineKeyboard, []gotgbot.InlineKeyboardButton{
		{Text: "❌ Close", CallbackData: fmt.Sprintf("close|%d", cb.From.Id)},
	})

	_, _ = cb.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Loading details..."})

	if item.Poster != "" {
		_, err = bot.SendPhoto(cb.Message.GetChat().Id, gotgbot.InputFileByURL(item.Poster), &gotgbot.SendPhotoOpts{
			Caption:     text,
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: kb,
		})
		if err != nil {
			// Fallback to text message
			_, err = bot.SendMessage(cb.Message.GetChat().Id, text, &gotgbot.SendMessageOpts{
				ParseMode:          gotgbot.ParseModeHTML,
				LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: false},
				ReplyMarkup:        kb,
			})
		}
	} else {
		_, err = bot.SendMessage(cb.Message.GetChat().Id, text, &gotgbot.SendMessageOpts{
			ParseMode:          gotgbot.ParseModeHTML,
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: false},
			ReplyMarkup:        kb,
		})
	}
	return err
}

// LatestNoopCallback answers the page counter button clicks.
func LatestNoopCallback(bot *gotgbot.Bot, ctx *ext.Context) error {
	_, err := ctx.CallbackQuery.Answer(bot, nil)
	return err
}

const latestPageSize = 10

func buildLatestKeyboard(sessionKey string, page int, items []ott.ReleaseItem) gotgbot.InlineKeyboardMarkup {
	total := len(items)
	totalPages := (total + latestPageSize - 1) / latestPageSize
	if totalPages == 0 {
		totalPages = 1
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * latestPageSize
	end := start + latestPageSize
	if end > total {
		end = total
	}

	var rows [][]gotgbot.InlineKeyboardButton
	for idx := start; idx < end; idx++ {
		item := items[idx]
		kind := "🎬"
		if item.Type != "movie" {
			kind = "📺"
		}

		title := item.Title
		if len(title) > 42 {
			title = title[:39] + "..."
		}

		if item.Source == "database" {
			// Construct deep link URL button
			queryStr := item.Title
			if item.ReleaseDate != "" {
				queryStr += " " + item.ReleaseDate
			}
			encodedQuery := base64.RawURLEncoding.EncodeToString([]byte("s" + queryStr))
			deepLink := fmt.Sprintf("https://t.me/%s?start=%s", _app.Bot.Username, encodedQuery)

			rows = append(rows, []gotgbot.InlineKeyboardButton{
				{
					Text: fmt.Sprintf("%s %s %s", kind, title, item.ReleaseDate),
					Url:  deepLink,
				},
			})
		} else {
			rows = append(rows, []gotgbot.InlineKeyboardButton{
				{
					Text:         fmt.Sprintf("%s %s", kind, title),
					CallbackData: fmt.Sprintf("latest_it|%s|%d", sessionKey, idx),
				},
			})
		}
	}

	var nav []gotgbot.InlineKeyboardButton
	if page > 0 {
		nav = append(nav, gotgbot.InlineKeyboardButton{Text: "⬅️ Prev", CallbackData: fmt.Sprintf("latest_pg|%s|%d", sessionKey, page-1)})
	}
	nav = append(nav, gotgbot.InlineKeyboardButton{Text: fmt.Sprintf("%d/%d", page+1, totalPages), CallbackData: "latest_noop"})
	if page < totalPages-1 {
		nav = append(nav, gotgbot.InlineKeyboardButton{Text: "Next ➡️", CallbackData: fmt.Sprintf("latest_pg|%s|%d", sessionKey, page+1)})
	}
	rows = append(rows, nav)
	rows = append(rows, []gotgbot.InlineKeyboardButton{
		{Text: "❌ Close", CallbackData: "close"},
	})

	return gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildLatestText(items []ott.ReleaseItem, page int) string {
	total := len(items)
	start := page * latestPageSize
	end := start + latestPageSize
	if end > total {
		end = total
	}
	return fmt.Sprintf("🍿 <b>Latest Uploaded Movies & Series</b>\nShowing <b>%d-%d</b> of <b>%d</b>.\n\nTap a title to get the download links:", start+1, end, total)
}
