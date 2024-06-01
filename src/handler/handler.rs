// SPDX-License-Identifier: GPL-2.0-or-later

#![allow(clippy::unused_async)]

#[cfg(test)]
mod test;

use axum::{
    body::Body,
    extract::{Path, Query, State, WebSocketUpgrade},
    http::{header, HeaderMap, Method, StatusCode, Uri},
    response::{IntoResponse, Response},
    Json,
};
use common::{
    monitor::{MonitorConfig, MonitorConfigs},
    AccountSetRequest, AccountsMap, AuthAccountDeleteError, DynAuth, DynLogger, ILogger, LogEntry,
    LogLevel, MonitorId,
};
use hls::{HlsQuery, HlsServer};
use http::{HeaderValue, Request};
use log::{
    log_db::{LogDbHandle, LogQuery},
    Logger,
};
use monitor::{MonitorDeleteError, MonitorManager};
use recdb::{DeleteRecordingError, RecDb, RecDbQuery, RecordingResponse};
use recording::{new_video_reader, RecordingId, VideoCache};
use rust_embed::EmbeddedFiles;
use serde::Deserialize;
use std::{path::PathBuf, sync::Arc};
use thiserror::Error;
use tokio::sync::{broadcast::error::RecvError, Mutex};
use tokio_util::io::ReaderStream;
use web::{serve_mp4_content, Templater};

#[derive(Clone)]
pub struct TemplateHandlerState<'a> {
    pub templater: Arc<Templater<'a>>,
    pub auth: DynAuth,
}

pub async fn template_handler(
    uri: Uri,
    headers: HeaderMap,
    State(s): State<TemplateHandlerState<'_>>,
) -> Response {
    let path = uri.to_string();
    let path = path.strip_prefix('/').unwrap_or(&path);

    let log = |msg: String| {
        s.templater
            .logger()
            .log(LogEntry::new(LogLevel::Info, "app", None, msg));
    };

    let Some(template) = s.templater.get_template(path) else {
        log(format!("handle_templates: get template for path: {path}"));
        return (StatusCode::INTERNAL_SERVER_ERROR, "template does not exist").into_response();
    };

    let (is_admin, token) = match s.auth.validate_request(&headers).await {
        Some(valid_login) => (valid_login.is_admin, valid_login.token),
        None => (false, String::new()),
    };

    let data = s.templater.get_data(path.to_owned(), is_admin, token).await;

    match template.render(data).to_string() {
        Ok(content) => (
            [(header::CONTENT_TYPE, "text/html; charset=UTF-8")],
            content,
        )
            .into_response(),
        Err(e) => {
            log(format!("handle_templates: render template '{path}': {e}"));
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                "failed to render template",
            )
                .into_response()
        }
    }
}

pub async fn asset_handler(
    Path(path): Path<String>,
    State(assets): State<EmbeddedFiles>,
) -> Response {
    match assets.get(path.as_str()) {
        Some(content) => {
            let body = Body::from(content.clone());
            let mime = mime_guess::from_path(path).first_or_octet_stream();
            ([(header::CONTENT_TYPE, mime.as_ref())], body).into_response()
        }
        None => (StatusCode::NOT_FOUND, "404").into_response(),
    }
}

#[allow(clippy::unwrap_used)]
pub async fn hls_handler(
    Path(path): Path<String>,
    State(hls_server): State<Arc<HlsServer>>,
    method: Method,
    req_headers: HeaderMap,
    query: Query<HlsQuery>,
) -> Response {
    let mut headers = HeaderMap::new();
    headers.insert("Server", HeaderValue::from_str("sentryshot").unwrap());
    headers.insert(
        "Access-Control-Allow-Credentials",
        HeaderValue::from_str("true").unwrap(),
    );

    match method {
        Method::GET => {}
        Method::OPTIONS => {
            headers.insert(
                "Access-Control-Allow-Methods",
                HeaderValue::from_static("GET, OPTIONS"),
            );
            headers.insert(
                "Access-Control-Allow-Headers",
                req_headers
                    .get("Access-Control-Request-Headers")
                    .unwrap_or(&HeaderValue::from_str("").unwrap())
                    .to_owned(),
            );
            return (StatusCode::OK, headers).into_response();
        }
        _ => return (StatusCode::METHOD_NOT_ALLOWED, headers).into_response(),
    };

    let (muxer_name, file_name) = match parse_path(path) {
        Ok(v) => v,
        Err(e) => {
            return (
                headers,
                Response::builder()
                    .status(StatusCode::BAD_REQUEST)
                    .body(format!("parse path: {e}"))
                    .unwrap(),
            )
                .into_response()
        }
    };

    let Some(Some(muxer)) = hls_server.muxer_by_name(muxer_name).await else {
        return (StatusCode::NOT_FOUND, headers).into_response();
    };
    let res = muxer.file(&file_name, &query.0).await;

    if let Some(h) = res.headers {
        for (k, v) in h {
            headers.insert(k, v);
        }
    }

    if let Some(body) = res.body {
        let stream = ReaderStream::new(body);
        let body = Body::from_stream(stream);
        (res.status, headers, body).into_response()
    } else {
        (headers, res.status).into_response()
    }
}

