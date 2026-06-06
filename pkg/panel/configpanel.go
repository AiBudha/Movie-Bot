/*
Package panel creates a modular panel to
*/
package panel

import (
	"fmt"

	"autofilterbot/internal/button"
	"autofilterbot/pkg/callbackdata"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

const (
	PathHome = "home"
)

// ContentGenerator is a function that returns a string which will be used as the text content for a message.
// The text should use html styling.
type ContentGenerator func() string

// Panel is the entrypoint to the configpanel to which pages can be added. Use the NewPanel function to create a new panel.
type Panel struct {
	// List of pages.
	Pages []*Page
	// Function to generate the text content for the homepage.
	HomepageGenerator ContentGenerator
}

// NewPanel intializes a new empty config panel.
func NewPanel() *Panel {
	return &Panel{
		Pages: make([]*Page, 0, 5),
	}
}

// WithHomepageGenerator sets the HomepageGenerator field.
func (p *Panel) WithHomepageGenerator(g ContentGenerator) *Panel {
	p.HomepageGenerator = g
	return p
}

// AddPage adds a new page to the root panel.
func (p *Panel) AddPage(page *Page) *Page {
	p.Pages = append(p.Pages, page)
	return page
}

// NewPage creates a new empty page with given name used in callback data and displayName showed onn the button.
func (p *Panel) NewPage(name, displayName string) *Page {
	newPage := &Page{
		Name:        name,
		DisplayName: displayName,
	}
	p.Pages = append(p.Pages, newPage)

	return newPage
}

// HandleUpdate processes the update and then edits the message's content with result.
func (p *Panel) HandleUpdate(ctx *ext.Context, bot *gotgbot.Bot) error {
	update := ctx.CallbackQuery
	_, _ = update.Answer(bot, nil)

	content, markup, err := ProcessUpdate(p, ctx, bot)
	if err != nil {
		content = fmt.Sprintf("An error occurred while handling request: %s", err.Error())
	}

	if len(markup) == 0 {
		markup = [][]gotgbot.InlineKeyboardButton{{button.Close()}}
	}

	if len(markup) > 0 {
		lastRowIdx := len(markup) - 1
		if len(markup[lastRowIdx]) == 1 && (markup[lastRowIdx][0].CallbackData == "close" || markup[lastRowIdx][0].Text == "🗑️ Close") {
			markup[lastRowIdx] = []gotgbot.InlineKeyboardButton{
				{Text: "🔙 Back", CallbackData: "admin:back"},
				{Text: "Close ❌", CallbackData: "admin:close"},
			}
		}
	}

	_, _, err = update.Message.EditText(bot, content, &gotgbot.EditMessageTextOpts{
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: markup},
		ParseMode:   gotgbot.ParseModeHTML,
	})

	return err
}

// ProcessUpdate processes the update and returns the text and buttons to edit the message with.
// Returns a [PageNotFoundError] if page was not found.
func ProcessUpdate(p *Panel, update *ext.Context, bot *gotgbot.Bot) (string, [][]gotgbot.InlineKeyboardButton, error) {
	data := callbackdata.FromString(update.CallbackQuery.Data)

	ctx := &Context{
		Bot:           bot,
		Update:        update,
		CallbackQuery: update.CallbackQuery,
		CallbackData:  &data,
	}

	if len(data.Path) < 2 || data.Path[1] == PathHome {
		var content string

		if p.HomepageGenerator != nil {
			content = p.HomepageGenerator()
		} else {
			content = "<b>Welcome</b> to your config panel 👋\n\n🤖 Use the buttons below to navigate and customize your bot 👇"
		}

		return content, buttonsFromPages(ctx.CallbackData, p.Pages), nil
	}

	rootPage, ok := findPage(p.Pages, data.Path[1])
	if !ok {
		return "", nil, PageNotFoundError{PageName: data.Path[1]}
	}

	currentPage := rootPage

	if len(data.Path) > 2 { // if data has subroutes
		for _, subRoute := range data.Path[2:] {
			nextPage, ok := findPage(currentPage.SubPages, subRoute)
			if !ok {
				return "", nil, PageNotFoundError{PageName: subRoute}
			}

			currentPage = nextPage
		}
	}

	// If page has no subpages call CallbackFunc and return result
	// Otherwise return page content and generated markup

	if len(currentPage.SubPages) == 0 && currentPage.CallbackFunc != nil {
		ctx.Page = currentPage
		s, b, err := currentPage.CallbackFunc(ctx)

		return s, addBackOrCloseButton(b, data.BackOrCloseButton()), err
	}

	return currentPage.GetContent(), buttonsFromPages(ctx.CallbackData, currentPage.SubPages), nil
}
