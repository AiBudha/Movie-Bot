package model

// GroupConfig contains data for a single telegram group chat's configurations.
type GroupConfig struct {
	ChatID         int64           `json:"chat_id" bson:"_id"`
	Rules          string          `json:"rules,omitempty" bson:"rules,omitempty"`
	WelcomeText    string          `json:"welcome_text,omitempty" bson:"welcome_text,omitempty"`
	WelcomeEnabled bool            `json:"welcome_enabled" bson:"welcome_enabled"`
	Locks          map[string]bool `json:"locks,omitempty" bson:"locks,omitempty"`
	WarnLimit      int             `json:"warn_limit,omitempty" bson:"warn_limit,omitempty"`
	WarnMode       string          `json:"warn_mode,omitempty" bson:"warn_mode,omitempty"`
	FloodLimit     int             `json:"flood_limit,omitempty" bson:"flood_limit,omitempty"`
	CaptchaEnabled bool            `json:"captcha_enabled" bson:"captcha_enabled"`
	CaptchaTime    int             `json:"captcha_time,omitempty" bson:"captcha_time,omitempty"`
	AntiRaidEnabled bool           `json:"antiraid_enabled" bson:"antiraid_enabled"`
	MessageCount    int64           `json:"message_count,omitempty" bson:"message_count,omitempty"`
	SearchCount     int64           `json:"search_count,omitempty" bson:"search_count,omitempty"`
}

// UserWarning tracks warning metrics for a specific user within a specific group chat.
type UserWarning struct {
	ID     string `json:"_id" bson:"_id"` // format: "chatID_userID"
	ChatID int64  `json:"chat_id" bson:"chat_id"`
	UserID int64  `json:"user_id" bson:"user_id"`
	Count  int    `json:"count" bson:"count"`
}
