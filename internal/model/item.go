package model

import "go.mongodb.org/mongo-driver/bson/primitive"

type Item struct {
	ID                   primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	Site                 string             `bson:"site" json:"site"`
	MerchantID           string             `bson:"merchant_id" json:"merchant_id"`
	ProductID            string             `bson:"product_id" json:"product_id"`
	URL                  string             `bson:"url" json:"url"`
	Name                 string             `bson:"name" json:"name"`
	Price                int                `bson:"price" json:"price"`
	PriceLastChangedAt   primitive.DateTime `bson:"price_last_changed_at" json:"price_last_changed_at"`
	PriceHistoryPrevious int                `bson:"price_history_previous" json:"price_history_previous"`
	PriceHistoryHighest  int                `bson:"price_history_highest" json:"price_history_highest"`
	PriceHistoryLowest   int                `bson:"price_history_lowest" json:"price_history_lowest"`
	Stock                int                `bson:"stock" json:"stock"`
	ImageURL             string             `bson:"image_url" json:"image_url"`
	Description          string             `bson:"description" json:"description"`
	Rating               float64            `bson:"rating" json:"rating"`
	Sold                 int                `bson:"sold" json:"sold"`
	CreatedAt            primitive.DateTime `bson:"created_at" json:"-"`
	UpdatedAt            primitive.DateTime `bson:"updated_at" json:"-"`
}
