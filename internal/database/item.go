package database

import (
	"context"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"time"
)

type Item struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	URL            string             `bson:"url" json:"url"`
	Name           string             `bson:"name" json:"name"`
	ProductID      string             `bson:"product_id" json:"product_id"`
	ProductVariant string             `bson:"product_variant" json:"product_variant"`
	Price          int                `bson:"price" json:"price"`
	Stock          int                `bson:"stock" json:"stock"`
	ImageURL       string             `bson:"image_url" json:"image_url"`
	MerchantName   string             `bson:"merchant_name" json:"merchant_name"`
	Site           string             `bson:"site" json:"site"`
	CreatedAt      primitive.DateTime `bson:"created_at" json:"-"`
	UpdatedAt      primitive.DateTime `bson:"updated_at" json:"-"`
}

func (db Database) ItemInsert(ctx context.Context, i Item) (id string, err error) {
	var existingI Item
	err = db.Collection(CollectionItems).FindOne(
		ctx,
		bson.M{
			"site":            i.Site,
			"product_id":      i.ProductID,
			"product_variant": i.ProductVariant,
		},
	).Decode(&existingI)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			i.CreatedAt = primitive.NewDateTimeFromTime(time.Now())
			i.UpdatedAt = primitive.NewDateTimeFromTime(time.Now())
			r, err := db.Collection(CollectionItems).InsertOne(ctx, i)
			if err != nil {
				return "", errors.Wrapf(err, "error inserting Item: %+v", i)
			}
			return r.InsertedID.(primitive.ObjectID).Hex(), nil
		} else {
			return "", errors.Wrapf(err, "error trying to find existing Item: %+v", i)
		}
	}
	return existingI.ID.Hex(), nil
}

func (db Database) ItemPriceAndStockUpdate(ctx context.Context, itemID primitive.ObjectID, price int, stock int) error {
	res, err := db.Collection(CollectionItems).UpdateOne(
		ctx,
		bson.M{"_id": itemID},
		bson.M{"$set": bson.M{
			"price":      price,
			"stock":      stock,
			"updated_at": primitive.NewDateTimeFromTime(time.Now()),
		}},
	)
	if err != nil {
		return errors.Wrapf(err, "error when updating Item Stock and Price, ItemID: %s, Price: %d, Stock: %d",
			itemID.Hex(), price, stock)
	}
	if res.ModifiedCount == 0 {
		return errors.Errorf("Item not modified when updating Item Stock and Price, ItemID: %s, Price: %d, Stock: %d",
			itemID.Hex(), price, stock)
	}
	return nil
}

func (db Database) ItemFindOne(ctx context.Context, itemID string) (Item, error) {
	var i Item
	objID, err := primitive.ObjectIDFromHex(itemID)
	if err != nil {
		return i, errors.Wrapf(err, "error generating ObjectID from hex: %s", itemID)
	}
	err = db.Collection(CollectionItems).FindOne(ctx, bson.M{"_id": objID}).Decode(&i)
	return i, errors.Wrapf(err, "error finding Item with ID: %s", itemID)
}

func (db Database) ItemsFind(ctx context.Context, itemIDs []primitive.ObjectID) ([]Item, error) {
	var is []Item
	cur, err := db.Collection(CollectionItems).Find(ctx, bson.M{"_id": bson.M{"$in": itemIDs}})
	if err != nil {
		return nil, errors.Wrapf(err, "error getting cursor to find Items, itemIDs: %v", itemIDs)
	}
	if err = cur.All(ctx, &is); err != nil {
		return nil, errors.Wrapf(err, "error getting Items from cursor, itemIDs: %v", itemIDs)
	}
	return is, nil
}

func (db Database) ItemsFindAll(ctx context.Context) ([]Item, error) {
	var is []Item
	cur, err := db.Collection(CollectionItems).Find(ctx, bson.M{})
	if err != nil {
		return nil, errors.Wrapf(err, "error getting cursor to find all Items")
	}
	if err = cur.All(ctx, &is); err != nil {
		return nil, errors.Wrapf(err, "error getting all Items from cursor")
	}
	return is, nil
}
