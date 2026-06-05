package core

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"autofilterbot/internal/autofilter"
	"autofilterbot/internal/button"
	"autofilterbot/internal/format"
	"autofilterbot/internal/fsub"
	"autofilterbot/internal/functions"
	"autofilterbot/internal/middleware"
	"autofilterbot/internal/model"
	"autofilterbot/pkg/callbackdata"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
)

func cleanCompareString(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, ".", " ")
	return strings.Join(strings.Fields(s), " ")
}

func Autofilter(bot *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat != nil && ctx.EffectiveChat.Type != "private" {
		_ = _app.DB.IncrementGroupSearchCount(ctx.EffectiveChat.Id)
	}

	// 2. Wrap search in a job and push to queue
	job := Job{
		Bot: bot,
		Ctx: ctx,
		Handler: func(bot *gotgbot.Bot, ctx *ext.Context) error {
			msg, err := _autofilter(bot, ctx)
			if err != nil {
				return err
			}
			if msg != nil && _app.Config.GetAutodeleteTime() != 0 {
				_app.AutoDelete.SaveMessage(msg, time.Minute*time.Duration(_app.Config.GetAutodeleteTime()))
			}
			if ctx.CallbackQuery != nil {
				c := ctx.CallbackQuery
				callbackData := callbackdata.FromString(c.Data)
				if callbackData.Path[0] == "suggest" || callbackData.Path[0] == "reset" {
					if m, ok := c.Message.(*gotgbot.Message); ok {
						_, _ = m.Delete(bot, nil)
					}
				}
			}
			return nil
		},
	}

	if pushed := _app.WorkerPool.(*WorkerPool).Push(job); !pushed {
		// Queue full, maybe notify user or just ignore
		_app.Log.Warn("autofilter: job queue full, skipping search")
	}

	// Register user message for auto-deletion
	if ctx.Message != nil {
		delTime := _app.Config.GetAutodeleteTime()
		if delTime != 0 {
			err := _app.AutoDelete.SaveMessage(ctx.Message, time.Minute*time.Duration(delTime))
			if err != nil {
				_app.Log.Warn("autofilter: save user query autodelete failed", zap.Error(err))
			}
		}
	}

	return nil
}

