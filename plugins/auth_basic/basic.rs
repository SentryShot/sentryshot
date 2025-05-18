// SPDX-License-Identifier: GPL-2.0-or-later

use argon2::{
    password_hash::{rand_core::OsRng, SaltString},
    Argon2, PasswordHash, PasswordHasher, PasswordVerifier,
};
use async_trait::async_trait;
use axum::{extract::State, response::IntoResponse, routing::get};
use common::{
    Account, AccountId, AccountObfuscated, AccountSetRequest, AccountsMap, ArcAuth, ArcLogger,
    AuthAccountDeleteError, AuthAccountSetError, AuthSaveToFileError, AuthenticatedUser,
    Authenticator, LogEntry, LogLevel, LogSource, Username, ValidateResponse,
};
use headers::authorization::{Basic, Credentials};
use http::{header, HeaderMap, HeaderValue, StatusCode};
use plugin::{
    types::{NewAuthError, NewAuthFn, Router, Templates},
    Application, Plugin, PreLoadPlugin,
};
use rand::{distr::Alphanumeric, Rng};
use std::{
    collections::HashMap,
    ffi::c_char,
    fs::{self, File},
    io::Write,
    path::{Path, PathBuf},
    sync::Arc,
};
use tokio::{runtime::Handle, sync::Mutex};

#[no_mangle]
pub extern "C" fn version() -> *const c_char {
    plugin::get_version()
}

#[no_mangle]
pub extern "Rust" fn pre_load() -> Box<dyn PreLoadPlugin> {
    Box::new(PreLoadAuthBasic)
}

struct PreLoadAuthBasic;

impl PreLoadPlugin for PreLoadAuthBasic {
    fn add_log_source(&self) -> Option<LogSource> {
        #[allow(clippy::unwrap_used)]
        Some("auth".try_into().unwrap())
    }

    fn set_new_auth(&self) -> Option<NewAuthFn> {
        Some(BasicAuth::new)
    }
}

#[no_mangle]
pub extern "Rust" fn load(app: &dyn Application) -> Arc<dyn Plugin> {
    Arc::new(AuthBasicPlugin { auth: app.auth() })
}

struct AuthBasicPlugin {
    auth: ArcAuth,
}

#[async_trait]
impl plugin::Plugin for AuthBasicPlugin {
    fn edit_templates(&self, templates: &mut Templates) {
        edit_templates(templates);
    }

    fn route(&self, router: Router) -> Router {
        router.route_no_auth("/logout", get(logout).with_state(self.auth.clone()))
    }
}

pub struct BasicAuth {
    data: Mutex<BasicAuthData>,

    // Limit parallel hashing operations to mitigate resource exhaustion attacks.
    hash_lock: Mutex<()>,

    logger: ArcLogger,
    rt_handle: Handle,
}

impl BasicAuth {
    #[allow(clippy::new_ret_no_self)]
    pub fn new(
        rt_handle: Handle,
        configs_dir: &Path,
        logger: ArcLogger,
    ) -> Result<ArcAuth, NewAuthError> {
        use NewAuthError::*;

        let path = configs_dir.join("accounts.json");
        let path_string = path.to_string_lossy().to_string();

        let mut accounts = HashMap::<AccountId, Account>::new();

        let file_exist = Path::new(&path).exists();
        if file_exist {
            let accounts_json = fs::read_to_string(&path).map_err(|e| ReadFile(path_string, e))?;
            accounts = serde_json::from_str(&accounts_json).map_err(ParseFile)?;
        } else {
            let mut file = File::create(&path).map_err(|e| CreateFile(path_string.clone(), e))?;
            write!(file, "{{}}").map_err(|e| WriteInitialFile(path_string.clone(), e))?;
        }

        let mut data = BasicAuthData {
            path,
            accounts,
            response_cache: HashMap::new(),
            rt_handle: rt_handle.clone(),
        };
        data.reset_tokens();

        let auth = BasicAuth {
            data: Mutex::new(data),
            hash_lock: Mutex::new(()),
            logger,
            rt_handle,
        };

        Ok(Arc::new(auth))
    }