#[derive(Debug, Error)]
pub enum ParsePathError {
    #[error("no directory")]
    NoDir,

    #[error("invalid directory")]
    InvalidDir,

    #[error("no file name")]
    NoFileName,

    #[error("invalid file name")]
    InvalidFileName,
}

#[allow(clippy::case_sensitive_file_extension_comparisons)]
fn parse_path(path: String) -> Result<(String, String), ParsePathError> {
    use ParsePathError::*;
    if path.ends_with(".m3u8")
        || path.ends_with(".ts")
        || path.ends_with(".mp4")
        || path.ends_with(".mp")
    {
        let p = PathBuf::from(path);
        Ok((
            p.parent()
                .ok_or(NoDir)?
                .to_str()
                .ok_or(InvalidDir)?
                .to_owned(),
            p.file_name()
                .ok_or(NoFileName)?
                .to_str()
                .ok_or(InvalidFileName)?
                .to_owned(),
        ))
    } else {
        Ok((path, String::new()))
    }
}

const API_HTML: &str = include_str!("./api.html");

pub async fn api_page_handler() -> Response {
    (
        [(header::CONTENT_TYPE, "text/html; charset=UTF-8")],
        API_HTML,
    )
        .into_response()
}

#[derive(Debug, Deserialize)]
pub struct AccountDeleteQuery {
    pub id: String,
}

pub async fn account_delete_handler(
    State(auth): State<DynAuth>,
    query: Query<AccountDeleteQuery>,
) -> (StatusCode, String) {
    use AuthAccountDeleteError::*;
    match auth.account_delete(&query.id).await {
        Ok(()) => (StatusCode::OK, String::new()),
        Err(e @ AccountNotExist(_)) => (StatusCode::NOT_FOUND, e.to_string()),
        Err(SaveAccounts(e)) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()),
    }
}

pub async fn account_put_handler(
    State(auth): State<DynAuth>,
    Json(payload): Json<AccountSetRequest>,
) -> Response {
    match auth.account_set(payload).await {
        Ok(created) => {
            if created {
                StatusCode::CREATED.into_response()
            } else {
                StatusCode::OK.into_response()
            }
        }
        Err(e) => (StatusCode::BAD_REQUEST, e.to_string()).into_response(),
    }
}

pub async fn accounts_handler(State(auth): State<DynAuth>) -> Json<AccountsMap> {
    Json(auth.accounts().await)
}

pub async fn account_my_token_handler(
    State(auth): State<DynAuth>,
    request: Request<Body>,
) -> Response {
    match auth.validate_request(request.headers()).await {
        Some(res) => res.token.into_response(),
        None => (StatusCode::UNAUTHORIZED).into_response(),
    }
}

#[derive(Clone)]
pub struct RecordingQueryHandlerState {
    pub logger: Arc<Logger>,
    pub rec_db: Arc<RecDb>,
}

pub async fn recording_query_handler(
    State(s): State<RecordingQueryHandlerState>,
    query: Query<RecDbQuery>,
) -> Result<Json<Vec<RecordingResponse>>, StatusCode> {
    match s.rec_db.recordings_by_query(&query.0).await {
        Ok(v) => Ok(Json(v)),
        Err(e) => {
            s.logger.log(LogEntry::new(
                LogLevel::Error,
                "app",
                None,
                format!("crawler: could not process recording query: {e}"),
            ));
            Err(StatusCode::INTERNAL_SERVER_ERROR)
        }
    }
}

#[derive(Clone)]
pub struct LogFeedHandlerState {
    pub logger: Arc<log::Logger>,
    pub auth: DynAuth,
}

pub async fn log_feed_handler(
    State(s): State<LogFeedHandlerState>,
    headers: HeaderMap,
    query: Query<LogQuery>,
    ws: WebSocketUpgrade,
) -> Response {
    use axum::extract::ws::Message;

    let q = query.0;
    ws.on_upgrade(move |mut socket| async move {
        let mut feed = s.logger.subscribe();

        loop {
            let entry = match feed.recv().await {
                Ok(entry) => entry,
                Err(RecvError::Closed) => return,
                Err(RecvError::Lagged(_)) => continue,
            };

            if !q.entry_matches_filter(&entry) {
                continue;
            }

            // Validate auth before each message.
            if let Some(valid_login) = s.auth.validate_request(&headers).await {
                if !valid_login.is_admin {
                    return;
                }
            } else {
                return;
            };

            let entry_json =
                serde_json::to_string(&entry).expect("serializing `log::Entry` to never fail");

            if let Err(e) = socket.send(Message::Text(entry_json)).await {
                if e.to_string() == "IO error: Broken pipe (os error 32)" {
                    return;
                }
                s.logger.log(LogEntry::new(
                    LogLevel::Error,
                    "app",
                    None,
                    format!("log feed: {e}"),
                ));
                return;
            }
        }
    })
}