// autofilter runs the autofilter task and returns the sent message.
func _autofilter(bot *gotgbot.Bot, ctx *ext.Context) (*gotgbot.Message, error) {
	_app.Log.Info("_autofilter function called")
	var (
		query        string
		inputMessage gotgbot.MaybeInaccessibleMessage
		fromUser     *gotgbot.User
	)

	switch {
	case ctx.CallbackQuery != nil:
		c := ctx.CallbackQuery

		callbackData := callbackdata.FromString(c.Data)
		if callbackData.Path[0] == "suggest" || callbackData.Path[0] == "reset" || callbackData.Path[0] == "trend" {
			if len(callbackData.Args) >= 2 {
				lastIdx := len(callbackData.Args) - 1
				userId, err := strconv.ParseInt(callbackData.Args[lastIdx], 10, 64)
				if err == nil {
					if callbackData.Path[0] != "trend" && c.From.Id != userId {
						_, err := c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
							Text:      "You Can't Use This Button!",
							ShowAlert: true,
						})
						return nil, err
					}
					query = strings.Join(callbackData.Args[:lastIdx], "_")
				} else {
					query = strings.Join(callbackData.Args, "_")
				}
			} else if len(callbackData.Args) == 1 {
				query = callbackData.Args[0]
			}
			inputMessage = c.Message
			fromUser = &c.From
		} else {
			if len(callbackData.Args) < 2 {
				_, err := c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
					Text:      "Malformed Query: Not Enough Arguments",
					ShowAlert: true,
				})
				_app.Log.Warn("autofilter: bad callback data", zap.Strings("args", callbackData.Args))
				return nil, err
			}

			userId, err := strconv.ParseInt(callbackData.Args[1], 10, 64)
			if err != nil {
				_, err := c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
					Text:      "Sorry An Error occurred :{",
					ShowAlert: true,
				})
				_app.Log.Warn("autofilter: parse user id failed", zap.Error(err))
				return nil, err
			}

			if c.From.Id != userId {
				_, err := c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
					Text:      "You Can't Use This Button!",
					ShowAlert: true,
				})
				return nil, err
			}

			inputMessage = c.Message
			if m, ok := c.Message.(*gotgbot.Message); ok {
				inputMessage = m.ReplyToMessage
			}

			query = callbackData.Args[0]
			fromUser = &c.From
		}
	case ctx.Message != nil:
		m := ctx.Message

		text := m.Text
		if text == "" {
			return nil, nil
		}

		fromStartSearch := false
		if strings.HasPrefix(text, "/start ") {
			args := strings.Split(text, " ")
			if len(args) > 1 {
				// Try RawURLEncoding first (new standard)
				decodedBytes, err := base64.RawURLEncoding.DecodeString(args[1])
				// Fallback to StdEncoding for any older generated links
				if err != nil {
					decodedBytes, err = base64.StdEncoding.DecodeString(args[1])
				}
				
				if err == nil {
					decoded := string(decodedBytes)
					if len(decoded) > 1 && decoded[0] == 's' {
						text = decoded[1:]
						fromStartSearch = true
					}
				}
			}
		}

		if !fromStartSearch && autofilter.IsBadQuery(text, m.Entities) {
			_app.Log.Warn("autofilter: bad query", zap.String("text", text), zap.Any("entities", m.Entities))
			return nil, nil
		}

		text = autofilter.Sanitize(text)

		inputMessage = m
		query = text
		fromUser = m.From
		_app.Log.Info("_autofilter message query parsed", zap.String("query", query), zap.Bool("fromStartSearch", fromStartSearch))
	default:
		_app.Log.Warn("autofilter: unsupported update type", zap.Int64("update_id", ctx.UpdateId))
		return nil, nil
	}

	// 3. Track search language stats & user analytics
	go func() {
		if fromUser != nil {
			lang := functions.DetectLanguage(query)
			if lang == "Unknown" && fromUser.LanguageCode != "" {
				lang = functions.MapLanguageCode(fromUser.LanguageCode)
			}
			if lang != "" {
				_app.DB.IncrementGlobalLangStat(_app.Config.BotId, lang)
				_app.DB.IncrementUserLangStat(fromUser.Id, lang)
			}

			if query != "" {
				_app.DB.TrackSearch(query)
			}

			isPrivate := false
			if ctx.Message != nil && ctx.Message.Chat.Type == "private" {
				isPrivate = true
			} else if ctx.CallbackQuery != nil && ctx.CallbackQuery.Message != nil && ctx.CallbackQuery.Message.GetChat().Type == "private" {
				isPrivate = true
			}

			source := "group_search"
			if isPrivate {
				source = "pm_search"
			}

			err := _app.DB.SaveUserExtended(fromUser.Id, source, 0, fromUser.LanguageCode)
			if err == nil {
				country := functions.DetectCountry(fromUser.LanguageCode)
				_app.DB.UpdateUserCountry(fromUser.Id, country)
			}
			_app.DB.UpdateUserLastSeen(fromUser.Id)
		}
	}()

	cursor, err := _app.DB.SearchFiles(query)
	if err != nil {
		_app.Log.Warn("autofilter: search files failed", zap.Error(err))
		return bot.SendMessage(inputMessage.GetChat().Id, "<i>I'm Having Some Database Issues Right Now 😓\nPlease Try Again Later!</i>", &gotgbot.SendMessageOpts{
			ReplyParameters: &gotgbot.ReplyParameters{
				MessageId: inputMessage.GetMessageId(),
			},
			ParseMode: gotgbot.ParseModeHTML,
		})
	}

	files, err := autofilter.FilesFromCursor(context.Background(), cursor, _app.Config)
	if err != nil {
		_app.Log.Warn("autofilter: files from cursor failed", zap.Error(err))
		return bot.SendMessage(inputMessage.GetChat().Id, "<i>Processing Results Failed 🤖</i>", &gotgbot.SendMessageOpts{
			ReplyParameters: &gotgbot.ReplyParameters{
				MessageId: inputMessage.GetMessageId(),
			},
			ParseMode: gotgbot.ParseModeHTML,
		})
	}

	files = processSearchResults(files)
	_app.Log.Info("_autofilter search result retrieved", zap.String("query", query), zap.Int("files_count", len(files)))

	if len(files) == 0 {
		// 1. Try a local database fuzzy spelling search first (so suggestions are actual files in the DB)
		localSugs, err := _app.DB.GetSpellingSuggestions(query)
		var suggestions []string
		if err == nil && len(localSugs) > 0 {
			suggestions = localSugs
		}

		// 2. Fallback: Try external TMDB/OMDB suggestions if local suggestions are empty
		if len(suggestions) == 0 {
			suggestions = autofilter.GetSearchSuggestions(query)
		}

		if len(suggestions) > 0 {
			text := fmt.Sprintf("I couldn't find anything for <b>%s</b>. Did you mean any of these below:", query)
			if _app.Config.GetAutodeleteTime() != 0 {
				text += fmt.Sprintf("\n\n<blockquote>○ 𝖠𝗎𝗍𝗈-𝖣𝖾𝗅𝖾𝗍𝖾: <b>%d 𝗆𝗂𝗇𝗌</b></blockquote>", _app.Config.GetAutodeleteTime())
			}

			buttons := [][]gotgbot.InlineKeyboardButton{}
			for _, sug := range suggestions {
				// Clean suggestion for actual query
				cleanSug := strings.ReplaceAll(sug, "(", " ")
				cleanSug = strings.ReplaceAll(cleanSug, ")", " ")
				cleanSug = strings.ReplaceAll(cleanSug, "[", " ")
				cleanSug = strings.ReplaceAll(cleanSug, "]", " ")
				cleanSug = strings.Join(strings.Fields(cleanSug), " ")

				// Truncate to fit callback query 64-byte limit:
				// format: "suggest|" + cleanSug + "_" + userId
				limit := 64 - len("suggest|") - len(fmt.Sprintf("_%d", fromUser.Id))
				if len(cleanSug) > limit {
					cleanSug = cleanSug[:limit]
				}

				buttons = append(buttons, []gotgbot.InlineKeyboardButton{{
					Text:         sug,
					CallbackData: fmt.Sprintf("suggest|%s_%d", cleanSug, fromUser.Id),
				}})
			}
			// Close button at the bottom
			buttons = append(buttons, []gotgbot.InlineKeyboardButton{button.Close(fromUser.Id)})

			return bot.SendMessage(inputMessage.GetChat().Id, text, &gotgbot.SendMessageOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId: inputMessage.GetMessageId(),
				},
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons},
				ParseMode:   gotgbot.ParseModeHTML,
			})
		}

		// Absolute fallback: No suggestions at all
		vals := _app.BasicMessageValues(ctx, map[string]any{"query": query})
		text := format.KeyValueFormat(_app.Config.GetNoResultText(), vals)
		if _app.Config.GetAutodeleteTime() != 0 {
			text += fmt.Sprintf("\n\n<blockquote>○ 𝖠𝗎𝗍𝗈-𝖣𝖾𝗅𝖾𝗍𝖾: <b>%d 𝗆𝗂𝗇𝗌</b></blockquote>", _app.Config.GetAutodeleteTime())
		}

		buttons := [][]gotgbot.InlineKeyboardButton{
			{{Text: "Sᴇᴀʀᴄʜ Oɴ Gᴏᴏɢʟᴇ 🔎", Url: fmt.Sprintf("https://google.com/?q=%s", query), Style: "primary"}},
			{{Text: "Cᴏᴘʏ", CopyText: &gotgbot.CopyTextButton{Text: query}, Style: "primary"}, button.Close(fromUser.Id)},
		}

		return bot.SendMessage(inputMessage.GetChat().Id, text, &gotgbot.SendMessageOpts{
			ReplyParameters: &gotgbot.ReplyParameters{
				MessageId: inputMessage.GetMessageId(),
			},
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons},
			ParseMode:   gotgbot.ParseModeHTML,
		})
	}

	var warn string
	if delTime := _app.Config.GetAutodeleteTime(); delTime != 0 {
		warn = fmt.Sprintf("<blockquote>○ 𝖠𝗎𝗍𝗈-𝖣𝖾𝗅𝖾𝗍𝖾: <b>%d 𝗆𝗂𝗇𝗌</b></blockquote>", delTime)
	}

	var (
		buttons   = make([][]gotgbot.InlineKeyboardButton, 0, 10)
		uniqueId  = functions.RandString(15)
		isPrivate = inputMessage.GetChat().Type == "private"

		// Combine all results for detection
		allFiles []autofilter.File
	)
	for _, page := range files {
		allFiles = append(allFiles, page...)
	}

	searchType := autofilter.DetectType(allFiles)
	isSeries := searchType == "series"

	if !isSeries {
		type movieGroup struct {
			Title string
			Year  string
		}
		var movieGroups []movieGroup
		seenMovie := make(map[string]bool)

		for _, f := range allFiles {
			title := autofilter.ExtractBaseTitle(f.FileName)
			if title == "" {
				continue
			}

			// Extract year
			year := ""
			yearRegex := regexp.MustCompile(`\b(19|20)\d{2}\b`)
			if match := yearRegex.FindString(f.FileName); match != "" {
				year = match
			}

			key := strings.ToLower(title)
			if year != "" {
				key += "_" + year
			}

			if !seenMovie[key] {
				seenMovie[key] = true
				movieGroups = append(movieGroups, movieGroup{Title: title, Year: year})
			}
		}

		if len(movieGroups) > 1 {
			// Check if the query matches one of the groups exactly to avoid selection loops
			matchedGroupIdx := -1
			normalizedQuery := cleanCompareString(query)
			for idx, mg := range movieGroups {
				specificQuery := mg.Title
				if mg.Year != "" {
					specificQuery = mg.Title + " " + mg.Year
				}
				if cleanCompareString(specificQuery) == normalizedQuery {
					matchedGroupIdx = idx
					break
				}
			}

			if matchedGroupIdx != -1 {
				// Filter files to only contain files from the matched movie group
				targetMg := movieGroups[matchedGroupIdx]
				var filteredFiles []autofilter.Files
				for _, page := range files {
					var filteredPage autofilter.Files
					for _, f := range page {
						title := autofilter.ExtractBaseTitle(f.FileName)
						year := ""
						yearRegex := regexp.MustCompile(`\b(19|20)\d{2}\b`)
						if match := yearRegex.FindString(f.FileName); match != "" {
							year = match
						}

						specificQuery := title
						if year != "" {
							specificQuery = title + " " + year
						}

						if cleanCompareString(specificQuery) == normalizedQuery {
							filteredPage = append(filteredPage, f)
						}
					}
					if len(filteredPage) > 0 {
						filteredFiles = append(filteredFiles, filteredPage)
					}
				}

				if len(filteredFiles) > 0 {
					files = filteredFiles
					// Re-evaluate allFiles and movieGroups since we filtered
					allFiles = nil
					for _, page := range files {
						allFiles = append(allFiles, page...)
					}
					movieGroups = []movieGroup{targetMg}
				}
			}
		}

		if len(movieGroups) > 1 {
			// Show movie choices
			text := fmt.Sprintf("🍿 <b>Multiple movies found matching:</b> <code>%s</code>\n\n<i>Please select the correct movie below to get the files and poster:</i>", query)
			if _app.Config.GetAutodeleteTime() != 0 {
				text += fmt.Sprintf("\n\n<blockquote>○ 𝖠𝗎𝗍𝗈-𝖣𝖾𝗅𝖾𝗍𝖾: <b>%d 𝗆𝗂𝗇𝗌</b></blockquote>", _app.Config.GetAutodeleteTime())
			}

			var userId int64
			if fromUser != nil {
				userId = fromUser.Id
			}

			choiceButtons := [][]gotgbot.InlineKeyboardButton{}
			limit := 10
			if len(movieGroups) < limit {
				limit = len(movieGroups)
			}
			for i := 0; i < limit; i++ {
				mg := movieGroups[i]
				label := mg.Title
				if mg.Year != "" {
					label = fmt.Sprintf("🎬 %s (%s)", mg.Title, mg.Year)
				} else {
					label = fmt.Sprintf("🎬 %s", mg.Title)
				}

				specificQuery := mg.Title
				if mg.Year != "" {
					specificQuery = mg.Title + " " + mg.Year
				}
				cleanSug := strings.ReplaceAll(specificQuery, "(", " ")
				cleanSug = strings.ReplaceAll(cleanSug, ")", " ")
				cleanSug = strings.ReplaceAll(cleanSug, "[", " ")
				cleanSug = strings.ReplaceAll(cleanSug, "]", " ")
				cleanSug = strings.Join(strings.Fields(cleanSug), " ")

				callLimit := 64 - len("suggest|") - len(fmt.Sprintf("_%d", userId))
				if len(cleanSug) > callLimit {
					cleanSug = cleanSug[:callLimit]
				}

				choiceButtons = append(choiceButtons, []gotgbot.InlineKeyboardButton{{
					Text:         label,
					CallbackData: fmt.Sprintf("suggest|%s_%d", cleanSug, userId),
				}})
			}
			choiceButtons = append(choiceButtons, []gotgbot.InlineKeyboardButton{button.Close(userId)})

			var replyMarkup gotgbot.InlineKeyboardMarkup
			replyMarkup.InlineKeyboard = choiceButtons

			sentMsg, err := bot.SendMessage(inputMessage.GetChat().Id, text, &gotgbot.SendMessageOpts{
				ReplyParameters: &gotgbot.ReplyParameters{
					MessageId: inputMessage.GetMessageId(),
				},
				ReplyMarkup: replyMarkup,
				ParseMode:   gotgbot.ParseModeHTML,
			})
			return sentMsg, err
		}
	}

	if isSeries {
		// Season row
		seasonButtons := autofilter.Files(allFiles).ProcessSeasons(uniqueId)
		// Set first season button to success style (Green) as per image
		if len(seasonButtons) > 0 && len(seasonButtons[0]) > 0 {
			seasonButtons[0][0].Style = "success"
		}
		buttons = append(buttons, seasonButtons...)

		// Language row
		languages := autofilter.DetectLanguages(allFiles)
		if len(languages) > 0 {
			var langRow []gotgbot.InlineKeyboardButton
			for _, l := range languages {
				langRow = append(langRow, gotgbot.InlineKeyboardButton{
					Text:         l,
					CallbackData: fmt.Sprintf("lang|%s_%s", uniqueId, l),
					Style:        "primary",
				})
				if len(langRow) == 2 {
					buttons = append(buttons, langRow)
					langRow = nil
				}
			}
			if len(langRow) > 0 {
				buttons = append(buttons, langRow)
			}
		}
	}

	fileButtons := files[0].Process(inputMessage.GetChat().Id, bot.Username, _app.Config)

	// Divider
	latestReleasesBtn := gotgbot.InlineKeyboardButton{Text: "🌟 Latest Releases 🌟", CallbackData: "ignore", Style: "success"}
	if _app.Config.LatestReleasesUrl != "" {
		latestReleasesBtn.Url = _app.Config.LatestReleasesUrl
		latestReleasesBtn.CallbackData = ""
	}
	buttons = append(buttons, []gotgbot.InlineKeyboardButton{latestReleasesBtn})

	// Add file buttons
	buttons = append(buttons, fileButtons...)

	// Multi-select
	if isPrivate {
		buttons = append(buttons, []gotgbot.InlineKeyboardButton{{Text: "✅ Select Multiple Files", CallbackData: fmt.Sprintf("sel|%s_0", uniqueId), Style: "primary"}})
	}

	// Footer Action Row 1
	newMoviesBtn := gotgbot.InlineKeyboardButton{Text: "🍿 New Movies", Style: "success", CallbackData: "btn_new"}
	if _app.Config.NewMoviesUrl != "" {
		newMoviesBtn.Url = _app.Config.NewMoviesUrl
		newMoviesBtn.CallbackData = ""
	}
	updatesBtn := gotgbot.InlineKeyboardButton{Text: "📺 Updates", Style: "success", CallbackData: "ignore"}
	if _app.Config.UpdatesUrl != "" {
		updatesBtn.Url = _app.Config.UpdatesUrl
		updatesBtn.CallbackData = ""
	}

	buttons = append(buttons, []gotgbot.InlineKeyboardButton{newMoviesBtn, updatesBtn})

	// Navigation
	buttons = append(buttons, footerRow(uniqueId, 0, len(files)))

	// Footer Action Row 2
	buttons = append(buttons, []gotgbot.InlineKeyboardButton{
		{Text: "📢 Share", SwitchInlineQuery: &query, Style: "success"},
		{Text: "❌ Close", CallbackData: "close", Style: "danger"},
		{Text: "♻️ Reset", CallbackData: "reset|" + query, Style: "primary"},
	})

	text := format.KeyValueFormat(_app.Config.GetResultTemplate(), _app.BasicMessageValues(ctx, map[string]any{
		"query":         query,
		"warn":          warn,
		"total":         len(allFiles),
		"results_count": len(allFiles),
	}))

	var (
		msg     *gotgbot.Message
		sendErr error
	)

	resultsChannelID := _app.Config.GetResultsChannelID()
	useResultsChannel := resultsChannelID != 0 && inputMessage.GetChat().Type != "private"
	sendChatID := inputMessage.GetChat().Id
	if useResultsChannel {
		sendChatID = resultsChannelID
	}

	var replyParams *gotgbot.ReplyParameters
	if !useResultsChannel {
		replyParams = &gotgbot.ReplyParameters{
			MessageId: inputMessage.GetMessageId(),
		}
	}

	var posterUrl string
	if _app.Config.GetPosterEnabled() {
		if len(allFiles) > 0 {
			specificQuery := autofilter.ExtractBaseTitle(allFiles[0].FileName)
			if !isSeries {
				// Add year if available for movie
				yearRegex := regexp.MustCompile(`\b(19|20)\d{2}\b`)
				if match := yearRegex.FindString(allFiles[0].FileName); match != "" {
					specificQuery += " " + match
				}
			}
			posterUrl = autofilter.GetPosterUrlWithType(specificQuery, isSeries)
		} else {
			posterUrl = autofilter.GetPosterUrlWithType(query, isSeries)
		}
	}
	if posterUrl != "" {
		msg, sendErr = bot.SendPhoto(sendChatID, gotgbot.InputFileByURL(posterUrl), &gotgbot.SendPhotoOpts{
			Caption: text,
			ReplyParameters: replyParams,
			HasSpoiler:  true,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons},
			ParseMode:   gotgbot.ParseModeHTML,
		})
		if sendErr != nil {
			_app.Log.Warn("autofilter: send photo failed, falling back to text message", zap.Error(sendErr))
			posterUrl = "" // clear so redirect links or downstream know it's text
			msg, sendErr = bot.SendMessage(sendChatID, text, &gotgbot.SendMessageOpts{
				ReplyParameters: replyParams,
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons},
				ParseMode:   gotgbot.ParseModeHTML,
			})
		}
	} else {
		msg, sendErr = bot.SendMessage(sendChatID, text, &gotgbot.SendMessageOpts{
			ReplyParameters: replyParams,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons},
			ParseMode:   gotgbot.ParseModeHTML,
		})
	}

	if sendErr != nil {
		_app.Log.Warn("autofilter: send result failed", zap.Error(sendErr))
		if useResultsChannel {
			// Fallback: send directly to original chat
			_app.Log.Info("autofilter: results channel send failed, falling back to direct chat delivery")
			replyParamsFallback := &gotgbot.ReplyParameters{
				MessageId: inputMessage.GetMessageId(),
			}
			if posterUrl != "" {
				msg, err = bot.SendPhoto(inputMessage.GetChat().Id, gotgbot.InputFileByURL(posterUrl), &gotgbot.SendPhotoOpts{
					Caption: text,
					ReplyParameters: replyParamsFallback,
					HasSpoiler:  true,
					ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons},
					ParseMode:   gotgbot.ParseModeHTML,
				})
				if err != nil {
					_app.Log.Warn("autofilter: fallback send photo failed, trying fallback text message", zap.Error(err))
					msg, err = bot.SendMessage(inputMessage.GetChat().Id, text, &gotgbot.SendMessageOpts{
						ReplyParameters: replyParamsFallback,
						ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons},
						ParseMode:   gotgbot.ParseModeHTML,
					})
				}
			} else {
				msg, err = bot.SendMessage(inputMessage.GetChat().Id, text, &gotgbot.SendMessageOpts{
					ReplyParameters: replyParamsFallback,
					ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons},
					ParseMode:   gotgbot.ParseModeHTML,
				})
			}
			if err != nil {
				_app.Log.Warn("autofilter: fallback send failed", zap.Error(err))
				return nil, err
			}
			// Set useResultsChannel to false so we don't try to send redirect message
			useResultsChannel = false
		} else {
			return nil, sendErr
		}
	} else {
		middleware.ReactWithRandomEmoji(bot, msg.Chat.Id, msg.MessageId, _app.Config, _app.Log)
	}

	err = _app.Cache.Autofilter.Save(&autofilter.SearchResult{
		UniqueId: uniqueId,
		Query:    query,
		FromUser: fromUser.Id,
		ChatID:   msg.Chat.Id,
		Files:    files,
		IsPhoto:  posterUrl != "",
	})
	if err != nil {
		_app.Log.Warn("autofilter: save cache failed", zap.Error(err), zap.String("query", query))
	}

	if useResultsChannel {
		// Send redirect message to original group chat and return it so it gets auto-deleted
		channelIDStr := fmt.Sprint(resultsChannelID)
		channelIDStr = strings.TrimPrefix(channelIDStr, "-100")
		link := fmt.Sprintf("https://t.me/c/%s/%d", channelIDStr, msg.MessageId)
		if msg.Chat.Username != "" {
			link = fmt.Sprintf("https://t.me/%s/%d", msg.Chat.Username, msg.MessageId)
		}

		redirectText := format.KeyValueFormat(`<b>🍿 Hᴇʏ {mention}, I'ᴠᴇ Fᴏᴜɴᴅ Sᴏᴍᴇ ᴍᴀᴛᴄʜᴇs ғᴏʀ ʏᴏᴜ!</b>
<blockquote><b>🔍 Sᴇᴀʀᴄʜ Query:</b> <code>{query}</code>
<b>📂 TᴏᴛᴀTx Fɪʟᴇs Fᴏᴜɴᴅ:</b> <code>{total}</code></blockquote>

<i>👇 Cʟɪᴄᴋ ᴛʜᴇ ʙᴜᴛᴛᴏɴ ʙᴇʟᴏᴡ ᴛᴏ ᴠɪᴇᴡ ʏᴏᴜʀ ʀᴇsᴜʟᴛs ɪɴ ᴛʜᴇ ᴄʜᴀɴɴᴇʟ:</i>`, _app.BasicMessageValues(ctx, map[string]any{
			"query":         query,
			"warn":          warn,
			"total":         len(allFiles),
			"results_count": len(allFiles),
		}))

		// Fix "TᴏᴛᴀTx" to "Tᴏᴛᴀʟ"
		redirectText = strings.Replace(redirectText, "TᴏᴛᴀTx", "Tᴏᴛᴀʟ", 1)

		redirectButtons := [][]gotgbot.InlineKeyboardButton{
			{{Text: "📂 View Results 🍿", Url: link}},
		}

		redirectMsg, err := bot.SendMessage(inputMessage.GetChat().Id, redirectText, &gotgbot.SendMessageOpts{
			ReplyParameters: &gotgbot.ReplyParameters{
				MessageId: inputMessage.GetMessageId(),
			},
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: redirectButtons},
			ParseMode:   gotgbot.ParseModeHTML,
		})
		if err != nil {
			_app.Log.Warn("autofilter: send redirect message failed", zap.Error(err))
			return msg, nil
		}
		return redirectMsg, nil
	}

	return msg, nil
}

