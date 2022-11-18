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
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"testing"

	"nvr/pkg/log"
	"nvr/pkg/storage"
	"nvr/pkg/web/auth"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

var (
	pass1 = []byte("$2a$04$M0InS5zIFKk.xmjtcabjrudhKhukxJo6cnhJBq9I.J/slbgWE0F.S")
	pass2 = []byte("$2a$04$A.F3L5bXO/5nF0e6dpmqM.VuOB66.vSt6MbvWvcxeoAqqnvchBMOq")
)

func newTestAuth(t *testing.T) (string, *Authenticator, func()) {
	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)

	cancelFunc := func() {
		os.RemoveAll(tempDir)
	}

	usersPath := tempDir + "/users.json"

	users := map[string]auth.Account{
		"1": {
			ID:       "1",
			Username: "admin",
			Password: pass1,
			IsAdmin:  true,
		},
		"2": {
			ID:       "2",
			Username: "user",
			Password: pass2,
			IsAdmin:  false,
		},
	}
	data, err := json.MarshalIndent(users, "", "    ")
	require.NoError(t, err)

	err = os.WriteFile(usersPath, data, 0o600)
	require.NoError(t, err)

	auth := Authenticator{
		path:      usersPath,
		accounts:  users,
		authCache: make(map[string]auth.ValidateResponse),

		hashCost: bcrypt.MinCost,
		logger:   &log.Logger{},
	}
	return tempDir, &auth, cancelFunc
}

func clearTokens(auth *Authenticator) {
	for key, account := range auth.accounts {
		account.Token = ""
		auth.accounts[key] = account
	}
}

