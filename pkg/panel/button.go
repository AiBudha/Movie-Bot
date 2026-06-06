package panel

import (
	"autofilterbot/pkg/callbackdata"
	"github.com/PaulSonOfLars/gotgbot/v2"
)

// buttonsFromPages creates keyboard button with subpages from given CallbackData.
func buttonsFromPages(callbackData *callbackdata.CallbackData, pages []*Page) [][]gotgbot.InlineKeyboardButton {
	var backRow []gotgbot.InlineKeyboardButton

	if len(callbackData.Path) <= 1 {
		// root page so add back button to admin panel
		backRow = []gotgbot.InlineKeyboardButton{backButton("admin:back")}
	} else {
		backPath := callbackData.RemoveLastPath().ToString()
		if backPath == "config" || backPath == "config:home" {
			backPath = "admin:back"
		}
		// nested page so add back button
		backRow = []gotgbot.InlineKeyboardButton{backButton(backPath)}
	}

	if len(pages) == 0 {
		return [][]gotgbot.InlineKeyboardButton{backRow}
	}

	// determine the number of buttons per row (2 by default, 3 if divisible by 3)
	buttonsPerRow := 2
	if len(pages)%3 == 0 {
		buttonsPerRow = 3
	}

	totalButtons := make([][]gotgbot.InlineKeyboardButton, 0, (len(pages)+buttonsPerRow-1)/buttonsPerRow)

	for i := 0; i < len(pages); i += buttonsPerRow {
		end := i + buttonsPerRow
		if end > len(pages) {
			end = len(pages)
		}

		row := make([]gotgbot.InlineKeyboardButton, 0, end-i)
		for _, page := range pages[i:end] {
			row = append(row, gotgbot.InlineKeyboardButton{
				Text:         page.DisplayName,
				CallbackData: callbackData.AddPath(page.Name).ToString(),
				Style:        "primary",
			})
		}

		totalButtons = append(totalButtons, row)
	}

	totalButtons = append(totalButtons, backRow)

	return totalButtons
}

// backButton creates a button with back text with given callback data.
func backButton(callbackData string) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{
		Text:         "‹- ʙᴀᴄᴋ",
		CallbackData: callbackData,
		Style:        "primary",
	}
}

// addBackOrCloseButton dynamically adds the back or close button to an existing keyboard based on the number of buttons in the last row.
func addBackOrCloseButton(btns [][]gotgbot.InlineKeyboardButton, b gotgbot.InlineKeyboardButton) [][]gotgbot.InlineKeyboardButton {
	if len(btns) == 0 {
		return [][]gotgbot.InlineKeyboardButton{{b}}
	}

	lastRowIndex := len(btns) - 1
	switch len(btns[lastRowIndex]) {
	case 1:
		btns[lastRowIndex] = append([]gotgbot.InlineKeyboardButton{b}, btns[lastRowIndex]...)
	default:
		btns = append(btns, []gotgbot.InlineKeyboardButton{b})
	}

	return btns
}
