// Package config contains types for the bot's global configuration.
package config

import (
	"os"
	"strconv"

	"autofilterbot/internal/button"
	"autofilterbot/internal/model"
	"autofilterbot/pkg/env"
	"autofilterbot/pkg/shortener"
)

// Config contains custom values saved for the bot using the config panel.
type Config struct {
	BotId int64 `json:"_id" bson:"_id" `

	// Autofilter result settings

	MaxResults int `json:"max_results,omitempty" bson:"max_results,omitempty"`
	MaxPerPage int `json:"max_per_page,omitempty" bson:"max_per_page,omitempty"`
	MaxPages   int `json:"max_pages,omitempty" bson:"max_pages,omitempty"`

	// Custom Start Message
	StartText    string                          `json:"start_text,omitempty" bson:"start_text,omitempty"`
	StartButtons [][]button.InlineKeyboardButton `json:"start_buttons,omitempty" bson:"start_buttons,omitempty"`
	// Custom About Message
	AboutText    string                          `json:"about_text,omitempty" bson:"about_text,omitempty"`
	AboutButtons [][]button.InlineKeyboardButton `json:"about_buttons,omitempty" bson:"about_buttons,omitempty"`
	// Custom Help Message
	HelpText    string                          `json:"help_text,omitempty" bson:"help_text,omitempty"`
	HelpButtons [][]button.InlineKeyboardButton `json:"help_buttons,omitempty" bson:"help_buttons,omitempty"`
	// Custom Stats Message
	StatsText    string                          `json:"stats_text,omitempty" bson:"stats_text,omitempty"`
	StatsButtons [][]button.InlineKeyboardButton `json:"stats_buttons,omitempty" bson:"stats_buttons,omitempty"`
	// Custom Privacy Message
	PrivacyText    string                          `json:"privacy_text,omitempty" bson:"privacy_text,omitempty"`
	PrivacyButtons [][]button.InlineKeyboardButton `json:"privacy_buttons,omitempty" bson:"privacy_buttons,omitempty"`
	// Custom Copyright Message
	CopyrightText    string                          `json:"copyright_text,omitempty" bson:"copyright_text,omitempty"`
	CopyrightButtons [][]button.InlineKeyboardButton `json:"copyright_buttons,omitempty" bson:"copyright_buttons,omitempty"`
	// Custom Movies Message
	MoviesText     string                          `json:"movies_text,omitempty" bson:"movies_text,omitempty"`
	MoviesButtons  [][]button.InlineKeyboardButton `json:"movies_buttons,omitempty" bson:"movies_buttons,omitempty"`
	// Poster feature toggle
	PosterEnabled bool `json:"poster_enabled,omitempty" bson:"poster_enabled,omitempty"`
	// Custom Series Message
	SeriesText     string                          `json:"series_text,omitempty" bson:"series_text,omitempty"`
	SeriesButtons  [][]button.InlineKeyboardButton `json:"series_buttons,omitempty" bson:"series_buttons,omitempty"`

	// Force Subscribe Channels.
	FsubChannels []model.Channel `json:"fsub,omitempty" bson:"fsub,omitempty"`
	// Fsub message text.
	FsubText string `json:"fsub_text,omitempty" bson:"fsub_text,omitempty"`
	// Html formatted file caption.
	FileCaption string `json:"file_caption,omitempty" bson:"file_caption,omitempty"`
	// File autodelete time in minutes.
	FileAutoDelete int `json:"file_autodel,omitempty" bson:"file_autodel,omitempty"`

	// Template to use for autofilter result message
	ResultTemplate string `json:"af_template,omitempty" bson:"af_template,omitempty"`
	// Message sent when no results are available.
	NoResultText string `json:"no_result_text,omitempty" bson:"no_result_text,omitempty"`
	// Template to use for result buttons
	ButtonTemplate string `json:"btn_template,omitempty" bson:"btn_template,omitempty"`
	// File Details Calbback Template.
	FileDetailsTemplate string `json:"fdetails_template,omitempty" bson:"fdetails_template,omitempty"`

	// Maximum number of message in a single batch.
	BatchSizeLimit int64 `json:"batch_size,omitempty" bson:"batch_size,omitempty"`

	// File size is shown in separate button if set
	SizeButton bool `json:"size_btn,omitempty" bson:"size_btn,omitempty"`

	Shortener shortener.Shortener `json:"shortener,omitempty" bson:"shortener,omitempty"`

	// Time in minutes after which result message should be deleted.
	AutodeleteTime int `json:"autodel_time,omitempty" bson:"autodel_time,omitempty"`

	// Index of collection to use to store files.
	FileCollectionIndex int `json:"collection_index,omitempty" bson:"collection_index,omitempty"`
	// Indicates wether the updater should be run to update file collection periodically.
	FileCollectionUpdater bool `json:"collection_updater,omitempty" bson:"collection_updater,omitempty"`

	// Reactions settings
	EmojiMode bool     `json:"emoji_mode,omitempty" bson:"emoji_mode,omitempty"`
	Reactions []string `json:"reactions,omitempty" bson:"reactions,omitempty"`

	// Global Search Statistics
	LangStats map[string]int64 `json:"lang_stats,omitempty" bson:"lang_stats,omitempty"`

	// Custom button for search results
	ResultButtonText string `json:"result_btn_text,omitempty" bson:"result_btn_text,omitempty"`
	ResultButtonUrl  string `json:"result_btn_url,omitempty" bson:"result_btn_url,omitempty"`

	// Custom URLs for footer buttons
	NewMoviesUrl      string `json:"new_movies_url,omitempty" bson:"new_movies_url,omitempty"`
	UpdatesUrl        string `json:"updates_url,omitempty" bson:"updates_url,omitempty"`
	LatestReleasesUrl string `json:"latest_releases_url,omitempty" bson:"latest_releases_url,omitempty"`

	// Results Channel
	ResultsChannel   string `json:"results_channel,omitempty" bson:"results_channel,omitempty"`
	ResultsChannelID int64  `json:"results_channel_id,omitempty" bson:"results_channel_id,omitempty"`

	// Monitored/Index Channels
	FileChannels []int64 `json:"file_channels,omitempty" bson:"file_channels,omitempty"`

	// Default Group Settings
	DefaultWelcomeText    string          `json:"def_welcome_text,omitempty" bson:"def_welcome_text,omitempty"`
	DefaultWelcomeEnabled bool            `json:"def_welcome_enabled" bson:"def_welcome_enabled"`
	DefaultLocks          map[string]bool `json:"def_locks,omitempty" bson:"def_locks,omitempty"`

	// cached value from ToMap, updated using UpdateMap
	cachedMap map[string]any
}

