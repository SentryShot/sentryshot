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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"nvr/pkg/log"
	"os"
	"testing"
)

var (
	pass1 = []byte("$2a$04$M0InS5zIFKk.xmjtcabjrudhKhukxJo6cnhJBq9I.J/slbgWE0F.S")
	pass2 = []byte("$2a$04$A.F3L5bXO/5nF0e6dpmqM.VuOB66.vSt6MbvWvcxeoAqqnvchBMOq")
)

func newTestAuth(t *testing.T) (string, *Authenticator, func()) {
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("could not create tempoary directory: %v", err)
	}

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
	if err != nil {
		t.Fatalf("could not marshal users: %v", err)
	}
	if err := ioutil.WriteFile(usersPath, data, 0o600); err != nil {
		t.Fatalf("could not write users file: %v", err)
	}

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
	t.Run("working", func(t *testing.T) {
		tempDir, testAuth, cancel := newTestAuth(t)
		defer cancel()

		auth, err := NewBasicAuthenticator(tempDir+"/users.json", &log.Logger{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		clearTokens(auth)

		actual := fmt.Sprintf("%v", auth.accounts)
		expected := fmt.Sprintf("%v", testAuth.accounts)

		if actual != expected {
			t.Fatalf("\nexpected\n%v.\ngot\n%v.", expected, actual)
		}
	})
	t.Run("readFile error", func(t *testing.T) {
		_, err := NewBasicAuthenticator("nil", &log.Logger{})
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %v, got: %v", os.ErrNotExist, err)
		}
	})
}

func TestBasicAuthenticator(t *testing.T) {
	adminExpected := "{1 admin " + fmt.Sprintf("%v", pass1) + "  true }"
	userExpected := "{2 user " + fmt.Sprintf("%v", pass2) + "  false }"

	t.Run("userByName", func(t *testing.T) {
		_, a, cancel := newTestAuth(t)
		defer cancel()

		cases := []struct {
			username    string
			shouldExist bool
			expected    string
		}{
			{"admin", true, adminExpected},
			{"user", true, userExpected},
			{"nil", false, "{  []  false }"},
		}

		for _, tc := range cases {
			t.Run(tc.username, func(t *testing.T) {
				account, exists := a.userByName(tc.username)
				if exists != tc.shouldExist {
					t.Fatalf("should exists: %v, got %v", tc.shouldExist, exists)
				}
				account.Token = ""

				actual := fmt.Sprintf("%v", account)

				if actual != tc.expected {
					t.Fatalf("\nexpected %v\n     got %v", tc.expected, actual)
				}
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
			expected string
		}{
			{"admin", "pass1", true, adminExpected},
			{"user", "pass2", true, userExpected},
			{"user", "pass2", true, userExpected}, // test cache
			{"user", "wrongPass", false, "{  []  false }"},
			{"nil", "", false, "{  []  false }"},
		}

		for _, tc := range cases {
			t.Run(tc.username, func(t *testing.T) {
				plainAuth := tc.username + ":" + tc.password
				auth := base64.StdEncoding.EncodeToString([]byte(plainAuth))

				response := a.ValidateAuth("Basic " + auth)
				if response.IsValid != tc.valid {
					t.Fatalf("expected valid: %v, got: %v", tc.valid, response.IsValid)
				}

				user := response.User
				user.Token = ""
				actual := fmt.Sprintf("%v", user)

				if actual != tc.expected {
					t.Fatalf("expected %v, got %v", tc.expected, actual)
				}
			})
		}

		t.Run("invalid prefix", func(t *testing.T) {
			auth := base64.StdEncoding.EncodeToString([]byte("admin:pass1"))
			response := a.ValidateAuth("nil" + auth)
			if response.IsValid {
				t.Fatal("expected invalid response")
			}
		})
		t.Run("invalid base64", func(t *testing.T) {
			response := a.ValidateAuth("Basic nil")
			if response.IsValid {
				t.Fatal("expected invalid response")
			}
		})
		t.Run("invalid auth", func(t *testing.T) {
			auth := base64.StdEncoding.EncodeToString([]byte("admin@pass1"))
			response := a.ValidateAuth("Basic " + auth)
			if response.IsValid {
				t.Fatal("expected invalid response")
			}
		})
	})

	t.Run("userList", func(t *testing.T) {
		_, a, cancel := newTestAuth(t)
		defer cancel()

		users := a.UsersList()

		actual := fmt.Sprintf("%v", users)
		expected := "map[1:{1 admin []  true } 2:{2 user []  false }]"

		if actual != expected {
			t.Fatalf("\nexpected\n%v.\ngot\n%v.", expected, actual)
		}
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
				if tc.err != gotError {
					t.Errorf("expected error: %v, error: %v", tc.err, err)
				}
				if tc.id != "" && !tc.err {
					u, _ := a.userByName(tc.username)
					if u.ID != tc.id {
						t.Errorf("id does not match")
					}
					if u.IsAdmin != tc.isAdmin {
						t.Errorf("isAdmin does not match")
					}
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
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			file, err := ioutil.ReadFile(tempDir + "/users.json")
			if err != nil {
				t.Fatalf("could not read file: %v", err)
			}
			var users map[string]Account
			err = json.Unmarshal(file, &users)
			if err != nil {
				t.Fatalf("could not unmarshal: %v", err)
			}

			u := users["10"]
			u.Password = nil

			expected := "{10 a []  true }"
			actual := fmt.Sprintf("%v", u)
			if actual != expected {
				t.Fatalf("\nexpected%v\n.\ngot\n%v\n", expected, actual)
			}
		})
		t.Run("saveErr", func(t *testing.T) {
			_, a, cancel := newTestAuth(t)
			defer cancel()

			a.path = ""

			if err := a.UserSet(Account{ID: "1", Username: "a"}); err == nil {
				t.Fatal("expected: error, got: nil")
			}
		})
	})

	t.Run("userDelete", func(t *testing.T) {
		_, a, cancel := newTestAuth(t)
		defer cancel()

		t.Run("unknown user", func(t *testing.T) {
			err := a.UserDelete("nil")
			if !errors.Is(err, ErrUserNotExist) {
				t.Fatalf("expected: %v, got: %v", ErrUserNotExist, err)
			}
		})
		t.Run("working", func(t *testing.T) {
			err := a.UserDelete("2")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, exist := a.userByName(""); exist {
				t.Fatal("user was not deleted")
			}
		})
		t.Run("save error", func(t *testing.T) {
			a.path = ""
			err := a.UserDelete("1")
			if err == nil {
				t.Fatal("nil")
			}
		})
	})
}
