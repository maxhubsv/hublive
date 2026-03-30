// Transition file for basic auth extraction (Phase 2A).
// Canonical definition now in pkg/api/middleware/basic_auth.go.
// Remove in Phase 8 after all callers updated.

package service

import (
	"github.com/maxhubsv/hublive-server/pkg/api/middleware"
)

// Deprecated: Use middleware.GenBasicAuthMiddleware from pkg/api/middleware
var GenBasicAuthMiddleware = middleware.GenBasicAuthMiddleware
