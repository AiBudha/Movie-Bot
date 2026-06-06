package core

import (
	"fmt"
	"strconv"

	"autofilterbot/internal/autofilter"
	"autofilterbot/pkg/callbackdata"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
)

// Navigate handles the navg callback query from autofilter results for pagination.
func Navigate(bot *gotgbot.Bot, ctx *ext.Context) error {
	var isPrivate bool
	if ctx.EffectiveChat != nil {
		isPrivate = ctx.EffectiveChat.Type == "private"
	}

	c := ctx.CallbackQuery

	data := callbackdata.FromString(c.Data)
	if len(data.Args) < 2 {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Error: Not Enough Arguments", ShowAlert: true, CacheTime: fiveHoursInSeconds})
		return nil
	}

	r, ok, err := _app.Cache.Autofilter.Get(data.Args[0])
	if err != nil {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "An Error occurred :\\", ShowAlert: true})
		_app.Log.Warn("navg: result from cache failed", zap.Error(err))
		return nil
	}

	if !ok {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "This Query Has Expired!\nPlease Request Again...", ShowAlert: true, CacheTime: fiveHoursInSeconds})
		return nil
	}

	if r.FromUser != c.From.Id {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "You Can't Use This, Please Ask For Your Own!", ShowAlert: true, CacheTime: fiveHoursInSeconds})
		return nil
	}

	pageIndex, err := strconv.Atoi(data.Args[1])
	if err != nil {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "An Error occurred :\\", ShowAlert: true})
		_app.Log.Warn("navg: parse page index failed", zap.Error(err))
		return nil
	}

	files := r.Files

	if pageIndex > len(files)-1 {
		c.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "404: Result Page Not Found", ShowAlert: true})
		_app.Log.Warn("navg: result page not found", zap.String("unique_id", r.UniqueId), zap.Int("index", pageIndex))
		return nil
	}

	pageFiles := files[pageIndex]
	var chatId int64
	if c.Message != nil {
		chatId = c.Message.GetChat().Id
	}

	var (
		buttons  = make([][]gotgbot.InlineKeyboardButton, 0, len(pageFiles)+10)
		allFiles []autofilter.File
	)
	for _, page := range r.Files {
		allFiles = append(allFiles, page...)
	}

	searchType := autofilter.DetectType(allFiles)
	isSeries := searchType == "series"

	if isSeries {
		// Season row
		seasonButtons := autofilter.Files(allFiles).ProcessSeasons(r.UniqueId)
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
					CallbackData: fmt.Sprintf("lang|%s_%s", r.UniqueId, l),
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

	// Add file buttons
	fileButtons := pageFiles.Process(chatId, bot.Username, _app.Config)
	buttons = append(buttons, fileButtons...)

	// Multi-select
	if isPrivate && len(allFiles) > 1 {
		buttons = append(buttons, []gotgbot.InlineKeyboardButton{{Text: "✅ Select Multiple Files", CallbackData: fmt.Sprintf("sel|%s_%d", r.UniqueId, pageIndex), Style: "primary"}})
	}
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
	if len(files) > 1 {
		buttons = append(buttons, footerRow(r.UniqueId, pageIndex, len(files)))
	}

	// Footer Action Row 2
	query := r.Query
	buttons = append(buttons, []gotgbot.InlineKeyboardButton{
		{Text: "📢 Share", SwitchInlineQuery: &query, Style: "success"},
		{Text: "❌ Close", CallbackData: "close", Style: "danger"},
		{Text: "♻️ Reset", CallbackData: "reset|" + query, Style: "primary"},
	})

	if c.InlineMessageId != "" {
		_, _, err = bot.EditMessageReplyMarkup(&gotgbot.EditMessageReplyMarkupOpts{
			InlineMessageId: c.InlineMessageId,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: buttons,
			},
		})
	} else if c.Message != nil {
		_, _, err = c.Message.EditReplyMarkup(bot, &gotgbot.EditMessageReplyMarkupOpts{
			ChatId:      chatId,
			MessageId:   c.Message.GetMessageId(),
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: buttons,
			},
		})
	}
	if err != nil {
		_app.Log.Warn("navg: edit markup failed", zap.Error(err), zap.String("unique_id", r.UniqueId))
	}

	return nil
}
