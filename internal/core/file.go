package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"autofilterbot/internal/autofilter"
	"autofilterbot/internal/database"
	"autofilterbot/internal/fsub"
	"autofilterbot/internal/functions"
	"autofilterbot/internal/model"
	"autofilterbot/pkg/conversation"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
)

// NewFile handles message updates in any authorized file channels.
func NewFile(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.EffectiveMessage

	if !functions.HasMedia(m) {
		return nil
	}

	file := functions.FileFromMessage(m)
	if file == nil {
		return nil
	}

	if file.FileName == "" {
		_app.Log.Debug("newfile: empty file name after sanitization", zap.String("file_id", file.FileName), zap.String("file_type", file.FileType))
		return nil
	}

	err := _app.DB.SaveFile(file)
	if err != nil {
		if _, ok := err.(database.FileAlreadyExistsError); ok {
			_app.Log.Debug("newfile: duplicate file skipped", zap.String("file_name", file.FileName))
			return nil
		}

		_app.Log.Warn("newfile: failed to save file", zap.Error(err))
	} else {
		if _app.Cache != nil && _app.Cache.SearchCache != nil {
			_app.Cache.SearchCache.Clear()
		}
		// Asynchronously check and trigger automated channel posting for new releases
		go AutoPostNewRelease(bot, file)
	}

	return nil
}

// DeleteFile handles the /delete command to delete a file.
func DeleteFile(bot *gotgbot.Bot, ctx *ext.Context) error {
	ok, _ := fsub.CheckFsub(_app, bot, ctx)
	if !ok {
		return nil
	}

	if !_app.AuthAdmin(ctx) {
		return nil
	}

	m := ctx.EffectiveMessage
	var fileUniqueId string
	var replyToMsg *gotgbot.Message

	if m != nil && m.ReplyToMessage != nil {
		if f := functions.FileFromMessage(m.ReplyToMessage); f != nil {
			fileUniqueId = f.UniqueId
			replyToMsg = m
		}
	}

	if fileUniqueId == "" {
		conv := conversation.NewConversatorFromUpdate(bot, ctx.Update)

		replyM, err := conv.Ask(_app.Ctx, "Please send me the file you would like to delete:", &gotgbot.SendMessageOpts{
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{Text: "❌ Cancel", CallbackData: "admin:cancel"}}},
			},
		})
		if err != nil {
			return nil
		}

		f := functions.FileFromMessage(replyM)
		if f == nil {
			replyM.Reply(bot, "No Media Found!", nil)
			return nil
		}

		fileUniqueId = f.UniqueId
		replyToMsg = replyM
	}

	err := _app.DB.DeleteFile(fileUniqueId)
	if err != nil {
		if replyToMsg != nil {
			replyToMsg.Reply(bot, fmt.Sprintf("Failed to Delete File: %v", err), nil)
		}
		_app.Log.Warn("delete file failed", zap.String("file_id", fileUniqueId), zap.Error(err))
		return nil
	} else {
		if _app.Cache != nil && _app.Cache.SearchCache != nil {
			_app.Cache.SearchCache.Clear()
		}
	}

	if replyToMsg != nil {
		replyToMsg.Reply(bot, "𝖥𝗂𝗅𝖾 𝖶𝖺𝗌 𝖣𝖾𝗅𝖾𝗍𝖾𝖽 𝖲𝗎𝖼𝖼𝖾𝗌𝗌𝖿𝗎𝗅𝗅𝗒 🗑️", nil)
	}

	return nil
}

const (
	delAllCountDangerous = 20 // if more than this many files are to be deleted, user must be prompted for confirmation
)

