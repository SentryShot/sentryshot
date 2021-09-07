// Copyright 2020-2021 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; version 2.
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
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	stdLog "log"
	"net/http"
	"nvr/pkg/log"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// Account contains user information.
type Account struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Password    []byte `json:"password,omitempty"`    // Hashed password.
	RawPassword string `json:"rawPassword,omitempty"` // Plaintext password only used when changing password
	IsAdmin     bool   `json:"isAdmin"`
	Token       string `json:"-"` // CSRF token.
}

// Response is returned by ValidateAuth.
type Response struct {
	IsValid bool
	User    Account
}

// Authenticator authenticates http requests.
type Authenticator struct {
	path      string // Path to save file.
	accounts  map[string]Account
	authCache map[string]Response

	hashCost int

	log *log.Logger
	mu  sync.Mutex
}

const defaultHashCost = 10

// NewBasicAuthenticator returns authenticator using basicAuth.
func NewBasicAuthenticator(path string, logger *log.Logger) (*Authenticator, error) {
	a := Authenticator{
		path:      path,
		accounts:  make(map[string]Account),
		authCache: make(map[string]Response),

		hashCost: defaultHashCost,
		log:      logger,
	}

	file, err := ioutil.ReadFile(path)
	if err != nil {
		return &Authenticator{}, err
	}

	json.Unmarshal(file, &a.accounts) // nolint:errcheck

	a.resetTokens()

	return &a, nil
}

func (a *Authenticator) userByName(name string) (Account, bool) {
	defer a.mu.Unlock()
	a.mu.Lock()

	users := a.accounts
	for _, u := range users {
		if u.Username == name {
			return u, true
		}
	}
	return Account{}, false
}

// ValidateAuth Should always take about the same amount of
// time to run, even when username or password is invalid.
func (a *Authenticator) ValidateAuth(auth string) Response {
	defer a.mu.Unlock()
	a.mu.Lock()
	if _, cacheExist := a.authCache[auth]; cacheExist {
		return a.authCache[auth]
	}
	a.mu.Unlock()

	name, pass := parseBasicAuth(auth)
	user, found := a.userByName(name)

	var r = Response{}
	if !found || name != user.Username {
		// Generate fake hash to prevent timing based attacks.
		bcrypt.GenerateFromPassword([]byte(name), a.hashCost) //nolint:errcheck
	} else if passwordsMatch(user.Password, pass) {
		r = Response{IsValid: true, User: user}
	}
	a.mu.Lock()

	a.authCache[auth] = r
	return a.authCache[auth]
}

func (a *Authenticator) logFailedLogin(r *http.Request) {
	username, _ := parseBasicAuth(r.Header.Get("Authorization"))

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
	a.log.Info().Src("auth").Msgf("failed login: username: %v %v\n", username, ip)
}

// Modified from net/http https://golang.org/src/net/http/request.go?s=30968:31034#L912
func parseBasicAuth(auth string) (username, password string) {
	const prefix = "Basic "
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return
	}
	c, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
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

func (a *Authenticator) resetTokens() {
	a.mu.Lock()
	for id, user := range a.accounts {
		user.Token = genToken()
		a.accounts[id] = user
	}
	a.mu.Unlock()
}

// UsersList returns a censored user list.
func (a *Authenticator) UsersList() map[string]Account {
	defer a.mu.Unlock()
	a.mu.Lock()

	u := make(map[string]Account)
	for id, user := range a.accounts {
		u[id] = Account{
			ID:       user.ID,
			Username: user.Username,
			IsAdmin:  user.IsAdmin,
		}
	}
	return u
}

// UserSet set user details.
func (a *Authenticator) UserSet(newUser Account) error {
	defer a.mu.Unlock()
	a.mu.Lock()

	if newUser.ID == "" {
		return errors.New("missing id")
	}

	if newUser.Username == "" {
		return errors.New("missing username")
	}

	_, exists := a.accounts[newUser.ID]
	if !exists && newUser.RawPassword == "" {
		return errors.New("password required for new users")
	}

	user := a.accounts[newUser.ID]
	a.mu.Unlock()

	user.ID = newUser.ID
	user.Username = newUser.Username
	user.IsAdmin = newUser.IsAdmin
	if newUser.RawPassword != "" {
		hashedNewPassword, _ := bcrypt.GenerateFromPassword([]byte(newUser.RawPassword), a.hashCost)
		user.Password = hashedNewPassword
	}
	user.Token = genToken()

	a.mu.Lock()
	a.accounts[user.ID] = user
	a.authCache = make(map[string]Response)

	if err := a.SaveUsersToFile(); err != nil {
		return fmt.Errorf("could not save users to file: %v", err)
	}

	return nil
}

// UserDelete deletes user by id.
func (a *Authenticator) UserDelete(id string) error {
	defer a.mu.Unlock()
	a.mu.Lock()
	if _, exists := a.accounts[id]; !exists {
		return errors.New("user does not exist")
	}
	delete(a.accounts, id)

	a.authCache = make(map[string]Response)
	if err := a.SaveUsersToFile(); err != nil {
		return err
	}

	return nil
}

// SaveUsersToFile saves json file.
func (a *Authenticator) SaveUsersToFile() error {
	users, _ := json.MarshalIndent(a.accounts, "", "  ")

	err := ioutil.WriteFile(a.path, users, 0600)
	if err != nil {
		return err
	}

	return nil
}

// User blocks unauthorized requests and prompts for login.
func (a *Authenticator) User(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := a.ValidateAuth(r.Header.Get("Authorization"))
		if !auth.IsValid {
			if r.Header.Get("Authorization") != "" {
				a.logFailedLogin(r)
			}

			w.Header().Set("WWW-Authenticate", `Basic realm=""`)
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Admin blocks requests from non-admin users.
func (a *Authenticator) Admin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := a.ValidateAuth(r.Header.Get("Authorization"))

		if !auth.IsValid || !auth.User.IsAdmin {
			if r.Header.Get("Authorization") != "" {
				a.logFailedLogin(r)
			}

			w.Header().Set("WWW-Authenticate", `Basic realm="NVR"`)
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// CSRF blocks invalid Cross-site request forgery tokens.
// Each user has a unique token, http request needs to
// have a matching token in the "X-CSRF-TOKEN" header.
func (a *Authenticator) CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := a.ValidateAuth(r.Header.Get("Authorization"))
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
		auth := a.ValidateAuth(r.Header.Get("Authorization"))
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
