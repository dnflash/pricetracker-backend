package server

import (
	"context"
	"crypto/sha256"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"pricetracker/internal/database"
	"strings"
)

type userContextKey struct{}

type userContext struct {
	id     string
	name   string
	email  string
	device database.Device
}

func (s Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lt := r.Header.Get("Authorization")

		if strings.HasPrefix(lt, "Bearer ") {
			lt = strings.TrimPrefix(lt, "Bearer ")
			token, err := jwt.Parse([]byte(lt), jwt.WithKey(jwa.HS256, s.AuthSecretKey), jwt.WithValidate(true))
			if err != nil {
				s.Logger.Debugf("authMiddleware: Failed to validate login token, err: %v", err)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			deviceID, _ := token.Get("device")
			deviceIDStr, ok := deviceID.(string)
			if !ok {
				tokenMap, err := token.AsMap(r.Context())
				s.Logger.Errorf("authMiddleware: Valid token contains no device claim, token: %#v, Token.asMap err: %v", tokenMap, err)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			u, err := s.DB.UserFindByID(r.Context(), token.Subject())
			if err != nil {
				s.Logger.Debug("authMiddleware: Error finding User from login token, err:", err)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			tokenHash := sha256.New()
			tokenHash.Write([]byte(lt))
			for _, d := range u.Devices {
				if d.DeviceID != deviceIDStr {
					continue
				}

				err = bcrypt.CompareHashAndPassword(d.LoginToken.Token, tokenHash.Sum(nil))
				if err != nil {
					break
				}

				if err = s.DB.UserDeviceLastSeenUpdate(r.Context(), u.ID.Hex(), d.DeviceID); err != nil {
					s.Logger.Errorf("authMiddleware: Error updating Device LastSeen, err: %+v", err)
				}

				userCtx := userContext{
					id:     u.ID.Hex(),
					name:   u.Name,
					email:  u.Email,
					device: d,
				}
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, userCtx)))
				return
			}
		}
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
	}
}
