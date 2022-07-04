package model

import "go.mongodb.org/mongo-driver/bson/primitive"

type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"`
	Name         string             `bson:"name"`
	Email        string             `bson:"email"`
	Password     []byte             `bson:"password"`
	Devices      []Device           `bson:"devices"`
	TrackedItems []TrackedItem      `bson:"tracked_items"`
	CreatedAt    primitive.DateTime `bson:"created_at"`
	UpdatedAt    primitive.DateTime `bson:"updated_at"`
}

type Device struct {
	DeviceID   string             `bson:"device_id"`
	LoginToken LoginToken         `bson:"login_token"`
	FCMToken   string             `bson:"fcm_token"`
	LastSeen   primitive.DateTime `bson:"last_seen"`
	CreatedAt  primitive.DateTime `bson:"created_at"`
}

type LoginToken struct {
	Token      []byte             `bson:"token"`
	Expiration primitive.DateTime `bson:"expiration"`
	CreatedAt  primitive.DateTime `bson:"created_at"`
}

type TrackedItem struct {
	ItemID                 primitive.ObjectID `bson:"item_id" json:"-"`
	PriceLowerThreshold    int                `bson:"price_lower_threshold" json:"price_lower_threshold"`
	NotificationEnabled    bool               `bson:"notification_enabled" json:"notification_enabled"`
	NotificationCount      int                `bson:"notification_count" json:"notification_count"`
	NotificationCountTotal int                `bson:"notification_count_total" json:"notification_count_total"`
	LastNotifiedAt         primitive.DateTime `bson:"last_notified_at" json:"last_notified_at"`
	CreatedAt              primitive.DateTime `bson:"created_at" json:"-"`
	UpdatedAt              primitive.DateTime `bson:"updated_at" json:"-"`
}
