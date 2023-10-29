// SPDX-License-Identifier: GPL-2.0-or-later

// False positive: https://github.com/dtolnay/async-trait/issues/228#issuecomment-1374848487
// RUSTC: remove in 1.69
#![allow(where_clauses_object_safety)]

use argon2::{
    password_hash::{rand_core::OsRng, SaltString},
    Argon2, PasswordHash, PasswordHasher, PasswordVerifier,
};
use async_trait::async_trait;
use axum::{
    body::{boxed, Full},
    extract::State,
    response::IntoResponse,
    routing::get,
    Router,
};
use common::{
    Account, AccountObfuscated, AccountSetRequest, AccountsMap, AuthAccountDeleteError,
    AuthAccountSetError, AuthSaveToFileError, Authenticator, DynAuth, DynLogger, LogEntry,
    LogLevel, LogSource, Username, ValidateLoginResponse, ValidateResponse,
};
use headers::authorization::{Basic, Credentials};
use http::{HeaderMap, HeaderValue, Response, StatusCode};
use plugin::{
    types::{NewAuthError, NewAuthFn, Templates},
    Application, Plugin, PreLoadPlugin,
};
use rand::{distributions::Alphanumeric, Rng};
use std::{
    collections::HashMap,
    fs::{self, File},
    io::Write,
    path::{Path, PathBuf},
    str::FromStr,
    sync::Arc,
};
use tokio::{runtime::Handle, sync::Mutex};

#[no_mangle]
pub fn version() -> String {
    plugin::get_version()
}

#[no_mangle]
pub fn pre_load() -> Box<dyn PreLoadPlugin> {
    Box::new(PreLoadAuthBasic)
}

struct PreLoadAuthBasic;

impl PreLoadPlugin for PreLoadAuthBasic {
    fn add_log_source(&self) -> Option<LogSource> {
        Some("auth".parse().unwrap())
    }

    fn set_new_auth(&self) -> Option<NewAuthFn> {
        Some(BasicAuth::new)
    }
}

#[no_mangle]
pub fn load(app: &dyn Application) -> Arc<dyn Plugin> {
    Arc::new(AuthBasicPlugin { auth: app.auth() })
}

struct AuthBasicPlugin {
    auth: DynAuth,
}

#[async_trait]
impl plugin::Plugin for AuthBasicPlugin {
    fn edit_templates(&self, templates: &mut Templates) {
        edit_templates(templates);
    }

    fn route(&self, router: Router) -> Router {
        router.route("/logout", get(logout).with_state(self.auth.clone()))
    }
}

pub struct BasicAuth {
    data: Mutex<BasicAuthData>,

    // Limit parallel hashing operations to mitigate resource exhaustion attacks.
    hash_lock: Mutex<()>,

    logger: DynLogger,
    rt_handle: Handle,
}

