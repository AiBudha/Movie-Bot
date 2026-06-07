package config

import (
	"fmt"

	"autofilterbot/internal/button"
	"autofilterbot/internal/model/message"
)

// GetStartMessage returns the custom start message if available or the default values.
// botUsername must be provided to create the add to group button.
func (c *Config) GetStartMessage(botUsername string) *message.Message {
	var (
		text    string
		buttons [][]button.InlineKeyboardButton
	)

	addToGroupUrl := fmt.Sprintf("https://t.me/%s?startgroup&admin=delete_messages+pin_messages+invite_users+ban_users+promote_members", botUsername)

	if c.StartText != "" {
		text = c.StartText
	} else {
		text = `<b>Welcome to MovieBot Pro 🎬</b>

Hi {mention}, I am your ultimate companion for high-quality movies and series. 

<blockquote><b>🚀 Fast & Direct</b>
Type the exact title to get instant direct links. No redirects, no ads, no complexity.</blockquote>

<b>✨ Key Features</b>
• ⚡️ <b>Ultra-fast search</b> across 100k+ files
• 📂 <b>Direct download</b> links
• 🍿 <b>Crystal clear</b> quality options
• 🛡 <b>Safe & Ad-free</b> experience

<i>Join our community to stay updated with latest OTT releases and daily additions!</i>`
	}

	if len(c.StartButtons) != 0 {
		buttons = c.StartButtons
	} else {
		buttons = [][]button.InlineKeyboardButton{
			{{Text: "🔰 Add me to your Group 🔰", Url: addToGroupUrl, Style: "primary"}},
			{
				{Text: "Help 📢", CallbackData: "cmd:help", Style: "primary"},
				{Text: "About 📖", CallbackData: "cmd:about", Style: "primary"},
			},
			{{Text: "Top Searching ⭐", CallbackData: "cmd:top", Style: "primary"}},
		}
	}

	return &message.Message{
		Text:     text,
		Keyboard: buttons,
	}
}

func (c *Config) GetAboutMessage() *message.Message {
	var (
		text    string
		buttons [][]button.InlineKeyboardButton
	)

	if c.AboutText != "" {
		text = c.AboutText
	} else {
		text = `⌬ SYSTEM STATUS
╭ CPU  : {cpu}
┊ RAM  : {ram}
┊ Bot RAM : {bot_ram}
┊ FREE : {free}
┊ Language : {go_version}
┊ os    : {os}
┊ version : {go_version}
┊ Latency      : {latency}
┊ database     : {database}
╰ UP   : {uptime}`
	}

	if len(c.AboutButtons) != 0 {
		buttons = c.AboutButtons
	} else {
		buttons = [][]button.InlineKeyboardButton{
			{{Text: "« ʙᴀᴄᴋ", CallbackData: "cmd:start", Style: "primary"}, {Text: "sᴛᴀᴛs", CallbackData: "cmd:stats", Style: "primary"}},
		}
	}

	return &message.Message{
		Text:     text,
		Keyboard: buttons,
	}
}

func (c *Config) GetHelpMessage() *message.Message {
	var (
		text    string
		buttons [][]button.InlineKeyboardButton
	)

	if c.HelpText != "" {
		text = c.HelpText
	} else {
		text = `⌬ HELP
╭ USER COMMANDS
┊ /start      - check if I'm alive
┊ /about      - learn a bit about me
┊ /help       - get this message
┊ /movies     - movie request format/instructions
┊ /series     - series request format/instructions
┊ /stats      - some number crushing
┊ /privacy    - what data I steal
┊ /uinfo      - get user data stored
┊ /id         - if you know u know
┊ /formatting - formatting guide
╰ END
╭ OTT RELEASE COMMANDS
┊ /latest      - browse latest movie & show releases
┊ /subscribe   - subscribe to release updates in PM
┊ /unsubscribe - unsubscribe from PM release updates
┊ /platforms   - view list of supported OTT platforms
╰ END
╭ EXCLUSIVE COMMANDS
┊ /broadcast - spam users with ads
┊ /batch     - bunch up messages
┊ /genlink   - link to single file
┊ /index     - gather up files
┊ /delete    - assassinate a file
┊ /deleteall - massacre matching files
┊ /sendnow   - check for releases manually
╰ END`
	}

	if len(c.HelpButtons) != 0 {
		buttons = c.HelpButtons
	} else {
		buttons = [][]button.InlineKeyboardButton{
			{{Text: "🎥 Movie Format", CallbackData: "cmd:movies", Style: "primary"}, {Text: "📺 Series Format", CallbackData: "cmd:series", Style: "primary"}},
			{{Text: "🛡️ Group Help", CallbackData: "cmd:ghelp", Style: "primary"}, {Text: "ᴘʀɪᴠᴀᴄʏ", CallbackData: "cmd:privacy", Style: "primary"}},
			{{Text: "« ʙᴀᴄᴋ", CallbackData: "cmd:start", Style: "primary"}},
		}
	}

	return &message.Message{
		Text:     text,
		Keyboard: buttons,
	}
}

