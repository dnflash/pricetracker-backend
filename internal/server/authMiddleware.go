package server

import (
	"context"
	"crypto/sha256"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strings"
)

type userContextKey struct{}

type userContext struct {
	id           string
	name         string
	email        string
	loginTokenID string
}

func (s Server) authenticateUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lt := r.Header.Get("Authorization")

		if strings.HasPrefix(lt, "Bearer ") {
			lt = strings.TrimPrefix(lt, "Bearer ")
			token, err := jwt.Parse([]byte(lt), jwt.WithKey(jwa.HS256, s.AuthSecretKey), jwt.WithValidate(true))
			if err != nil {
				s.Logger.Debugf("Failed to validate login token, err: %v", err)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			u, err := s.DB.UserFindByID(r.Context(), token.Subject())
			if err != nil {
				s.Logger.Errorf("Error finding User from login token, ID: %s, err: %v", token.Subject(), err)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			tokenHash := sha256.New()
			tokenHash.Write([]byte(lt))
			for _, ult := range u.LoginTokens {
				if ult.TokenID != token.JwtID() {
					continue
				}

				err = bcrypt.CompareHashAndPassword(ult.Token, tokenHash.Sum(nil))
				if err != nil {
					break
				}

				userCtx := userContext{
					id:           u.ID.Hex(),
					name:         u.Name,
					email:        u.Email,
					loginTokenID: token.JwtID(),
				}
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, userCtx)))
				return
			}
		}
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
	}
}
