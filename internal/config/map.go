package config

const (
	FieldNameFsub              = "fsub"
	FieldNameMaxResults        = "max_results"
	FiledNameMaxPages          = "max_pages"
	FieldNameMaxPerPage        = "max_per_page"
	FieldNameStart             = "start"
	FieldNameAbout             = "about"
	FieldNameHelp              = "help"
	FieldNamePrivacy           = "privacy"
	FieldNameStats             = "stats"
	FieldNameMovies            = "movies"
	FieldNameSeries            = "series"
	FieldNameShortener         = "shortener"
	FieldNameNoResultText      = "no_result_text"
	FieldNameResultTemplate    = "af_template"
	FieldNameButtonTemplate    = "btn_template"
	FieldNameFdetailsTemplate  = "fdetails_template"
	FieldNameSizeButton        = "size_btn"
	FieldNamePosterEnabled     = "poster_enabled"
	FieldNameAutodeleteTime    = "autodel_time"
	FieldNameFsubText          = "fsub_text"
	FieldNameFileCaption       = "file_caption"
	FieldNameFileAutoDelete    = "file_autodel"
	FieldNameBatchSize         = "batch_size"
	FieldNameCollectionIndex   = "collection_index"
	FieldNameCollectionUpdater = "collection_updater"
	FieldNameEmojiMode         = "emoji_mode"
	FieldNameReactions         = "reactions"
	FieldNameResultButtonText  = "result_btn_text"
	FieldNameResultButtonUrl   = "result_btn_url"
	FieldNameNewMoviesUrl      = "new_movies_url"
	FieldNameUpdatesUrl        = "updates_url"
	FieldNameLatestReleasesUrl = "latest_releases_url"
	FieldNameResultsChannel    = "results_channel"
	FieldNameResultsChannelID  = "results_channel_id"
	FieldNameFileChannels      = "file_channels"
	FieldNameDefaultWelcomeText    = "def_welcome_text"
	FieldNameDefaultWelcomeEnabled = "def_welcome_enabled"
	FieldNameDefaultLocks          = "def_locks"
)


// ToMap converts the contents of the struct into map so fields can be dynamically accessed.
func (c *Config) ToMap() map[string]any {
	if c.cachedMap == nil {
		c.RefreshMap()
	}

	return c.cachedMap
}

func (c *Config) toMap() map[string]any {
	vals := make(map[string]any)

	vals[FieldNameFsub] = c.GetFsubChannels()
	vals[FieldNameMaxResults] = c.GetMaxResults()
	vals[FiledNameMaxPages] = c.GetMaxPages()
	vals[FieldNameMaxPerPage] = c.GetMaxPerPage()

	// all message values are saved by prefix appended with _text and _buttons for text and markup
	vals[FieldNameStart] = c.GetStartMessage("")
	vals[FieldNameAbout] = c.GetAboutMessage()
	vals[FieldNameHelp] = c.GetHelpMessage()
	vals[FieldNamePrivacy] = c.GetPrivacyMessage()
	vals[FieldNameStats] = c.GetStatsMessage()
	vals[FieldNameMovies] = c.GetMoviesMessage()
	vals[FieldNameSeries] = c.GetSeriesMessage()

	vals[FieldNameShortener] = c.GetShortener()
	vals[FieldNameNoResultText] = c.GetNoResultText()
	vals[FieldNameResultTemplate] = c.GetResultTemplate()
	vals[FieldNameButtonTemplate] = c.GetButtonTemplate()
	vals[FieldNameFdetailsTemplate] = c.GetFileDetailsTemplate()
	vals[FieldNameSizeButton] = c.GetSizeButton()
	vals[FieldNamePosterEnabled] = c.GetPosterEnabled()
	vals[FieldNameAutodeleteTime] = c.GetAutodeleteTime()

	vals[FieldNameFsubText] = c.GetFsubText()
	vals[FieldNameFileCaption] = c.GetFileCaption()
	vals[FieldNameAutodeleteTime] = c.GetAutodeleteTime()

	vals[FieldNameBatchSize] = c.GetBatchSizeLimit()

	vals[FieldNameCollectionIndex] = c.GetFileCollectionIndex()
	vals[FieldNameCollectionUpdater] = c.GetFileCollectiionUpdater()

	vals[FieldNameEmojiMode] = c.EmojiMode
	vals[FieldNameReactions] = c.Reactions

	vals[FieldNameResultButtonText] = c.ResultButtonText
	vals[FieldNameResultButtonUrl] = c.ResultButtonUrl
	vals[FieldNameNewMoviesUrl] = c.NewMoviesUrl
	vals[FieldNameUpdatesUrl] = c.UpdatesUrl
	vals[FieldNameLatestReleasesUrl] = c.LatestReleasesUrl
	vals[FieldNameResultsChannel] = c.ResultsChannel
	vals[FieldNameResultsChannelID] = c.ResultsChannelID
	vals[FieldNameFileChannels] = c.FileChannels
	vals[FieldNameDefaultWelcomeText] = c.GetDefaultWelcomeText()
	vals[FieldNameDefaultWelcomeEnabled] = c.GetDefaultWelcomeEnabled()
	vals[FieldNameDefaultLocks] = c.GetDefaultLocks()

	return vals

}

// RefreshMap refreshes the value of the cached map.
func (c *Config) RefreshMap() {
	c.cachedMap = c.toMap()
}
