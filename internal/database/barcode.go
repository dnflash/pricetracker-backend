package database

import (
	"context"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"pricetracker/internal/model"
)

func (db Database) BarcodeFind(ctx context.Context, barcodeNumber string) (model.Barcode, error) {
	var b model.Barcode
	err := db.Collection(CollectionBarcodes).FindOne(ctx, bson.M{"barcode": barcodeNumber}).Decode(&b)
	return b, errors.WithMessagef(err, "error finding barcode: %s", barcodeNumber)
}
