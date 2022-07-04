package server

import (
	"context"
	"crypto/sha256"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"pricetracker/internal/model"
	"runtime/debug"
	"strings"
	"time"
)

type userContextKey struct{}
type userContext struct {
	user     model.User
	deviceID string
}

type traceContextKey struct{}
type traceContext struct {
	traceID string
}

func setUserContext(ctx context.Context, uc userContext) context.Context {
	return context.WithValue(ctx, userContextKey{}, uc)
}
func getUserContext(ctx context.Context) (userContext, error) {
	uc, ok := ctx.Value(userContextKey{}).(userContext)
	if !ok {
		return uc, errors.New("failed to get UserContext")
	}
	return uc, nil
}

func setTraceContext(ctx context.Context, tc traceContext) context.Context {
	return context.WithValue(ctx, traceContextKey{}, tc)
}
func getTraceContext(ctx context.Context) traceContext {
	tc, _ := ctx.Value(traceContextKey{}).(traceContext)
	return tc
}

func (s Server) maxBytesMw(next http.Handler) http.Handler {
	return http.MaxBytesHandler(next, 3000)
}

func (s Server) loggingMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		traceID := uuid.NewString()
		s.Logger.Debugf("loggingMw: New incoming request %s %s from %s, UA: %s, Host: %#v, TraceID: %s",
			r.Method, r.URL.Path, r.RemoteAddr, r.UserAgent(), r.Host, traceID)

		defer func() {
			if re := recover(); re != nil {
				s.Logger.Errorf("loggingMw: Handler crashed, err: %v, TraceID: %s, stack trace:\n%s", re, traceID, debug.Stack())
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()

		tc := traceContext{traceID: traceID}
		next.ServeHTTP(w, r.WithContext(setTraceContext(r.Context(), tc)))

		s.Logger.Tracef("loggingMw: Incoming request %s %s took %dms, TraceID: %s",
			r.Method, r.URL.Path, time.Now().Sub(start).Milliseconds(), traceID)
	})
}

func (s Server) authMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tid := getTraceContext(r.Context()).traceID
		lt := r.Header.Get("Authorization")
		if strings.HasPrefix(lt, "Bearer ") {
			lt = strings.TrimPrefix(lt, "Bearer ")
			token, err := jwt.Parse([]byte(lt), jwt.WithKey(jwa.HS256, s.AuthSecretKey), jwt.WithValidate(true))
			if err != nil {
				s.Logger.Debugf("authMw: Failed to validate login token, err: %v, TraceID: %s", err, tid)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			deviceID, _ := token.Get("device")
			deviceIDStr, ok := deviceID.(string)
			if !ok {
				tokenMap, err := token.AsMap(r.Context())
				s.Logger.Errorf("authMw: Valid token contains no device claim, token: %#v, Token.asMap err: %v, TraceID: %s", tokenMap, err, tid)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			u, err := s.DB.UserFindByID(r.Context(), token.Subject())
			if err != nil {
				s.Logger.Debugf("authMw: Error finding User from login token, err: %v, TraceID: %s", err, tid)
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
					s.Logger.Debugf("authMw: Error when comparing LoginToken hashes for UserID: %s, DeviceID: %s, err: %v, TraceID: %s",
						u.ID.Hex(), d.DeviceID, err, tid)
					break
				}

				s.Logger.Debugf("authMw: UserID: %s, DeviceID: %s, TraceID: %s", u.ID.Hex(), d.DeviceID, tid)

				if err = s.DB.UserDeviceLastSeenUpdate(r.Context(), u.ID.Hex(), d.DeviceID); err != nil {
					s.Logger.Errorf("authMw: Error updating Device LastSeen, err: %v, TraceID: %s", err, tid)
				}

				uc := userContext{
					user:     u,
					deviceID: d.DeviceID,
				}
				next.ServeHTTP(w, r.WithContext(setUserContext(r.Context(), uc)))
				return
			}
		}
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
	})
}
