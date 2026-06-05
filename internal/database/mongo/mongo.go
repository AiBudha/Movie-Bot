// Package mongo implements database.Database using mongodb.
package mongo

import (
	"context"
	"fmt"

	"autofilterbot/internal/database"
	"autofilterbot/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// Ensure *Client implements database.Database
var _ database.Database = (*Client)(nil)

// Client implements database.Database using mongodb
type Client struct {
	// userCollections stores data about users of the bot.
	userCollection *mongo.Collection
	// joinRequestCollection stores join requests sent by users.
	joinRequestsCollection *mongo.Collection
	// joinRequestsLogsCollection stores timestamps of join requests.
	joinRequestsLogsCollection *mongo.Collection
	// fileCollection stores all saved files.
	fileCollection *MultiCollection
	// configCollection stores settings configuration of the bot.
	configCollection *mongo.Collection
	// groupCollection contains data about group chats.
	groupCollection *mongo.Collection
	// Collection of long operations like index.
	opsCollection *mongo.Collection
	// broadcastCollection stores broadcast history.
	broadcastCollection *mongo.Collection
	// Collection of channel indexing state (last message index).
	indexedChannelsCollection *mongo.Collection
	// warningCollection contains data about user warnings in group chats.
	warningCollection *mongo.Collection
	// ottSubscriberCollection stores OTT release subscribers.
	ottSubscriberCollection *mongo.Collection
	// ottSentItemsCollection stores OTT release items already sent.
	ottSentItemsCollection *mongo.Collection

	botId  int64
	ctx    context.Context
	client *mongo.Client
	db     *mongo.Database
}

// NewClientOpts provides optional parameters to NewClient().
type NewClientOpts struct {
	// Name of the dabase within the cluster. Defaults to database.DefaultDatabaseName.
	DatabaseName string
	// Name of the collection where files are stored. Defaults to database.DefaultCollectionNameFiles.
	FilesCollectionName string
	// Additional database urls aside from the primary db, used to store files.
	AdditionalURLs []string
	// Index of the file collection to use for storage, defaults to 0. Can be updated from config panel.
	MultiCollectionIndex int
}

// NewClient creates a new client and connect to mongodb.
//
// - ctx: context that will be further used for every db query.
// - mongodbUri: primary database uri.
// - botId: id of the bot.
// - log: logger.
func NewClient(ctx context.Context, mongodbUri string, botId int64, log *zap.Logger, opts ...NewClientOpts) (*Client, error) {
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongodbUri))
	if err != nil {
		return nil, err
	}

	// Verify connection
	if err := mongoClient.Ping(ctx, nil); err != nil {
		log.Warn("mongo: initial ping failed, will retry in background", zap.Error(err))
	}

	var clientOpts NewClientOpts
	if len(opts) != 0 {
		clientOpts = opts[0]
	}

	databaseName := database.DefaultDatabaseName
	if clientOpts.DatabaseName != "" {
		databaseName = clientOpts.DatabaseName
	}

	collectionName := database.CollectionNameFiles
	if clientOpts.FilesCollectionName != "" {
		collectionName = clientOpts.FilesCollectionName
	}

	dataBase := mongoClient.Database(databaseName)
	primaryFileCollection := dataBase.Collection(collectionName)

	fileCollections := []*mongo.Collection{primaryFileCollection}

	for i, url := range clientOpts.AdditionalURLs {
		c, err := mongo.Connect(ctx, options.Client().ApplyURI(url))
		if err != nil {
			log.Warn("mongo: newclient: failed to connect to additional database", zap.Int("num", i+1))
			continue
		}

		fileCollections = append(fileCollections, c.Database(databaseName).Collection(collectionName))
	}

	fileCollection := NewMultiCollection(fileCollections, clientOpts.MultiCollectionIndex, log)

	for _, coll := range fileCollections {
		coll.Indexes().CreateOne(context.TODO(), mongo.IndexModel{
			Keys: bson.D{{Key: "file_name", Value: "text"}, {Key: "time", Value: 1}},
		})
		coll.Indexes().CreateOne(context.TODO(), mongo.IndexModel{
			Keys: bson.D{{Key: "file_id", Value: 1}},
		})
		coll.Indexes().CreateOne(context.TODO(), mongo.IndexModel{
			Keys: bson.D{{Key: "file_name", Value: 1}, {Key: "file_size", Value: 1}},
		})
	}

	client := &Client{
		botId:                  botId,
		ctx:                    ctx,
		client:                 mongoClient,
		db:                     dataBase,
		userCollection:         dataBase.Collection(database.CollectionNameUsers),
		fileCollection:         fileCollection,
		configCollection:       dataBase.Collection(database.CollectionNameConfigs),
		groupCollection:        dataBase.Collection(database.CollectionNameGroups),
		opsCollection:              dataBase.Collection(database.CollectionNameOperations),
		joinRequestsCollection:     dataBase.Collection(database.CollectionNameJoinRequests),
		joinRequestsLogsCollection: dataBase.Collection(database.CollectionNameJoinRequestsLogs),
		broadcastCollection:        dataBase.Collection(database.CollectionNameBroadcasts),
		indexedChannelsCollection:  dataBase.Collection(database.CollectionNameIndexedChannels),
		warningCollection:          dataBase.Collection("Warnings"),
		ottSubscriberCollection:    dataBase.Collection(database.CollectionNameOTTSubscribers),
		ottSentItemsCollection:    dataBase.Collection(database.CollectionNameOTTSentItems),
	}

	// Create unique index for subscribers chat_id
	_, _ = client.ottSubscriberCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "chat_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	// Create unique index for sent items item_id
	_, _ = client.ottSentItemsCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "item_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	// Create TTL index for sent items sent_at (30 days = 2592000 seconds)
	var ttlSecs int32 = 30 * 24 * 60 * 60
	_, _ = client.ottSentItemsCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "sent_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(ttlSecs),
	})

	return client, nil
}

