package mongo

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type AutoPost struct {
	ID        string    `bson:"_id"` // format: "clean_title:year"
	CreatedAt time.Time `bson:"created_at"`
}

func (c *Client) IsMoviePosted(title string, year string) (bool, error) {
	key := title
	if year != "" {
		key = title + ":" + year
	}
	var ap AutoPost
	err := c.autoPostsCollection.FindOne(c.ctx, bson.M{"_id": key}).Decode(&ap)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *Client) MarkMoviePosted(title string, year string) error {
	key := title
	if year != "" {
		key = title + ":" + year
	}
	_, err := c.autoPostsCollection.InsertOne(c.ctx, AutoPost{
		ID:        key,
		CreatedAt: time.Now(),
	})
	return err
}
