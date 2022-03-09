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

package basic

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdLog "log"
	"net/http"
	"nvr"
	"nvr/pkg/log"
	"nvr/pkg/storage"
	"nvr/pkg/web/auth"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

func init() {
	nvr.SetAuthenticator(NewBasicAuthenticator)
}

// DefaultHashCost bcrypt hash cost.
const DefaultHashCost = 10

// Authenticator implements auth.Authenticator.
type Authenticator struct {
	path      string // Path to save user information.
	accounts  map[string]auth.Account
	authCache map[string]auth.ValidateRes

	hashCost int

	log *log.Logger
	mu  sync.Mutex
}

// NewBasicAuthenticator creates basic authenticator.
func NewBasicAuthenticator(env storage.ConfigEnv, logger *log.Logger) (auth.Authenticator, error) {
	path := filepath.Join(env.ConfigDir, "users.json")
	a := Authenticator{
		path:      path,
		accounts:  make(map[string]auth.Account),
		authCache: make(map[string]auth.ValidateRes),

		hashCost: DefaultHashCost,
		log:      logger,
	}

	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(file, &a.accounts)
	if err != nil {
		return nil, err
	}

	a.resetTokens()

	return &a, nil
}

// ValidateRequest Should always take the same amount of
// time to run, even when username or password is invalid.
func (a *Authenticator) ValidateRequest(r *http.Request) auth.ValidateRes {
	req := r.Header.Get("Authorization")
	defer a.mu.Unlock()
	a.mu.Lock()
	if _, cacheExist := a.authCache[req]; cacheExist {
		return a.authCache[req]
	}
	a.mu.Unlock()

	name, pass := parseBasicAuth(req)
	user, found := a.userByName(name)

	res := auth.ValidateRes{}

	if !found || name != user.Username {
		// Generate fake hash to prevent timing based attacks.
		bcrypt.GenerateFromPassword([]byte(name), a.hashCost) //nolint:errcheck
	} else if passwordsMatch(user.Password, pass) {
		res = auth.ValidateRes{IsValid: true, User: user}
	}
	a.mu.Lock()

	a.authCache[req] = res
	return a.authCache[req]
}

func (a *Authenticator) userByName(name string) (auth.Account, bool) {
	defer a.mu.Unlock()
	a.mu.Lock()

	users := a.accounts
	for _, u := range users {
		if u.Username == name {
			return u, true
		}
	}
	return auth.Account{}, false
}

// Modified from net/http. Link:
// https://cs.opensource.google/go/go/+/refs/tags/go1.17.8:src/net/http/request.go;l=949
func parseBasicAuth(str string) (username, password string) {
	const prefix = "Basic "
	if len(str) < len(prefix) || !strings.EqualFold(str[:len(prefix)], prefix) {
		return
	}
	c, err := base64.StdEncoding.DecodeString(str[len(prefix):])
	if err != nil {
		return
	}
	cs := string(c)
	s := strings.IndexByte(cs, ':')
	if s < 0 {
		return
	}
	return cs[:s], cs[s+1:]
}

func passwordsMatch(hash []byte, plaintext string) bool {
	if err := bcrypt.CompareHashAndPassword(hash, []byte(plaintext)); err != nil {
		return false
	}
	return true
}

func genToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		stdLog.Fatal("failed to generate random token")
	}
	return hex.EncodeToString(b)
}

// resetTokens creates new random token for each user.
func (a *Authenticator) resetTokens() {
	a.mu.Lock()
	for id, user := range a.accounts {
		user.Token = genToken()
		a.accounts[id] = user
	}
	a.mu.Unlock()
}

// UsersList returns a obfuscated user list.
func (a *Authenticator) UsersList() map[string]auth.AccountObfuscated {
	defer a.mu.Unlock()
	a.mu.Lock()

	list := make(map[string]auth.AccountObfuscated)
	for id, user := range a.accounts {
		list[id] = auth.AccountObfuscated{
			ID:       user.ID,
			Username: user.Username,
			IsAdmin:  user.IsAdmin,
		}
	}
	return list
}

// Errors.
var (
	ErrIDMissing       = errors.New("missing ID")
	ErrUsernameMissing = errors.New("missing username")
	ErrPasswordMissing = errors.New("password is required for new users")
	ErrUserNotExist    = errors.New("user does not exist")
)