func (c *Client) Shutdown() error {
	return c.client.Disconnect(context.Background())
}

// fileCounts generates a better visual list of file collection counts.
type fileCounts []int64

func (f fileCounts) String() string {
	if len(f) == 0 {
		return "No Collections Found"
	}
	if len(f) == 1 {
		return fmt.Sprint(f[0])
	}
	var s string
	for i, n := range f {
		s += fmt.Sprintf("\n├┄┄Collection %d: %d", i, n)
	}
	return s
}

func (c *Client) Stats() (*model.Stats, error) {
	users, err := c.userCollection.CountDocuments(c.ctx, bson.M{})
	if err != nil {
		return nil, err
	}

	groups, err := c.groupCollection.CountDocuments(c.ctx, bson.M{})
	if err != nil {
		return nil, err
	}

	var files []int64
	for _, coll := range c.fileCollection.allCollections {
		n, err := coll.EstimatedDocumentCount(c.ctx)
		if err != nil {
			return nil, err
		}
		files = append(files, n)
	}

	// 1. Top Languages From Global Config
	topLanguages := make(map[string]int64)
	if config, err := c.GetConfig(c.botId); err == nil && config != nil {
		topLanguages = config.LangStats
	}

	// 2. Top Sources from User Aggregation
	sourcePipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$ifNull", Value: []interface{}{"$source", "Direct/Unknown"}}}}, {Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
		{{Key: "$limit", Value: 10}},
	}
	
	topSources := make(map[string]int64)
	cursor, err := c.userCollection.Aggregate(c.ctx, sourcePipeline)
	if err == nil {
		var results []struct {
			ID    string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := cursor.All(c.ctx, &results); err == nil {
			for _, r := range results {
				if r.ID == "" {
					r.ID = "Direct/Unknown"
				}
				topSources[r.ID] = r.Count
			}
		}
	}

	return &model.Stats{
		Users:        users,
		Groups:       groups,
		Files:        fileCounts(files),
		TopSources:   topSources,
		TopLanguages: topLanguages,
	}, nil
}

func (c *Client) GetName() string {
	return "MongoDB Atlas"
}

func (c *Client) UpdateStorageCollection(index int) error {
	return c.fileCollection.SetStorageCollection(index)
}

func (c *Client) RunCollectionUpdater(ctx context.Context, log *zap.Logger) {
	go c.fileCollection.RunCollectionUpdater(ctx, log)
}