// InlineSearch handles telegram inline queries.
func InlineSearch(bot *gotgbot.Bot, ctx *ext.Context) error {
	iq := ctx.InlineQuery
	query := iq.Query

	if query == "" {
		// Optional: Show trending or help
		_, err := iq.Answer(bot, nil, &gotgbot.AnswerInlineQueryOpts{
			CacheTime:  func(v int64) *int64 { return &v }(300),
			IsPersonal: true,
			Button: &gotgbot.InlineQueryResultsButton{
				Text:           "Type movie name to search...",
				StartParameter: "inline_help",
			},
		})
		return err
	}

	cursor, err := _app.DB.SearchFiles(query)
	if err != nil {
		_app.Log.Warn("inline: search files failed", zap.Error(err), zap.String("query", query))
		return nil
	}

	files, err := autofilter.FilesFromCursor(context.Background(), cursor, _app.Config)
	if err != nil {
		_app.Log.Warn("inline: files from cursor failed", zap.Error(err))
		return nil
	}

	files = processSearchResults(files)
	allFiles := slices.Concat(files...)

	if len(allFiles) == 0 {
		_, err := iq.Answer(bot, nil, &gotgbot.AnswerInlineQueryOpts{
			CacheTime: func(v int64) *int64 { return &v }(60),
			Button: &gotgbot.InlineQueryResultsButton{
				Text:           "No results found!",
				StartParameter: "no_results",
			},
		})
		return err
	}

	// Limit to 50 results as per Telegram API
	if len(allFiles) > 50 {
		allFiles = allFiles[:50]
	}

	// Group files by their base title (+ year for movies) to show distinct titles with unique posters
	type titleGroup struct {
		Title    string
		Year     string
		IsSeries bool
		Count    int
	}
	seen := make(map[string]*titleGroup)
	var orderedTitles []string

	for _, f := range allFiles {
		baseTitle := autofilter.ExtractBaseTitle(f.FileName)
		if baseTitle == "" {
			baseTitle = strings.Title(query)
		}

		isSeries := autofilter.IsSeriesFile(f.FileName)
		year := ""
		if !isSeries {
			yearRegex := regexp.MustCompile(`\b(19|20)\d{2}\b`)
			if match := yearRegex.FindString(f.FileName); match != "" {
				year = match
			}
		}

		key := strings.ToLower(baseTitle)
		if year != "" {
			key += "_" + year
		}

		if g, ok := seen[key]; ok {
			g.Count++
			if !g.IsSeries && isSeries {
				g.IsSeries = true
			}
		} else {
			seen[key] = &titleGroup{
				Title:    baseTitle,
				Year:     year,
				IsSeries: isSeries,
				Count:    1,
			}
			orderedTitles = append(orderedTitles, key)
		}
	}

	// Limit to 10 distinct titles max (each needs a TMDB lookup)
	if len(orderedTitles) > 10 {
		orderedTitles = orderedTitles[:10]
	}

	results := make([]gotgbot.InlineQueryResult, 0, len(orderedTitles))
	for i, key := range orderedTitles {
		g := seen[key]

		var groupFiles autofilter.Files
		for _, f := range allFiles {
			baseTitle := autofilter.ExtractBaseTitle(f.FileName)
			if baseTitle == "" {
				baseTitle = strings.Title(query)
			}
			isSeries := autofilter.IsSeriesFile(f.FileName)
			year := ""
			if !isSeries {
				yearRegex := regexp.MustCompile(`\b(19|20)\d{2}\b`)
				if match := yearRegex.FindString(f.FileName); match != "" {
					year = match
				}
			}
			fileKey := strings.ToLower(baseTitle)
			if year != "" {
				fileKey += "_" + year
			}
			if fileKey == key {
				groupFiles = append(groupFiles, f)
			}
		}

		pagedGroupFiles := processSearchResults([]autofilter.Files{groupFiles})
		if len(pagedGroupFiles) == 0 {
			continue
		}

		var groupAllFiles autofilter.Files
		for _, p := range pagedGroupFiles {
			groupAllFiles = append(groupAllFiles, p...)
		}

		mediaType := "Movie"
		if g.IsSeries {
			mediaType = "Series"
		}

		displayTitle := g.Title
		if !g.IsSeries && g.Year != "" {
			displayTitle = fmt.Sprintf("%s (%s)", g.Title, g.Year)
		}
		description := fmt.Sprintf("📂 %s • %d files available", mediaType, len(groupAllFiles))

		var buttons [][]gotgbot.InlineKeyboardButton
		uniqueId := functions.RandString(15)

		specificQuery := g.Title
		if !g.IsSeries && g.Year != "" {
			specificQuery = g.Title + " " + g.Year
		}
		var posterUrl string
		if _app.Config.GetPosterEnabled() {
			posterUrl = autofilter.GetPosterUrlWithType(specificQuery, g.IsSeries)
		}

		err = _app.Cache.Autofilter.Save(&autofilter.SearchResult{
			UniqueId: uniqueId,
			Query:    specificQuery,
			FromUser: iq.From.Id,
			ChatID:   0,
			Files:    pagedGroupFiles,
			IsPhoto:  posterUrl != "",
		})
		if err != nil {
			_app.Log.Warn("inline search: save cache failed", zap.Error(err))
		}

		if g.IsSeries {
			// Season row
			seasonButtons := groupAllFiles.ProcessSeasons(uniqueId)
			if len(seasonButtons) > 0 && len(seasonButtons[0]) > 0 {
				seasonButtons[0][0].Style = "success"
			}
			buttons = append(buttons, seasonButtons...)

			// Language row
			languages := autofilter.DetectLanguages(groupAllFiles)
			if len(languages) > 0 {
				var langRow []gotgbot.InlineKeyboardButton
				for _, l := range languages {
					langRow = append(langRow, gotgbot.InlineKeyboardButton{
						Text:         l,
						CallbackData: fmt.Sprintf("lang|%s_%s", uniqueId, l),
						Style:        "primary",
					})
					if len(langRow) == 2 {
						buttons = append(buttons, langRow)
						langRow = nil
					}
				}
				if len(langRow) > 0 {
					buttons = append(buttons, langRow)
				}
			}
		}

		// Divider
		latestReleasesBtn := gotgbot.InlineKeyboardButton{Text: "🌟 Latest Releases 🌟", CallbackData: "ignore", Style: "success"}
		if _app.Config.LatestReleasesUrl != "" {
			latestReleasesBtn.Url = _app.Config.LatestReleasesUrl
			latestReleasesBtn.CallbackData = ""
		}
		buttons = append(buttons, []gotgbot.InlineKeyboardButton{latestReleasesBtn})

		// File buttons for first page
		fileButtons := pagedGroupFiles[0].Process(0, bot.Username, _app.Config)
		buttons = append(buttons, fileButtons...)

		// Footer Action Row 1
		newMoviesBtn := gotgbot.InlineKeyboardButton{Text: "🍿 New Movies", Style: "success", CallbackData: "btn_new"}
		if _app.Config.NewMoviesUrl != "" {
			newMoviesBtn.Url = _app.Config.NewMoviesUrl
			newMoviesBtn.CallbackData = ""
		}
		updatesBtn := gotgbot.InlineKeyboardButton{Text: "📺 Updates", Style: "success", CallbackData: "ignore"}
		if _app.Config.UpdatesUrl != "" {
			updatesBtn.Url = _app.Config.UpdatesUrl
			updatesBtn.CallbackData = ""
		}
		buttons = append(buttons, []gotgbot.InlineKeyboardButton{newMoviesBtn, updatesBtn})

		// Navigation
		if len(pagedGroupFiles) > 1 {
			buttons = append(buttons, footerRow(uniqueId, 0, len(pagedGroupFiles)))
		}

		// Footer Action Row 2
		buttons = append(buttons, []gotgbot.InlineKeyboardButton{
			{Text: "📢 Share", SwitchInlineQuery: &specificQuery, Style: "success"},
			{Text: "❌ Close", CallbackData: "close", Style: "danger"},
			{Text: "♻️ Reset", CallbackData: "reset|" + specificQuery, Style: "primary"},
		})

		replyMarkup := &gotgbot.InlineKeyboardMarkup{
			InlineKeyboard: buttons,
		}

		caption := fmt.Sprintf("<b>🎬 %s</b>\n📂 <i>%s • %d files available</i>\n\n⚡️ 𝖲𝖾𝗅𝖾𝖼𝗍 𝗍𝗁𝖾 𝖿𝗂𝗅𝖾 𝗒𝗈υ 𝗐𝖺𝗇𝗍 𝖻𝖾𝗅𝗈𝗐:", displayTitle, mediaType, len(groupAllFiles))

		resultId := fmt.Sprintf("title_%d_%s", i, key)

		if posterUrl != "" {
			results = append(results, gotgbot.InlineQueryResultPhoto{
				Id:           resultId,
				PhotoUrl:     posterUrl,
				ThumbnailUrl: posterUrl,
				Title:        displayTitle,
				Description:  description,
				Caption:      caption,
				ParseMode:    gotgbot.ParseModeHTML,
				ReplyMarkup:  replyMarkup,
			})
		} else {
			msgContent := gotgbot.InputTextMessageContent{
				MessageText: caption,
				ParseMode:   gotgbot.ParseModeHTML,
			}
			results = append(results, gotgbot.InlineQueryResultArticle{
				Id:                  resultId,
				Title:               displayTitle,
				Description:         description,
				InputMessageContent: msgContent,
				ReplyMarkup:         replyMarkup,
			})
		}
	}

	_, err = iq.Answer(bot, results, &gotgbot.AnswerInlineQueryOpts{
		CacheTime:  func(v int64) *int64 { return &v }(300),
		IsPersonal: true,
	})
	if err != nil {
		_app.Log.Warn("inline: answer failed", zap.Error(err))
	}

	return err
}

