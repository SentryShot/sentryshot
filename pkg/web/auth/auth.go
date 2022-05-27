// Copyright 2020-2022 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; either version 2 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package auth

import (
	"net/http"
	"nvr/pkg/log"
	"nvr/pkg/storage"
)

// Account contains user information.
type Account struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Password []byte `json:"password"` // Hashed password.
	IsAdmin  bool   `json:"isAdmin"`
	Token    string `json:"-"` // CSRF token.
}

// AccountObfuscated Account without sensitive information.
type AccountObfuscated struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	IsAdmin  bool   `json:"isAdmin"`
}

// ValidateResponse ValidateRequest response.
type ValidateResponse struct {
	IsValid bool
	User    Account
}

// SetUserRequest set user details request.
type SetUserRequest struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	PlainPassword string `json:"plainPassword,omitempty"`
	IsAdmin       bool   `json:"isAdmin"`
}

// NewAuthenticatorFunc function to create authenticator.
type NewAuthenticatorFunc func(storage.ConfigEnv, *log.Logger) (Authenticator, error)

// Authenticator is responsible for blocking all
// unauthenticated requests and storing user information.
type Authenticator interface {
	// ValidateRequest validates raw http requests.
	ValidateRequest(*http.Request) ValidateResponse

	// AuthDisabled if all requests should be allowed.
	AuthDisabled() bool
	// UsersList returns a obfuscated user list.
	UsersList() map[string]AccountObfuscated
	// UserSet sets the information of a user.
	UserSet(SetUserRequest) error
	// UserDelete deletes a user by id.
	UserDelete(string) error

	// Handler wrappers.
	// User blocks unauthenticated requests.
	User(http.Handler) http.Handler
	// Admin only allows authenticated requests from users with admin privileges.
	Admin(http.Handler) http.Handler
	// CSRF blocks invalid Cross-site request forgery tokens.
	// Each user has a unique token. The request needs to
	// have a matching token in the "X-CSRF-TOKEN" header.
	CSRF(http.Handler) http.Handler

	// Handlers.
	MyToken() http.Handler
	Logout() http.Handler
}

// LogFailedLogin finds and logs the ip.
func LogFailedLogin(log *log.Logger, r *http.Request, username string) {
	ip := ""
	realIP := r.Header.Get("X-Real-Ip")
	if realIP != "" {
		ip += "real:" + realIP + " "
	}
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" && forwarded != realIP {
		ip += "forwarded:" + forwarded + " "
	}
	remoteAddr := r.RemoteAddr
	if remoteAddr != "" && remoteAddr != forwarded {
		ip += "addr:" + remoteAddr
	}

	log.Info().Src("auth").Msgf("failed login: username: %v %v\n", username, ip)
}

// DefaultBcryptHashCost bcrypt hash cost.
const DefaultBcryptHashCost = 10
