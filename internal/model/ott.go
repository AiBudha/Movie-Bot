package model

import "time"

// OTTSubscriber represents a subscriber to OTT release updates.
type OTTSubscriber struct {
	ChatID       int64     `json:"chat_id" bson:"chat_id"`
	Username     string    `json:"username,omitempty" bson:"username,omitempty"`
	AutoSend     bool      `json:"auto_send" bson:"auto_send"`
	SubscribedAt time.Time `json:"subscribed_at" bson:"subscribed_at"`
}

// OTTSentItem represents an item that has already been sent to subscribers to avoid duplicates.
type OTTSentItem struct {
	ItemID string    `json:"item_id" bson:"item_id"`
	Title  string    `json:"title" bson:"title"`
	SentAt time.Time `json:"sent_at" bson:"sent_at"`
}
