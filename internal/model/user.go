package model

// User contains data of a single user of the bot saved in the database.
type User struct {
	// UserId is the unique telegram id of the user.
	UserId int64 `json:"_id" bson:"_id"`
	// JoinRequests contains a list of channel to which the user has sent a join request.
	JoinRequests []int64 `json:"join_requests,omitempty" bson:"join_requests,omitempty"`
	// Source is the referral source of the user.
	Source string `json:"source,omitempty" bson:"source,omitempty"`
	// DC is the telegram data center of the user.
	DC int `json:"dc,omitempty" bson:"dc,omitempty"`
	// Language is the telegram language code of the user.
	Language string `json:"lang,omitempty" bson:"lang,omitempty"`
	// LastAction stores the last command or data for Fsub resume.
	LastAction string `bson:"last_action"`
	// LangStats stores the search language statistics for the user.
	LangStats map[string]int `json:"lang_stats,omitempty" bson:"lang_stats,omitempty"`
	// Country is the detected country based on language code.
	Country string `json:"country,omitempty" bson:"country,omitempty"`
	// CreatedAt is the time when the user first started the bot.
	CreatedAt int64 `json:"created_at,omitempty" bson:"created_at,omitempty"`
	// FsubMessageID stores the ID of the current fsub prompt for sequential logic.
	FsubMessageID int64 `bson:"fsub_message_id"`
	// LastSearchAt is the time of the last search performed by the user.
	LastSearchAt int64 `json:"last_search_at,omitempty" bson:"last_search_at,omitempty"`
	ConnectedChatID  int64   `json:"connected_chat_id,omitempty" bson:"connected_chat_id,omitempty"`
	ConnectedChatIDs []int64 `json:"connected_chat_ids,omitempty" bson:"connected_chat_ids,omitempty"`
}

// FsubStats contains analytics for a single fsub channel.
type FsubStats struct {
	TotalRequests   int64
	BotUsers        int64
	Requested       int64
	Joined          int64
	DailyRequests   int64
	WeeklyRequests  int64
	MonthlyRequests int64
}
