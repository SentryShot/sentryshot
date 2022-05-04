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

package disable

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"nvr"
	"nvr/pkg/log"
	"nvr/pkg/storage"
	"nvr/pkg/web/auth"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// DefaultHashCost bcrypt hash cost.
const DefaultHashCost = 10

func init() {
	nvr.SetAuthenticator(NewAuthenticator)

	// Remove logout button.
	nvr.RegisterTplSubHook(func(pageFiles map[string]string) error {
		re := regexp.MustCompile(`<div id="logout">(\n.*){6}`)

		pageFiles["sidebar.tpl"] = re.ReplaceAllString(pageFiles["sidebar.tpl"], "")

		return nil
	})
}

// Authenticator implements Authenticator.
type Authenticator struct {
	path     string // Path to save user information.
	accounts map[string]auth.Account
	hashCost int

	mu sync.Mutex
}

// NewAuthenticator creates a authenticator similar to
// basic.Authenticator but it allows all requests.
func NewAuthenticator(env storage.ConfigEnv, logger *log.Logger) (auth.Authenticator, error) {
	path := filepath.Join(env.ConfigDir, "users.json")
	a := Authenticator{
		path:     path,
		accounts: make(map[string]auth.Account),

		hashCost: DefaultHashCost,
	}

	file, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			a.accounts = make(map[string]auth.Account)
			return &a, nil
		}
		return nil, err
	}

	err = json.Unmarshal(file, &a.accounts)
	if err != nil {
		return nil, err
	}

	return &a, nil
}

// ValidateRequest allows all requests.
func (a *Authenticator) ValidateRequest(r *http.Request) auth.ValidateRes {
	return auth.ValidateRes{
		IsValid: true,
		User: auth.Account{
			ID:       "none",
			Username: "noAuth",
			IsAdmin:  true,
		},
	}
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

	a.mu.Lock()
	a.accounts[user.ID] = user

	if err := a.SaveUsersToFile(); err != nil {
		return fmt.Errorf("could not save users to file: %w", err)
	}

	return nil
}

// UserDelete allows basic auth users to be deleted.
func (a *Authenticator) UserDelete(id string) error {
	defer a.mu.Unlock()
	a.mu.Lock()
	if _, exists := a.accounts[id]; !exists {
		return ErrUserNotExist
	}
	delete(a.accounts, id)

	if err := a.SaveUsersToFile(); err != nil {
		return err
	}

	return nil
}

// User allows all requests.
func (a *Authenticator) User(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

// Admin allows all requests.
func (a *Authenticator) Admin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

// CSRF doesn't check anything.
func (a *Authenticator) CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

// MyToken returns unused token.
func (a *Authenticator) MyToken() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("disable")); err != nil {
			http.Error(w, "could not write", http.StatusInternalServerError)
			return
		}
	})
}

// Logout returns 404.
func (a *Authenticator) Logout() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
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