// DeleteAllFiles handles the /deleteall command to delete all matching files.
func DeleteAllFiles(bot *gotgbot.Bot, ctx *ext.Context) error {
	ok, _ := fsub.CheckFsub(_app, bot, ctx)
	if !ok {
		return nil
	}
	m := ctx.EffectiveMessage

	if !_app.AuthAdmin(ctx) {
		return nil
	}

	var keyword string
	if ctx.CallbackQuery != nil {
		conv := conversation.NewConversatorFromUpdate(bot, ctx.Update)
		askM, err := conv.Ask(_app.Ctx, "<b>𝖯𝗅𝖾𝖺𝗌𝖾 𝗌𝖾𝗇𝖽 𝗍𝗁𝖾 𝗄𝖾𝗒𝗐𝗈𝗋𝖽 𝗍𝗈 𝗌𝖾𝖺𝗋𝗀𝗁 𝖺𝗇𝖽 𝖽𝖾𝗅𝖾𝗍𝖾 𝖿𝗂𝗅𝖾𝗌:</b>", &gotgbot.SendMessageOpts{
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{Text: "❌ Cancel", CallbackData: "admin:cancel"}}},
			},
			ParseMode: gotgbot.ParseModeHTML,
		})
		if err != nil {
			_app.Log.Warn("delall: ask keyword failed", zap.Error(err))
			return nil
		}
		keyword = askM.Text
	} else {
		split := strings.SplitN(m.Text, " ", 2)
		if len(split) != 2 {
			m.Reply(bot, "<b>Improper Usage, Keyword Missing!</b>\n<blockquote>Format:\n /deleteall &lt keyword&gt</blockquote>\n<blockquote>Example:\n /deleteall 720p</blockquote>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
			return nil
		}
		keyword = split[1]
	}

	if keyword == "" {
		m.Reply(bot, "Keyword Is Empty :/", nil)
		return nil
	}

	cursor, err := _app.DB.SearchFiles(keyword)
	if err != nil {
		m.Reply(bot, fmt.Sprintf("An Error occurred: %v", err), nil)
		_app.Log.Warn("delall: search files failed", zap.Error(err), zap.String("keyword", keyword))
		return nil
	}

	f, err := autofilter.FilesFromCursor(context.Background(), cursor, DeleteAllCursorOptions{})
	if err != nil {
		m.Reply(bot, fmt.Sprintf("An Error occurred: %v", err), nil)
		_app.Log.Warn("delall: files from cursor failed", zap.Error(err), zap.String("keyword", keyword))
		return nil
	}

	if len(f) == 0 || len(f[0]) == 0 {
		m.Reply(bot, fmt.Sprintf("I Couldn't Find Anything Matching %s", keyword), nil)
		return nil
	}

	files := f[0]
	if len(files) > delAllCountDangerous {
		conv := conversation.NewConversatorFromUpdate(bot, ctx.Update)

		msg, err := conv.Ask(_app.Ctx, fmt.Sprintf("<b>⛔ Dangerous Operation</b>\n<i>Are you sure you want to delete %d files? Send <code>yes</code> to confirm or click cancel:</i>", len(files)), &gotgbot.SendMessageOpts{
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{Text: "❌ Cancel", CallbackData: "admin:cancel"}}},
			},
			ParseMode: gotgbot.ParseModeHTML,
		})
		if err != nil {
			_app.Log.Warn("delall: ask confirmation failed", zap.Error(err))
			return nil
		}

		if !strings.EqualFold(msg.Text, "yes") {
			msg.Reply(bot, "<i>Operation Cancelled !</i>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
			return nil
		}
	}

	progM, err := m.Reply(bot, "🗑️ Deleting Files, Please Wait...", nil)
	if err != nil {
		_app.Log.Warn("delall: send progress msg failed", zap.Error(err))
		return nil
	}

	var (
		allErrors []error
		deleted   int
	)

	for _, file := range files {
		err := _app.DB.DeleteFile(file.UniqueId)
		if err != nil {
			allErrors = append(allErrors, err)
			continue
		}

		deleted++
	}

	if deleted > 0 {
		if _app.Cache != nil && _app.Cache.SearchCache != nil {
			_app.Cache.SearchCache.Clear()
		}
	}

	text := fmt.Sprintf("<i><b>✅ Deleted %d Files Successfully !</b></i>", deleted)

	if len(allErrors) != 0 {
		errs := errors.Join(allErrors...)
		text += fmt.Sprintf("\nErrors occurred: %v", errs)
		_app.Log.Info("delall: errors occurs", zap.Error(errs))
	}

	_, _, err = progM.EditText(bot, text, &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML})
	if err != nil {
		_app.Log.Warn("delall: update progress msg failed", zap.Error(err))
	}

	return nil
}

// DeleteAllCursorOptions implements autofilter.FilesFromCursorOptions with optimal options for the operation.
type DeleteAllCursorOptions struct{}

// ensure it implements autofilter.FilesFromCursorOptions
var _ autofilter.FilesFromCursorOptions = DeleteAllCursorOptions{}

func (DeleteAllCursorOptions) GetMaxResults() int {
	return 100 // max 100 files can be deleted in a go
}

func (DeleteAllCursorOptions) GetMaxPages() int {
	return 1 // all files are in first page
}

func (DeleteAllCursorOptions) GetMaxPerPage() int {
	return 200 // just has to be more than max results
}

