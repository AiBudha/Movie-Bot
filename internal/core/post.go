package core

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"net/url"
	"strings"

	"autofilterbot/internal/autofilter"
	"autofilterbot/internal/model"
	"autofilterbot/pkg/conversation"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
)

// PostCommand handles the /post command for generating movie/series ads.
func PostCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	if !_app.AuthAdmin(ctx) {
		return nil
	}

	var query string
	var replyToMsg *gotgbot.Message

	if ctx.CallbackQuery != nil {
		conv := conversation.NewConversatorFromUpdate(bot, ctx.Update)
		askM, err := conv.Ask(_app.Ctx, "<b>𝖯𝗅𝖾𝖺𝗌𝖾 𝗌𝖾𝗇𝖽 𝗍𝗁𝖾 𝗆𝗈𝗏𝗂𝖾 𝗈𝗋 𝗌𝖾𝗋𝗂𝖾𝗌 𝗇𝖺𝗆𝖾:</b>", &gotgbot.SendMessageOpts{
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{Text: "❌ Cancel", CallbackData: "admin:cancel"}}},
			},
			ParseMode: gotgbot.ParseModeHTML,
		})
		if err != nil {
			return nil
		}
		query = strings.TrimSpace(askM.Text)
		replyToMsg = askM
	} else {
		m := ctx.EffectiveMessage
		args := strings.SplitN(m.Text, " ", 2)
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			_, err := m.Reply(bot, "<b>Please provide a movie or series name.</b>\nExample: <code>/post Spider-Noir</code>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
			return err
		}
		query = strings.TrimSpace(args[1])
		replyToMsg = m
	}

	if strings.HasPrefix(strings.ToLower(query), "movie ") {
		query = strings.TrimSpace(query[6:])
	} else if strings.HasPrefix(strings.ToLower(query), "series ") {
		query = strings.TrimSpace(query[7:])
	}

	progressMsg, err := replyToMsg.Reply(bot, "🔍 <i>Searching database for files...</i>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	if err != nil {
		_app.Log.Warn("post: failed to send progress msg", zap.Error(err))
		return nil
	}

	// 1. Fetch files from database
	cursor, err := _app.DB.SearchFiles(query)
	if err != nil {
		progressMsg.EditText(bot, "❌ Database search failed.", nil)
		return nil
	}

	// Read all matching files
	filesFromDb, err := autofilter.FilesFromCursor(context.Background(), cursor, _app.Config)
	if err != nil {
		progressMsg.EditText(bot, "❌ Failed to read files from database.", nil)
		return nil
	}

	// Flatten files
	var allFiles []autofilter.File
	for _, page := range filesFromDb {
		allFiles = append(allFiles, page...)
	}

	if len(allFiles) == 0 {
		progressMsg.EditText(bot, fmt.Sprintf("❌ No files found for <b>%s</b> in the database.", html.EscapeString(query)), &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}

	// 2. Extract Metadata
	searchType := autofilter.DetectType(allFiles)
	isSeries := searchType == "series"

	langs := autofilter.DetectLanguages(allFiles)
	langStr := "Unknown"
	if len(langs) > 0 {
		langStr = strings.Join(langs, ", ")
	}

	// Find best quality
	bestQualityLevel := 0
	bestQualityStr := "Unknown"
	var totalSize int64
	for _, f := range allFiles {
		totalSize += f.FileSize
		qLvl := autofilter.QualityLevel(f.FileName)
		if qLvl > bestQualityLevel {
			bestQualityLevel = qLvl
			fileNameLower := strings.ToLower(f.FileName)
			if strings.Contains(fileNameLower, "2160p") || strings.Contains(fileNameLower, "4k") {
				bestQualityStr = "4K 2160p"
			} else if strings.Contains(fileNameLower, "1080p") {
				bestQualityStr = "1080p"
			} else if strings.Contains(fileNameLower, "720p") {
				bestQualityStr = "720p"
			} else if strings.Contains(fileNameLower, "480p") {
				bestQualityStr = "480p"
			} else if bestQualityStr == "Unknown" {
				bestQualityStr = "HD"
			}
		}
	}

	// 3. Fetch Poster
	progressMsg.EditText(bot, "🖼 <i>Fetching poster from TMDB...</i>", &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML})
	posterUrl := autofilter.GetPosterUrlWithType(query, isSeries)

	// 4. Build Caption and Buttons
	baseTitle := autofilter.ExtractBaseTitle(query)
	if baseTitle == "" {
		baseTitle = query
	}

	caption := fmt.Sprintf("<b>%s</b>\n\n", html.EscapeString(baseTitle))
	caption += fmt.Sprintf("🎧 <b>Language:</b> %s\n", html.EscapeString(langStr))
	caption += fmt.Sprintf("🎥 <b>Quality:</b> %s\n", bestQualityStr)
	caption += fmt.Sprintf("📁 <b>Files:</b> %d\n", len(allFiles))

	var keyboard [][]gotgbot.InlineKeyboardButton

	botUsername := bot.User.Username

	encodeQuery := func(q string) string {
		return base64.RawURLEncoding.EncodeToString([]byte("s" + q))
	}

	if isSeries {
		// Group by season
		groups := autofilter.GroupBySeason(allFiles)

		// Sort seasons
		var seasons []int
		for s := range groups {
			seasons = append(seasons, s)
		}
		// Sort seasons ascending
		for i := 0; i < len(seasons); i++ {
			for j := i + 1; j < len(seasons); j++ {
				if seasons[i] > seasons[j] {
					seasons[i], seasons[j] = seasons[j], seasons[i]
				}
			}
		}

		for _, s := range seasons {
			files := groups[s]
			// Count unique episodes
			eps := make(map[int]bool)
			for _, f := range files {
				_, e := autofilter.ExtractSeriesMetadata(f.FileName)
				if e > 0 {
					eps[e] = true
				}
			}

			btnText := fmt.Sprintf("📀 Season %d (%d eps \u00B7 %d files)", s, len(eps), len(files))
			searchStr := fmt.Sprintf("%s S%02d", baseTitle, s)

			encodedQuery := encodeQuery(searchStr)
			url := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, encodedQuery)

			keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{{Text: btnText, Url: url}})
		}
	} else {
		// Movie buttons
		var langRow []gotgbot.InlineKeyboardButton
		for _, l := range langs {
			encodedQuery := encodeQuery(baseTitle + " " + l)
			url := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, encodedQuery)
			langRow = append(langRow, gotgbot.InlineKeyboardButton{Text: l, Url: url})
			if len(langRow) == 2 {
				keyboard = append(keyboard, langRow)
				langRow = nil
			}
		}
		if len(langRow) > 0 {
			keyboard = append(keyboard, langRow)
		}

		// Also add a generic search button if no languages detected
		if len(langs) == 0 {
			encodedQuery := encodeQuery(baseTitle)
			url := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, encodedQuery)
			keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{{Text: "🔍 Search Movie", Url: url}})
		}
	}

	// Add Share to Friends button
	shareText := fmt.Sprintf("Check out %s on Telegram!", baseTitle)
	shareUrl := fmt.Sprintf("https://t.me/share/url?url=https://t.me/%s&text=%s", botUsername, url.QueryEscape(shareText))
	keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{{Text: "🎁 Share to Friends 🚀", Url: shareUrl}})

	// Add Preview controls
	encodedOriginalQuery := encodeQuery(query)
	previewKeyboard := append([][]gotgbot.InlineKeyboardButton(nil), keyboard...)
	previewKeyboard = append(previewKeyboard, []gotgbot.InlineKeyboardButton{
		{Text: "✅ Send to Channel", CallbackData: "post_send:" + encodedOriginalQuery},
		{Text: "❌ Cancel", CallbackData: "post_cancel"},
	})

	progressMsg.EditText(bot, "👀 <i>Generating preview...</i>", &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML})

	var errSend error
	if posterUrl != "" {
		_, errSend = bot.SendPhoto(replyToMsg.Chat.Id, gotgbot.InputFileByURL(posterUrl), &gotgbot.SendPhotoOpts{
			Caption:     caption,
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: previewKeyboard},
		})
	} else {
		_, errSend = bot.SendMessage(replyToMsg.Chat.Id, caption, &gotgbot.SendMessageOpts{
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: previewKeyboard},
		})
	}

	if errSend != nil {
		_app.Log.Warn("post: failed to send preview", zap.Error(errSend))
		progressMsg.EditText(bot, fmt.Sprintf("❌ Failed to generate preview: %s", errSend.Error()), nil)
		return nil
	}

	progressMsg.Delete(bot, nil)
	return nil
}

