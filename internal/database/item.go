package database

import (
	"context"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Item struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"`
	URL            string             `bson:"url"`
	Name           string             `bson:"name"`
	ProductID      string             `bson:"product_id"`
	ProductVariant string             `bson:"product_variant"`
	MerchantName   string             `bson:"merchant_name"`
	Site           string             `bson:"site"`
	CreatedAt      primitive.DateTime `bson:"created_at"`
	UpdatedAt      primitive.DateTime `bson:"updated_at"`
}

func (db Database) ItemInsert(ctx context.Context, i Item) (id string, err error) {
	var existingI Item
	err = db.Collection(CollectionItems).FindOne(
		ctx,
		bson.M{
			"product_id":      i.ProductID,
			"product_variant": i.ProductVariant,
			"site":            i.Site,
		},
	).Decode(&existingI)
	if err != nil {
		if err == mongo.ErrNoDocuments {
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

func (db Database) ItemFind(ctx context.Context, id string) (Item, error) {
	var i Item
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return i, errors.Wrapf(err, "error generating objectID from hex: %+v", id)
	}
	err = db.Collection(CollectionItems).FindOne(ctx, bson.M{"_id": objID}).Decode(&i)
	return i, errors.Wrapf(err, "error finding Item from id: %+v", id)
}

func (db Database) ItemFindAll(ctx context.Context) ([]Item, error) {
	var is []Item
	cur, err := db.Collection(CollectionItems).Find(ctx, bson.M{})
	if err != nil {
		return nil, errors.Wrapf(err, "error getting cursor to find all Item")
	}
	if err = cur.All(ctx, &is); err != nil {
		return nil, errors.Wrapf(err, "error getting all Item from cursor")
	}
	return is, nil
}
