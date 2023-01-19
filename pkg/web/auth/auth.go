// SPDX-License-Identifier: GPL-2.0-or-later

package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"nvr/pkg/log"
	"nvr/pkg/storage"

	stdLog "log"
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
func LogFailedLogin(logger *log.Logger, r *http.Request, username string) {
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

	logger.Log(log.Entry{
		Level: log.LevelInfo,
		Src:   "auth",
		Msg:   fmt.Sprintf("failed login: username: %v %v\n", username, ip),
	})
}

// GenToken generates a CSRF-token.
func GenToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		stdLog.Fatalf("failed to generate random token: %v", err)
	}
	return hex.EncodeToString(b)
}

// DefaultBcryptHashCost bcrypt hash cost.
const DefaultBcryptHashCost = 10