// PostCallbackHandler handles the Send to Channel and Cancel buttons.
func PostCallbackHandler(bot *gotgbot.Bot, ctx *ext.Context) error {
	c := ctx.CallbackQuery
	if !_app.AuthAdmin(ctx) {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "You are not an admin!", ShowAlert: true})
		return nil
	}

	data := c.Data
	if data == "post_cancel" {
		c.Message.Delete(bot, nil)
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Post cancelled."})
		return nil
	}

	if !strings.HasPrefix(data, "post_send:") {
		return nil
	}

	channelID := _app.Config.GetResultsChannelID()
	if channelID == 0 {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Results Channel is not configured. Set it in the bot settings.", ShowAlert: true})
		return nil
	}

	// Extract the query and decode it
	encodedQuery := strings.TrimPrefix(data, "post_send:")
	decodedBytes, err := base64.RawURLEncoding.DecodeString(encodedQuery)
	if err != nil {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Failed to decode query.", ShowAlert: true})
		return nil
	}
	
	// The encoded query had an 's' prefix we added, so strip it
	decodedStr := string(decodedBytes)
	if len(decodedStr) < 1 || decodedStr[0] != 's' {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid query data.", ShowAlert: true})
		return nil
	}
	query := decodedStr[1:]

	c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Sending to channel..."})

	// 1. Fetch files again to build the post
	cursor, err := _app.DB.SearchFiles(query)
	if err != nil {
		bot.SendMessage(c.Message.GetChat().Id, "❌ Database search failed during send.", nil)
		return nil
	}

	filesFromDb, err := autofilter.FilesFromCursor(context.Background(), cursor, _app.Config)
	if err != nil {
		bot.SendMessage(c.Message.GetChat().Id, "❌ Failed to read files from database during send.", nil)
		return nil
	}

	var allFiles []autofilter.File
	for _, page := range filesFromDb {
		allFiles = append(allFiles, page...)
	}

	if len(allFiles) == 0 {
		bot.SendMessage(c.Message.GetChat().Id, "❌ No files found during send.", nil)
		return nil
	}

	searchType := autofilter.DetectType(allFiles)
	isSeries := searchType == "series"
	langs := autofilter.DetectLanguages(allFiles)
	langStr := "Unknown"
	if len(langs) > 0 {
		langStr = strings.Join(langs, ", ")
	}

	bestQualityLevel := 0
	bestQualityStr := "Unknown"
	var totalSize int64
	for _, f := range allFiles {
		totalSize += f.FileSize
		qLvl := autofilter.QualityLevel(f.FileName)
		if qLvl > bestQualityLevel {
			bestQualityLevel = qLvl
			fileNameLower := strings.ToLower(f.FileName)
			if strings.Contains(fileNameLower, "2160p") || strings.Contains(fileNameLower, "4k") {
				bestQualityStr = "4K 2160p"
			} else if strings.Contains(fileNameLower, "1080p") {
				bestQualityStr = "1080p"
			} else if strings.Contains(fileNameLower, "720p") {
				bestQualityStr = "720p"
			} else if strings.Contains(fileNameLower, "480p") {
				bestQualityStr = "480p"
			} else if bestQualityStr == "Unknown" {
				bestQualityStr = "HD"
			}
		}
	}

	posterUrl := autofilter.GetPosterUrlWithType(query, isSeries)
	baseTitle := autofilter.ExtractBaseTitle(query)
	if baseTitle == "" {
		baseTitle = query
	}

	caption := fmt.Sprintf("<b>%s</b>\n\n", html.EscapeString(baseTitle))
	caption += fmt.Sprintf("🎧 <b>Language:</b> %s\n", html.EscapeString(langStr))
	caption += fmt.Sprintf("🎥 <b>Quality:</b> %s\n", bestQualityStr)
	caption += fmt.Sprintf("📁 <b>Files:</b> %d\n", len(allFiles))

	var keyboard [][]gotgbot.InlineKeyboardButton
	botUsername := bot.User.Username
	encodeQueryFunc := func(q string) string {
		return base64.RawURLEncoding.EncodeToString([]byte("s" + q))
	}

	if isSeries {
		groups := autofilter.GroupBySeason(allFiles)
		var seasons []int
		for s := range groups {
			seasons = append(seasons, s)
		}
		for i := 0; i < len(seasons); i++ {
			for j := i + 1; j < len(seasons); j++ {
				if seasons[i] > seasons[j] {
					seasons[i], seasons[j] = seasons[j], seasons[i]
				}
			}
		}
		for _, s := range seasons {
			files := groups[s]
			eps := make(map[int]bool)
			for _, f := range files {
				_, e := autofilter.ExtractSeriesMetadata(f.FileName)
				if e > 0 {
					eps[e] = true
				}
			}
			btnText := fmt.Sprintf("📀 Season %d (%d eps \u00B7 %d files)", s, len(eps), len(files))
			searchStr := fmt.Sprintf("%s S%02d", baseTitle, s)
			encodedQ := encodeQueryFunc(searchStr)
			urlStr := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, encodedQ)
			keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{{Text: btnText, Url: urlStr}})
		}
	} else {
		var langRow []gotgbot.InlineKeyboardButton
		for _, l := range langs {
			encodedQ := encodeQueryFunc(baseTitle + " " + l)
			urlStr := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, encodedQ)
			langRow = append(langRow, gotgbot.InlineKeyboardButton{Text: l, Url: urlStr})
			if len(langRow) == 2 {
				keyboard = append(keyboard, langRow)
				langRow = nil
			}
		}
		if len(langRow) > 0 {
			keyboard = append(keyboard, langRow)
		}
		if len(langs) == 0 {
			encodedQ := encodeQueryFunc(baseTitle)
			urlStr := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, encodedQ)
			keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{{Text: "🔍 Search Movie", Url: urlStr}})
		}
	}

	shareText := fmt.Sprintf("Check out %s on Telegram!", baseTitle)
	shareUrl := fmt.Sprintf("https://t.me/share/url?url=https://t.me/%s&text=%s", botUsername, url.QueryEscape(shareText))
	keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{{Text: "🎁 Share to Friends 🚀", Url: shareUrl}})

	var errSend error
	if posterUrl != "" {
		_, errSend = bot.SendPhoto(channelID, gotgbot.InputFileByURL(posterUrl), &gotgbot.SendPhotoOpts{
			Caption:     caption,
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard},
		})
	} else {
		_, errSend = bot.SendMessage(channelID, caption, &gotgbot.SendMessageOpts{
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard},
		})
	}

	if errSend != nil {
		_app.Log.Warn("post: failed to send to channel", zap.Error(errSend))
		bot.SendMessage(c.Message.GetChat().Id, fmt.Sprintf("❌ Failed to post to channel: %s", errSend.Error()), nil)
		return nil
	}

	// Change the original preview message to show it was posted
	c.Message.EditText(bot, "✅ <b>Successfully posted to channel!</b>", &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML})
	return nil
}