    /// Should always take the same amount of time to run,
    /// even when username or password is invalid.
    async fn validate_login(
        &self,
        headers: &HeaderMap<HeaderValue>,
    ) -> Option<ValidateLoginResponse> {
        let auth_header = headers.get("Authorization")?;
        let Ok(auth_header_str) = auth_header.to_str() else {
            return None;
        };

        let auth_header = Basic::decode(auth_header)?;
        let Ok(username) = Username::try_from(auth_header.username().to_owned()) else {
            return None;
        };
        let plain_password = auth_header.password();

        let account = {
            let data_guard = self.data.lock().await;
            if let Some(res) = data_guard.response_cache.get(auth_header_str) {
                return res.clone();
            }
            let account = data_guard.account_by_name(&username)?;
            if username != account.username {
                return None;
            }
            // Release lock.
            drop(data_guard);
            account
        };

        if self
            .passwords_match(account.password, plain_password.to_owned())
            .await
        {
            let response = Some(ValidateLoginResponse {
                is_admin: account.is_admin,
                stored_token: account.token,
            });
            // Only cache valid responses.
            self.data
                .lock()
                .await
                .response_cache
                .insert(auth_header_str.to_owned(), response.clone());
            response
        } else {
            log_failed_login(&self.logger, &username);
            None
        }
    }

    async fn passwords_match(&self, hash: String, plaintext: String) -> bool {
        // Lock hash_lock to prevent parallel password verifications.
        let _hash_guard = self.hash_lock.lock();

        self.rt_handle
            .spawn_blocking(move || {
                let Ok(parsed_hash) = PasswordHash::new(&hash) else {
                    return false;
                };
                Argon2::default()
                    .verify_password(plaintext.as_bytes(), &parsed_hash)
                    .is_ok()
            })
            .await
            .expect("join")
    }
}

#[derive(Clone)]
pub struct ValidateLoginResponse {
    pub is_admin: bool,
    pub stored_token: String,
}

