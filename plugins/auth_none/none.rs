// SPDX-License-Identifier: GPL-2.0-or-later

use argon2::{
    Argon2, PasswordHasher,
    password_hash::{SaltString, rand_core::OsRng},
};
use async_trait::async_trait;
use common::{
    Account, AccountId, AccountObfuscated, AccountSetRequest, AccountsMap, ArcAuth, ArcLogger,
    AuthAccountDeleteError, AuthAccountSetError, AuthSaveToFileError, AuthenticatedUser,
    Authenticator, LogSource, ValidateResponse, write_file_atomic2,
};
use http::{HeaderMap, HeaderValue};
use plugin::{
    Application, Plugin, PreLoadPlugin,
    types::{NewAuthError, NewAuthFn},
};
use rand::{Rng, distr::Alphanumeric};
use std::{
    collections::HashMap,
    ffi::c_char,
    fs::{self},
    path::{Path, PathBuf},
    sync::Arc,
};
use tokio::{runtime::Handle, sync::Mutex};

#[unsafe(no_mangle)]
pub extern "C" fn version() -> *const c_char {
    plugin::get_version()
}

#[unsafe(no_mangle)]
pub extern "Rust" fn pre_load() -> Box<dyn PreLoadPlugin> {
    Box::new(PreLoadAuthNone)
}

struct PreLoadAuthNone;

impl PreLoadPlugin for PreLoadAuthNone {
    fn add_log_source(&self) -> Option<LogSource> {
        #[allow(clippy::unwrap_used)]
        Some("auth".try_into().unwrap())
    }

    fn set_new_auth(&self) -> Option<NewAuthFn> {
        Some(NoneAuth::new)
    }
}

#[unsafe(no_mangle)]
pub extern "Rust" fn load(_app: &dyn Application) -> Arc<dyn Plugin> {
    Arc::new(AuthNonePlugin)
}
struct AuthNonePlugin;
#[async_trait]
impl Plugin for AuthNonePlugin {}

pub struct NoneAuth {
    data: Mutex<BasicAuthData>,
    csrf_token: String,
    rt_handle: Handle,
}

impl NoneAuth {
    #[allow(clippy::new_ret_no_self)]
    pub fn new(
        rt_handle: Handle,
        configs_dir: &Path,
        _: ArcLogger,
    ) -> Result<ArcAuth, NewAuthError> {
        use NewAuthError::*;
        let path = configs_dir.join("accounts.json");

        let mut accounts = HashMap::<AccountId, Account>::new();

        let file_exist = Path::new(&path).exists();
        if file_exist {
            let accounts_json = fs::read_to_string(&path).map_err(|e| ReadFile(path.clone(), e))?;
            accounts = serde_json::from_str(&accounts_json).map_err(ParseFile)?;
        } else {
            common::write_file(&path, b"{}").map_err(|e| WriteInitialFile(path.clone(), e))?;
        }

        let data = BasicAuthData {
            path,
            accounts,
            rt_handle: rt_handle.clone(),
        };

        let csrf_token = rand::rng()
            .sample_iter(&Alphanumeric)
            .take(32)
            .map(char::from)
            .collect();

        let auth = NoneAuth {
            data: Mutex::new(data),
            csrf_token,
            rt_handle,
        };

        Ok(Arc::new(auth))
    }
}

#[async_trait]
impl Authenticator for NoneAuth {
    async fn validate_request(&self, _: &HeaderMap<HeaderValue>) -> ValidateResponse {
        Some(AuthenticatedUser {
            is_admin: true,
            stored_token: self.csrf_token.clone(),
            token_valid: true,
        })
    }

    // Returns an obfuscated account map.
    async fn accounts(&self) -> AccountsMap {
        let mut list = HashMap::new();
        for (id, account) in &self.data.lock().await.accounts {
            list.insert(
                id.clone(),
                AccountObfuscated {
                    id: account.id.clone(),
                    username: account.username.clone(),
                    is_admin: account.is_admin,
                },
            );
        }
        list
    }

