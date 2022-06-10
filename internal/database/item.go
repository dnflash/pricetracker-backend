package database

import (
	"context"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"pricetracker/internal/model"
	"time"
)

func (db Database) ItemInsert(ctx context.Context, i model.Item) (id string, err error) {
	i.CreatedAt = primitive.NewDateTimeFromTime(time.Now())
	i.UpdatedAt = primitive.NewDateTimeFromTime(time.Now())
	r, err := db.Collection(CollectionItems).InsertOne(ctx, i)
	if err != nil {
		return "", errors.Wrapf(err, "error inserting Item: %+v", i)
	}
	return r.InsertedID.(primitive.ObjectID).Hex(), nil
}

func (db Database) ItemUpdate(ctx context.Context, i model.Item) error {
	if i.ID.IsZero() {
		return errors.Errorf("Item ID is empty, Item: %+v", i)
	}
	i.UpdatedAt = primitive.NewDateTimeFromTime(time.Now())
	res, err := db.Collection(CollectionItems).ReplaceOne(ctx, bson.M{"_id": i.ID}, i)
	if err != nil {
		return errors.Wrapf(err, "error when updating Item: %+v", i)
	}
	if res.ModifiedCount == 0 {
		return errors.Errorf("Item not modified when updating Item: %+v", i)
	}
	return nil
}

func (db Database) ItemFindExisting(ctx context.Context, i model.Item) (model.Item, error) {
	var existingI model.Item
	err := db.Collection(CollectionItems).FindOne(
		ctx,
		bson.M{
			"site":        i.Site,
			"merchant_id": i.MerchantID,
			"product_id":  i.ProductID,
		},
	).Decode(&existingI)
	return existingI, errors.Wrapf(err, "error trying to find existing Item: %+v", i)
}

func (db Database) ItemFindOne(ctx context.Context, itemID string) (model.Item, error) {
	var i model.Item
	objID, err := primitive.ObjectIDFromHex(itemID)
	if err != nil {
		return i, errors.Wrapf(err, "error generating ObjectID from hex: %s", itemID)
	}
	err = db.Collection(CollectionItems).FindOne(ctx, bson.M{"_id": objID}).Decode(&i)
	return i, errors.Wrapf(err, "error finding Item with ID: %s", itemID)
}

func (db Database) ItemsFind(ctx context.Context, itemIDs []primitive.ObjectID) ([]model.Item, error) {
	var is []model.Item
	cur, err := db.Collection(CollectionItems).Find(ctx, bson.M{"_id": bson.M{"$in": itemIDs}})
	if err != nil {
		return nil, errors.Wrapf(err, "error getting cursor to find Items, itemIDs: %v", itemIDs)
	}
	if err = cur.All(ctx, &is); err != nil {
		return nil, errors.Wrapf(err, "error getting Items from cursor, itemIDs: %v", itemIDs)
	}
	return is, nil
}

func (db Database) ItemsFindAll(ctx context.Context) ([]model.Item, error) {
	var is []model.Item
	cur, err := db.Collection(CollectionItems).Find(ctx, bson.M{})
	if err != nil {
		return nil, errors.Wrapf(err, "error getting cursor to find all Items")
	}
	if err = cur.All(ctx, &is); err != nil {
		return nil, errors.Wrapf(err, "error getting all Items from cursor")
	}
	return is, nil
}