// AutoPostNewRelease checks if the newly indexed file is a new release (movie or series from 2025/2026),
// checks for duplicates, formats a premium channel post (with poster, specs, and inline search buttons),
// and automatically posts it to the results channel.
func AutoPostNewRelease(bot *gotgbot.Bot, f *model.File) {
	// 1. Ensure ResultsChannelID is set
	channelID := _app.Config.GetResultsChannelID()
	if channelID == 0 {
		return
	}

	// 2. Extract base title and year from the filename
	baseTitle := autofilter.ExtractBaseTitle(f.FileName)
	if baseTitle == "" {
		return
	}
	year := autofilter.ExtractYear(f.FileName)

	// Only auto-post new releases (e.g. year is 2025 or 2026, or current year 2026)
	if year == "" {
		_app.Log.Debug("AutoPostNewRelease: no year found, skipping auto-post", zap.String("file", f.FileName))
		return
	}
	// Verify it's a new release
	var parsedYear int
	_, _ = fmt.Sscanf(year, "%d", &parsedYear)
	if parsedYear < 2025 {
		_app.Log.Debug("AutoPostNewRelease: old release year, skipping auto-post", zap.String("file", f.FileName), zap.Int("year", parsedYear))
		return
	}

	// 3. Prevent duplicate posts: check if already posted
	posted, err := _app.DB.IsMoviePosted(baseTitle, year)
	if err != nil {
		_app.Log.Error("AutoPostNewRelease: database check for duplicates failed", zap.Error(err))
		return
	}
	if posted {
		_app.Log.Debug("AutoPostNewRelease: movie already posted, skipping duplicate", zap.String("title", baseTitle), zap.String("year", year))
		return
	}

	// 4. Fetch all matching files from DB to build a complete post (all qualities/languages)
	searchQuery := baseTitle
	if year != "" {
		searchQuery = baseTitle + " " + year
	}
	cursor, err := _app.DB.SearchFiles(searchQuery)
	if err != nil {
		_app.Log.Warn("AutoPostNewRelease: database search failed for post", zap.String("query", searchQuery), zap.Error(err))
		return
	}

	filesFromDb, err := autofilter.FilesFromCursor(context.Background(), cursor, _app.Config)
	if err != nil {
		_app.Log.Warn("AutoPostNewRelease: failed to read files from cursor", zap.Error(err))
		return
	}

	var allFiles []autofilter.File
	for _, page := range filesFromDb {
		allFiles = append(allFiles, page...)
	}

	if len(allFiles) == 0 {
		return
	}

	// Double check to make sure we don't race and write duplicate post records
	// Mark as posted first to avoid race conditions with multiple files of the same movie arriving concurrently.
	err = _app.DB.MarkMoviePosted(baseTitle, year)
	if err != nil {
		_app.Log.Error("AutoPostNewRelease: failed to mark movie as posted", zap.Error(err))
		return
	}

	searchType := autofilter.DetectType(allFiles)
	isSeries := searchType == "series"
	langs := autofilter.DetectLanguages(allFiles)
	langStr := "Unknown"
	if len(langs) > 0 {
		langStr = strings.Join(langs, ", ")
	}

	bestQualityLevel := 0
	bestQualityStr := "Unknown"
	for _, fileItem := range allFiles {
		qLvl := autofilter.QualityLevel(fileItem.FileName)
		if qLvl > bestQualityLevel {
			bestQualityLevel = qLvl
			fileNameLower := strings.ToLower(fileItem.FileName)
			if strings.Contains(fileNameLower, "2160p") || strings.Contains(fileNameLower, "4k") {
				bestQualityStr = "4K 2160p"
			} else if strings.Contains(fileNameLower, "1080p") {
				bestQualityStr = "1080p"
			} else if strings.Contains(fileNameLower, "720p") {
				bestQualityStr = "720p"
			} else if strings.Contains(fileNameLower, "480p") {
				bestQualityStr = "480p"
			} else if bestQualityStr == "Unknown" {
				bestQualityStr = "HD"
			}
		}
	}

	posterUrl := autofilter.GetPosterUrlWithType(searchQuery, isSeries)

	// Format Premium Caption
	caption := fmt.Sprintf("✅ <b>%s (%s)</b> #%s\n\n", html.EscapeString(baseTitle), year, strings.ToUpper(searchType))
	caption += fmt.Sprintf("🎙️ <b>Language:</b> %s\n", html.EscapeString(langStr))
	caption += fmt.Sprintf("🎥 <b>Quality:</b> %s\n", bestQualityStr)
	caption += fmt.Sprintf("📁 <b>Files Available:</b> %d\n", len(allFiles))

	var keyboard [][]gotgbot.InlineKeyboardButton
	botUsername := bot.User.Username
	encodeQueryFunc := func(q string) string {
		return base64.RawURLEncoding.EncodeToString([]byte("s" + q))
	}

	if isSeries {
		groups := autofilter.GroupBySeason(allFiles)
		var seasons []int
		for s := range groups {
			seasons = append(seasons, s)
		}
		for i := 0; i < len(seasons); i++ {
			for j := i + 1; j < len(seasons); j++ {
				if seasons[i] > seasons[j] {
					seasons[i], seasons[j] = seasons[j], seasons[i]
				}
			}
		}
		for _, s := range seasons {
			files := groups[s]
			eps := make(map[int]bool)
			for _, fileItem := range files {
				_, e := autofilter.ExtractSeriesMetadata(fileItem.FileName)
				if e > 0 {
					eps[e] = true
				}
			}
			btnText := fmt.Sprintf("📀 Season %d (%d eps \u00B7 %d files)", s, len(eps), len(files))
			searchStr := fmt.Sprintf("%s S%02d", baseTitle, s)
			encodedQ := encodeQueryFunc(searchStr)
			urlStr := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, encodedQ)
			keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{{Text: btnText, Url: urlStr}})
		}
	} else {
		var langRow []gotgbot.InlineKeyboardButton
		for _, l := range langs {
			encodedQ := encodeQueryFunc(baseTitle + " " + year + " " + l)
			urlStr := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, encodedQ)
			langRow = append(langRow, gotgbot.InlineKeyboardButton{Text: l, Url: urlStr})
			if len(langRow) == 2 {
				keyboard = append(keyboard, langRow)
				langRow = nil
			}
		}
		if len(langRow) > 0 {
			keyboard = append(keyboard, langRow)
		}
		if len(langs) == 0 {
			encodedQ := encodeQueryFunc(baseTitle + " " + year)
			urlStr := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, encodedQ)
			keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{{Text: "🔍 Search Movie", Url: urlStr}})
		}
	}

	shareText := fmt.Sprintf("Check out %s (%s) on Telegram!", baseTitle, year)
	shareUrl := fmt.Sprintf("https://t.me/share/url?url=https://t.me/%s&text=%s", botUsername, url.QueryEscape(shareText))
	keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{{Text: "🎁 Share to Friends 🚀", Url: shareUrl}})

	var errSend error
	if posterUrl != "" {
		_, errSend = bot.SendPhoto(channelID, gotgbot.InputFileByURL(posterUrl), &gotgbot.SendPhotoOpts{
			Caption:     caption,
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard},
		})
	} else {
		_, errSend = bot.SendMessage(channelID, caption, &gotgbot.SendMessageOpts{
			ParseMode:   gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard},
		})
	}

	if errSend != nil {
		_app.Log.Warn("AutoPostNewRelease: failed to send to channel", zap.Error(errSend))
	} else {
		_app.Log.Info("AutoPostNewRelease: successfully auto-posted new release to channel", zap.String("title", baseTitle), zap.String("year", year))
	}
}
