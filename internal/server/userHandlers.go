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
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		_, err := mail.ParseAddress(req.Email)
		if err != nil {
			http.Error(w, "Email invalid", http.StatusBadRequest)
			return
		}

		password, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			s.Logger.Error("Error generating bcrypt from password", err)
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
				s.Logger.Debugf("Error duplicate key when inserting User with email: %s, err: %+v", req.Email, err)
				http.Error(w, "User with email: "+req.Email+" already exists", http.StatusBadRequest)
				return
			}

			s.Logger.Errorf("Error inserting User with email: %s, err: %+v", req.Email, err)
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
	}
	type response struct {
		LoginToken string `json:"login_token"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		req := &request{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		u, err := s.DB.UserFindByEmail(r.Context(), req.Email)
		if err != nil {
			s.Logger.Debugf("Error finding User with email: %s, err: %+v", err)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		err = bcrypt.CompareHashAndPassword(u.Password, []byte(req.Password))
		if err != nil {
			s.Logger.Debugf("Error logging in User with email: %s, err: %+v", u.Email, err)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		tID, lt, exp, err := s.createLoginToken(u.ID.Hex())
		if err != nil {
			s.Logger.Errorf("Error creating login token for User with email: %s, err: %+v", u.Email, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		tokenHash := sha256.New()
		tokenHash.Write([]byte(lt))
		bcryptTokenHash, err := bcrypt.GenerateFromPassword(tokenHash.Sum(nil), bcrypt.DefaultCost-3)
		if err != nil {
			s.Logger.Error("Error generating bcrypt from login token hash", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		ult := database.LoginToken{
			TokenID:    tID,
			Token:      bcryptTokenHash,
			Expiration: primitive.NewDateTimeFromTime(exp),
		}

		if err = s.DB.UserAddLoginToken(r.Context(), u.ID.Hex(), ult); err != nil {
			s.Logger.Errorf("Error adding login token for User with email: %s, err: %+v", u.Email, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
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

		if err := s.DB.UserRemoveLoginToken(r.Context(), uc.id, uc.loginTokenID); err != nil {
			s.Logger.Errorf("Error removing login token from User with ID: %s, err: %+v", uc.id, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		s.writeJsonResponse(w, response{Success: true})
	}
}

func (s Server) userInfo() http.HandlerFunc {
	type response struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		uc := r.Context().Value(userContextKey{}).(userContext)
		resp := response{
			Name:  uc.name,
			Email: uc.email,
		}
		s.writeJsonResponse(w, resp)
	}
}

func (s Server) createLoginToken(userID string) (string, string, time.Time, error) {
	exp := time.Now().AddDate(0, 0, 90)

	tokenID := make([]byte, 16)
	_, err := rand.Read(tokenID)
	if err != nil {
		return "", "", exp, errors.Wrapf(err, "error generating token ID for login token for User with ID: %s", userID)
	}

	salt := make([]byte, 128)
	_, err = rand.Read(salt)
	if err != nil {
		return "", "", exp, errors.Wrapf(err, "error generating salt for login token for User with ID: %s", userID)
	}

	t, err := jwt.NewBuilder().
		Subject(userID).
		Issuer("price-tracker-app").
		Expiration(exp).
		JwtID(base64.StdEncoding.EncodeToString(tokenID)).
		Claim("s", base64.StdEncoding.EncodeToString(salt)).
		Build()
	if err != nil {
		return "", "", exp, errors.Wrapf(err, "error creating login token for User with ID: %s", userID)
	}

	lt, err := jwt.Sign(t, jwt.WithKey(jwa.HS256, s.AuthSecretKey))
	if err != nil {
		return "", "", exp, errors.Wrapf(err, "error signing login token for User with ID: %s", userID)
	}

	return t.JwtID(), string(lt), t.Expiration(), nil
}
