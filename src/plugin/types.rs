// SPDX-License-Identifier: GPL-2.0-or-later

use axum::{
    Extension,
    body::Body,
    extract::State,
    middleware::{self, Next},
    response::{IntoResponse, Response},
    routing::MethodRouter,
};
use common::{ArcAuth, ArcLogger, AuthenticatedUser};
use http::{Request, StatusCode, header};
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

async fn block_unauthenticted_requests_middleware(
    State(auth): State<ArcAuth>,
    mut request: Request<Body>,
    next: Next,
) -> Response {
    let Some(valid_login) = auth.validate_request(request.headers()).await else {
        return unauthorized();
    };
    request.extensions_mut().insert(valid_login);
    next.run(request).await
}

async fn block_non_admins_middleware(
    Extension(valid_login): Extension<AuthenticatedUser>,
    request: Request<Body>,
    next: Next,
) -> Response {
    if valid_login.is_admin {
        next.run(request).await
    } else {
        unauthorized()
    }
}

// Blocks invalid Cross-site request forgery tokens.
// Each account has a unique token. The request needs to
// have a matching token in the "X-CSRF-TOKEN" header.
async fn block_invalid_csrf_middleware(
    Extension(valid_login): Extension<AuthenticatedUser>,
    request: Request<Body>,
    next: Next,
) -> Response {
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
    pub fn route_admin(mut self, path: &str, method_router: MethodRouter<()>) -> Self {
        self.router = self.router.route(
            path,
            method_router.layer(
                ServiceBuilder::new()
                    .layer(middleware::from_fn_with_state(
                        self.auth.clone(),
                        block_unauthenticted_requests_middleware,
                    ))
                    .layer(middleware::from_fn(block_non_admins_middleware))
                    .layer(middleware::from_fn(block_invalid_csrf_middleware)),
            ),
        );
        self
    }

    #[must_use]
    pub fn route_admin_no_csrf(mut self, path: &str, method_router: MethodRouter<()>) -> Self {
        self.router = self.router.route(
            path,
            method_router.layer(
                ServiceBuilder::new()
                    .layer(middleware::from_fn_with_state(
                        self.auth.clone(),
                        block_unauthenticted_requests_middleware,
                    ))
                    .layer(middleware::from_fn(block_non_admins_middleware)),
            ),
        );
        self
    }

    #[must_use]
    pub fn route_user_no_csrf(mut self, path: &str, method_router: MethodRouter<()>) -> Self {
        self.router = self.router.route(
            path,
            method_router.layer(middleware::from_fn_with_state(
                self.auth.clone(),
                block_unauthenticted_requests_middleware,
            )),
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
