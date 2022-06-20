package model

import "go.mongodb.org/mongo-driver/bson/primitive"

type Barcode struct {
	ID            primitive.ObjectID `bson:"id"`
	BarcodeNumber string             `bson:"barcode"`
	ProductName   string             `bson:"product_name"`
	Query1        string             `bson:"q1"`
	Query2        string             `bson:"q2"`
	Source        string             `bson:"source"`
}
