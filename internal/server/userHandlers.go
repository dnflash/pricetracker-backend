package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/mail"
	"pricetracker/internal/database"
	"time"
)

func (s Server) userRegister() http.HandlerFunc {
	type request struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		DeviceID string `json:"device_id"`
		FCMToken string `json:"fcm_token"`
	}
	type response struct {
		Success    bool   `json:"success"`
		LoginToken string `json:"login_token"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		req := request{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Debugf("userRegister: Error decoding JSON, err: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_, err := mail.ParseAddress(req.Email)
		if err != nil {
			s.Logger.Debugf("userRegister: Invalid email, err: %v", err)
			http.Error(w, "Invalid email", http.StatusBadRequest)
			return
		}
		password, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			s.Logger.Errorf("userRegister: Error generating bcrypt from password, err: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		d := database.Device{
			DeviceID:  req.DeviceID,
			FCMToken:  req.FCMToken,
			CreatedAt: primitive.NewDateTimeFromTime(time.Now()),
		}
		u := database.User{
			Name:     req.Name,
			Email:    req.Email,
			Password: password,
			Devices:  []database.Device{d},
		}

		id, err := s.DB.UserInsert(r.Context(), u)
		if err != nil {
			if mongo.IsDuplicateKeyError(err) {
				s.Logger.Debugf("userRegister: Error duplicate key when inserting User, err: %v", err)
				http.Error(w, http.StatusText(http.StatusUnprocessableEntity), http.StatusUnprocessableEntity)
				return
			}
			s.Logger.Errorf("userRegister: Error inserting User, err: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		lt, exp, tokenHash, err := s.createLoginTokenAndHash(id, req.DeviceID)
		if err != nil {
			s.Logger.Errorf("userRegister: Error creating login token for User, err: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		d.LoginToken = database.LoginToken{
			Token:      tokenHash,
			Expiration: primitive.NewDateTimeFromTime(exp),
			CreatedAt:  primitive.NewDateTimeFromTime(time.Now()),
		}
		d.LastSeen = primitive.NewDateTimeFromTime(time.Now())
		if err = s.DB.UserDeviceUpdate(r.Context(), id, d); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				s.Logger.Debugf("userRegister: Error duplicate key when updating Device on User, err: %v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			s.Logger.Errorf("userRegister: Error updating Device on User, err: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		s.writeJsonResponse(w, response{
			Success:    true,
			LoginToken: lt,
		}, http.StatusCreated)
	}
}

func (s Server) userLogin() http.HandlerFunc {
	type request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		DeviceID string `json:"device_id"`
		FCMToken string `json:"fcm_token"`
	}
	type response struct {
		LoginToken string `json:"login_token"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		req := request{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Debugf("userLogin: Error decoding JSON, err: %v", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		u, err := s.DB.UserFindByEmail(r.Context(), req.Email)
		if err != nil {
			s.Logger.Debugf("userLogin: Error finding User, err: %v", err)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		err = bcrypt.CompareHashAndPassword(u.Password, []byte(req.Password))
		if err != nil {
			s.Logger.Debugf("userLogin: Error comparing hash and password for User with email: %s, err: %v", u.Email, err)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		lt, exp, tokenHash, err := s.createLoginTokenAndHash(u.ID.Hex(), req.DeviceID)
		if err != nil {
			s.Logger.Errorf("userLogin: Error creating login token for User, err: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		var device *database.Device
		for _, d := range u.Devices {
			if d.DeviceID == req.DeviceID {
				device = &d
				break
			}
		}
		if device == nil {
			if err = s.DB.UserDeviceAdd(r.Context(), u.ID.Hex(), database.Device{
				DeviceID: req.DeviceID,
				LoginToken: database.LoginToken{
					Token:      tokenHash,
					Expiration: primitive.NewDateTimeFromTime(exp),
					CreatedAt:  primitive.NewDateTimeFromTime(time.Now()),
				},
				FCMToken: req.FCMToken,
			}); err != nil {
				if mongo.IsDuplicateKeyError(err) {
					s.Logger.Debugf("userLogin: Error duplicate key when adding Device to User, err: %v", err)
					http.Error(w, "Invalid fcm_token", http.StatusBadRequest)
					return
				}
				s.Logger.Errorf("userLogin: Error adding Device to User, err: %v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		} else {
			device.LoginToken = database.LoginToken{
				Token:      tokenHash,
				Expiration: primitive.NewDateTimeFromTime(exp),
				CreatedAt:  primitive.NewDateTimeFromTime(time.Now()),
			}
			device.FCMToken = req.FCMToken
			device.LastSeen = primitive.NewDateTimeFromTime(time.Now())
			if err = s.DB.UserDeviceUpdate(r.Context(), u.ID.Hex(), *device); err != nil {
				if mongo.IsDuplicateKeyError(err) {
					s.Logger.Debugf("userLogin: Error duplicate key when updating Device on User, err: %v", err)
					http.Error(w, "Invalid fcm_token", http.StatusBadRequest)
					return
				}
				s.Logger.Errorf("userLogin: Error updating Device on User, err: %v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		}
		s.writeJsonResponse(w, response{LoginToken: lt}, http.StatusOK)
	}
}

func (s Server) userLogout() http.HandlerFunc {
	type response struct {
		Success bool `json:"success"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		uc, err := getUserContext(r.Context())
		if err != nil {
			s.Logger.Errorf("userLogout: Error getting userContext, err: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		if err = s.DB.UserDeviceTokensRemove(r.Context(), uc.user.ID.Hex(), uc.deviceID); err != nil {
			s.Logger.Errorf("userLogout: Error removing Device tokens, err: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		s.writeJsonResponse(w, response{Success: true}, http.StatusOK)
	}
}

func (s Server) userInfo() http.HandlerFunc {
	type request struct {
		FCMToken string `json:"fcm_token"`
	}
	type response struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		uc, err := getUserContext(r.Context())
		if err != nil {
			s.Logger.Errorf("userInfo: Error getting userContext, err: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		req := request{}
		if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Debugf("userInfo: Error decoding JSON, err: %v", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		var currentFCMToken string
		for _, d := range uc.user.Devices {
			if d.DeviceID == uc.deviceID {
				currentFCMToken = d.FCMToken
			}
		}

		if req.FCMToken != currentFCMToken {
			if err = s.DB.UserDeviceFCMTokenUpdate(r.Context(), uc.user.ID.Hex(), uc.deviceID, req.FCMToken); err != nil {
				if mongo.IsDuplicateKeyError(err) {
					s.Logger.Debugf("userInfo: Error duplicate key when updating Device FCMToken, err: %v", err)
					http.Error(w, "Invalid fcm_token", http.StatusBadRequest)
					return
				}
				s.Logger.Errorf("userInfo: Error updating Device FCMToken, err: %v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		}
		s.writeJsonResponse(w, response{
			Name:  uc.user.Name,
			Email: uc.user.Email,
		}, http.StatusOK)
	}
}

func (s Server) createLoginTokenAndHash(userID string, deviceID string) (string, time.Time, []byte, error) {
	exp := time.Now().AddDate(0, 0, 90)
	salt := make([]byte, 128)
	if _, err := rand.Read(salt); err != nil {
		return "", exp, nil, errors.Wrapf(err, "error generating salt for login token for UserID: %s, DeviceID: %s", userID, deviceID)
	}
	t, err := jwt.NewBuilder().
		Subject(userID).
		Issuer("price-tracker-app").
		Expiration(exp).
		Claim("device", deviceID).
		Claim("s", base64.StdEncoding.EncodeToString(salt)).
		Build()
	if err != nil {
		return "", exp, nil, errors.Wrapf(err, "error creating login token for UserID: %s, DeviceID: %s", userID, deviceID)
	}
	lt, err := jwt.Sign(t, jwt.WithKey(jwa.HS256, s.AuthSecretKey))
	if err != nil {
		return "", exp, nil, errors.Wrapf(err, "error signing login token for UserID: %s, DeviceID: %s", userID, deviceID)
	}
	tokenHash := sha256.New()
	tokenHash.Write(lt)
	bcryptTokenHash, err := bcrypt.GenerateFromPassword(tokenHash.Sum(nil), bcrypt.DefaultCost-3)
	if err != nil {
		return "", exp, nil, errors.Wrapf(err, "error generating bcrypt from login token hash for UserID: %s, DeviceID: %s", userID, deviceID)
	}
	return string(lt), t.Expiration(), bcryptTokenHash, nil
}
