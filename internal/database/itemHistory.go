package database

import (
	"context"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"pricetracker/internal/model"
	"time"
)

func (db Database) ItemHistoryInsert(ctx context.Context, ih model.ItemHistory) (err error) {
	_, err = db.Collection(CollectionItemHistories).InsertOne(ctx, ih)
	return errors.Wrapf(err, "error inserting ItemHistory: %+v", ih)
}

func (db Database) ItemHistoryFindRange(
	ctx context.Context, itemID string, start time.Time, end time.Time) ([]model.ItemHistory, error) {
	itemOID, err := primitive.ObjectIDFromHex(itemID)
	if err != nil {
		return nil, errors.Wrapf(err, "error generating ObjectID from hex: %s", itemID)
	}
	var ihs []model.ItemHistory
	cur, err := db.Collection(CollectionItemHistories).Find(ctx, bson.M{
		"item_id": itemOID,
		"ts": bson.M{
			"$gte": primitive.NewDateTimeFromTime(start),
			"$lte": primitive.NewDateTimeFromTime(end),
		},
	}, options.Find().SetSort(bson.M{"ts": -1}))
	if err != nil {
		return nil, errors.Wrapf(err,
			"error getting cursor to find ItemHistory for ItemID: %s, start: %s, end: %s",
			itemID, start.Format(time.RFC3339), end.Format(time.RFC3339))
	}
	if err = cur.All(ctx, &ihs); err != nil {
		return nil, errors.Wrapf(err,
			"error getting all ItemHistory from cursor for ItemID: %s, start: %s, end: %s",
			itemID, start.Format(time.RFC3339), end.Format(time.RFC3339))
	}
	return ihs, nil
}