func SendFileCallback(bot *gotgbot.Bot, ctx *ext.Context) error {
	ok, err := fsub.CheckFsub(_app, bot, ctx)
	if err != nil {
		_app.Log.Warn("SendFileCallback: check fsub failed", zap.Error(err))
	}
	if !ok {
		return nil
	}

	c := ctx.CallbackQuery
	_, fileUniqueId, found := strings.Cut(c.Data, "|")
	if !found {
		return nil
	}
	f, err := _app.DB.GetFile(fileUniqueId)
	if err != nil {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "404: File Not Found!", ShowAlert: true})
		return nil
	}

	var targetChatId int64
	if c.Message != nil {
		targetChatId = c.Message.GetChat().Id
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Sending File... 📤"})
	} else {
		targetChatId = c.From.Id
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Sending File to your PM... 📥"})
	}

	var (
		warn    string
		delTime = _app.Config.GetFileAutoDelete()
	)
	if delTime != 0 {
		warn = fmt.Sprintf("<blockquote><b>⚠️ This File Will Be Automatically Deleted in %d Minutes.\n\nPlease Forward it to Another Chat or Saved Messages to save it forever! 📥</b></blockquote>", delTime)
	}

	msg, err := f.Send(bot, targetChatId, &model.SendFileOpts{
		Caption: _app.FormatText(ctx, _app.Config.GetFileCaption(), map[string]any{
			"file_size": functions.FileSizeToString(f.FileSize),
			"file_name": autofilter.CleanFileNameForDisplay(f.FileName),
			"warn":      warn,
		}),
		Keyboard: [][]gotgbot.InlineKeyboardButton{{{Text: "🗑️ ᴅᴇʟᴇᴛᴇ ғɪʟᴇ 🗑️", CallbackData: "close"}}},
	})
	if err != nil {
		if functions.IsChatNotFoundErr(err) {
			c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
				Text:      "⚠️ Please START the bot first in PM to receive files!",
				ShowAlert: true,
				Url:       fmt.Sprintf("https://t.me/%s?start=start", bot.Username),
			})
			return nil
		}
		_app.Log.Warn("callback: send file failed", zap.Error(err), zap.String("file_id", f.FileId))
		return nil
	}

	if delTime != 0 && msg != nil {
		err = _app.AutoDelete.SaveMessage(msg, time.Minute*time.Duration(delTime))
		if err != nil {
			_app.Log.Warn("callback: insert auto delete failed", zap.Error(err))
		}
	}

	return nil
}

