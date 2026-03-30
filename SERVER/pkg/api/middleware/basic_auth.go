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
	"net/http"
)

func GenBasicAuthMiddleware(username string, password string) func(http.ResponseWriter, *http.Request, http.HandlerFunc) {
	return func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		given_username, given_password, ok := r.BasicAuth()
		unauthorized := func() {
			rw.Header().Set("WWW-Authenticate", "Basic realm=\"Protected Area\"")
			rw.WriteHeader(http.StatusUnauthorized)
		}
		if !ok {
			unauthorized()
			return
		}

		if given_username != username {
			unauthorized()
			return
		}

		if given_password != password {
			unauthorized()
			return
		}

		next(rw, r)
	}
}
