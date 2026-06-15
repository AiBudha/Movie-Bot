// Package database holds interfaces and types used for database storage.
package database

import (
	"autofilterbot/internal/config"
	"autofilterbot/internal/model"
)

const (
	CollectionNameUsers        = "Users"
	CollectionNameFiles        = "Files"
	CollectionNameConfigs      = "Configs"
	CollectionNameOperations   = "Operations"
	CollectionNameGroups       = "Groups"
	CollectionNameJoinRequests = "JoinRequests"
	CollectionNameJoinRequestsLogs = "JoinRequestsLogs"
	CollectionNameBroadcasts   = "Broadcasts"
	CollectionNameIndexedChannels = "IndexedChannels"
	CollectionNameOTTSubscribers  = "subscribers"
	CollectionNameOTTSentItems    = "sent_items"
	CollectionNameAutoPosts       = "AutoPosts"

	DefaultDatabaseName = "AutoFilterBot"
)

// DEPRECATED
type Database interface {
	// Shutdown gracefully closes the database.
	Shutdown() error

	// SaveUser saves the id of a user to the database if it does not exist.
	SaveUser(userId int64) error
	// SaveUserExtended saves user with additional metadata like source and DC.
	SaveUserExtended(userId int64, source string, dc int, lang string) error
	SetUserLastAction(userId int64, action string) error
	GetUserLastAction(userId int64) (string, error)
	// GetUser gets a user from the database using their id.
	GetUser(userId int64) (*model.User, error)
	// DeleteUser deletes a user from the database. This could be because the user has blocked the bot.
	DeleteUser(userId int64) error
	// SaveUserJoinRequest saves the chat id to which a user has sent a join request.
	// The join request is not saved if the user is not saved in the database.
	SaveUserJoinRequest(userId, chatId int64) error
	// DeletUserJoinRequest deletes the chat from the join requests list.
	DeleteUserJoinRequest(userId, chatId int64) error
	// GetUsers returns a cursor to loop through all saved users.
	GetAllUsers() (Cursor, error)
	// GetAllUserIDs returns a slice of all user IDs.
	GetAllUserIDs() ([]int64, error)

	// IncrementUserLangStat increments the search count for a language for a specific user.
	IncrementUserLangStat(userId int64, lang string) error
	// IncrementGlobalLangStat increments the search count for a language globally.
	IncrementGlobalLangStat(botId int64, lang string) error

	// TrackSearch increments the frequency count for a search query.
	TrackSearch(query string) error
	// GetTopSearches returns the most popular search queries.
	GetTopSearches(limit int) ([]model.SearchStat, error)
	// UpdateUserLastSeen updates the last search time for a user.
	UpdateUserLastSeen(userId int64) error
	// GetUserAnalytics returns aggregated user statistics.
	GetUserAnalytics() (total, newToday, active24h, activeWeekly, activeMonthly int64, countries map[string]int64, err error)

	// SaveFile saves a file to the database and returns a FileAlreadyExistsError if the file already exists.
	// The file can be a duplicate if it has the same file_id or file_name-file_size combination.
	SaveFile(f *model.File) error
	// SaveFiles saves multiple files to the database and returns a list of errors.
	SaveFiles(files ...*model.File) []error
	// BulkSaveFiles saves multiple files to the database efficiently in a single operation.
	BulkSaveFiles(files []*model.File) error
	// GetFile fetches a file from the database using its unique_id.
	GetFile(fileId string) (*model.File, error)
	// DeleteFile deletes a file from the database using its unique_id.
	DeleteFile(fileId string) error
	// SearchFiles searches for files in the database by their name. The query should be sanitized first.
	SearchFiles(query string) (Cursor, error)
	// GetRecent2026Files retrieves the most recent 2026 uploaded files.
	GetRecent2026Files(limit int) ([]*model.File, error)
	// GetRecentFiles retrieves the most recent uploaded files.
	GetRecentFiles(limit int) ([]*model.File, error)

	// SaveGroup inserts a group id into the database to keep track of them.
	SaveGroup(id int64) error
	GetGroupConfig(chatID int64) (*model.GroupConfig, error)
	SaveGroupConfig(cfg *model.GroupConfig) error
	DeleteGroupConfig(chatID int64) error
	GetUserWarning(chatID, userID int64) (int, error)
	AddUserWarning(chatID, userID int64) (int, error)
	ResetUserWarnings(chatID, userID int64) error
	IncrementGroupMsgCount(chatID int64) error
	IncrementGroupSearchCount(chatID int64) error
	SetUserConnection(userId int64, chatID int64) error
	GetUserConnection(userId int64) (int64, error)
	AddUserConnection(userId int64, chatID int64) error
	RemoveUserConnection(userId int64, chatID int64) error
	GetUserConnections(userId int64) ([]int64, error)

	IsMoviePosted(title string, year string) (bool, error)
	MarkMoviePosted(title string, year string) error

	// GetConfig fetches the bot configs from the database.
	GetConfig(botId int64) (*config.Config, error)
	// UpdateConfig updates a single element of config.
	UpdateConfig(botId int64, key string, value interface{}) error
	// SaveConfig saves the config struct. Useful for importing configs.
	SaveConfig(botId int64, data *config.Config) error
	// ResetConfig removes a config field, resetting it to it's default value.
	ResetConfig(botId int64, key string) error

	// Stats gets the database usage statistics.
	Stats() (*model.Stats, error)
	// GetName returns the name of the database as a user friendly string.
	GetName() string

	// NewIndexOperation inserts a new index operation into the collection.
	NewIndexOperation(i *model.Index) error
	// UpdateIndexOperation updates an index operation.
	UpdateIndexOperation(pid string, vals map[string]interface{}) (bool, error)
	// GetIndexOperation fetches an index operation by it's id.
	GetIndexOperation(pid string) (*model.Index, error)
	// GetAllIndexOperations fetches all active index operations.
	GetActiveIndexOperations() ([]*model.Index, error)
	// DeleteOperation deletes an active operation by id.
	DeleteOperation(pid string) error
	// GetSpellingSuggestions queries local DB files for fuzzy matching suggestions.
	GetSpellingSuggestions(query string) ([]string, error)

	// SaveIndexedChannel saves the last indexed message ID for a channel.
	SaveIndexedChannel(channelID int64, lastMessageID int64) error
	// GetIndexedChannel retrieves the last indexed message ID for a channel.
	GetIndexedChannel(channelID int64) (int64, error)
	// GetIndexOperationByChannel retrieves any active/paused index operation for a channel.
	GetIndexOperationByChannel(channelID int64) (*model.Index, error)

	// Broadcasts
	SaveBroadcast(b *model.Broadcast) error
	GetBroadcast(id string) (*model.Broadcast, error)
	UpdateBroadcast(id string, updates map[string]interface{}) error
	GetAllBroadcasts() ([]model.Broadcast, error)
	DeleteBroadcast(id string) error

	// OTT
	AddOTTSubscriber(chatID int64, username string) (bool, error)
	RemoveOTTSubscriber(chatID int64) (bool, error)
	GetOTTSubscribers() ([]int64, error)
	SetOTTAutoSend(chatID int64, enabled bool) error
	GetOTTAutoSendSubscribers() ([]int64, error)
	IsOTTSubscriber(chatID int64) (bool, error)
	IsOTTItemSent(itemID string) (bool, error)
	MarkOTTItemSent(itemID string, title string) error
}

// KeyValuePair represents a single key-value pair in a document.
type KeyValuePair struct {
	Key   string
	Value interface{}
}