#[async_trait]
impl Authenticator for BasicAuth {
    async fn validate_request(&self, headers: &HeaderMap<HeaderValue>) -> ValidateResponse {
        let valid_login = self.validate_login(headers).await?;

        let token_matches = || {
            let Some(csrf_header) = headers.get("X-CSRF-TOKEN") else {
                return false;
            };
            let Ok(csrf_string) = csrf_header.to_str() else {
                return false;
            };
            csrf_string == valid_login.stored_token
        };

        Some(AuthenticatedUser {
            is_admin: valid_login.is_admin,
            stored_token: valid_login.stored_token.clone(),
            token_valid: token_matches(),
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

        data_guard.reset_cache_and_save_to_file().await?;
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

        data_guard.reset_cache_and_save_to_file().await?;
        Ok(())
    }
}

struct BasicAuthData {
    path: PathBuf, // Path to `accounts.json`.
    accounts: HashMap<AccountId, Account>,
    response_cache: HashMap<String, Option<ValidateLoginResponse>>,
    rt_handle: Handle,
}

impl BasicAuthData {
    async fn reset_cache_and_save_to_file(&mut self) -> Result<(), AuthSaveToFileError> {
        // Reset cache.
        self.response_cache = HashMap::new();

        let accounts_json = serde_json::to_vec_pretty(&self.accounts)?;

        let path = self.path.clone();
        let mut temp_path = self.path.clone();
        temp_path.set_file_name("accounts.json.tmp");

        self.rt_handle
            .spawn_blocking(move || -> Result<(), AuthSaveToFileError> {
                use AuthSaveToFileError::*;
                let mut file = fs::OpenOptions::new()
                    .create(true)
                    .write(true)
                    .truncate(true)
                    .open(&temp_path)
                    .map_err(OpenFile)?;

                file.write_all(&accounts_json).map_err(WriteFile)?;
                file.sync_all().map_err(SyncFile)?;
                fs::rename(temp_path, &path).map_err(RenameFile)?;

                Ok(())
            })
            .await
            .expect("join")
    }

    fn account_by_name(&self, name: &Username) -> Option<Account> {
        for account in self.accounts.values() {
            if account.username == *name {
                return Some(account.clone());
            }
        }
        None
    }

    /// Generates new random tokens for each account.
    fn reset_tokens(&mut self) {
        for account in self.accounts.values_mut() {
            account.token = gen_token();
        }
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

fn edit_templates(tmpls: &mut Templates) {
    let sidebar = tmpls.get_mut("sidebar").expect("sidebar template to exist");

    let target = "<!-- NAVBAR_BOTTOM -->";

    let logout_button = "
<div id=\"logout\">
	<button onclick='if (confirm(\"logout?\")) { window.location.href = \"logout\"; }'>
		Logout
	</button>
</div>";

    *sidebar = sidebar.replace(target, &(logout_button.to_owned() + target));
}

#[allow(clippy::unwrap_used)]
async fn logout(State(auth): State<ArcAuth>, headers: HeaderMap) -> impl IntoResponse {
    let auth_is_valid = auth.validate_request(&headers).await.is_some();
    if auth_is_valid {
        (
            StatusCode::UNAUTHORIZED,
            [(header::WWW_AUTHENTICATE, "Basic realm=\"NVR\"")],
            "Enter any invalid login to logout",
        )
            .into_response()
    } else {
        // User successfully logged out.
        (
            StatusCode::TEMPORARY_REDIRECT,
            [
                (header::LOCATION, "/logged-out"),
                (header::CACHE_CONTROL, "no-cache, no-store, must-revalidate"),
            ],
        )
            .into_response()
    }
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

// LogFailedLogin finds and logs the ip.
pub fn log_failed_login(logger: &ArcLogger, username: &Username) {
    logger.log(LogEntry::new(
        LogLevel::Warning,
        "auth",
        None,
        format!("failed login: username: '{username}' "),
    ));
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use common::{AccountId, DummyLogger};
    use pretty_assertions::assert_eq;
    use std::fs::File;
    use tempfile::{tempdir, TempDir};
    use test_case::test_case;

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

    fn new_test_auth() -> (TempDir, BasicAuth) {
        let temp_dir = tempdir().unwrap();

        let accounts_path = temp_dir.path().join("accounts.json");

        let test_accounts = HashMap::from([(id1(), test_admin()), (id2(), test_user())]);

        let file = File::create(&accounts_path).unwrap();
        serde_json::to_writer(file, &test_accounts).unwrap();

        let data = BasicAuthData {
            path: accounts_path,
            accounts: test_accounts,
            response_cache: HashMap::new(),
            rt_handle: tokio::runtime::Handle::current(),
        };

        let auth = BasicAuth {
            data: Mutex::new(data),
            hash_lock: Mutex::new(()),
            logger: DummyLogger::new(),
            rt_handle: tokio::runtime::Handle::current(),
        };
        (temp_dir, auth)
    }

    #[test_case("admin", Some(test_admin()))]
    #[test_case("user", Some(test_user()))]
    #[test_case("nil", None)]
    #[tokio::test]
    async fn test_auth_account_by_name(username: &str, want: Option<Account>) {
        let (_, auth) = new_test_auth();
        let got = auth.data.lock().await.account_by_name(&name(username));
        assert_eq!(want, got);
    }

    #[test_case("admin", "pass1", true, true, "token1", true; "ok_admin")]
    #[test_case("user", "pass2",  true, false, "token2", true; "ok_user")]
    #[test_case("user", "wrongPass",  false, false, "token2", false; "wrong_pass")]
    #[test_case("admin", "pass1", true, true, "invalid", false;"invalid_token")]
    #[test_case("nil", "", false, false, "", false; "nil")]
    #[tokio::test]
    async fn test_auth_validate_request(
        username: &str,
        password: &str,
        login_valid: bool,
        is_admin: bool,
        token: &str,
        token_valid: bool,
    ) {
        let (_, auth) = new_test_auth();

        let auth_header = headers::Authorization::basic(username, password);

        let mut headers = HeaderMap::new();
        headers.insert("Authorization", auth_header.0.encode());
        headers.insert("X-CSRF-TOKEN", HeaderValue::from_str(token).unwrap());

        let result = auth.validate_request(&headers).await;
        assert_eq!(login_valid, result.is_some());

        if let Some(valid_login) = result {
            assert_eq!(is_admin, valid_login.is_admin);
            assert_eq!(token_valid, valid_login.token_valid);
        }
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
