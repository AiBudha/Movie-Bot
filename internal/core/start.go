package core

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"autofilterbot/internal/autofilter"
	"autofilterbot/internal/fsub"
	"autofilterbot/internal/functions"
	"autofilterbot/internal/limiter"
	"autofilterbot/internal/middleware"
	"autofilterbot/internal/model"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
)

const (
	DataPrefixFile   = 'f'
	DataPrefixBatch  = 'b'
	DataPrefixRetry  = 'r'
	DataPrefixSearch = 's'
)

// StartCommand handles the start command.
func StartCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	m := ctx.Message
	user := m.From

	args := ctx.Args()
	_app.Log.Info("StartCommand triggered", zap.Int64("user_id", user.Id), zap.Strings("args", args))

	// 1. Initial user capture with source and DC
	var source string
	if len(args) > 1 {
		// Detect if it's a functional link or a referral tag
		arg := args[1]
		// Try RawURLEncoding first
		bytes, err := base64.RawURLEncoding.DecodeString(arg)
		// Fallback to StdEncoding
		if err != nil {
			bytes, err = base64.StdEncoding.DecodeString(arg)
		}

		if err == nil {
			data := string(bytes)
			if len(data) > 0 && (data[0] == DataPrefixFile || data[0] == DataPrefixBatch || data[0] == DataPrefixRetry || data[0] == DataPrefixSearch) {
				// Functional link, not a referral tag
				source = "direct_file"
			} else {
				source = arg
			}
		} else {
			// Not base64, likely a referral tag
			source = arg
		}
	} else {
		source = "organic"
	}

	go func() {
		// Detect DC and Country
		dc := functions.SetUserDC(bot, user.Id)
		country := functions.DetectCountry(user.LanguageCode)
		
		err := _app.DB.SaveUserExtended(user.Id, source, dc, user.LanguageCode)
		if err == nil {
			_app.DB.UpdateUserCountry(user.Id, country)
		} else {
			_app.Log.Warn("start: save user extended failed", zap.Error(err))
		}
	}()

	if len(args) < 2 {
		return StaticCommands(bot, ctx)
	}

	if strings.HasPrefix(args[1], "connect_") {
		chatIDStr := strings.TrimPrefix(args[1], "connect_")
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			_, _ = m.Reply(bot, "❌ Invalid connection link.", nil)
			return nil
		}

		_, err = bot.GetChatMember(chatID, user.Id, nil)
		if err != nil {
			_, _ = m.Reply(bot, "❌ You must be a member of the group to connect to it.", nil)
			return nil
		}

		err = _app.DB.SetUserConnection(user.Id, chatID)
		if err != nil {
			_, _ = m.Reply(bot, "❌ Failed to connect: DB error.", nil)
			return nil
		}

		chat, err := bot.GetChat(chatID, nil)
		title := chatIDStr
		if err == nil && chat != nil {
			title = chat.Title
		}

		_, err = m.Reply(bot, fmt.Sprintf("✅ Successfully connected to <b>%s</b>!\nNow you can search this group's files directly in my PM.", htmlEscape(title)), &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return err
	}

	// Any start data is expected to be base64 encoded
	bytes, err := base64.RawURLEncoding.DecodeString(args[1])
	if err != nil {
		bytes, err = base64.StdEncoding.DecodeString(args[1])
	}

	if err != nil {
		// If decoding fails, it might just be a referral source, not a file link
		_app.Log.Debug("start: decode data failed (might be a referral source)", zap.Error(err))
		return StaticCommands(bot, ctx) // Fallback to welcome message
	}

	data := string(bytes)
	if len(data) == 0 {
		return StaticCommands(bot, ctx)
	}

	switch data[0] {
	case DataPrefixFile:
		ok, err := fsub.CheckFsub(_app, bot, ctx)
		if err != nil {
			_app.Log.Warn("start: check fsub failed", zap.Error(err))
		}

		if !ok {
			return nil
		}

		d, err := autofilter.URLDataFromString(data)
		if err != nil {
			_app.Log.Warn("start: parse sendfile start data failed", zap.Error(err))
			return nil
		}

		f, err := _app.DB.GetFile(d.FileUniqueId)
		if err != nil {
			_app.Log.Warn("start: get file failed", zap.Error(err))
			m.Reply(bot, "<i>📛 I Couldn't Find the File You're Looking for, Please Report This to Admins :/</i>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
			return nil
		}

		// Fetch and send poster if enabled and available
		if _app.Config.GetPosterEnabled() {
			isSeries := autofilter.IsSeriesFile(f.FileName)
			posterUrl := autofilter.GetPosterUrlWithType(f.FileName, isSeries)
			if posterUrl != "" {
				limiter.Wait()
				_, err = bot.SendPhoto(m.Chat.Id, gotgbot.InputFileByURL(posterUrl), &gotgbot.SendPhotoOpts{
					Caption: fmt.Sprintf("<b>🎬 Poster for: %s</b>", autofilter.CleanFileNameForDisplay(f.FileName)),
					ParseMode: gotgbot.ParseModeHTML,
				})
				if err != nil {
					_app.Log.Warn("start: send poster failed", zap.Error(err))
				}
			}
		}

		var (
			warn    string
			delTime = _app.Config.GetFileAutoDelete()
		)
		if delTime != 0 {
			warn = fmt.Sprintf("<blockquote>⚠️ 𝖳𝗁𝗂𝗌 𝖥𝗂𝗅𝖾 𝖶𝗂𝗅𝗅 𝖻𝖾 𝖠𝗎𝗍𝗈𝗆𝖺𝗍𝗂𝖼𝖺𝗅𝗅𝗒 𝖣𝖾𝗅𝖾𝗍𝖾𝖽 𝗂𝗇 %d 𝖬𝗂𝗇𝗎𝗍𝖾𝗌. 𝖥𝗈𝗋𝗐𝖺𝗋𝖽 𝗂𝗍 𝗍𝗈 𝖠𝗇𝗈𝗍𝗁𝖾𝗋 𝖢𝗁𝖺𝗍 𝗈𝗋 𝖲𝖺𝗏𝖾𝖽 𝖬𝖾𝗌𝗌𝖺𝗀𝖾𝗌.</blockquote>", delTime)
		}

		msg, err := f.Send(bot, m.Chat.Id, &model.SendFileOpts{
			Caption: _app.FormatText(ctx, _app.Config.GetFileCaption(), map[string]any{
				"file_size": functions.FileSizeToString(f.FileSize),
				"file_name": autofilter.CleanFileNameForDisplay(f.FileName),
				"warn":      warn,
			}),
			Keyboard:        [][]gotgbot.InlineKeyboardButton{{{Text: "🗑️ ᴅᴇʟᴇᴛᴇ ғɪʟᴇ 🗑️", CallbackData: "close"}}},
			MessageEffectId: "5046509860340391262", // Confetti Effect
		})
		if err != nil {
			_app.Log.Warn("start: send file failed", zap.Error(err), zap.String("file_id", f.FileId))
		}

		if delTime != 0 && msg != nil {
			err = _app.AutoDelete.SaveMessage(msg, time.Minute*time.Duration(delTime))
			if err != nil {
				_app.Log.Warn("start: insert auto delete failed", zap.Error(err))
			}
		}
	case DataPrefixRetry:
		d, err := RetryDataFromString(data)
		if err != nil {
			_app.Log.Warn("start: parse retry data failed", zap.Error(err), zap.String("data", data))
			return nil
		}

		url := fmt.Sprintf("https://t.me/c/%d/%d", functions.ChatIdToMtproto(d.ChatId), d.MessageId)
		text := fmt.Sprintf("<b>𝖯𝗅𝖾𝖺𝗌𝖾 𝖧𝖾𝖺𝖽 𝖡𝖺𝖼𝗄 𝗍𝗈 𝖳𝗁𝖾 𝖢𝗁𝖺𝗍 𝖺𝗇𝖽 𝖳𝗋𝗒 𝖠𝗀𝖺𝗂𝗇 <a href='%s'>» 𝖦𝗈 𝖡𝖺𝖼𝗄</a></b>", url)

		limiter.Wait()
		msg, err := bot.SendMessage(m.Chat.Id, text, &gotgbot.SendMessageOpts{
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{{Text: "« ɢᴏ ʙᴀᴄᴋ", Url: url}}},
			},
			ParseMode: gotgbot.ParseModeHTML,
		})
		if err != nil {
			_app.Log.Warn("start: send retry msg failed", zap.Error(err))
		} else {
			middleware.ReactWithRandomEmoji(bot, msg.Chat.Id, msg.MessageId, _app.Config, _app.Log)
		}
	case DataPrefixBatch:
		ok, err := fsub.CheckFsub(_app, bot, ctx)
		if err != nil {
			_app.Log.Warn("start: check fsub failed", zap.Error(err))
		}

		if !ok {
			return nil
		}

		limiter.Wait()
		pm, _ := m.Reply(bot, "<b>Fetching Media 📥</b>", &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})

		SendBatch(bot, m.Chat.Id, data)

		if pm != nil {
			pm.Delete(bot, nil)
		}
	case DataPrefixSearch:
		msg, err := _autofilter(bot, ctx)
		if err != nil {
			return err
		}

		delTime := _app.Config.GetAutodeleteTime()
		if delTime != 0 {
			if msg != nil {
				_ = _app.AutoDelete.SaveMessage(msg, time.Minute*time.Duration(delTime))
			}
			if ctx.Message != nil {
				_ = _app.AutoDelete.SaveMessage(ctx.Message, time.Minute*time.Duration(delTime))
			}
		}
		return nil
	default:
		// Unknown prefix, treat as referral start
		return StaticCommands(bot, ctx)
	}

	return nil
}