func headerRow(uniqueId string, pageIndex int, isSeries bool) []gotgbot.InlineKeyboardButton {
	row := []gotgbot.InlineKeyboardButton{selectButton(uniqueId, pageIndex)}
	if isSeries {
		row = append(row, gotgbot.InlineKeyboardButton{Text: "📂 sᴇᴀsᴏɴs", CallbackData: "af|" + uniqueId, Style: "primary"})
	}
	return row
}

func allButton(uniqueId string, pageIndex int) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{Text: "ᴀʟʟ", CallbackData: fmt.Sprintf("all|%s_%d", uniqueId, pageIndex), Style: "success"}
}

func selectButton(uniqueId string, pageIndex int) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{Text: "sᴇʟᴇᴄᴛ", CallbackData: fmt.Sprintf("sel|%s_%d", uniqueId, pageIndex), Style: "primary"}
}

func footerRow(uniqueId string, pageIndex, totalPages int) []gotgbot.InlineKeyboardButton {
	btns := make([]gotgbot.InlineKeyboardButton, 0, 3)
	if pageIndex != 0 {
		btns = append(btns, backButton(uniqueId, pageIndex-1))
	}

	btns = append(btns, gotgbot.InlineKeyboardButton{Text: fmt.Sprintf("💎 %d / %d", pageIndex+1, totalPages), CallbackData: "ignore"})

	if pageIndex+1 != totalPages {
		btns = append(btns, nextButton(uniqueId, pageIndex+1))
	}

	return btns
}