pub async fn log_query_handler(
    State(log_db): State<Arc<LogDbHandle>>,
    query: Query<LogQuery>,
) -> Response {
    match log_db.query(query.0).await {
        Ok(v) => Json::from(v).into_response(),
        Err(_) => StatusCode::INTERNAL_SERVER_ERROR.into_response(),
    }
}

#[derive(Debug, Deserialize)]
pub struct MonitorIdQuery {
    id: MonitorId,
}

pub async fn monitor_delete_handler(
    State(monitor_manager): State<Arc<Mutex<MonitorManager>>>,
    query: Query<MonitorIdQuery>,
) -> (StatusCode, String) {
    match monitor_manager.lock().await.monitor_delete(&query.id).await {
        Ok(()) => (StatusCode::OK, String::new()),
        Err(e @ MonitorDeleteError::NotExist(_)) => (StatusCode::NOT_FOUND, e.to_string()),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()),
    }
}

pub async fn monitor_put_handler(
    State(monitor_manager): State<Arc<Mutex<MonitorManager>>>,
    Json(payload): Json<MonitorConfig>,
) -> Response {
    match monitor_manager.lock().await.monitor_set(payload).await {
        Ok(created) => {
            if created {
                StatusCode::CREATED.into_response()
            } else {
                StatusCode::OK.into_response()
            }
        }
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response(),
    }
}

pub async fn monitor_restart_handler(
    State(monitor_manager): State<Arc<Mutex<MonitorManager>>>,
    query: Query<MonitorIdQuery>,
) -> Response {
    if let Err(e) = monitor_manager
        .lock()
        .await
        .monitor_restart(&query.id)
        .await
    {
        return (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response();
    };

    StatusCode::OK.into_response()
}

pub async fn monitors_handler(
    State(monitor_manager): State<Arc<Mutex<MonitorManager>>>,
) -> Json<MonitorConfigs> {
    Json(monitor_manager.lock().await.monitor_configs())
}

pub async fn recording_delete_handler(
    State(rec_db): State<Arc<RecDb>>,
    Path(rec_id): Path<RecordingId>,
) -> Response {
    match rec_db.delete_recording(rec_id).await {
        Ok(()) => StatusCode::OK.into_response(),
        Err(e @ DeleteRecordingError::Active) => {
            (StatusCode::BAD_REQUEST, e.to_string()).into_response()
        }
        Err(DeleteRecordingError::NotExist) => StatusCode::NOT_FOUND.into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response(),
    }
}

pub async fn recording_thumbnail_handler(
    State(rec_db): State<Arc<RecDb>>,
    Path(rec_id): Path<RecordingId>,
) -> Response {
    let Some(path) = rec_db.thumbnail_path(&rec_id).await else {
        return (StatusCode::NOT_FOUND).into_response();
    };

    let file = match tokio::fs::OpenOptions::new().read(true).open(path).await {
        Ok(v) => v,
        Err(e) => {
            return (StatusCode::INTERNAL_SERVER_ERROR, format!("open file: {e}")).into_response()
        }
    };

    let stream = ReaderStream::new(file);
    let body = Body::from_stream(stream);

    (([(header::CONTENT_TYPE, "image/jpeg")]), body).into_response()
}

#[derive(Clone)]
pub struct RecordingVideoState {
    pub rec_db: Arc<RecDb>,
    pub video_cache: Arc<Mutex<VideoCache>>,
    pub logger: DynLogger,
}

#[derive(Debug, Deserialize)]
pub struct RecordingVideoQuery {
    cache: Option<bool>,
}

pub async fn recording_video_handler(
    State(state): State<RecordingVideoState>,
    Path(rec_id): Path<RecordingId>,
    query: Query<RecordingVideoQuery>,
    headers: HeaderMap,
) -> Response {
    let Some(path) = state.rec_db.recording_file_by_ext(&rec_id, "meta").await else {
        return (StatusCode::NOT_FOUND).into_response();
    };

    let cache = query.cache.map_or(true, |v| v).then_some(state.video_cache);
    let video = match new_video_reader(path, &cache).await {
        Ok(v) => v,
        Err(e) => {
            state.logger.log(LogEntry::new(
                LogLevel::Error,
                "app",
                None,
                format!("video request: {e}"),
            ));
            return (StatusCode::INTERNAL_SERVER_ERROR, "see logs for details").into_response();
        }
    };

    serve_mp4_content(
        &Method::GET,
        &headers,
        video.last_modified(),
        video.size(),
        video,
    )
    .await
}
