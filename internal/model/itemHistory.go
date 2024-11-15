package model

import "go.mongodb.org/mongo-driver/bson/primitive"

type ItemHistory struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	ItemID    primitive.ObjectID `bson:"item_id" json:"-"`
	Price     int                `bson:"pr" json:"pr"`
	Stock     int                `bson:"st" json:"st"`
	Rating    float64            `bson:"rt" json:"rt"`
	Sold      int                `bson:"sl" json:"sl"`
	Timestamp primitive.DateTime `bson:"ts" json:"ts"`
}