func (c *Config) GetGroupHelpMessage() *message.Message {
	text := `⌬ GROUP HELP
╭ CONFIGURATION & CONNECTION
┊ /gsettings   - Open group settings panel (welcome & content locks)
┊ /connect     - Connect group to PM for private search
┊ /disconnect  - Disconnect group from PM
┊ /setwelcome &lt;text&gt; - Set custom welcome message
┊ /clearwelcome - Disable welcome message
┊ /setrules &lt;text&gt; - Set group rules
┊ /clearrules - Delete group rules
┊ /rules - View group rules
┊ /locks - View lock status of content types
┊ /lock &lt;type&gt; - Lock a content type (stickers, gifs, media, forwards, links, service)
┊ /unlock &lt;type&gt; - Unlock a content type
┊ /setflood &lt;num&gt; - Set flood control limit
┊ /captcha &lt;on/off&gt; - Enable/disable CAPTCHA
┊ /captchatime &lt;seconds&gt; - Set CAPTCHA timeout
┊ /antiraid &lt;on/off&gt; - Enable/disable raid protection
┊ /msgstats - View group statistics
╰ END
╭ MODERATION
┊ /ban /unban - Ban or unban a user
┊ /sban /dban /tban - Silent, Delete, or Temp ban
┊ /kick /dkick /kickme - Kick, Delete kick, or self‑kick
┊ /mute /unmute - Mute or unmute a user
┊ /smute /dmute /tmute - Silent, Delete, or Temp mute
┊ /warn /unwarn - Warn or reset warnings for a user
┊ /dwarn /swarn /rmwarn - Delete warn, Silent warn, or reset warnings
┊ /setwarnlimit /setwarnmode - Configure warn settings
┊ /pin /unpin - Pin or unpin group messages
╰ END
╭ ADMINISTRATION
┊ /promote /demote - Manage admin status
┊ /title &lt;text&gt; - Set custom admin title
┊ /adminlist - List chat admins
╰ END`

	buttons := [][]button.InlineKeyboardButton{
		{{Text: "« Back to Help", CallbackData: "cmd:help", Style: "primary"}, button.CloseLocal()},
	}

	return &message.Message{
		Text:     text,
		Keyboard: buttons,
	}
}

