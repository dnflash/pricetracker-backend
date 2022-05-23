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
	}
	type response struct {
		Success bool `json:"success"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		req := &request{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Debug("userRegister: Error decoding JSON, err:", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		_, err := mail.ParseAddress(req.Email)
		if err != nil {
			s.Logger.Debug("userRegister: Invalid email, err:", err)
			http.Error(w, "Invalid email", http.StatusBadRequest)
			return
		}

		password, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			s.Logger.Error("userRegister: Error generating bcrypt from password, err:", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		u := database.User{
			Name:     req.Name,
			Email:    req.Email,
			Password: password,
		}

		_, err = s.DB.UserInsert(r.Context(), u)
		if err != nil {
			if mongo.IsDuplicateKeyError(err) {
				s.Logger.Debugf("userRegister: Error duplicate key when inserting User, err: %+v", err)
				http.Error(w, "User with email: "+req.Email+" already exists", http.StatusBadRequest)
				return
			}

			s.Logger.Errorf("userRegister: Error inserting User, err: %+v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		s.writeJsonResponse(w, response{Success: true})
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
		req := &request{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Debug("userLogin: Error decoding JSON, err:", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		u, err := s.DB.UserFindByEmail(r.Context(), req.Email)
		if err != nil {
			s.Logger.Debugf("userLogin: Error finding User, err: %+v", err)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		err = bcrypt.CompareHashAndPassword(u.Password, []byte(req.Password))
		if err != nil {
			s.Logger.Debugf("userLogin: Error comparing hash and password for User with email: %s, err: %+v", u.Email, err)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		lt, exp, err := s.createLoginToken(u.ID.Hex(), req.DeviceID)
		if err != nil {
			s.Logger.Errorf("userLogin: Error creating login token for User, err: %+v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		tokenHash := sha256.New()
		tokenHash.Write([]byte(lt))
		bcryptTokenHash, err := bcrypt.GenerateFromPassword(tokenHash.Sum(nil), bcrypt.DefaultCost-3)
		if err != nil {
			s.Logger.Errorf("userLogin: Error generating bcrypt from login token hash, err: %+v", err)
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
					Token:      bcryptTokenHash,
					Expiration: primitive.NewDateTimeFromTime(exp),
					CreatedAt:  primitive.NewDateTimeFromTime(time.Now()),
				},
				FCMToken: req.FCMToken,
			}); err != nil {
				if mongo.IsDuplicateKeyError(err) {
					s.Logger.Debugf("userLogin: Error duplicate key when adding Device to User, err: %+v", err)
					http.Error(w, "Invalid fcm_token", http.StatusBadRequest)
					return
				}

				s.Logger.Errorf("userLogin: Error adding Device to User, err: %+v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		} else {
			device.LoginToken = database.LoginToken{
				Token:      bcryptTokenHash,
				Expiration: primitive.NewDateTimeFromTime(exp),
				CreatedAt:  primitive.NewDateTimeFromTime(time.Now()),
			}
			device.FCMToken = req.FCMToken
			device.LastSeen = primitive.NewDateTimeFromTime(time.Now())

			if err = s.DB.UserDeviceUpdate(r.Context(), u.ID.Hex(), *device); err != nil {
				if mongo.IsDuplicateKeyError(err) {
					s.Logger.Debugf("userLogin: Error duplicate key when updating Device on User, err: %+v", err)
					http.Error(w, "Invalid fcm_token", http.StatusBadRequest)
					return
				}

				s.Logger.Errorf("userLogin: Error updating Device on User, err: %+v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		}

		s.writeJsonResponse(w, response{LoginToken: lt})
	}
}

func (s Server) userLogout() http.HandlerFunc {
	type response struct {
		Success bool `json:"success"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		uc := r.Context().Value(userContextKey{}).(userContext)

		if err := s.DB.UserDeviceTokensRemove(r.Context(), uc.id, uc.device.DeviceID); err != nil {
			s.Logger.Errorf("userLogout: Error removing Device tokens, err: %+v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		s.writeJsonResponse(w, response{Success: true})
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
		req := request{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Debug("userInfo: Error decoding JSON, err:", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		uc := r.Context().Value(userContextKey{}).(userContext)

		if req.FCMToken != uc.device.FCMToken {
			if err := s.DB.UserDeviceFCMTokenUpdate(r.Context(), uc.id, uc.device.DeviceID, req.FCMToken); err != nil {
				if mongo.IsDuplicateKeyError(err) {
					s.Logger.Debugf("userInfo: Error duplicate key when updating Device FCMToken, err: %+v", err)
					http.Error(w, "Invalid fcm_token", http.StatusBadRequest)
					return
				}

				s.Logger.Errorf("userInfo: Error updating Device FCMToken, err: %+v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		}

		resp := response{
			Name:  uc.name,
			Email: uc.email,
		}
		s.writeJsonResponse(w, resp)
	}
}

func (s Server) createLoginToken(userID string, deviceID string) (string, time.Time, error) {
	exp := time.Now().AddDate(0, 0, 90)

	salt := make([]byte, 128)
	if _, err := rand.Read(salt); err != nil {
		return "", exp, errors.Wrapf(err, "error generating salt for login token for UserID: %s, DeviceID: %s", userID, deviceID)
	}

	t, err := jwt.NewBuilder().
		Subject(userID).
		Issuer("price-tracker-app").
		Expiration(exp).
		Claim("device", deviceID).
		Claim("s", base64.StdEncoding.EncodeToString(salt)).
		Build()
	if err != nil {
		return "", exp, errors.Wrapf(err, "error creating login token for UserID: %s, DeviceID: %s", userID, deviceID)
	}

	lt, err := jwt.Sign(t, jwt.WithKey(jwa.HS256, s.AuthSecretKey))
	if err != nil {
		return "", exp, errors.Wrapf(err, "error signing login token for UserID: %s, DeviceID: %s", userID, deviceID)
	}

	return string(lt), t.Expiration(), nil
}