// CleanQuality searches for and deletes all low-quality files.
func CleanQuality(bot *gotgbot.Bot, ctx *ext.Context) error {
	if !_app.AuthAdmin(ctx) {
		return nil
	}

	keywords := []string{"camrip", "predvd", "hdcam", "telecine", "hc", "hdtc", "tc", "p-dvd", "ts", "hd-cam", "telesync", "screener", "dvdscr", "scr", "pre-dvd", "hq-cam", "hqcam"}

	m := ctx.EffectiveMessage
	var progM *gotgbot.Message
	var err error

	if ctx.CallbackQuery != nil {
		_, _, err = ctx.CallbackQuery.Message.EditText(bot, "🗑️ Scanning and Cleaning Low Quality Files...", &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML})
		switch msgT := ctx.CallbackQuery.Message.(type) {
		case gotgbot.Message:
			progM = &msgT
		case *gotgbot.Message:
			progM = msgT
		}
	} else {
		progM, err = m.Reply(bot, "🗑️ Scanning and Cleaning Low Quality Files...", nil)
	}

	if err != nil {
		_app.Log.Warn("cleanquality: send/edit progress failed", zap.Error(err))
		return nil
	}

	// Run low quality clean asynchronously in a goroutine to prevent blocking the dispatcher.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				_app.Log.Error("cleanquality: panic recovered", zap.Any("panic", r))
				_, _, _ = progM.EditText(bot, "❌ <b>Cleanup failed:</b> An unexpected internal panic occurred.", &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML})
			}
		}()

		var totalDeleted int
		for _, kw := range keywords {
			cursor, err := _app.DB.SearchFiles(kw)
			if err != nil {
				continue
			}

			for cursor.Next(context.Background()) {
				var file model.File
				if err := cursor.Decode(&file); err == nil {
					// Only delete if it matches one of the garbage/leak keywords or has low quality score
					if autofilter.IsGarbageFile(file.FileName) || autofilter.QualityLevel(file.FileName) <= 15 {
						if _app.DB.DeleteFile(file.UniqueId) == nil {
							totalDeleted++
						}
					}
				}
			}
			cursor.Close(context.Background())
		}

		if totalDeleted > 0 {
			if _app.Cache != nil && _app.Cache.SearchCache != nil {
				_app.Cache.SearchCache.Clear()
			}
		}

		text := fmt.Sprintf("<b>✅ Quality Clean Complete !</b>\n\nDeleted <code>%d</code> low-quality files.", totalDeleted)
		_, _, err = progM.EditText(bot, text, &gotgbot.EditMessageTextOpts{
			ParseMode: gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{
				Text: "🔙 Back", CallbackData: "admin:back",
			}}}},
		})
		if err != nil {
			_app.Log.Warn("cleanquality: edit final result failed", zap.Error(err))
		}
	}()

	return nil
}

// CleanDuplicatesCommand handles duplicates deletion triggered from the admin panel.
func CleanDuplicatesCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	if !_app.AuthAdmin(ctx) {
		return nil
	}

	m := ctx.EffectiveMessage
	var progM *gotgbot.Message
	var err error

	if ctx.CallbackQuery != nil {
		_, _, err = ctx.CallbackQuery.Message.EditText(bot, "🧹 <i>Scanning and cleaning duplicate files... This may take a moment.</i>", &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML})
		switch msgT := ctx.CallbackQuery.Message.(type) {
		case gotgbot.Message:
			progM = &msgT
		case *gotgbot.Message:
			progM = msgT
		}
	} else {
		progM, err = m.Reply(bot, "🧹 <i>Scanning and cleaning duplicate files... This may take a moment.</i>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	}

	if err != nil {
		_app.Log.Warn("cleandups: send/edit progress failed", zap.Error(err))
		return nil
	}

	// Run cleanup asynchronously in a goroutine to prevent blocking the dispatcher.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				_app.Log.Error("cleandups: panic recovered", zap.Any("panic", r))
				_, _, _ = progM.EditText(bot, "❌ <b>Cleanup failed:</b> An unexpected internal panic occurred.", &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML})
			}
		}()

		deletedCount, err := _app.DB.CleanDuplicates(_app.Ctx, _app.Log)
		if err != nil {
			_, _, _ = progM.EditText(bot, fmt.Sprintf("❌ <b>Cleanup failed:</b> <code>%s</code>", err.Error()), &gotgbot.EditMessageTextOpts{ParseMode: gotgbot.ParseModeHTML})
			return
		}

		if deletedCount > 0 {
			if _app.Cache != nil && _app.Cache.SearchCache != nil {
				_app.Cache.SearchCache.Clear()
			}
		}

		text := fmt.Sprintf("<b>✅ Duplicate Cleanup Complete !</b>\n\nDeleted <code>%d</code> duplicate file records.", deletedCount)
		_, _, err = progM.EditText(bot, text, &gotgbot.EditMessageTextOpts{
			ParseMode: gotgbot.ParseModeHTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{
				Text: "🔙 Back", CallbackData: "admin:back",
			}}}},
		})
		if err != nil {
			_app.Log.Warn("cleandups: edit final result failed", zap.Error(err))
		}
	}()

	return nil
}
