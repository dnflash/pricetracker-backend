package database

import (
	"context"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

type ItemHistory struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	ItemID    primitive.ObjectID `bson:"item_id"`
	Price     int                `bson:"pr"`
	Stock     int                `bson:"st"`
	Timestamp primitive.DateTime `bson:"ts"`
}

func (db Database) ItemHistoryInsert(ctx context.Context, ih ItemHistory) (err error) {
	_, err = db.Collection(CollectionItemHistories).InsertOne(ctx, ih)
	return errors.Wrapf(err, "error inserting ItemHistory: %+v", ih)
}

func (db Database) ItemHistoryFindLatest(ctx context.Context, itemID primitive.ObjectID) (ItemHistory, error) {
	var ih ItemHistory
	opts := options.FindOne().SetSort(bson.M{"ts": -1})
	err := db.Collection(CollectionItemHistories).FindOne(ctx, bson.M{"item_id": itemID}, opts).Decode(&ih)
	return ih, errors.Wrapf(err, "error finding latest ItemHistory for ItemID: %s", itemID.Hex())
}

func (db Database) ItemHistoryFindRange(
	ctx context.Context, itemID primitive.ObjectID, start time.Time, end time.Time,
) ([]ItemHistory, error) {
	var ihs []ItemHistory
	opts := options.Find().SetSort(bson.M{"ts": -1})
	cur, err := db.Collection(CollectionItemHistories).Find(ctx, bson.M{
		"item_id": itemID,
		"ts": bson.M{
			"$gte": primitive.NewDateTimeFromTime(start),
			"$lte": primitive.NewDateTimeFromTime(end),
		},
	}, opts)
	if err != nil {
		return nil, errors.Wrapf(err,
			"error getting cursor to find ItemHistory for ItemID: %s, start: %s, end: %s",
			itemID.Hex(), start.Format(time.RFC3339), end.Format(time.RFC3339))
	}
	if err = cur.All(ctx, &ihs); err != nil {
		return nil, errors.Wrapf(err,
			"error getting all ItemHistory from cursor for ItemID: %s, start: %s, end: %s",
			itemID.Hex(), start.Format(time.RFC3339), end.Format(time.RFC3339))
	}
	return ihs, nil
}
