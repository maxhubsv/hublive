// Copyright 2023 HubLive, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/twitchtv/twirp"

	"__GITHUB_HUBLIVE__protocol/auth"
	"__GITHUB_HUBLIVE__protocol/hublive"
)

const (
	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "
	accessTokenParam    = "access_token"
)

type grantsKey struct{}

type grantsValue struct {
	claims *auth.ClaimGrants
	apiKey string
}

var (
	ErrPermissionDenied          = errors.New("permissions denied")
	ErrMissingAuthorization      = errors.New("invalid authorization header. Must start with " + bearerPrefix)
	ErrInvalidAuthorizationToken = errors.New("invalid authorization token")
	ErrInvalidAPIKey             = errors.New("invalid API key")
)

// ErrorHandler is a function that handles HTTP errors.
// Inject service.HandleError to avoid circular imports.
type ErrorHandler func(w http.ResponseWriter, r *http.Request, status int, err error, keysAndValues ...any)

// authentication middleware
type APIKeyAuthMiddleware struct {
	provider     auth.KeyProvider
	errorHandler ErrorHandler
}

func NewAPIKeyAuthMiddleware(provider auth.KeyProvider, errorHandler ErrorHandler) *APIKeyAuthMiddleware {
	return &APIKeyAuthMiddleware{
		provider:     provider,
		errorHandler: errorHandler,
	}
}

func (m *APIKeyAuthMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if r.URL != nil && (r.URL.Path == "/rtc/validate" || r.URL.Path == "/rtc/v1/validate") {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	authHeader := r.Header.Get(authorizationHeader)
	var authToken string

	if authHeader != "" {
		if !strings.HasPrefix(authHeader, bearerPrefix) {
			m.errorHandler(w, r, http.StatusUnauthorized, ErrMissingAuthorization)
			return
		}

		authToken = authHeader[len(bearerPrefix):]
	} else {
		// attempt to find from request header
		authToken = r.FormValue(accessTokenParam)
	}

	if authToken != "" {
		v, err := auth.ParseAPIToken(authToken)
		if err != nil {
			m.errorHandler(w, r, http.StatusUnauthorized, ErrInvalidAuthorizationToken)
			return
		}

		secret := m.provider.GetSecret(v.APIKey())
		if secret == "" {
			m.errorHandler(w, r, http.StatusUnauthorized, errors.New("invalid API key: "+v.APIKey()))
			return
		}

		_, grants, err := v.Verify(secret)
		if err != nil {
			m.errorHandler(w, r, http.StatusUnauthorized, errors.New("invalid token: "+authToken+", error: "+err.Error()))
			return
		}

		// set grants in context
		ctx := r.Context()
		r = r.WithContext(context.WithValue(ctx, grantsKey{}, &grantsValue{
			claims: grants,
			apiKey: v.APIKey(),
		}))
	}

	next.ServeHTTP(w, r)
}

func WithAPIKey(ctx context.Context, grants *auth.ClaimGrants, apiKey string) context.Context {
	return context.WithValue(ctx, grantsKey{}, &grantsValue{
		claims: grants,
		apiKey: apiKey,
	})
}

func GetGrants(ctx context.Context) *auth.ClaimGrants {
	val := ctx.Value(grantsKey{})
	v, ok := val.(*grantsValue)
	if !ok {
		return nil
	}
	return v.claims
}

func GetAPIKey(ctx context.Context) string {
	val := ctx.Value(grantsKey{})
	v, ok := val.(*grantsValue)
	if !ok {
		return ""
	}
	return v.apiKey
}

func WithGrants(ctx context.Context, grants *auth.ClaimGrants, apiKey string) context.Context {
	return context.WithValue(ctx, grantsKey{}, &grantsValue{
		claims: grants,
		apiKey: apiKey,
	})
}

func SetAuthorizationToken(r *http.Request, token string) {
	r.Header.Set(authorizationHeader, bearerPrefix+token)
}

func EnsureJoinPermission(ctx context.Context) (name hublive.RoomName, err error) {
	claims := GetGrants(ctx)
	if claims == nil || claims.Video == nil {
		err = ErrPermissionDenied
		return
	}

	if claims.Video.RoomJoin {
		name = hublive.RoomName(claims.Video.Room)
	} else {
		err = ErrPermissionDenied
	}
	return
}

func EnsureAdminPermission(ctx context.Context, room hublive.RoomName) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.Video == nil {
		return ErrPermissionDenied
	}

	if !claims.Video.RoomAdmin || room != hublive.RoomName(claims.Video.Room) {
		return ErrPermissionDenied
	}

	return nil
}

func EnsureCreatePermission(ctx context.Context) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.Video == nil || !claims.Video.RoomCreate {
		return ErrPermissionDenied
	}
	return nil
}

func EnsureListPermission(ctx context.Context) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.Video == nil || !claims.Video.RoomList {
		return ErrPermissionDenied
	}
	return nil
}

func EnsureRecordPermission(ctx context.Context) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.Video == nil || !claims.Video.RoomRecord {
		return ErrPermissionDenied
	}
	return nil
}

func EnsureIngressAdminPermission(ctx context.Context) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.Video == nil || !claims.Video.IngressAdmin {
		return ErrPermissionDenied
	}
	return nil
}

func EnsureSIPAdminPermission(ctx context.Context) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.SIP == nil || !claims.SIP.Admin {
		return ErrPermissionDenied
	}
	return nil
}

func EnsureSIPCallPermission(ctx context.Context) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.SIP == nil || !claims.SIP.Call {
		return ErrPermissionDenied
	}
	return nil
}

func EnsureDestRoomPermission(ctx context.Context, source hublive.RoomName, destination hublive.RoomName) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.Video == nil {
		return ErrPermissionDenied
	}

	if !claims.Video.RoomAdmin || source != hublive.RoomName(claims.Video.Room) || destination != hublive.RoomName(claims.Video.DestinationRoom) {
		return ErrPermissionDenied
	}

	return nil
}

// TwirpAuthError wraps authentication errors around Twirp
func TwirpAuthError(err error) error {
	return twirp.NewError(twirp.Unauthenticated, err.Error())
}
