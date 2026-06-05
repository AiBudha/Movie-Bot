/*
Package configpanel handles the /settings command
*/
package configpanel

import (
	"context"

	"autofilterbot/internal/config"
	"autofilterbot/internal/database/mongo"
	"autofilterbot/pkg/panel"
	"go.uber.org/zap"
)

const (
	OperationDelete  = "del"
	OperationSet     = "set"
	OperationReset   = "reset"
	OperationRefresh = "ref"
)

type AppPreview interface {
	GetContext() context.Context
	GetDB() *mongo.Client
	GetConfig() *config.Config
	GetLog() *zap.Logger
	RefreshConfig()
	GetAdditionalCollectionCount() int
	SetCollectionIndex(index int)
}

// CreatePanel creates the bot's configpanel and adds all pages.
func CreatePanel(app AppPreview) *panel.Panel {
	p := panel.NewPanel()

	p.AddPage(panel.NewPage("sizebtn", "Size Button").WithCallbackFunc(BoolField(app, config.FieldNameSizeButton)))
	p.AddPage(panel.NewPage("poster", "Poster").WithCallbackFunc(BoolField(app, config.FieldNamePosterEnabled)))
	p.AddPage(panel.NewPage("autodel", "Auto Delete").WithCallbackFunc(TimeField(app, config.FieldNameAutodeleteTime, []int{5, 10, 15, 20, 30, 45})))
	p.AddPage(panel.NewPage("filedel", "File AutoDelete").WithCallbackFunc(TimeField(app, config.FieldNameFileAutoDelete, []int{5, 10, 15, 20, 30, 45})))

	p.NewPage("fsub", "Force Sub").WithCallbackFunc(ChannelField(app, config.FieldNameFsub, ChannelFieldOpts{Description: "Force Subcribe Channels are Channels that the User Must Join to get Files.", AllowRequestInvite: true}))

	p.NewPage("moniterd", "Monitored Chans").WithCallbackFunc(MonitoredChannelsField(app))

	p.AddPage(panel.NewPage("reschan", "Results Channel").WithCallbackFunc(ResultsChannelField(app)))

	groupPage := panel.NewPage("group", "Group Defaults").WithContent("👥 Configure default settings for Group Chats.")
	groupPage.NewSubPage("wtext", "Default Welcome Text").WithCallbackFunc(StringField(app, config.FieldNameDefaultWelcomeText, StringFieldOpts{Description: "Default welcome message text for new members. Supports {mention}, {title}, {first_name}, {id}."}))
	groupPage.NewSubPage("wenable", "Default Welcome Status").WithCallbackFunc(BoolField(app, config.FieldNameDefaultWelcomeEnabled, "Toggle if welcome message is enabled by default in new groups.\n\n"))
	p.AddPage(groupPage)


	dbPage := panel.NewPage("db", "Database").WithContent("📂 Configure Database Settings from the Options Below.")
	dbPage.NewSubPage("coll", "File Database").WithCallbackFunc(IntField(app, config.FieldNameCollectionIndex, IntFieldOpts{
		Range:       &IntRange{Start: 0, End: app.GetAdditionalCollectionCount()},
		Description: "Collection/Database to Store Files. 0 is your Main Database.",
		Middleware:  func(val int) { app.SetCollectionIndex(val) },
	}))
	dbPage.NewSubPage("updater", "Auto Collection Updater").WithCallbackFunc(BoolField(
		app,
		config.FieldNameCollectionUpdater,
		"The Auto Collection-Updater periodically runs to change the database used to store files, to the database with least files.\n\nWhen Enabled, the Collection set from Config Panel will bee Ignored\n\nNOTE: Application must be restarted for changes to take effect.\n\n",
	))

	p.AddPage(dbPage)

	resBtnPage := p.NewPage("resbtn", "Result Button")
	resBtnPage.NewSubPage("text", "Button Text").WithCallbackFunc(StringField(app, config.FieldNameResultButtonText, StringFieldOpts{Description: "Text to show on the custom button in search results."}))
	resBtnPage.NewSubPage("url", "Button URL").WithCallbackFunc(StringField(app, config.FieldNameResultButtonUrl, StringFieldOpts{Description: "URL the button should open (e.g., your channel link)."}))

	footerPage := p.NewPage("footer", "Footer Buttons")
	footerPage.NewSubPage("releases", "Latest Releases URL").WithCallbackFunc(StringField(app, config.FieldNameLatestReleasesUrl, StringFieldOpts{Description: "URL for the 'Latest Releases' banner above search results."}))
	footerPage.NewSubPage("movies", "New Movies URL").WithCallbackFunc(StringField(app, config.FieldNameNewMoviesUrl, StringFieldOpts{Description: "URL for the 'New Movies' button in search results."}))
	footerPage.NewSubPage("updates", "Updates URL").WithCallbackFunc(StringField(app, config.FieldNameUpdatesUrl, StringFieldOpts{Description: "URL for the 'Updates' button in search results."}))

	return p
}