func backButton(uniqueId string, pageIndex int) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{Text: "« ʙᴀᴄᴋ", CallbackData: fmt.Sprintf("navg|%s_%d", uniqueId, pageIndex), Style: "primary"}
}

func nextButton(uniqueId string, pageIndex int) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{Text: "ɴᴇxᴛ »", CallbackData: fmt.Sprintf("navg|%s_%d", uniqueId, pageIndex), Style: "primary"}
}

// SeasonListCallback handles returning to the season selection list.
func SeasonListCallback(bot *gotgbot.Bot, ctx *ext.Context) error {
	c := ctx.CallbackQuery
	callbackData := callbackdata.FromString(c.Data)
	if len(callbackData.Args) < 1 {
		return nil
	}

	uniqueId := callbackData.Args[0]
	res, ok, err := _app.Cache.Autofilter.Get(uniqueId)
	if err != nil || !ok {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Session Expired!", ShowAlert: true})
		return nil
	}

	if c.From.Id != res.FromUser {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Not your session!", ShowAlert: true})
		return nil
	}

	var allFiles autofilter.Files
	for _, page := range res.Files {
		allFiles = append(allFiles, page...)
	}
	buttons := allFiles.ProcessSeasons(uniqueId)
	text := "<b>Select Season:</b>"
	var mainPosterUrl string
	if res.IsPhoto {
		mainPosterUrl = autofilter.GetPosterUrlWithType(res.Query, true)
	}
	err = editMessageOrCaption(bot, c.Message, c.InlineMessageId, text, gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}, res.IsPhoto, mainPosterUrl)
	if err == nil && c.Message != nil {
		middleware.ReactWithRandomEmoji(bot, c.Message.GetChat().Id, c.Message.GetMessageId(), _app.Config, _app.Log)
	}
	return err
}

