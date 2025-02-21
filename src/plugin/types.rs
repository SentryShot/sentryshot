// SPDX-License-Identifier: GPL-2.0-or-later

use axum::{
    body::Body,
    extract::State,
    middleware::{self, Next},
    response::{IntoResponse, Response},
    routing::MethodRouter,
};
use common::{ArcAuth, ArcLogger, Username};
use http::{header, Request, StatusCode};
use serde::{Deserialize, Serialize};
use std::{borrow::Cow, collections::HashMap, path::Path};
use thiserror::Error;
use tokio::runtime::Handle;
use tower::ServiceBuilder;

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
    fn(rt_handle: Handle, configs_dir: &Path, logger: ArcLogger) -> Result<ArcAuth, NewAuthError>;

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
async fn user_middleware(
    State(auth): State<ArcAuth>,
    request: Request<Body>,
    next: Next,
) -> Response {
    let is_valid_user = auth.validate_request(request.headers()).await.is_some();
    if !is_valid_user {
        return unauthorized();
    }
    next.run(request).await
}

// Only allows authenticated requests from accounts with admin privileges.
async fn admin_middleware(
    State(auth): State<ArcAuth>,
    request: Request<Body>,
    next: Next,
) -> Response {
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
async fn csrf_middleware(
    State(auth): State<ArcAuth>,
    request: Request<Body>,
    next: Next,
) -> Response {
    let valid_login = auth
        .validate_request(request.headers())
        .await
        .expect("someone put the csrf middleware in the wrong order");

    if !valid_login.token_valid {
        return (StatusCode::UNAUTHORIZED, "Invalid CSRF-token").into_response();
    }

    next.run(request).await
}

#[derive(Clone)]
#[allow(clippy::module_name_repetitions)]
pub struct Router {
    router: axum::Router,
    auth: ArcAuth,
}

impl Router {
    pub fn new(router: axum::Router, auth: ArcAuth) -> Self {
        Self { router, auth }
    }

    #[must_use]
    pub fn route_admin(mut self, path: &str, method_router: MethodRouter<ArcAuth>) -> Self {
        self.router = self.router.route(
            path,
            method_router
                .route_layer(
                    ServiceBuilder::new()
                        .layer(middleware::from_fn_with_state(
                            self.auth.clone(),
                            admin_middleware,
                        ))
                        .layer(middleware::from_fn_with_state(
                            self.auth.clone(),
                            csrf_middleware,
                        )),
                )
                .with_state(self.auth.clone()),
        );
        self
    }

    #[must_use]
    pub fn route_admin_no_csrf(mut self, path: &str, method_router: MethodRouter<ArcAuth>) -> Self {
        self.router = self.router.route(
            path,
            method_router
                .layer(middleware::from_fn_with_state(
                    self.auth.clone(),
                    admin_middleware,
                ))
                .with_state(self.auth.clone()),
        );
        self
    }

    #[must_use]
    pub fn route_user_no_csrf(mut self, path: &str, method_router: MethodRouter<ArcAuth>) -> Self {
        self.router = self.router.route(
            path,
            method_router
                .layer(middleware::from_fn_with_state(
                    self.auth.clone(),
                    user_middleware,
                ))
                .with_state(self.auth.clone()),
        );
        self
    }

    #[must_use]
    pub fn route_no_auth(mut self, path: &str, method_router: MethodRouter<()>) -> Self {
        self.router = self.router.route(path, method_router);
        self
    }

    pub fn inner(self) -> axum::Router {
        self.router
    }
}