func (c *Config) GetDefaultWelcomeText() string {
	if c.DefaultWelcomeText != "" {
		return c.DefaultWelcomeText
	}
	return "Welcome {mention} to {title}!"
}

func (c *Config) GetDefaultWelcomeEnabled() bool {
	return c.DefaultWelcomeEnabled
}

func (c *Config) GetDefaultLocks() map[string]bool {
	if c.DefaultLocks == nil {
		return make(map[string]bool)
	}
	return c.DefaultLocks
}


func (c *Config) GetShortener() shortener.Shortener {
	return c.Shortener
}

func (c *Config) GetAutodeleteTime() int {
	if s := os.Getenv("AUTO_DELETE_TIME"); s != "" {
		if i, err := strconv.Atoi(s); err == nil {
			return i
		}
	}
	return c.AutodeleteTime
}

func (c *Config) GetFileDetailsTemplate() string {
	if c.FileDetailsTemplate != "" {
		return c.FileDetailsTemplate
	}

	return `Name: {file_name}
Size: {file_size}
Type: {file_type}
Uploaded: {date}`
}

// GetPosterEnabled returns whether poster sending is enabled (default true)
func (c *Config) GetPosterEnabled() bool {
    if s := os.Getenv("POSTER_ENABLED"); s != "" {
        return s == "true" || s == "1" || s == "ON"
    }
    // Return the stored config value (default true should be set during config initialization)
    return c.PosterEnabled
}

func (c *Config) GetFsubChannels() []model.Channel {
	return c.FsubChannels
}

func (c *Config) GetFsubText() string {
	if c.FsubText != "" {
		return c.FsubText
	}

	return `<i><b>👋 Hᴇʏ ᴛʜᴇʀᴇ {mention}</b></i>
<i>Pʟᴇᴀsᴇ ᴊᴏɪɴ ᴍʏ ᴄʜᴀɴɴᴇʟs ғɪʀsᴛ ᴛᴏ ɢᴇᴛ ʏᴏᴜʀ ғɪʟᴇ</i>

<i>Cʟɪᴄᴋ ᴛʜᴇ <b>JOIN</b> ʙᴜᴛᴛᴏɴ ʙᴇʟᴏᴡ ᴀɴᴅ ᴛʜᴇɴ <b>RETRY</b> ᴛᴏ ɢᴇᴛ ʏᴏᴜʀ ғɪʟᴇ 👇</i>`
}

func (c *Config) GetFileCaption() string {
	if c.FileCaption != "" {
		return c.FileCaption
	}

	return "<i>{file_name}</i>\n\n<b>📂 File Size</b>: <code>{file_size}</code>\n{warn}"
}

func (c *Config) GetFileAutoDelete() int {
	if s := os.Getenv("FILE_AUTO_DELETE"); s != "" {
		if i, err := strconv.Atoi(s); err == nil {
			return i
		}
	}
	return c.FileAutoDelete
}

func (c *Config) GetBatchSizeLimit() int64 {
	if c.BatchSizeLimit != 0 {
		return c.BatchSizeLimit
	}

	return 50
}

func (c *Config) GetFileCollectionIndex() int {
	return c.FileCollectionIndex
}

func (c *Config) GetFileCollectiionUpdater() bool {
	return c.FileCollectionUpdater
}

func (c *Config) GetEmojiMode() bool {
	if s := os.Getenv("EMOJI_MODE"); s != "" {
		return s == "true" || s == "1" || s == "ON"
	}
	return c.EmojiMode
}

func (c *Config) GetReactions() []string {
	if s := env.Strings("REACTIONS"); len(s) != 0 {
		return s
	}
	return c.Reactions
}

func (c *Config) GetResultsChannel() string {
	return c.ResultsChannel
}

func (c *Config) GetResultsChannelID() int64 {
	return c.ResultsChannelID
}
