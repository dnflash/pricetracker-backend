package database

import (
	"context"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"pricetracker/internal/model"
	"time"
)

func (db Database) UserInsert(ctx context.Context, u model.User) (id string, err error) {
	u.TrackedItems = []model.TrackedItem{}
	u.CreatedAt = primitive.NewDateTimeFromTime(time.Now())
	u.UpdatedAt = primitive.NewDateTimeFromTime(time.Now())

	r, err := db.Collection(CollectionUsers).InsertOne(ctx, u)
	if err != nil {
		return "", errors.Wrapf(err, "error inserting User with email: %s", u.Email)
	}
	return r.InsertedID.(primitive.ObjectID).Hex(), nil
}

func (db Database) UserFindByEmail(ctx context.Context, email string) (model.User, error) {
	var u model.User
	err := db.Collection(CollectionUsers).FindOne(ctx, bson.M{"email": email}).Decode(&u)
	return u, errors.Wrapf(err, "error finding User with email: %s", email)
}

func (db Database) UserFindByID(ctx context.Context, id string) (model.User, error) {
	var u model.User
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return u, errors.Wrapf(err, "error creating ObjectID from hex: %s", id)
	}
	err = db.Collection(CollectionUsers).FindOne(ctx, bson.M{"_id": objID}).Decode(&u)
	return u, errors.Wrapf(err, "error finding User with ID: %s", id)
}

func (db Database) UserDeviceFCMTokensFindByTrackedItem(ctx context.Context, itemID primitive.ObjectID) ([]model.User, error) {
	var us []model.User
	cur, err := db.Collection(CollectionUsers).Find(ctx,
		bson.M{"tracked_items.item_id": itemID},
		options.Find().SetProjection(bson.M{"tracked_items.$": 1, "devices.fcm_token": 1}),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting cursor to find Users that tracked ItemID: %s", itemID.Hex())
	}
	if err = cur.All(ctx, &us); err != nil {
		return nil, errors.Wrapf(err, "error getting Users that tracked ItemID: %s from cursor", itemID.Hex())
	}
	return us, nil
}

func (db Database) UserTrackedItemUpdateOrAdd(ctx context.Context, userID string, ti model.TrackedItem) error {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return errors.Wrapf(err, "error creating ObjectID from hex: %s", userID)
	}
	res, err := db.Collection(CollectionUsers).UpdateOne(
		ctx,
		bson.M{"_id": objID, "tracked_items.item_id": ti.ItemID},
		bson.M{"$set": bson.M{
			"tracked_items.$.price_lower_threshold": ti.PriceLowerThreshold,
			"tracked_items.$.notification_enabled":  ti.NotificationEnabled,
			"tracked_items.$.notification_count":    ti.NotificationCount,
			"tracked_items.$.updated_at":            primitive.NewDateTimeFromTime(time.Now()),
			"updated_at":                            primitive.NewDateTimeFromTime(time.Now()),
		}},
	)
	if err != nil {
		return errors.Wrapf(err, "error updating or adding TrackedItem to User with ID: %s, ItemID: %s", userID, ti.ItemID.Hex())
	}
	if res.MatchedCount != 0 {
		if _, err := db.Collection(CollectionUsers).UpdateOne(
			ctx,
			bson.M{"_id": objID},
			bson.M{"$push": bson.M{
				"tracked_items": bson.M{
					"$each": []model.TrackedItem{},
					"$sort": bson.M{"updated_at": -1},
				},
			}},
		); err != nil {
			return errors.Wrapf(err, "error sorting TrackedItems after updating on User with ID: %s, ItemID: %s", userID, ti.ItemID.Hex())
		}
	} else {
		ti.CreatedAt = primitive.NewDateTimeFromTime(time.Now())
		ti.UpdatedAt = primitive.NewDateTimeFromTime(time.Now())
		res, err = db.Collection(CollectionUsers).UpdateOne(
			ctx,
			bson.M{"_id": objID},
			bson.M{
				"$push": bson.M{
					"tracked_items": bson.M{
						"$each":     []model.TrackedItem{ti},
						"$position": 0,
						"$sort":     bson.M{"updated_at": -1},
						"$slice":    25,
					},
				},
				"$set": bson.M{
					"updated_at": primitive.NewDateTimeFromTime(time.Now()),
				},
			},
		)
		if err != nil {
			return errors.Wrapf(err, "error when adding TrackedItem to User with ID: %s, ItemID: %s", userID, ti.ItemID.Hex())
		}
	}
	if res.ModifiedCount == 0 {
		return errors.Wrapf(ErrNoDocumentsModified, "User not modified when updating or adding TrackedItem to User with ID: %s, ItemID: %s", userID, ti.ItemID.Hex())
	}
	return nil
}

func (db Database) UserTrackedItemRemove(ctx context.Context, userID string, itemID string) error {
	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return errors.Wrapf(err, "error creating User ObjectID from hex: %s", userID)
	}
	itemOID, err := primitive.ObjectIDFromHex(itemID)
	if err != nil {
		return errors.Wrapf(err, "error creating Item ObjectID from hex: %s", itemID)
	}
	res, err := db.Collection(CollectionUsers).UpdateOne(
		ctx,
		bson.M{"_id": userOID, "tracked_items.item_id": itemOID},
		bson.M{
			"$pull": bson.M{"tracked_items": bson.M{"item_id": itemOID}},
			"$set":  bson.M{"updated_at": primitive.NewDateTimeFromTime(time.Now())},
		},
	)
	if res.ModifiedCount == 0 {
		return errors.Wrapf(ErrNoDocumentsModified, "User not modified when removing TrackedItem on User with ID: %s, ItemID: %s", userID, itemID)
	}
	return nil
}

