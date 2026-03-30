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

// Transition file for auth extraction (Phase 2A).
// Canonical definitions now in pkg/api/middleware/auth.go.
// These aliases keep existing service/ code working.
// Remove in Phase 8 after all callers updated.

package service

import (
	"__GITHUB_HUBLIVE__protocol/auth"

	"github.com/maxhubsv/hublive-server/pkg/api/middleware"
)

// Type alias
type APIKeyAuthMiddleware = middleware.APIKeyAuthMiddleware

// Constructor — wraps middleware.NewAPIKeyAuthMiddleware with HandleError injected
func NewAPIKeyAuthMiddleware(provider auth.KeyProvider) *APIKeyAuthMiddleware {
	return middleware.NewAPIKeyAuthMiddleware(provider, HandleError)
}

// Error aliases
var (
	ErrPermissionDenied          = middleware.ErrPermissionDenied
	ErrMissingAuthorization      = middleware.ErrMissingAuthorization
	ErrInvalidAuthorizationToken = middleware.ErrInvalidAuthorizationToken
	ErrInvalidAPIKey             = middleware.ErrInvalidAPIKey
)

// Function aliases
var (
	WithAPIKey             = middleware.WithAPIKey
	GetGrants              = middleware.GetGrants
	GetAPIKey              = middleware.GetAPIKey
	WithGrants             = middleware.WithGrants
	SetAuthorizationToken  = middleware.SetAuthorizationToken
	EnsureJoinPermission   = middleware.EnsureJoinPermission
	EnsureAdminPermission  = middleware.EnsureAdminPermission
	EnsureCreatePermission = middleware.EnsureCreatePermission
	EnsureListPermission   = middleware.EnsureListPermission
	EnsureRecordPermission = middleware.EnsureRecordPermission
	EnsureIngressAdminPermission = middleware.EnsureIngressAdminPermission
	EnsureSIPAdminPermission     = middleware.EnsureSIPAdminPermission
	EnsureSIPCallPermission      = middleware.EnsureSIPCallPermission
	EnsureDestRoomPermission     = middleware.EnsureDestRoomPermission
)

// twirpAuthError wraps authentication errors around Twirp.
// Kept as unexported since callers within service/ use it directly.
func twirpAuthError(err error) error {
	return middleware.TwirpAuthError(err)
}
