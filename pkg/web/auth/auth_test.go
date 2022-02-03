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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"nvr/pkg/log"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
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

	users := map[string]Account{
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
		authCache: make(map[string]Response),

		hashCost: defaultHashCost,
		log:      &log.Logger{},
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

		auth, err := NewBasicAuthenticator(tempDir+"/users.json", &log.Logger{})
		require.NoError(t, err)

		clearTokens(auth)

		require.Equal(t, auth.accounts, testAuth.accounts)
	})
	t.Run("readFile error", func(t *testing.T) {
		_, err := NewBasicAuthenticator("nil", &log.Logger{})
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestBasicAuthenticator(t *testing.T) {
	adminExpected := Account{
		ID:       "1",
		Username: "admin",
		Password: pass1,
		IsAdmin:  true,
	}
	userExpected := Account{
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
			expected    Account
		}{
			{"admin", true, adminExpected},
			{"user", true, userExpected},
			{"nil", false, Account{}},
		}

		for _, tc := range cases {
			t.Run(tc.username, func(t *testing.T) {
				account, exists := a.userByName(tc.username)
				require.Equal(t, exists, tc.shouldExist)

				account.Token = ""
				require.Equal(t, account, tc.expected)
			})
		}
	})

	t.Run("validateAuth", func(t *testing.T) {
		_, a, cancel := newTestAuth(t)
		defer cancel()

		a.hashCost = 4

		cases := []struct {
			username string
			password string
			valid    bool
			expected Account
		}{
			{"admin", "pass1", true, adminExpected},
			{"user", "pass2", true, userExpected},
			{"user", "pass2", true, userExpected}, // test cache
			{"user", "wrongPass", false, Account{}},
			{"nil", "", false, Account{}},
		}

		for _, tc := range cases {
			t.Run(tc.username, func(t *testing.T) {
				plainAuth := tc.username + ":" + tc.password
				auth := base64.StdEncoding.EncodeToString([]byte(plainAuth))

				response := a.ValidateAuth("Basic " + auth)
				require.Equal(t, response.IsValid, tc.valid)

				user := response.User
				user.Token = ""
				require.Equal(t, user, tc.expected)
			})
		}

		t.Run("invalid prefix", func(t *testing.T) {
			auth := base64.StdEncoding.EncodeToString([]byte("admin:pass1"))
			response := a.ValidateAuth("nil" + auth)
			require.False(t, response.IsValid, "expected invalid response")
		})
		t.Run("invalid base64", func(t *testing.T) {
			response := a.ValidateAuth("Basic nil")
			require.False(t, response.IsValid, "expected invalid response")
		})
		t.Run("invalid auth", func(t *testing.T) {
			auth := base64.StdEncoding.EncodeToString([]byte("admin@pass1"))
			response := a.ValidateAuth("Basic " + auth)
			require.False(t, response.IsValid, "expected invalid response")
		})
	})

	t.Run("userList", func(t *testing.T) {
		_, a, cancel := newTestAuth(t)
		defer cancel()

		users := a.UsersList()

		actual := fmt.Sprintf("%v", users)
		expected := "map[1:{1 admin []  true } 2:{2 user []  false }]"
		require.Equal(t, actual, expected)
	})

	t.Run("userSet", func(t *testing.T) {
		cases := []struct {
			id       string
			username string
			password string
			isAdmin  bool
			err      bool
		}{
			{"1", "admin", "", false, false},
			{"10", "noPass", "", false, true},
			{"", "noID", "pass", false, true},
			{"1", "", "noUsername", false, true},
		}
		for _, tc := range cases {
			t.Run(tc.username, func(t *testing.T) {
				_, a, cancel := newTestAuth(t)
				defer cancel()

				a.hashCost = 4

				err := a.UserSet(Account{
					ID:          tc.id,
					Username:    tc.username,
					RawPassword: tc.password,
					IsAdmin:     tc.isAdmin,
				})
				gotError := err != nil
				require.Equal(t, gotError, tc.err)

				if tc.id != "" && !tc.err {
					u, _ := a.userByName(tc.username)
					require.Equal(t, u.ID, tc.id, "IDs does not match")
					require.Equal(t, u.IsAdmin, tc.isAdmin, "isAdmin does not match")
				}
			})
		}
		t.Run("saveToFile", func(t *testing.T) {
			tempDir, a, cancel := newTestAuth(t)
			defer cancel()

			a.hashCost = 4

			user := Account{
				ID:          "10",
				Username:    "a",
				Password:    []byte("b"),
				RawPassword: "c",
				IsAdmin:     true,
				Token:       "d",
			}
			err := a.UserSet(user)
			require.NoError(t, err)

			file, err := fs.ReadFile(os.DirFS(tempDir), "users.json")
			require.NoError(t, err)

			var users map[string]Account
			err = json.Unmarshal(file, &users)
			require.NoError(t, err)

			u := users["10"]
			u.Password = nil

			expected := Account{
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

			err := a.UserSet(Account{ID: "1", Username: "a"})
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

			_, exist := a.userByName("")
			require.False(t, exist, "user was not deleted")
		})
		t.Run("save error", func(t *testing.T) {
			a.path = ""
			err := a.UserDelete("1")
			require.Error(t, err)
		})
	})
}