func (db Database) UserTrackedItemNotificationCountIncrement(
	ctx context.Context, userIDs []primitive.ObjectID, itemID primitive.ObjectID) (int, error) {
	res, err := db.Collection(CollectionUsers).UpdateMany(
		ctx,
		bson.M{"_id": bson.M{"$in": userIDs}, "tracked_items.item_id": itemID},
		bson.M{
			"$set": bson.M{
				"tracked_items.$.last_notified_at": primitive.NewDateTimeFromTime(time.Now()),
				"tracked_items.$.updated_at":       primitive.NewDateTimeFromTime(time.Now()),
			},
			"$inc": bson.M{
				"tracked_items.$.notification_count":       1,
				"tracked_items.$.notification_count_total": 1,
			},
		},
	)
	if err != nil {
		return -1, errors.Wrapf(err, "error when incrementing Users TrackedItem Notification Count, UserIDs: %v, ItemID: %s", userIDs, itemID.Hex())
	}
	return int(res.ModifiedCount), nil
}

func (db Database) UserDeviceAdd(ctx context.Context, userID string, d model.Device) error {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return errors.Wrapf(err, "error creating ObjectID from hex: %s", userID)
	}
	d.LastSeen = primitive.NewDateTimeFromTime(time.Now())
	d.CreatedAt = primitive.NewDateTimeFromTime(time.Now())
	res, err := db.Collection(CollectionUsers).UpdateOne(
		ctx,
		bson.M{"_id": objID},
		bson.M{
			"$push": bson.M{
				"devices": bson.M{
					"$each":     []model.Device{d},
					"$position": 0,
					"$sort":     bson.M{"last_seen": -1},
					"$slice":    5,
				},
			},
			"$set": bson.M{
				"updated_at": primitive.NewDateTimeFromTime(time.Now()),
			},
		},
	)
	if err != nil {
		return errors.Wrapf(err, "error when adding Device to User with ID: %s, DeviceID: %s", userID, d.DeviceID)
	}
	if res.ModifiedCount == 0 {
		return errors.Wrapf(ErrNoDocumentsModified, "User not modified when adding Device to User with ID: %s, DeviceID: %s", userID, d.DeviceID)
	}
	return nil
}

func (db Database) UserDeviceUpdate(ctx context.Context, userID string, d model.Device) error {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return errors.Wrapf(err, "error creating ObjectID from hex: %s", userID)
	}
	res, err := db.Collection(CollectionUsers).UpdateOne(
		ctx,
		bson.M{"_id": objID, "devices.device_id": d.DeviceID},
		bson.M{"$set": bson.M{
			"devices.$":  d,
			"updated_at": primitive.NewDateTimeFromTime(time.Now()),
		}},
	)
	if err != nil {
		return errors.Wrapf(err, "error when updating Device on User with ID: %s, DeviceID: %s", userID, d.DeviceID)
	}
	if res.ModifiedCount == 0 {
		return errors.Wrapf(ErrNoDocumentsModified, "User not modified when updating Device on User with ID: %s, DeviceID: %s", userID, d.DeviceID)
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
		bson.M{"$set": bson.M{
			"devices.$.fcm_token": fcmToken,
			"updated_at":          primitive.NewDateTimeFromTime(time.Now()),
		}},
	)
	if err != nil {
		return errors.Wrapf(err, "error when updating Device FCMToken on User with ID: %s, DeviceID: %s", userID, deviceID)
	}
	if res.ModifiedCount == 0 {
		return errors.Wrapf(ErrNoDocumentsModified, "User not modified when updating Device FCMToken on User with ID: %s, DeviceID: %s", userID, deviceID)
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
		bson.M{"$set": bson.M{
			"devices.$.last_seen": primitive.NewDateTimeFromTime(time.Now()),
			"updated_at":          primitive.NewDateTimeFromTime(time.Now()),
		}},
	)
	if err != nil {
		return errors.Wrapf(err, "error when updating Device LastSeen on User with ID: %s, DeviceID: %s", userID, deviceID)
	}
	if res.ModifiedCount == 0 {
		return errors.Wrapf(ErrNoDocumentsModified, "User not modified when updating Device LastSeen on User with ID: %s, DeviceID: %s", userID, deviceID)
	}
	return nil
}

func (db Database) UserDeviceTokensRemove(ctx context.Context, userID string, deviceID string) error {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return errors.Wrapf(err, "error creating ObjectID from hex: %s", userID)
	}
	res, err := db.Collection(CollectionUsers).UpdateOne(
		ctx,
		bson.M{"_id": objID, "devices.device_id": deviceID},
		bson.M{
			"$unset": bson.M{
				"devices.$.login_token": "",
				"devices.$.fcm_token":   "",
			},
			"$set": bson.M{"updated_at": primitive.NewDateTimeFromTime(time.Now())},
		},
	)
	if err != nil {
		return errors.Wrapf(err, "error when removing Device LoginToken from User with ID: %s, DeviceID: %s", userID, deviceID)
	}
	if res.ModifiedCount == 0 {
		return errors.Wrapf(ErrNoDocumentsModified, "User not modified when removing Device LoginToken from User with ID: %s, DeviceID: %s", userID, deviceID)
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
		bson.M{"_id": objID, "devices.device_id": deviceID},
		bson.M{
			"$pull": bson.M{"devices": bson.M{"device_id": deviceID}},
			"$set":  bson.M{"updated_at": primitive.NewDateTimeFromTime(time.Now())},
		},
	)
	if err != nil {
		return errors.Wrapf(err, "error when removing Device from User with ID: %s, DeviceID: %s", userID, deviceID)
	}
	if res.ModifiedCount == 0 {
		return errors.Wrapf(ErrNoDocumentsModified, "User not modified when removing Device from User with ID: %s, DeviceID: %s", userID, deviceID)
	}
	return nil
}
