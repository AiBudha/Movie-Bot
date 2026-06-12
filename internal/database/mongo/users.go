package mongo

import (
	"time"
	"autofilterbot/internal/database"
	"autofilterbot/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SaveUser creates a new document in the user collection with the user id.
func (c *Client) SaveUser(userId int64) error {
	_, err := c.userCollection.InsertOne(c.ctx, model.User{UserId: userId})
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		return err
	}
	return nil
}

// SaveUserExtended saves a user with additional metadata like source and DC.
func (c *Client) SaveUserExtended(userId int64, source string, dc int, lang string) error {
	filter := idFilter(userId)
	setOnInsert := bson.M{
		"_id":        userId,
		"source":     source,
		"lang":       lang,
		"created_at": time.Now().Unix(),
	}
	update := bson.M{
		"$setOnInsert": setOnInsert,
	}
	if dc > 0 {
		update["$set"] = bson.M{
			"dc": dc,
		}
	}
	opts := options.Update().SetUpsert(true)
	_, err := c.userCollection.UpdateOne(c.ctx, filter, update, opts)
	return err
}

// UpdateUserCountry updates the country field for a user.
func (c *Client) UpdateUserCountry(userId int64, country string) error {
	filter := idFilter(userId)
	update := bson.M{"$set": bson.M{"country": country}}
	_, err := c.userCollection.UpdateOne(c.ctx, filter, update)
	return err
}

// IncrementUserLangStat increments the count for a language for a specific user.
func (c *Client) IncrementUserLangStat(userId int64, lang string) error {
	filter := idFilter(userId)
	update := bson.M{"$inc": bson.M{"lang_stats." + lang: 1}}
	_, err := c.userCollection.UpdateOne(c.ctx, filter, update)
	return err
}

// GetUser fetches a user from the database by id.
func (c *Client) GetUser(userId int64) (*model.User, error) {
	var u model.User

	res := c.userCollection.FindOne(c.ctx, idFilter(userId))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			return &model.User{UserId: userId}, nil
		}
		return nil, err
	}

	err := res.Decode(&u)

	return &u, err
}

// DeleteUser deletes a user by their id.
func (c *Client) DeleteUser(userId int64) error {
	_, err := c.userCollection.DeleteOne(c.ctx, idFilter(userId))
	return err
}

// GetAllUsers return a cursor to loop over all users.
func (c *Client) GetAllUsers() (database.Cursor, error) {
	return c.userCollection.Find(c.ctx, bson.M{})
}

// GetAllUserIDs returns a slice of all user IDs using projection.
func (c *Client) GetAllUserIDs() ([]int64, error) {
	projection := bson.M{"_id": 1}
	cursor, err := c.userCollection.Find(c.ctx, bson.M{}, options.Find().SetProjection(projection))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(c.ctx)

	var ids []int64
	for cursor.Next(c.ctx) {
		var doc struct {
			ID int64 `bson:"_id"`
		}
		if err := cursor.Decode(&doc); err == nil {
			ids = append(ids, doc.ID)
		}
	}
	return ids, nil
}

// idFilter creates a basic bson filter to find documents with matching _id.
func idFilter(id interface{}) bson.D {
	return bson.D{{Key: "_id", Value: id}}
}
func (c *Client) SetUserLastAction(userId int64, action string) error {
	filter := idFilter(userId)
	update := bson.M{"$set": bson.M{"last_action": action}}
	_, err := c.userCollection.UpdateOne(c.ctx, filter, update)
	return err
}

func (c *Client) GetUserLastAction(userId int64) (string, error) {
	user, err := c.GetUser(userId)
	if err != nil {
		return "", err
	}
	return user.LastAction, nil
}
func (c *Client) SetUserFsubMessage(userId int64, messageId int64) error {
	filter := idFilter(userId)
	update := bson.M{"$set": bson.M{"fsub_message_id": messageId}}
	_, err := c.userCollection.UpdateOne(c.ctx, filter, update)
	return err
}

func (c *Client) GetUserFsubMessage(userId int64) (int64, error) {
	user, err := c.GetUser(userId)
	if err != nil {
		return 0, err
	}
	return user.FsubMessageID, nil
}

func (c *Client) SetUserConnection(userId int64, chatID int64) error {
	filter := idFilter(userId)
	update := bson.M{"$set": bson.M{"connected_chat_id": chatID}}
	_, err := c.userCollection.UpdateOne(c.ctx, filter, update)
	return err
}

func (c *Client) GetUserConnection(userId int64) (int64, error) {
	user, err := c.GetUser(userId)
	if err != nil {
		return 0, err
	}
	return user.ConnectedChatID, nil
}

// AddUserConnection adds a group chat ID to the user's list of connected groups (no duplicates).
func (c *Client) AddUserConnection(userId int64, chatID int64) error {
	filter := idFilter(userId)
	// $addToSet ensures no duplicates
	update := bson.M{"$addToSet": bson.M{"connected_chat_ids": chatID}}
	opts := options.Update().SetUpsert(true)
	_, err := c.userCollection.UpdateOne(c.ctx, filter, update, opts)
	return err
}

// RemoveUserConnection removes a group chat ID from the user's connected groups list.
func (c *Client) RemoveUserConnection(userId int64, chatID int64) error {
	filter := idFilter(userId)
	update := bson.M{"$pull": bson.M{"connected_chat_ids": chatID}}
	_, err := c.userCollection.UpdateOne(c.ctx, filter, update)
	return err
}

// GetUserConnections returns all group chat IDs the user has connected.
func (c *Client) GetUserConnections(userId int64) ([]int64, error) {
	user, err := c.GetUser(userId)
	if err != nil {
		return nil, err
	}
	return user.ConnectedChatIDs, nil
}
