package database

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	Name                    = "price_tracker_db"
	CollectionItems         = "items"
	CollectionItemHistories = "item_histories"
	CollectionUsers         = "users"
)

type Database struct {
	*mongo.Database
}

func ConnectDB(dbURI string) (*mongo.Client, error) {
	c, err := mongo.Connect(context.Background(), options.Client().ApplyURI(dbURI))
	if err != nil {
		return nil, err
	}

	_, err = c.Database(Name).Collection(CollectionItems).Indexes().CreateOne(
		context.Background(),
		mongo.IndexModel{
			Keys: bson.D{
				{Key: "product_id", Value: 1},
				{Key: "product_variant", Value: 1},
				{Key: "site", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
	)
	if err != nil {
		return nil, err
	}

	_, err = c.Database(Name).Collection(CollectionItemHistories).Indexes().CreateOne(
		context.Background(),
		mongo.IndexModel{
			Keys: bson.D{
				{Key: "item_id", Value: 1},
				{Key: "ts", Value: -1},
			},
			Options: options.Index().SetUnique(true),
		},
	)
	if err != nil {
		return nil, err
	}

	_, err = c.Database(Name).Collection(CollectionUsers).Indexes().CreateOne(
		context.Background(),
		mongo.IndexModel{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	)
	if err != nil {
		return nil, err
	}

	return c, nil
}
