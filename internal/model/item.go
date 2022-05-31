package model

import "go.mongodb.org/mongo-driver/bson/primitive"

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