func TestNewBasicAuthenticator(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		tempDir, testAuth, cancel := newTestAuth(t)
		defer cancel()

		env := storage.ConfigEnv{ConfigDir: tempDir}

		a, err := NewBasicAuthenticator(env, &log.Logger{})
		require.NoError(t, err)

		auth := a.(*Authenticator)

		clearTokens(auth)

		require.Equal(t, auth.accounts, testAuth.accounts)
	})
	t.Run("readFileErr", func(t *testing.T) {
		_, err := NewBasicAuthenticator(storage.ConfigEnv{}, &log.Logger{})
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestBasicAuthenticator(t *testing.T) {
	adminExpected := auth.Account{
		ID:       "1",
		Username: "admin",
		Password: pass1,
		IsAdmin:  true,
	}
	userExpected := auth.Account{
		ID:       "2",
		Username: "user",
		Password: pass2,
		IsAdmin:  false,
	}

	t.Run("userByName", func(t *testing.T) {
		_, a, cancel := newTestAuth(t)
		defer cancel()

		cases := []struct {
			username    string
			shouldExist bool
			expected    auth.Account
		}{
			{"admin", true, adminExpected},
			{"user", true, userExpected},
			{"nil", false, auth.Account{}},
		}

		for _, tc := range cases {
			t.Run(tc.username, func(t *testing.T) {
				account, exists := a.userByNameUnsafe(tc.username)
				require.Equal(t, exists, tc.shouldExist)

				account.Token = ""
				require.Equal(t, account, tc.expected)
			})
		}
	})

	authHeader := func(auth string) *http.Request {
		return &http.Request{Header: http.Header{"Authorization": []string{auth}}}
	}

	t.Run("validateRequest", func(t *testing.T) {
		_, a, cancel := newTestAuth(t)
		defer cancel()

		cases := map[string]struct {
			username string
			password string
			valid    bool
			expected auth.Account
		}{
			"okAdmin":   {"admin", "pass1", true, adminExpected},
			"okUser":    {"user", "pass2", true, userExpected},
			"uppercase": {"User", "pass2", true, userExpected},
			"cache":     {"user", "pass2", true, userExpected},
			"wrongPass": {"user", "wrongPass", false, auth.Account{}},
			"nil":       {"nil", "", false, auth.Account{}},
		}

		for _, tc := range cases {
			t.Run(tc.username, func(t *testing.T) {
				plainAuth := tc.username + ":" + tc.password
				auth := base64.StdEncoding.EncodeToString([]byte(plainAuth))

				response := a.ValidateRequest(authHeader("Basic " + auth))
				require.Equal(t, response.IsValid, tc.valid)

				user := response.User
				user.Token = ""
				require.Equal(t, user, tc.expected)
			})
		}

		t.Run("invalid prefix", func(t *testing.T) {
			auth := base64.StdEncoding.EncodeToString([]byte("admin:pass1"))
			response := a.ValidateRequest(authHeader("nil" + auth))
			require.False(t, response.IsValid, "expected invalid response")
		})
	})

	t.Run("userList", func(t *testing.T) {
		_, a, cancel := newTestAuth(t)
		defer cancel()

		users := a.UsersList()
		expected := map[string]auth.AccountObfuscated{
			"1": {
				ID:       "1",
				Username: "admin",
				IsAdmin:  true,
			},
			"2": {
				ID:       "2",
				Username: "user",
				IsAdmin:  false,
			},
		}

		require.Equal(t, users, expected)
	})

	t.Run("userSet", func(t *testing.T) {
		cases := map[string]struct {
			req auth.SetUserRequest
			err error
		}{
			"ok": {
				auth.SetUserRequest{
					ID:            "1",
					Username:      "admin",
					PlainPassword: "",
					IsAdmin:       true,
				}, nil,
			},
			"missingPassword": {
				auth.SetUserRequest{
					ID:            "10",
					Username:      "admin",
					PlainPassword: "",
					IsAdmin:       false,
				}, ErrPasswordMissing,
			},
			"missingID": {
				auth.SetUserRequest{
					ID:            "",
					Username:      "admin",
					PlainPassword: "pass",
					IsAdmin:       false,
				}, ErrIDMissing,
			},
			"missingUsername": {
				auth.SetUserRequest{
					ID:            "1",
					Username:      "",
					PlainPassword: "pass",
					IsAdmin:       false,
				}, ErrUsernameMissing,
			},
		}
		for _, tc := range cases {
			t.Run(tc.req.Username, func(t *testing.T) {
				_, a, cancel := newTestAuth(t)
				defer cancel()

				err := a.UserSet(tc.req)
				require.ErrorIs(t, err, tc.err)

				if tc.req.ID != "" && err == nil {
					u, _ := a.userByNameUnsafe(tc.req.Username)
					require.Equal(t, u.ID, tc.req.ID, "IDs does not match")
					require.Equal(t, u.Username, tc.req.Username, "Username does not match")
					require.Equal(t, u.IsAdmin, tc.req.IsAdmin, "IsAdmin does not match")
				}
			})
		}
		t.Run("saveToFile", func(t *testing.T) {
			tempDir, a, cancel := newTestAuth(t)
			defer cancel()

			user := auth.Account{
				ID:       "10",
				Username: "a",
				Password: []byte("b"),
				IsAdmin:  true,
				Token:    "d",
			}

			req := auth.SetUserRequest{
				ID:            user.ID,
				Username:      user.Username,
				PlainPassword: "c",
				IsAdmin:       user.IsAdmin,
			}

			err := a.UserSet(req)
			require.NoError(t, err)

			file, err := fs.ReadFile(os.DirFS(tempDir), "users.json")
			require.NoError(t, err)

			var users map[string]auth.Account
			err = json.Unmarshal(file, &users)
			require.NoError(t, err)

			u := users["10"]
			u.Password = nil

			expected := auth.Account{
				ID:       "10",
				Username: "a",
				IsAdmin:  true,
			}
			require.Equal(t, u, expected)
		})
		t.Run("saveErr", func(t *testing.T) {
			_, a, cancel := newTestAuth(t)
			defer cancel()

			a.path = ""

			err := a.UserSet(auth.SetUserRequest{ID: "1", Username: "a"})
			require.Error(t, err)
		})
	})

	t.Run("userDelete", func(t *testing.T) {
		_, a, cancel := newTestAuth(t)
		defer cancel()

		t.Run("unknown user", func(t *testing.T) {
			err := a.UserDelete("nil")
			require.ErrorIs(t, err, ErrUserNotExist)
		})
		t.Run("ok", func(t *testing.T) {
			err := a.UserDelete("2")
			require.NoError(t, err)

			_, exist := a.userByNameUnsafe("")
			require.False(t, exist, "user was not deleted")
		})
		t.Run("save error", func(t *testing.T) {
			a.path = ""
			err := a.UserDelete("1")
			require.Error(t, err)
		})
	})

	// Ensure cached requests aren't blocked when hackLock is active.
	t.Run("hashLock", func(t *testing.T) {
		_, a, cancel := newTestAuth(t)
		defer cancel()

		auth := base64.StdEncoding.EncodeToString([]byte("admin:pass1"))
		req := authHeader("Basic " + auth)

		response := a.ValidateRequest(req)
		require.True(t, response.IsValid)

		a.hashLock.Lock()

		response2 := a.ValidateRequest(req)
		require.True(t, response2.IsValid)
	})
}