    // Set account details.
    async fn account_set(&self, req: AccountSetRequest) -> Result<bool, AuthAccountSetError> {
        use AuthAccountSetError::*;

        let data_guard = &mut self.data.lock().await;
        let created = if let Some(accont) = data_guard.accounts.get_mut(&req.id) {
            // Update existing account.
            accont.id = req.id;
            accont.username = req.username;
            if let Some(new_password) = req.plain_password {
                accont.password = generate_password_hash(&self.rt_handle, new_password).await;
            }
            accont.is_admin = req.is_admin;
            accont.token = gen_token();
            false
        } else {
            // Create new account.
            let Some(new_password) = req.plain_password else {
                return Err(PasswordMissing());
            };

            let updated_account = Account {
                id: req.id,
                username: req.username,
                password: generate_password_hash(&self.rt_handle, new_password).await,
                is_admin: req.is_admin,
                token: gen_token(),
            };

            data_guard
                .accounts
                .insert(updated_account.id.clone(), updated_account);
            true
        };

        data_guard.save_to_file().await?;
        Ok(created)
    }

    // Delete account by id and save changes.
    async fn account_delete(&self, id: &AccountId) -> Result<(), AuthAccountDeleteError> {
        use AuthAccountDeleteError::*;

        let mut data_guard = self.data.lock().await;

        // Try to remove account.
        if data_guard.accounts.remove(id).is_none() {
            return Err(AccountNotExist(id.to_owned()));
        }

        data_guard.save_to_file().await?;
        Ok(())
    }
}

struct BasicAuthData {
    path: PathBuf, // Path to `accounts.json`.
    accounts: HashMap<AccountId, Account>,
    rt_handle: Handle,
}

impl BasicAuthData {
    async fn save_to_file(&mut self) -> Result<(), AuthSaveToFileError> {
        let accounts_json = serde_json::to_vec_pretty(&self.accounts)?;

        let path = self.path.clone();
        let mut temp_path = self.path.clone();
        temp_path.set_file_name("accounts.json.tmp");

        write_file_atomic2(self.rt_handle.clone(), path, temp_path, accounts_json)
            .await
            .map_err(AuthSaveToFileError::WriteFile)
    }

    #[cfg(test)]
    fn account_by_name(&self, name: &common::Username) -> Option<Account> {
        for account in self.accounts.values() {
            if account.username == *name {
                return Some(account.clone());
            }
        }
        None
    }
}

async fn generate_password_hash(rt_handle: &Handle, plain_password: String) -> String {
    rt_handle
        .spawn_blocking(move || {
            let salt = SaltString::generate(&mut OsRng);
            Argon2::default()
                .hash_password(plain_password.as_bytes(), &salt)
                .expect("panic if password generation fails")
                .to_string()
        })
        .await
        .expect("join")
}

// Generates a CSRF-token.
fn gen_token() -> String {
    rand::rng()
        .sample_iter(&Alphanumeric)
        .take(32)
        .map(char::from)
        .collect()
}

