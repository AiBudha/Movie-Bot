package mongo

import (
	"autofilterbot/internal/model"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AddOTTSubscriber adds a chat ID to the subscribers list.
// Returns true if new subscriber, false if already exists.
func (c *Client) AddOTTSubscriber(chatID int64, username string) (bool, error) {
	filter := bson.M{"chat_id": chatID}
	update := bson.M{
		"$setOnInsert": bson.M{
			"username":      username,
			"auto_send":     true,
			"subscribed_at": time.Now(),
		},
	}
	opts := options.Update().SetUpsert(true)
	res, err := c.ottSubscriberCollection.UpdateOne(c.ctx, filter, update, opts)
	if err != nil {
		return false, err
	}
	return res.UpsertedCount > 0, nil
}

// RemoveOTTSubscriber removes a subscriber.
// Returns true if found and removed, false otherwise.
func (c *Client) RemoveOTTSubscriber(chatID int64) (bool, error) {
	filter := bson.M{"chat_id": chatID}
	res, err := c.ottSubscriberCollection.DeleteOne(c.ctx, filter)
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}

// GetOTTSubscribers fetches chat IDs of all subscribers.
func (c *Client) GetOTTSubscribers() ([]int64, error) {
	cursor, err := c.ottSubscriberCollection.Find(c.ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(c.ctx)

	var chatIDs []int64
	for cursor.Next(c.ctx) {
		var sub model.OTTSubscriber
		if err := cursor.Decode(&sub); err == nil {
			chatIDs = append(chatIDs, sub.ChatID)
		}
	}
	return chatIDs, nil
}

// SetOTTAutoSend toggles auto-send for a subscriber.
func (c *Client) SetOTTAutoSend(chatID int64, enabled bool) error {
	filter := bson.M{"chat_id": chatID}
	update := bson.M{"$set": bson.M{"auto_send": enabled}}
	_, err := c.ottSubscriberCollection.UpdateOne(c.ctx, filter, update)
	return err
}

// GetOTTAutoSendSubscribers fetches chat IDs of subscribers with auto-send enabled.
func (c *Client) GetOTTAutoSendSubscribers() ([]int64, error) {
	filter := bson.M{"auto_send": true}
	cursor, err := c.ottSubscriberCollection.Find(c.ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(c.ctx)

	var chatIDs []int64
	for cursor.Next(c.ctx) {
		var sub model.OTTSubscriber
		if err := cursor.Decode(&sub); err == nil {
			chatIDs = append(chatIDs, sub.ChatID)
		}
	}
	return chatIDs, nil
}

// IsOTTSubscriber checks if a chat ID is subscribed.
func (c *Client) IsOTTSubscriber(chatID int64) (bool, error) {
	filter := bson.M{"chat_id": chatID}
	count, err := c.ottSubscriberCollection.CountDocuments(c.ctx, filter)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// IsOTTItemSent checks if an item has already been marked as sent.
func (c *Client) IsOTTItemSent(itemID string) (bool, error) {
	filter := bson.M{"item_id": itemID}
	count, err := c.ottSentItemsCollection.CountDocuments(c.ctx, filter)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// MarkOTTItemSent records that an item has been sent.
func (c *Client) MarkOTTItemSent(itemID string, title string) error {
	item := model.OTTSentItem{
		ItemID: itemID,
		Title:  title,
		SentAt: time.Now(),
	}
	opts := options.Update().SetUpsert(true)
	_, err := c.ottSentItemsCollection.UpdateOne(
		c.ctx,
		bson.M{"item_id": itemID},
		bson.M{"$set": item},
		opts,
	)
	return err
}
