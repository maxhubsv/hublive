// Copyright 2024 HubLive, Inc.
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

package transport

import (
	"net/http"
)

// Transport defines a protocol that can accept client connections.
// Implement this interface to add new transport protocols.
type Transport interface {
	// Protocol returns the transport protocol name (e.g., "websocket", "whip")
	Protocol() string
	// SetupRoutes mounts HTTP routes on the provided mux.
	SetupRoutes(mux *http.ServeMux)
}
