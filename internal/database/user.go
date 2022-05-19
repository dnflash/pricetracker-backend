package database

import (
	"context"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

type User struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	Name      string             `bson:"name"`
	Email     string             `bson:"email"`
	Password  []byte             `bson:"password"`
	Devices   []Device           `bson:"devices"`
	CreatedAt primitive.DateTime `bson:"created_at"`
	UpdatedAt primitive.DateTime `bson:"updated_at"`
}

type LoginToken struct {
	Token      []byte             `bson:"token"`
	Expiration primitive.DateTime `bson:"expiration"`
	CreatedAt  primitive.DateTime `bson:"created_at"`
}

type Device struct {
	DeviceID   string             `bson:"device_id"`
	LoginToken LoginToken         `bson:"login_token"`
	FCMToken   string             `bson:"fcm_token"`
	LastSeen   primitive.DateTime `bson:"last_seen"`
	CreatedAt  primitive.DateTime `bson:"created_at"`
}

func (db Database) UserInsert(ctx context.Context, u User) (id string, err error) {
	u.Devices = []Device{}
	u.CreatedAt = primitive.NewDateTimeFromTime(time.Now())
	u.UpdatedAt = primitive.NewDateTimeFromTime(time.Now())

	r, err := db.Collection(CollectionUsers).InsertOne(ctx, u)
	if err != nil {
		return "", errors.Wrapf(err, "error inserting User with email: %s", u.Email)
	}
	return r.InsertedID.(primitive.ObjectID).Hex(), nil
}

func (db Database) UserFindByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := db.Collection(CollectionUsers).FindOne(ctx, bson.M{"email": email}).Decode(&u)
	return u, errors.Wrapf(err, "error finding User with email: %s", email)
}

func (db Database) UserFindByID(ctx context.Context, id string) (User, error) {
	var u User

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return u, errors.Wrapf(err, "error creating ObjectID from hex: %s", id)
	}

	err = db.Collection(CollectionUsers).FindOne(ctx, bson.M{"_id": objID}).Decode(&u)
	return u, errors.Wrapf(err, "error finding User with ID: %s", id)
}

func (db Database) UserDeviceAdd(ctx context.Context, userID string, d Device) error {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return errors.Wrapf(err, "error creating ObjectID from hex: %s", userID)
	}

	d.LastSeen = primitive.NewDateTimeFromTime(time.Now())
	d.CreatedAt = primitive.NewDateTimeFromTime(time.Now())

	res, err := db.Collection(CollectionUsers).UpdateOne(
		ctx,
		bson.M{"_id": objID},
		bson.M{"$push": bson.M{
			"devices": bson.M{
				"$each":     []Device{d},
				"$position": 0,
				"$sort":     bson.M{"last_seen": -1},
				"$slice":    5,
			},
		}},
	)
	if err != nil {
		return errors.Wrapf(err, "error when adding Device to User with ID: %s, Device ID: %s", userID, d.DeviceID)
	}

	if res.ModifiedCount == 0 {
		return errors.Errorf("User not modified when adding Device to User with ID: %s, Device ID: %s", userID, d.DeviceID)
	}

	return nil
}

func (db Database) UserDeviceUpdate(ctx context.Context, userID string, d Device) error {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return errors.Wrapf(err, "error creating ObjectID from hex: %s", userID)
	}

	res, err := db.Collection(CollectionUsers).UpdateOne(
		ctx,
		bson.M{"_id": objID, "devices.device_id": d.DeviceID},
		bson.M{"$set": bson.M{"devices.$": d}},
	)
	if err != nil {
		return errors.Wrapf(err, "error when updating Device on User with ID: %s, Device ID: %s", userID, d.DeviceID)
	}

	if res.ModifiedCount == 0 {
		return errors.Errorf("User not modified when updating Device on User with ID: %s, Device ID: %s", userID, d.DeviceID)
	}

	return nil
}

func (db Database) UserDeviceFCMTokenUpdate(ctx context.Context, userID string, deviceID string, fcmToken string) error {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return errors.Wrapf(err, "error creating ObjectID from hex: %s", userID)
	}

	res, err := db.Collection(CollectionUsers).UpdateOne(
		ctx,
		bson.M{"_id": objID, "devices.device_id": deviceID},
		bson.M{"$set": bson.M{"devices.$.fcm_token": fcmToken}},
	)
	if err != nil {
		return errors.Wrapf(err, "error when updating Device FCMToken on User with ID: %s, Device ID: %s", userID, deviceID)
	}

	if res.ModifiedCount == 0 {
		return errors.Errorf("User not modified when updating Device FCMToken on User with ID: %s, Device ID: %s", userID, deviceID)
	}

	return nil
}

func (db Database) UserDeviceLastSeenUpdate(ctx context.Context, userID string, deviceID string) error {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return errors.Wrapf(err, "error creating ObjectID from hex: %s", userID)
	}

	res, err := db.Collection(CollectionUsers).UpdateOne(
		ctx,
		bson.M{"_id": objID, "devices.device_id": deviceID},
		bson.M{"$set": bson.M{"devices.$.last_seen": primitive.NewDateTimeFromTime(time.Now())}},
	)
	if err != nil {
		return errors.Wrapf(err, "error when updating Device LastSeen on User with ID: %s, Device ID: %s", userID, deviceID)
	}

	if res.ModifiedCount == 0 {
		return errors.Errorf("User not modified when updating Device LastSeen on User with ID: %s, Device ID: %s", userID, deviceID)
	}

	return nil
}

func (db Database) UserDeviceRemove(ctx context.Context, userID string, deviceID string) error {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return errors.Wrapf(err, "error creating ObjectID from hex: %s", userID)
	}

	res, err := db.Collection(CollectionUsers).UpdateOne(
		ctx,
		bson.M{"_id": objID},
		bson.M{"$pull": bson.M{"devices": bson.M{"device_id": deviceID}}},
	)
	if err != nil {
		return errors.Wrapf(err, "error when removing Device from User with ID: %s, Device ID: %s", userID, deviceID)
	}

	if res.ModifiedCount == 0 {
		return errors.Errorf("User not modified when removing Device from User with ID: %s, Device ID: %s", userID, deviceID)
	}

	return nil
}