func (c *Config) GetMoviesMessage() *message.Message {
	var (
		text    string
		buttons [][]button.InlineKeyboardButton
	)

	if c.MoviesText != "" {
		text = c.MoviesText
	} else {
		text = `⚠️❗️ <b>𝗠𝗼𝘃𝗶𝗲 𝗥𝗲𝗾𝘂𝗲𝘀𝘁 𝗙𝗼𝗿𝗺𝗮𝘁</b> ❗️⚠️

📝 𝖬𝗈𝗏𝗂𝖾 𝖭𝖺𝗆𝖾, 𝖸𝖾𝖺𝗋,(𝖨𝖿 𝗒𝗈𝗎 𝖪𝗇𝗈𝗐) 𝖶𝗂𝗍𝗁 𝖢𝗈𝗋𝗋𝖾𝖼𝗍 𝖲𝗉𝖾𝗅𝗅𝗂𝗇𝗀 📚

🗣 𝖨𝖿 𝖨𝗍 𝗂𝗌 𝖺 𝖥𝗂𝗅𝗆 𝖲𝖾𝗋𝗂𝖾𝗌. 𝖱𝖾𝗊𝗎𝖾𝗌𝗍 𝖮𝗇𝖾 𝖡𝗒 𝖮𝗇𝖾 𝖶𝗂𝗍𝗁 𝖯𝗋𝗈𝗉𝖾𝗋 𝖭𝖺𝗆𝖾 🧠

🖇<b>𝐄𝐱𝐚𝐦𝐩𝐥𝐞:</b>
• <code>Robin Hood</code> ✅
• <code>Robin Hood 2010</code> ✅
• <code>Kurup 2021 Kan</code> ✅ 
• <code>Harry Potter and the Philosophers Stone</code> ✅
• <code>Harry Potter and the Prisoner of Azkaban</code> ✅

🥱 𝖥𝗈𝗋 𝖫𝖺𝗇𝗀𝗎𝖺𝗀𝖾 𝖠𝗎𝖽𝗂𝗈𝗌 - 𝖪𝖺𝗇 𝖿𝗈𝗋 𝖪𝖺𝗇𝗇𝖺𝖽𝖺, 𝖬𝖺𝗅 - 𝖬𝖺𝗅𝖺𝗒𝖺𝗅𝖺𝗆, 𝖳𝖺𝗆 - 𝖳𝖺𝗆𝗂𝗅

🔎 𝖴𝗌𝖾 𝖥𝗂𝗋𝗌𝗍 3 𝖫𝖾𝗍𝗍𝖾𝗋𝗌 𝖮𝖿 𝖫𝖺𝗇𝗀𝗎𝖺𝗀𝖾 𝖥𝗈𝗋 𝖠𝗎𝖽𝗂𝗈𝗌 [𝖪𝖺𝗇 𝖳𝖺𝗆 𝖳𝖾𝗅 𝖬𝖺𝗅 𝖧𝗂𝗇 𝖲𝗉𝖺 𝖤𝗇𝗀 𝖪𝗈𝗋 𝖾𝗍𝖼...]

❌ <b>[Don't Use words Like Dubbed/Movies/Send/HD , . : - etc]</b> ❌`
	}

	if len(c.MoviesButtons) != 0 {
		buttons = c.MoviesButtons
	} else {
		buttons = [][]button.InlineKeyboardButton{
			{{Text: "« ʙᴀᴄᴋ", CallbackData: "cmd:help", Style: "primary"}, button.CloseLocal()},
		}
	}

	return &message.Message{
		Text:     text,
		Keyboard: buttons,
	}
}

func (c *Config) GetSeriesMessage() *message.Message {
	var (
		text    string
		buttons [][]button.InlineKeyboardButton
	)

	if c.SeriesText != "" {
		text = c.SeriesText
	} else {
		text = `⚠️❗️ <b>𝗦𝗲𝗿𝗶𝗲𝘀 𝗥𝗲𝗾𝘂𝗲𝘀𝘁 𝗙𝗼𝗿𝗺𝗮𝘁</b> ❗️⚠️

🗣 𝖲𝖾𝗋𝗂𝖾𝗌 𝖭𝖺𝗆𝖾,𝖲𝖾𝖺𝗌𝗈𝗇 (𝖶𝗁𝗂𝖼𝗁 𝖲𝖾𝖺𝗌𝗈𝗇 𝗒𝗈𝗎 𝗐𝖺𝗇𝗍) 🧠

🖇<b>Example:</b> 
• <code>Game Of Thrones S03E02 720p</code> ✅
• <code>Sex Education S02 720p</code> ✅ 
• <code>Breaking Bad S01E05</code> ✅ 
• <code>Prison Break 1080p</code> ✅ 
• <code>Witcher S02</code> ✅

🥱 𝖥𝗈𝗋 𝖲𝖾𝖺𝗌𝗈𝗇 𝖬𝖾𝗇𝗍𝗂𝗈𝗇 𝖠𝗌 𝖲01 𝖥𝗈𝗋 𝖲𝖾𝖺𝗌𝗈𝗇 1, 𝖲02 𝖥𝗈𝗋 𝖲𝖾𝖺𝗌𝗈𝗇 2 𝖾𝗍𝖼 [𝖲03,𝖲04,𝖲06,𝖲10,𝖲17] 𝖦𝗈𝖾𝗌 𝖫𝗂𝗄𝖾 𝖳𝗁𝖺𝗍

🔎 𝖥𝗈𝗋 𝖤𝗉𝗂𝗌𝗈𝖽𝖾 𝖬𝖾𝗇𝗍𝗂𝗈𝗇 𝖠𝗌 𝖤𝗉01 𝖥𝗈𝗋 𝖤𝗉𝗂𝗌𝗈𝖽𝖾 1, 𝖤𝗉02 𝖥𝗈𝗋 𝖤𝗉𝗂𝗌𝗈𝖽𝖾 2 𝖾𝗍𝖼 [𝖤𝗉03,𝖤𝗉07,𝖤𝗉17,𝖤𝗉21] 𝖦𝗈'𝗌 𝖫𝗂𝗄𝖾 𝖳𝗁𝖺𝗍 

❌ <b>[Don't Use words Like Season/Episode/Series , . : - etc]</b> ❌`
	}

	if len(c.SeriesButtons) != 0 {
		buttons = c.SeriesButtons
	} else {
		buttons = [][]button.InlineKeyboardButton{
			{{Text: "« ʙᴀᴄᴋ", CallbackData: "cmd:help", Style: "primary"}, button.CloseLocal()},
		}
	}

	return &message.Message{
		Text:     text,
		Keyboard: buttons,
	}
}