/*
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
*/

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use common::{AccountId, DummyLogger, Username};
    use pretty_assertions::assert_eq;
    use std::fs::File;
    use tempfile::{TempDir, tempdir};

    fn id(v: &str) -> AccountId {
        v.to_owned().try_into().unwrap()
    }
    fn id1() -> AccountId {
        id("aaaaaaaaaaaaaaaa")
    }
    fn id2() -> AccountId {
        id("bbbbbbbbbbbbbbbb")
    }

    fn name(v: &str) -> Username {
        v.to_owned().try_into().unwrap()
    }

    const PASS1: &str = "$argon2id$v=19$m=4096,t=3,p=1$Yjsk9LHMODCVG0Sk4OPkaQ$dPvksQlleIce6EDHkchy4GMQlO0Q8e2f8e3wIf3m4H4";
    const PASS2: &str = "$argon2id$v=19$m=4096,t=3,p=1$k5jIgbJ0SfZOKF7uwsBLgg$FtdOr17iJZ8/+ZE/8EjWDN/wxo7peHgIMd3b9ZgaE9Q";

    fn test_admin() -> Account {
        Account {
            id: id1(),
            username: name("admin"),
            password: PASS1.to_owned(),
            is_admin: true,
            token: "token1".to_owned(),
        }
    }
    fn test_user() -> Account {
        Account {
            id: id2(),
            username: name("user"),
            password: PASS2.to_owned(),
            is_admin: false,
            token: "token2".to_owned(),
        }
    }

    fn new_test_auth() -> (TempDir, NoneAuth) {
        let temp_dir = tempdir().unwrap();

        let accounts_path = temp_dir.path().join("accounts.json");

        let test_accounts = HashMap::from([(id1(), test_admin()), (id2(), test_user())]);

        let file = File::create(&accounts_path).unwrap();
        serde_json::to_writer(file, &test_accounts).unwrap();

        let data = BasicAuthData {
            path: accounts_path,
            accounts: test_accounts,
            rt_handle: tokio::runtime::Handle::current(),
        };

        let auth = NoneAuth {
            data: Mutex::new(data),
            csrf_token: "123".to_owned(),
            rt_handle: tokio::runtime::Handle::current(),
        };
        (temp_dir, auth)
    }

    #[tokio::test]
    async fn test_new_auth() {
        let temp_dir = tempdir().unwrap();
        NoneAuth::new(Handle::current(), temp_dir.path(), DummyLogger::new()).unwrap();
        NoneAuth::new(Handle::current(), temp_dir.path(), DummyLogger::new()).unwrap();
    }

    #[tokio::test]
    async fn test_auth_accounts() {
        let (_, auth) = new_test_auth();

        let want = HashMap::from([
            (
                id1(),
                AccountObfuscated {
                    id: id1(),
                    username: name("admin"),
                    is_admin: true,
                },
            ),
            (
                id2(),
                AccountObfuscated {
                    id: id2(),
                    username: name("user"),
                    is_admin: false,
                },
            ),
        ]);

        assert_eq!(want, auth.accounts().await);
    }

    #[tokio::test]
    async fn test_auth_account_set() {
        let (temp_dir, auth) = new_test_auth();

        // Update username.
        let req = AccountSetRequest {
            id: id1(),
            username: name("new_name"),
            plain_password: None,
            is_admin: true,
        };
        assert_eq!(false, auth.account_set(req.clone()).await.unwrap());
        let account = auth
            .data
            .lock()
            .await
            .account_by_name(&req.username)
            .unwrap();
        assert_eq!(req.id, account.id);
        assert_eq!(req.username, account.username);
        assert_eq!(req.is_admin, account.is_admin);

        // Save to file.
        let file = fs::File::open(temp_dir.path().join("accounts.json")).unwrap();
        let accounts: HashMap<AccountId, Account> = serde_json::from_reader(file).unwrap();
        let account = &accounts[&id1()];
        assert_eq!(req.id, account.id);
        assert_eq!(req.username, account.username);
        assert_eq!(req.is_admin, account.is_admin);

        // Missing password.
        let err = auth
            .account_set(AccountSetRequest {
                id: id("xxxxxxxxxxxxxxxx"),
                username: name("admin"),
                plain_password: None,
                is_admin: false,
            })
            .await
            .unwrap_err();
        assert!(matches!(err, AuthAccountSetError::PasswordMissing()));
    }

    #[tokio::test]
    async fn test_auth_account_delete() {
        let (_temp_dir, auth) = new_test_auth();

        assert!(matches!(
            auth.account_delete(&id("xxxxxxxxxxxxxxxx")).await,
            Err(AuthAccountDeleteError::AccountNotExist(_)),
        ));

        auth.account_delete(&id2()).await.unwrap();
        assert_eq!(
            None,
            auth.data.lock().await.account_by_name(&name("2")),
            "user was not deleted"
        );
    }
}