// SeasonCallback handles clicking on a Season button.
func SeasonCallback(bot *gotgbot.Bot, ctx *ext.Context) error {
	c := ctx.CallbackQuery
	callbackData := callbackdata.FromString(c.Data)
	if len(callbackData.Args) < 2 {
		return nil
	}

	uniqueId := callbackData.Args[0]
	season, _ := strconv.Atoi(callbackData.Args[1])
	pageIndex := 0
	if len(callbackData.Args) > 2 {
		pageIndex, _ = strconv.Atoi(callbackData.Args[2])
	}

	res, ok, err := _app.Cache.Autofilter.Get(uniqueId)
	if err != nil {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "An Error occurred :\\", ShowAlert: true})
		_app.Log.Warn("sn: result from cache failed", zap.Error(err))
		return nil
	}
	if !ok {
		_, err := c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "Session Expired! Please search again.",
			ShowAlert: true,
		})
		return err
	}

	if c.From.Id != res.FromUser {
		_, err := c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "You Can't Use This Button!",
			ShowAlert: true,
		})
		return err
	}

	// Filter and Sort files for the selected season
	var allFiles autofilter.Files
	for _, page := range res.Files {
		allFiles = append(allFiles, page...)
	}
	seasonFiles := allFiles.FilterBySeason(season)
	seasonFiles.SortSeries()

	// Create a sub-result for this season to support ALL/SELECT
	seasonUniqueId := fmt.Sprintf("%s_s%d", uniqueId, season)
	seasonResult := &autofilter.SearchResult{
		UniqueId: seasonUniqueId,
		Query:    fmt.Sprintf("%s - Season %d", res.Query, season),
		FromUser: res.FromUser,
		ChatID:   res.ChatID,
		Files:    []autofilter.Files{seasonFiles}, // Store as one page for simplicity
	}
	_ = _app.Cache.Autofilter.Save(seasonResult)

	// Chunk episodes for pagination
	pageSize := 10 // Episodes per page
	totalPages := (len(seasonFiles) + pageSize - 1) / pageSize
	if pageIndex < 0 {
		pageIndex = 0
	}
	if pageIndex >= totalPages {
		pageIndex = totalPages - 1
	}

	start := pageIndex * pageSize
	end := start + pageSize
	if end > len(seasonFiles) {
		end = len(seasonFiles)
	}
	currentPageEpisodes := seasonFiles[start:end]

	var buttons [][]gotgbot.InlineKeyboardButton

	// Season row
	seasonButtons := allFiles.ProcessSeasons(uniqueId)
	for rIdx, row := range seasonButtons {
		for cIdx, btn := range row {
			expectedData := fmt.Sprintf("sn|%s_%d", uniqueId, season)
			if btn.CallbackData == expectedData {
				seasonButtons[rIdx][cIdx].Style = "success"
			}
		}
	}
	buttons = append(buttons, seasonButtons...)

	// Language row
	languages := autofilter.DetectLanguages(allFiles)
	if len(languages) > 0 {
		var langRow []gotgbot.InlineKeyboardButton
		for _, l := range languages {
			langRow = append(langRow, gotgbot.InlineKeyboardButton{
				Text:         l,
				CallbackData: fmt.Sprintf("lang|%s_%s", uniqueId, l),
			})
			if len(langRow) == 2 {
				buttons = append(buttons, langRow)
				langRow = nil
			}
		}
		if len(langRow) > 0 {
			buttons = append(buttons, langRow)
		}
	}

	// Divider
	latestReleasesBtn := gotgbot.InlineKeyboardButton{Text: "🌟 Latest Releases 🌟", CallbackData: "ignore", Style: "success"}
	if _app.Config.LatestReleasesUrl != "" {
		latestReleasesBtn.Url = _app.Config.LatestReleasesUrl
		latestReleasesBtn.CallbackData = ""
	}
	buttons = append(buttons, []gotgbot.InlineKeyboardButton{latestReleasesBtn})

	var chatId int64
	var chatType string
	if ctx.EffectiveChat != nil {
		chatId = ctx.EffectiveChat.Id
		chatType = ctx.EffectiveChat.Type
	}

	fileButtons := currentPageEpisodes.Process(chatId, bot.Username, _app.Config)
	buttons = append(buttons, fileButtons...)

	// Multi-select for this season
	if chatType == "private" {
		buttons = append(buttons, []gotgbot.InlineKeyboardButton{{Text: "✅ Select Multiple Files", CallbackData: fmt.Sprintf("sel|%s_%d", seasonUniqueId, pageIndex), Style: "primary"}})
	}

	// Navigation (Diamonds Style)
	navBtns := make([]gotgbot.InlineKeyboardButton, 0, 3)
	if pageIndex > 0 {
		navBtns = append(navBtns, gotgbot.InlineKeyboardButton{
			Text: "« ʙᴀᴄᴋ", CallbackData: fmt.Sprintf("sn|%s_%d_%d", uniqueId, season, pageIndex-1),
			Style: "primary",
		})
	}
	navBtns = append(navBtns, gotgbot.InlineKeyboardButton{
		Text: fmt.Sprintf("💎 %d / %d", pageIndex+1, totalPages), CallbackData: "ignore",
	})
	if pageIndex+1 < totalPages {
		navBtns = append(navBtns, gotgbot.InlineKeyboardButton{
			Text: "ɴᴇxᴛ »", CallbackData: fmt.Sprintf("sn|%s_%d_%d", uniqueId, season, pageIndex+1),
			Style: "primary",
		})
	}
	buttons = append(buttons, navBtns)

	// Footer Action Row 1 (Links)
	newMoviesBtn := gotgbot.InlineKeyboardButton{Text: "🍿 New Movies", Style: "success", CallbackData: "btn_new"}
	if _app.Config.NewMoviesUrl != "" {
		newMoviesBtn.Url = _app.Config.NewMoviesUrl
		newMoviesBtn.CallbackData = ""
	}
	updatesBtn := gotgbot.InlineKeyboardButton{Text: "📺 Updates", Style: "success", CallbackData: "ignore"}
	if _app.Config.UpdatesUrl != "" {
		updatesBtn.Url = _app.Config.UpdatesUrl
		updatesBtn.CallbackData = ""
	}
	buttons = append(buttons, []gotgbot.InlineKeyboardButton{newMoviesBtn, updatesBtn})

	// Footer Action Row 2 (Actions)
	buttons = append(buttons, []gotgbot.InlineKeyboardButton{
		{Text: "📢 Share", SwitchInlineQuery: &res.Query, Style: "success"},
		{Text: "❌ Close", CallbackData: "close", Style: "danger"},
		{Text: "🔙 Back", CallbackData: fmt.Sprintf("af|%s", uniqueId), Style: "primary"},
	})

	var seasonPosterUrl string
	if res.IsPhoto {
		seasonPosterUrl = autofilter.GetSeasonPosterUrl(res.Query, season)
	}
	err = editMessageOrCaption(bot, c.Message, c.InlineMessageId, "<b>Select Episode:</b>", gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}, res.IsPhoto, seasonPosterUrl)
	return err
}