impl BasicAuth {
    #[allow(clippy::new_ret_no_self)]
    pub fn new(
        rt_handle: Handle,
        configs_dir: &Path,
        logger: DynLogger,
    ) -> Result<DynAuth, NewAuthError> {
        use NewAuthError::*;

        let path = configs_dir.join("accounts.json");
        let path_string = path.to_string_lossy().to_string();

        let mut accounts = HashMap::<String, Account>::new();

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
        let Some(auth_header) = headers.get("Authorization") else {
            return None;
        };

        let Ok(auth_header_str) =  auth_header.to_str() else {
            return None;
        };

        let Some(auth_header) = Basic::decode(auth_header) else {
            return None;
        };

        let username = Username::from_str(auth_header.username()).expect("infallible");
        let plain_password = auth_header.password();

        let account = {
            let data_guard = self.data.lock().await;
            if let Some(res) = data_guard.response_cache.get(auth_header_str) {
                return res.clone();
            }
            let Some(account) = data_guard.account_by_name(&username) else {
                return None;
            };
            if username != account.username {
                return None;
            }
            account
            // Release lock.
        };

        if self
            .passwords_match(account.password, plain_password.to_owned())
            .await
        {
            let response = Some(ValidateLoginResponse {
                is_admin: account.is_admin,
                token: account.token,
            });
            // Only cache valid responses.
            self.data
                .lock()
                .await
                .response_cache
                .insert(auth_header_str.to_string(), response.clone());
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
            .unwrap()
    }
}

#[async_trait]
impl Authenticator for BasicAuth {
    async fn validate_request(&self, headers: &HeaderMap<HeaderValue>) -> Option<ValidateResponse> {
        let Some(valid_login) = self.validate_login(headers).await else {
            return None;
        };

        let token_matches = || {
            let Some(csrf_header) = headers.get("X-CSRF-TOKEN") else {
                return false
            };
            let Ok(csrf_string) = csrf_header.to_str() else {
                return false
            };
            csrf_string == valid_login.token
        };

        Some(ValidateResponse {
            is_admin: valid_login.is_admin,
            token: valid_login.token.to_owned(),
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

        if req.id.is_empty() {
            return Err(IdMissing());
        }

        if req.username.is_empty() {
            return Err(UsernameMissing());
        }

        let data_guard = &mut self.data.lock().await;
        let created = if let Some(accont) = data_guard.accounts.get_mut(&req.id) {
            // Update existing account.
            accont.id = req.id;
            accont.username = req.username;
            if let Some(new_password) = req.plain_password {
                accont.password = generate_password_hash(&self.rt_handle, new_password).await
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
    async fn account_delete(&self, id: &str) -> Result<(), AuthAccountDeleteError> {
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
    accounts: HashMap<String, Account>,
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
            .unwrap()
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
                .unwrap()
                .to_string()
        })
        .await
        .unwrap()
}

// Generates a CSRF-token.
fn gen_token() -> String {
    rand::thread_rng()
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

async fn logout(State(auth): State<DynAuth>, headers: HeaderMap) -> impl IntoResponse {
    let auth_is_valid = auth.validate_request(&headers).await.is_some();
    if auth_is_valid {
        Response::builder()
            .header("WWW-Authenticate", "Basic realm=\"NVR\"")
            .status(StatusCode::UNAUTHORIZED)
            .body(boxed(Full::from("Enter any invalid login to logout")))
            .unwrap()
    } else {
        // User successfully logged out.
        Response::builder()
            .header("Location", "/logged-out")
            .header("Cache-Control", "no-cache, no-store, must-revalidate")
            .status(StatusCode::TEMPORARY_REDIRECT)
            .body(boxed(Full::from("")))
            .unwrap()
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
pub fn log_failed_login(logger: &DynLogger, username: &str) {
    logger.log(LogEntry {
        level: LogLevel::Warning,
        source: "auth".parse().unwrap(),
        monitor_id: None,
        message: format!("failed login: username: '{}' ", username)
            .parse()
            .unwrap(),
    });
}

#[cfg(test)]
mod tests {
    use super::*;
    use common::new_dummy_logger;
    use pretty_assertions::assert_eq;
    use std::fs::File;
    use tempfile::{tempdir, TempDir};
    use test_case::test_case;

    const PASS1: &str = "$argon2id$v=19$m=4096,t=3,p=1$Yjsk9LHMODCVG0Sk4OPkaQ$dPvksQlleIce6EDHkchy4GMQlO0Q8e2f8e3wIf3m4H4";
    const PASS2: &str = "$argon2id$v=19$m=4096,t=3,p=1$k5jIgbJ0SfZOKF7uwsBLgg$FtdOr17iJZ8/+ZE/8EjWDN/wxo7peHgIMd3b9ZgaE9Q";

    fn test_admin() -> Account {
        Account {
            id: "1".to_string(),
            username: "admin".parse().unwrap(),
            password: PASS1.to_owned(),
            is_admin: true,
            token: "token1".to_string(),
        }
    }
    fn test_user() -> Account {
        Account {
            id: "2".to_string(),
            username: "user".parse().unwrap(),
            password: PASS2.to_owned(),
            is_admin: false,
            token: "token2".to_owned(),
        }
    }

    fn new_test_auth<'a>() -> (TempDir, BasicAuth) {
        let temp_dir = tempdir().unwrap();

        let accounts_path = temp_dir.path().join("accounts.json");

        let test_accounts = HashMap::from([
            ("1".to_owned(), test_admin()),
            ("2".to_owned(), test_user()),
        ]);

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
            logger: new_dummy_logger(),
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

        let username = Username::from_str(username).unwrap();
        let got = auth.data.lock().await.account_by_name(&username);
        assert_eq!(want, got);
    }

    #[test_case("admin", "pass1", true, true, "token1", true; "ok_admin")]
    #[test_case("user", "pass2",  true, false, "token2", true; "ok_user")]
    #[test_case("User", "pass2",  true, false, "token2", true; "uppercase")]
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
        headers.insert("X-CSRF-TOKEN", HeaderValue::from_str(&token).unwrap());

        let result = auth.validate_request(&headers).await;
        assert_eq!(login_valid, result.is_some());

        if let Some(valid_login) = result {
            assert_eq!(is_admin, valid_login.is_admin);
            assert_eq!(token_valid, valid_login.token_valid)
        }
    }

    #[tokio::test]
    async fn test_auth_accounts() {
        let (_, auth) = new_test_auth();

        let want = HashMap::from([
            (
                "1".to_owned(),
                AccountObfuscated {
                    id: "1".to_owned(),
                    username: "admin".parse().unwrap(),
                    is_admin: true,
                },
            ),
            (
                "2".to_owned(),
                AccountObfuscated {
                    id: "2".to_owned(),
                    username: "user".parse().unwrap(),
                    is_admin: false,
                },
            ),
        ]);

        assert_eq!(want, auth.accounts().await)
    }

    #[tokio::test]
    async fn test_auth_account_set() {
        let (temp_dir, auth) = new_test_auth();

        // Update username.
        let req = AccountSetRequest {
            id: "1".to_owned(),
            username: "new_name".parse().unwrap(),
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
        let accounts: HashMap<String, Account> = serde_json::from_reader(file).unwrap();
        let account = accounts.get("1").unwrap();
        assert_eq!(req.id, account.id);
        assert_eq!(req.username, account.username);
        assert_eq!(req.is_admin, account.is_admin);

        // Missing password.
        match auth
            .account_set(AccountSetRequest {
                id: "10".to_owned(),
                username: "admin".parse().unwrap(),
                plain_password: None,
                is_admin: false,
            })
            .await
        {
            Ok(_) => panic!("expected error"),
            Err(e) => assert!(matches!(e, AuthAccountSetError::PasswordMissing())),
        };

        // Missing Id.
        match auth
            .account_set(AccountSetRequest {
                id: "".to_owned(),
                username: "admin".parse().unwrap(),
                plain_password: Some("pass".to_owned()),
                is_admin: false,
            })
            .await
        {
            Ok(_) => panic!("expected error"),
            Err(e) => assert!(matches!(e, AuthAccountSetError::IdMissing())),
        };

        // Missing username.
        match auth
            .account_set(AccountSetRequest {
                id: "1".to_owned(),
                username: "".parse().unwrap(),
                plain_password: Some("pass".to_owned()),
                is_admin: false,
            })
            .await
        {
            Ok(_) => panic!("expected error"),
            Err(e) => assert!(matches!(e, AuthAccountSetError::UsernameMissing())),
        }
    }

    #[tokio::test]
    async fn test_auth_account_delete() {
        let (_temp_dir, auth) = new_test_auth();

        assert!(matches!(
            auth.account_delete("nil").await,
            Err(AuthAccountDeleteError::AccountNotExist(_)),
        ));

        auth.account_delete("2").await.unwrap();
        assert_eq!(
            None,
            auth.data
                .lock()
                .await
                .account_by_name(&"2".parse().unwrap()),
            "user was not deleted"
        )
    }
}