// UserSet set user details.
func (a *Authenticator) UserSet(req auth.SetUserRequest) error {
	defer a.mu.Unlock()
	a.mu.Lock()

	if req.ID == "" {
		return ErrIDMissing
	}

	if req.Username == "" {
		return ErrUsernameMissing
	}

	_, exists := a.accounts[req.ID]
	if !exists && req.PlainPassword == "" {
		return ErrPasswordMissing
	}

	user := a.accounts[req.ID]
	a.mu.Unlock()

	user.ID = req.ID
	user.Username = req.Username
	user.IsAdmin = req.IsAdmin
	if req.PlainPassword != "" {
		hashedNewPassword, _ := bcrypt.GenerateFromPassword([]byte(req.PlainPassword), a.hashCost)
		user.Password = hashedNewPassword
	}
	user.Token = genToken()

	a.mu.Lock()
	a.accounts[user.ID] = user
	a.authCache = make(map[string]auth.ValidateRes)

	if err := a.SaveUsersToFile(); err != nil {
		return fmt.Errorf("could not save users to file: %w", err)
	}

	return nil
}

// UserDelete deletes user by id.
func (a *Authenticator) UserDelete(id string) error {
	defer a.mu.Unlock()
	a.mu.Lock()
	if _, exists := a.accounts[id]; !exists {
		return ErrUserNotExist
	}
	delete(a.accounts, id)

	// Reset cache.
	a.authCache = make(map[string]auth.ValidateRes)

	if err := a.SaveUsersToFile(); err != nil {
		return err
	}

	return nil
}

// SaveUsersToFile saves json file.
func (a *Authenticator) SaveUsersToFile() error {
	users, _ := json.MarshalIndent(a.accounts, "", "  ")

	err := os.WriteFile(a.path, users, 0o600)
	if err != nil {
		return err
	}

	return nil
}

// User blocks unauthorized requests and prompts for login.
func (a *Authenticator) User(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := a.ValidateRequest(r)
		if !res.IsValid {
			if r.Header.Get("Authorization") != "" {
				username, _ := parseBasicAuth(r.Header.Get("Authorization"))
				auth.LogFailedLogin(a.log, r, username)
			}
			w.Header().Set("WWW-Authenticate", `Basic realm=""`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Admin blocks requests from non-admin users.
func (a *Authenticator) Admin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := a.ValidateRequest(r)

		if !res.IsValid || !res.User.IsAdmin {
			if r.Header.Get("Authorization") != "" {
				username, _ := parseBasicAuth(r.Header.Get("Authorization"))
				auth.LogFailedLogin(a.log, r, username)
			}

			w.Header().Set("WWW-Authenticate", `Basic realm="NVR"`)
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// CSRF blocks invalid Cross-site request forgery tokens.
// Each user has a unique token. The request needs to
// have a matching token in the "X-CSRF-TOKEN" header.
func (a *Authenticator) CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := a.ValidateRequest(r)
		token := r.Header.Get("X-CSRF-TOKEN")

		if token != auth.User.Token {
			http.Error(w, "Invalid CSRF-token.", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// MyToken return CSRF token for requesting user.
func (a *Authenticator) MyToken() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		auth := a.ValidateRequest(r)
		token := auth.User.Token
		if token == "" {
			http.Error(w, "token does not exist", http.StatusInternalServerError)
			return
		}
		if _, err := w.Write([]byte(token)); err != nil {
			http.Error(w, "could not write", http.StatusInternalServerError)
			return
		}
	})
}

// Logout prompts for login and redirects. Old login should be overwritten.
func (a *Authenticator) Logout() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Basic Og==":
		case "":
		default:
			w.Header().Set("WWW-Authenticate", `Basic realm=""`)
			http.Error(w, "", http.StatusUnauthorized)
			return
		}

		if _, err := io.WriteString(w, redirect); err != nil {
			http.Error(w, "could not write string", http.StatusInternalServerError)
			return
		}
	})
}

const redirect = `
	<head><script>
		window.location.href = window.location.href.replace("logout", "live")
	</script></head>`
