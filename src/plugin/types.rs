// SPDX-License-Identifier: GPL-2.0-or-later

use axum::{
    body::Body,
    extract::State,
    middleware::Next,
    response::{IntoResponse, Response},
};
use common::{DynAuth, DynLogger, Username};
use http::{header, Request, StatusCode};
use serde::{Deserialize, Serialize};
use std::{borrow::Cow, collections::HashMap, path::Path};
use thiserror::Error;
use tokio::runtime::Handle;

#[derive(Debug, Error)]
pub enum NewAuthError {
    #[error("create file: '{0}' {1}")]
    CreateFile(String, std::io::Error),

    #[error("write initial file: '{0}' {1}")]
    WriteInitialFile(String, std::io::Error),

    #[error("read file: '{0}' {1}")]
    ReadFile(String, std::io::Error),

    #[error("parse file: {0}")]
    ParseFile(serde_json::Error),
}

// Authenticator constructor function.
pub type NewAuthFn =
    fn(rt_handle: Handle, configs_dir: &Path, logger: DynLogger) -> Result<DynAuth, NewAuthError>;

/// Main account definition.
#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
pub struct Account {
    pub id: String,
    pub username: Username,
    pub password: String, // Hashed password PHC string.

    #[serde(rename = "isAdmin")]
    pub is_admin: bool,

    #[serde(skip)]
    pub token: String,
}

#[derive(Clone)]
pub struct ValidateResponse {
    pub is_admin: bool,
    pub token: String,
    pub token_valid: bool,
}

#[derive(Clone)]
pub struct ValidateLoginResponse {
    pub is_admin: bool,
    pub token: String,
}

pub type Assets<'a> = HashMap<String, Cow<'a, [u8]>>;
pub type Templates<'a> = HashMap<&'a str, String>;

fn unauthorized() -> Response {
    (
        StatusCode::UNAUTHORIZED,
        [(header::WWW_AUTHENTICATE, "Basic realm=\"NVR\"")],
        "Unauthorized.",
    )
        .into_response()
}

// Blocks unauthenticated requests.
pub async fn user(State(auth): State<DynAuth>, request: Request<Body>, next: Next) -> Response {
    let is_valid_user = auth.validate_request(request.headers()).await.is_some();
    if !is_valid_user {
        return unauthorized();
    }
    next.run(request).await
}

// Only allows authenticated requests from accounts with admin privileges.
pub async fn admin(State(auth): State<DynAuth>, request: Request<Body>, next: Next) -> Response {
    match auth.validate_request(request.headers()).await {
        Some(valid_login) => {
            if !valid_login.is_admin {
                return unauthorized();
            }
        }
        None => return unauthorized(),
    }
    next.run(request).await
}

// Blocks invalid Cross-site request forgery tokens.
// Each account has a unique token. The request needs to
// have a matching token in the "X-CSRF-TOKEN" header.
pub async fn csrf(State(auth): State<DynAuth>, request: Request<Body>, next: Next) -> Response {
    let valid_login = auth
        .validate_request(request.headers())
        .await
        .expect("someone put the csrf middleware in the wrong order");

    if !valid_login.token_valid {
        return (StatusCode::UNAUTHORIZED, "Invalid CSRF-token").into_response();
    }

    next.run(request).await
}