// LanguageCallback handles clicking on a Language filter button.
func LanguageCallback(bot *gotgbot.Bot, ctx *ext.Context) error {
	c := ctx.CallbackQuery
	callbackData := callbackdata.FromString(c.Data)
	if len(callbackData.Args) < 2 {
		return nil
	}

	uniqueId := callbackData.Args[0]
	language := callbackData.Args[1]

	res, ok, err := _app.Cache.Autofilter.Get(uniqueId)
	if err != nil || !ok {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Session Expired!", ShowAlert: true})
		return nil
	}

	if c.From.Id != res.FromUser {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Not your session!", ShowAlert: true})
		return nil
	}

	// Filter files by language
	var allFiles autofilter.Files
	for _, page := range res.Files {
		allFiles = append(allFiles, page...)
	}
	langFiles := allFiles.FilterByLanguage(language)
	
	if len(langFiles) == 0 {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "No files found for this language!", ShowAlert: true})
		return nil
	}

	// We'll show up to 10 results for simplicity in filter view
	if len(langFiles) > 10 {
		langFiles = langFiles[:10]
	}

	var chatId int64
	if ctx.EffectiveChat != nil {
		chatId = ctx.EffectiveChat.Id
	}

	var buttons [][]gotgbot.InlineKeyboardButton
	fileButtons := langFiles.Process(chatId, bot.Username, _app.Config)
	buttons = append(buttons, fileButtons...)

	// Footer Action Row
	buttons = append(buttons, []gotgbot.InlineKeyboardButton{
		{Text: "📢 Share", SwitchInlineQuery: &res.Query, Style: "success"},
		{Text: "❌ Close", CallbackData: "close", Style: "danger"},
		{Text: "🔙 Back", CallbackData: "reset|" + res.Query, Style: "primary"},
	})

	text := fmt.Sprintf("<b>Results for Language:</b> <code>%s</code>\n\n<i>Found %d files.</i>", language, len(langFiles))
	err = editMessageOrCaption(bot, c.Message, c.InlineMessageId, text, gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}, res.IsPhoto, "")
	return err
}

func editMessageOrCaption(bot *gotgbot.Bot, msg gotgbot.MaybeInaccessibleMessage, inlineMessageId string, text string, markup gotgbot.InlineKeyboardMarkup, isMedia bool, newPosterUrl string) error {
	if inlineMessageId != "" {
		if isMedia {
			if newPosterUrl != "" {
				_, _, err := bot.EditMessageMedia(
					gotgbot.InputMediaPhoto{
						Media:     gotgbot.InputFileByURL(newPosterUrl),
						Caption:   text,
						ParseMode: gotgbot.ParseModeHTML,
					},
					&gotgbot.EditMessageMediaOpts{
						InlineMessageId: inlineMessageId,
						ReplyMarkup:     markup,
					},
				)
				return err
			}
			_, _, err := bot.EditMessageCaption(&gotgbot.EditMessageCaptionOpts{
				InlineMessageId: inlineMessageId,
				Caption:         text,
				ReplyMarkup:     markup,
				ParseMode:       gotgbot.ParseModeHTML,
			})
			return err
		}
		_, _, err := bot.EditMessageText(text, &gotgbot.EditMessageTextOpts{
			InlineMessageId: inlineMessageId,
			ReplyMarkup:     markup,
			ParseMode:       gotgbot.ParseModeHTML,
		})
		return err
	}

	chatId := msg.GetChat().Id
	msgId := msg.GetMessageId()

	if !isMedia {
		if m, ok := msg.(*gotgbot.Message); ok {
			if len(m.Photo) > 0 || m.Video != nil || m.Document != nil || m.Audio != nil || m.Voice != nil || m.Animation != nil || m.VideoNote != nil || m.Sticker != nil {
				isMedia = true
			}
		}
	}

	if isMedia {
		if newPosterUrl != "" {
			_, _, err := bot.EditMessageMedia(
				gotgbot.InputMediaPhoto{
					Media:     gotgbot.InputFileByURL(newPosterUrl),
					Caption:   text,
					ParseMode: gotgbot.ParseModeHTML,
				},
				&gotgbot.EditMessageMediaOpts{
					ChatId:      chatId,
					MessageId:   msgId,
					ReplyMarkup: markup,
				},
			)
			return err
		}
		_, _, err := bot.EditMessageCaption(&gotgbot.EditMessageCaptionOpts{
			ChatId:      chatId,
			MessageId:   msgId,
			Caption:     text,
			ReplyMarkup: markup,
			ParseMode:   gotgbot.ParseModeHTML,
		})
		return err
	}

	_, _, err := bot.EditMessageText(text, &gotgbot.EditMessageTextOpts{
		ChatId:      chatId,
		MessageId:   msgId,
		ReplyMarkup: markup,
		ParseMode:   gotgbot.ParseModeHTML,
	})
	return err
}

func processSearchResults(files []autofilter.Files) []autofilter.Files {
	if len(files) == 0 {
		return nil
	}

	// 1. Flatten all files
	var allFiles autofilter.Files
	for _, page := range files {
		allFiles = append(allFiles, page...)
	}

	// 2. Deduplicate
	seenFileId := make(map[string]bool)
	seenNameSize := make(map[string]bool)
	var deduplicated autofilter.Files

	for _, f := range allFiles {
		fid := f.FileId
		nameSizeKey := fmt.Sprintf("%s_%d", f.FileName, f.FileSize)

		if fid != "" && seenFileId[fid] {
			continue
		}
		if seenNameSize[nameSizeKey] {
			continue
		}

		if fid != "" {
			seenFileId[fid] = true
		}
		seenNameSize[nameSizeKey] = true
		deduplicated = append(deduplicated, f)
	}

	// 3. Sort & Filter
	if len(deduplicated) > 0 {
		searchType := autofilter.DetectType(deduplicated)
		isSeries := searchType == "series"
		if isSeries {
			// Filter out any files that do not have series metadata (season > 0 or episode > 0)
			var seriesOnly autofilter.Files
			for _, f := range deduplicated {
				s, e := autofilter.ExtractSeriesMetadata(f.FileName)
				if s > 0 || e > 0 {
					seriesOnly = append(seriesOnly, f)
				}
			}
			// Only apply filter if we still have at least one series file left
			if len(seriesOnly) > 0 {
				deduplicated = seriesOnly
			}
			deduplicated.SortSeries()
		} else {
			deduplicated.SortMovies()
		}
	}

	// 4. Re-paginate
	pageSize := _app.Config.GetMaxPerPage()
	if pageSize <= 0 {
		pageSize = 10
	}

	var pagedFiles []autofilter.Files
	for i := 0; i < len(deduplicated); i += pageSize {
		end := i + pageSize
		if end > len(deduplicated) {
			end = len(deduplicated)
		}
		pagedFiles = append(pagedFiles, deduplicated[i:end])
	}

	return pagedFiles
}

