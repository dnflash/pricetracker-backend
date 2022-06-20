package database

import (
	"context"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	Name                    = "price_tracker_db"
	CollectionItems         = "items"
	CollectionItemHistories = "item_histories"
	CollectionUsers         = "users"
	CollectionBarcodes      = "barcodes"
)

type Database struct {
	*mongo.Database
}

var ErrNoDocumentsModified = errors.New("no documents modified")

func ConnectDB(ctx context.Context, dbURI string) (*mongo.Client, error) {
	c, err := mongo.Connect(ctx, options.Client().ApplyURI(dbURI))
	if err != nil {
		return nil, err
	}

	_, err = c.Database(Name).Collection(CollectionItems).Indexes().CreateOne(
		ctx,
		mongo.IndexModel{
			Keys: bson.D{
				{Key: "site", Value: 1},
				{Key: "merchant_id", Value: 1},
				{Key: "product_id", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
	)
	if err != nil {
		return nil, err
	}

	_, err = c.Database(Name).Collection(CollectionItemHistories).Indexes().CreateOne(
		ctx,
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

	_, err = c.Database(Name).Collection(CollectionUsers).Indexes().CreateMany(
		ctx,
		[]mongo.IndexModel{
			{
				Keys:    bson.D{{Key: "email", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
			{
				Keys:    bson.D{{Key: "tracked_items.item_id", Value: 1}},
				Options: options.Index().SetUnique(false),
			},
			{
				Keys:    bson.D{{Key: "devices.fcm_token", Value: 1}},
				Options: options.Index().SetUnique(true).SetSparse(true),
			},
		},
	)
	if err != nil {
		return nil, err
	}

	_, err = c.Database(Name).Collection(CollectionBarcodes).Indexes().CreateOne(
		ctx,
		mongo.IndexModel{
			Keys:    bson.D{{Key: "barcode", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	)
	if err != nil {
		return nil, err
	}

	return c, nil
}
