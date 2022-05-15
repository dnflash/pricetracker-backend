package database

import (
	"context"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

type User struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	Name        string             `bson:"name"`
	Email       string             `bson:"email"`
	Password    []byte             `bson:"password"`
	LoginTokens []LoginToken       `bson:"login_tokens"`
	Devices     []Device           `bson:"devices"`
	CreatedAt   primitive.DateTime `bson:"created_at"`
	UpdatedAt   primitive.DateTime `bson:"updated_at"`
}

type LoginToken struct {
	TokenID    string             `bson:"token_id"`
	Token      []byte             `bson:"token"`
	Expiration primitive.DateTime `bson:"expiration"`
	CreatedAt  primitive.DateTime `bson:"created_at"`
}

type Device struct {
	Name      string             `bson:"name"`
	Token     string             `bson:"token"`
	CreatedAt primitive.DateTime `bson:"created_at"`
	UpdatedAt primitive.DateTime `bson:"updated_at"`
}

func (db Database) UserInsert(ctx context.Context, u User) (id string, err error) {
	u.LoginTokens = []LoginToken{}
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

func (db Database) UserAddLoginToken(ctx context.Context, userID string, lt LoginToken) error {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return errors.Wrapf(err, "error creating ObjectID from hex: %s", userID)
	}

	lt.CreatedAt = primitive.NewDateTimeFromTime(time.Now())

	res, err := db.Collection(CollectionUsers).UpdateOne(
		ctx,
		bson.M{"_id": objID},
		bson.M{"$push": bson.M{
			"login_tokens": bson.M{
				"$each":     []LoginToken{lt},
				"$position": 0,
				"$slice":    8,
			},
		}},
	)
	if err != nil {
		return errors.Wrapf(err, "error when adding login token to User with ID: %s", userID)
	}

	if res.ModifiedCount == 0 {
		return errors.Errorf("User not modified when adding login token to User with ID: %s", userID)
	}

	return nil
}

func (db Database) UserRemoveLoginToken(ctx context.Context, userID string, tokenID string) error {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return errors.Wrapf(err, "error creating ObjectID from hex: %s", userID)
	}

	res, err := db.Collection(CollectionUsers).UpdateOne(
		ctx,
		bson.M{"_id": objID},
		bson.M{"$pull": bson.M{"login_tokens": bson.M{"token_id": tokenID}}},
	)
	if err != nil {
		return errors.Wrapf(err, "error when removing login token from User with ID: %s, token ID: %s", userID, tokenID)
	}

	if res.ModifiedCount == 0 {
		return errors.Errorf("User not modified when removing login token from User with ID: %s, token ID: %s", userID, tokenID)
	}

	return nil
}
