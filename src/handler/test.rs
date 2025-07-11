// SPDX-License-Identifier: GPL-2.0-or-later

#![allow(clippy::unwrap_used)]

use crate::asset_handler;
use axum::{
    body::to_bytes,
    extract::{Path, State},
    response::IntoResponse,
};
use http::{HeaderMap, StatusCode, header};
use pretty_assertions::assert_eq;
use std::{borrow::Cow, collections::HashMap};

#[tokio::test]
async fn handle_assets_ok() {
    let path = "a.json".to_owned();
    let files = HashMap::from([("a.json".to_owned(), Cow::from("test".as_bytes()))]);
    let response = asset_handler(Path(path), HeaderMap::new(), State((files, String::new())))
        .await
        .into_response();

    assert_eq!(StatusCode::OK, response.status());
    assert_eq!(
        "application/json",
        response.headers().get(header::CONTENT_TYPE).unwrap()
    );
    assert_eq!(
        "test",
        to_bytes(response.into_body(), usize::MAX).await.unwrap()
    );
}

#[tokio::test]
async fn handle_assets_404() {
    let path = "x".to_owned();
    let files = HashMap::from([("a".to_owned(), Cow::from("test".as_bytes()))]);
    let response = asset_handler(Path(path), HeaderMap::new(), State((files, String::new())))
        .await
        .into_response();

    assert_eq!(StatusCode::NOT_FOUND, response.status());
    assert_eq!(
        "404",
        to_bytes(response.into_body(), usize::MAX).await.unwrap()
    );
}
