package mongo

import (
	"autofilterbot/internal/model"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GetGroupConfig retrieves configuration settings for a given group chat.
func (c *Client) GetGroupConfig(chatID int64) (*model.GroupConfig, error) {
	var cfg model.GroupConfig
	err := c.groupCollection.FindOne(c.ctx, bson.M{"_id": chatID}).Decode(&cfg)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// Create new group config with defaults
			cfg = model.GroupConfig{
				ChatID:         chatID,
				WelcomeEnabled: false,
				Locks:          make(map[string]bool),
			}
			_, _ = c.groupCollection.InsertOne(c.ctx, cfg)
			return &cfg, nil
		}
		return nil, err
	}
	if cfg.Locks == nil {
		cfg.Locks = make(map[string]bool)
	}
	return &cfg, nil
}

// SaveGroupConfig saves configuration settings for a given group chat.
func (c *Client) SaveGroupConfig(cfg *model.GroupConfig) error {
	opts := options.Replace().SetUpsert(true)
	_, err := c.groupCollection.ReplaceOne(c.ctx, bson.M{"_id": cfg.ChatID}, cfg, opts)
	return err
}

// GetUserWarning retrieves warning count for a user in a group chat.
func (c *Client) GetUserWarning(chatID, userID int64) (int, error) {
	id := fmt.Sprintf("%d_%d", chatID, userID)
	var warn model.UserWarning
	err := c.warningCollection.FindOne(c.ctx, bson.M{"_id": id}).Decode(&warn)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 0, nil
		}
		return 0, err
	}
	return warn.Count, nil
}

// AddUserWarning increments warning count for a user in a group chat and returns the new count.
func (c *Client) AddUserWarning(chatID, userID int64) (int, error) {
	id := fmt.Sprintf("%d_%d", chatID, userID)
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	filter := bson.M{"_id": id}
	update := bson.M{
		"$inc": bson.M{"count": 1},
		"$set": bson.M{"chat_id": chatID, "user_id": userID},
	}
	var warn model.UserWarning
	err := c.warningCollection.FindOneAndUpdate(c.ctx, filter, update, opts).Decode(&warn)
	if err != nil {
		return 0, err
	}
	return warn.Count, nil
}

// ResetUserWarnings clears all warnings for a user in a group chat.
func (c *Client) ResetUserWarnings(chatID, userID int64) error {
	id := fmt.Sprintf("%d_%d", chatID, userID)
	_, err := c.warningCollection.DeleteOne(c.ctx, bson.M{"_id": id})
	return err
}

// IncrementGroupMsgCount increments the message count for a given group chat.
func (c *Client) IncrementGroupMsgCount(chatID int64) error {
	filter := bson.M{"_id": chatID}
	update := bson.M{"$inc": bson.M{"message_count": 1}}
	opts := options.Update().SetUpsert(true)
	_, err := c.groupCollection.UpdateOne(c.ctx, filter, update, opts)
	return err
}

// IncrementGroupSearchCount increments the search count for a given group chat.
func (c *Client) IncrementGroupSearchCount(chatID int64) error {
	filter := bson.M{"_id": chatID}
	update := bson.M{"$inc": bson.M{"search_count": 1}}
	opts := options.Update().SetUpsert(true)
	_, err := c.groupCollection.UpdateOne(c.ctx, filter, update, opts)
	return err
}

// DeleteGroupConfig deletes configuration settings for a given group chat.
func (c *Client) DeleteGroupConfig(chatID int64) error {
	_, err := c.groupCollection.DeleteOne(c.ctx, bson.M{"_id": chatID})
	return err
}