func (c *Config) GetStatsMessage() *message.Message {
	var (
		text    string
		buttons [][]button.InlineKeyboardButton
	)

	if c.StatsText != "" {
		text = c.StatsText
	} else {
		text = `
╭ ▸ 𝖴𝗌𝖾𝗋𝗌 : <code>{users}</code>
├ ▸ 𝖥𝗂𝗅𝖾𝗌 : <code>{files}</code>
├ ▸ 𝖦𝗋𝗈𝗎𝗉𝗌 : <code>{groups}</code>
╰ ▸ 𝖴𝗉𝗍𝗂𝗆𝖾 : <code>{uptime}</code>
`
	}

	if len(c.StatsButtons) != 0 {
		buttons = c.StatsButtons
	} else {
		buttons = [][]button.InlineKeyboardButton{
			{{Text: "« ʙᴀᴄᴋ", CallbackData: "cmd:about", Style: "primary"}, button.CloseLocal()},
		}
	}

	return &message.Message{
		Text:     text,
		Keyboard: buttons,
	}
}

func (c *Config) GetPrivacyMessage() *message.Message {
	var (
		text    string
		buttons [][]button.InlineKeyboardButton
	)

	if c.PrivacyText != "" {
		text = c.PrivacyText
	} else {
    text = `
<blockquote>
<b>Monetary</b>
    This bot is a free and self-coded side project and is independent from any other creator or developer.
    If anyone offers services like these for money please do not accept it.

<b>Legal</b>
     If you feel that this bot violates any copyrights or other rules please report it at the Support Group and we will be more than happy to solve the problem.
     The bot is almost completely community driven so the files available in the bot are uploaded by users of the bot or randomly scraped from Telegram channels.

<b>Note</b>
     If you have any inconvenience regarding the activities of the bot please let us know.

</blockquote>`
	}

	if len(c.PrivacyButtons) != 0 {
		buttons = c.PrivacyButtons
	} else {
		buttons = [][]button.InlineKeyboardButton{
			{{Text: "« ʙᴀᴄᴋ", CallbackData: "cmd:help", Style: "primary"}, button.CloseLocal()},
		}
	}

	return &message.Message{
		Text:     text,
		Keyboard: buttons,
	}
}
func (c *Config) GetCopyrightMessage() *message.Message {
	var (
		text    string
		buttons [][]button.InlineKeyboardButton
	)

	if c.CopyrightText != "" {
		text = c.CopyrightText
	} else {
		text = `
<blockquote expandable><b>Copyright Policy ©️</b>
<i>This bot does not host any copyrighted content itself. All files are sourced from publicly available links shared by users. If you believe any content infringes your rights, please contact the bot maintainer to request removal.</i>
</blockquote>`
	}

	if len(c.CopyrightButtons) != 0 {
		buttons = c.CopyrightButtons
	} else {
		buttons = [][]button.InlineKeyboardButton{{{Text: "« Back", CallbackData: "cmd:help", Style: "primary"}, button.CloseLocal()}}
	}

	return &message.Message{Text: text, Keyboard: buttons}
}

