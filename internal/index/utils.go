package index

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/amarnathcjd/gogram/telegram"
	"go.uber.org/zap"
)

const (
	getMessagesLimit = 200
)

// inputMessageSlice generates a slice of message ids upto the next 200 messages to be fetched and moves the pointer.
func (o *Operation) inputMessageSlice() []telegram.InputMessage {
	o.mu.Lock()
	defer o.mu.Unlock()

	ids := make([]telegram.InputMessage, 0)

	for i := 0; i < getMessagesLimit; i++ {
		id := o.CurrentMessageID + int64(i)
		if id > o.EndMessageID {
			break
		}

		ids = append(ids, &telegram.InputMessageID{ID: int32(id)})
	}

	if len(ids) > 0 {
		o.CurrentMessageID += int64(len(ids))
	}

	return ids
}

// ErrorMessage send an error level message to the user. Operation is expected to stop after this message.
// NOTE: The pid field is logged by default, only pass additional fields.
func (o *Operation) ErrorMessage(msg string, fields ...zap.Field) {
	o.log.Error(msg, zap.String("pid", o.ID))
	o.bot.SendMessage(o.ProgressMessageChatID, fmt.Sprintf("🛑 Index Stopped: Unable to Invoke Method: <code>%s</code>", msg), &gotgbot.SendMessageOpts{
		ParseMode:   gotgbot.ParseModeHTML,
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{o.ResumeButton()}}},
	})
}

const (
	// ZeroTDLibChannelID is minimum channel TDLib ID.
	ZeroTDLibChannelID = -1000000000000
)

// TDLibChannelIDToPlain converts a botapi/tdlib channel id to an mtproto one.
// Extracted from github.com/gotd/td/constants
func TDLibChannelIDToPlain(id int64) int64 {
	r := id - ZeroTDLibChannelID
	return -r
}

// regex expression to parse floodwait errors and extract seconds (supports wait [of] X seconds)
var floodRegex = regexp.MustCompile(`(?i)wait(?:\s+of)?\s+(\d+)`)

const (
	FloodwaitErrorRPCString = "FLOOD_WAIT_X"
)

// ParseMtProtoFloodwait parses the error string from a telegram api method and extracts number of seconds.
//
// Returns:
//   - int64: number of seconds
//   - bool: indicates wether given error is a floodwait.
//   - err: error during parsing (not api errors)
func ParseMtProtoFloodwait(err error) (int64, bool, error) {
	if !strings.Contains(err.Error(), FloodwaitErrorRPCString) {
		return 0, false, nil
	}

	matches := floodRegex.FindStringSubmatch(err.Error())
	if len(matches) < 2 {
		return 0, true, fmt.Errorf("no seconds found in the input string")
	}

	seconds, err := strconv.ParseInt(matches[1], 0, 64)
	if err != nil {
		return 0, true, err
	}

	return seconds, true, nil
}
