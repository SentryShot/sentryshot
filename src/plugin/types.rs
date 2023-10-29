// SPDX-License-Identifier: GPL-2.0-or-later

use async_trait::async_trait;
use axum::{
    extract::State,
    middleware::Next,
    response::{IntoResponse, Response},
};
use common::{monitor::MonitorConfig, DynAuth, DynLogger, DynMonitor, Username};
use http::{header, Request, StatusCode};
use sentryshot_util::Frame;
use serde::{Deserialize, Serialize};
use std::{borrow::Cow, collections::HashMap, path::Path, sync::Arc};
use thiserror::Error;
use tokio::runtime::Handle;
use tokio_util::sync::CancellationToken;

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

// Blocks unauthenticated requests.
pub async fn user<B>(State(auth): State<DynAuth>, request: Request<B>, next: Next<B>) -> Response {
    let is_valid_user = auth.validate_request(request.headers()).await.is_some();
    if !is_valid_user {
        return (
            StatusCode::UNAUTHORIZED,
            [(header::WWW_AUTHENTICATE, "Basic realm=\"NVR\"")],
            "Unauthorized.",
        )
            .into_response();
    }

    next.run(request).await
}

// Only allows authenticated requests from accounts with admin privileges.
pub async fn admin<B>(State(auth): State<DynAuth>, request: Request<B>, next: Next<B>) -> Response {
    let is_valid_admin = || async {
        let Some(valid_login) = auth.validate_request(request.headers()).await else {
            return false;
        };
        valid_login.is_admin
    };

    if !is_valid_admin().await {
        return (
            StatusCode::UNAUTHORIZED,
            [(header::WWW_AUTHENTICATE, "Basic realm=\"NVR\"")],
            "Unauthorized.",
        )
            .into_response();
    }

    next.run(request).await
}

// Blocks invalid Cross-site request forgery tokens.
// Each account has a unique token. The request needs to
// have a matching token in the "X-CSRF-TOKEN" header.
pub async fn csrf<B>(State(auth): State<DynAuth>, request: Request<B>, next: Next<B>) -> Response {
    let valid_login = auth
        .validate_request(request.headers())
        .await
        .expect("someone put the csrf middleware in the wrong order");

    if !valid_login.token_valid {
        return (StatusCode::UNAUTHORIZED, "Invalid CSRF-token").into_response();
    }

    next.run(request).await
}

pub type DynMonitorHooks = Arc<dyn MonitorHooks + Send + Sync>;

#[async_trait]
pub trait MonitorHooks {
    async fn on_monitor_start(&self, token: CancellationToken, monitor: DynMonitor);
    // Blocking.
    fn on_thumb_save(&self, config: &MonitorConfig, frame: Frame) -> Frame;
}
