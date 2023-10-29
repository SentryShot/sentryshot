// SPDX-License-Identifier: GPL-2.0-or-later

#[cfg(test)]
mod test;

use axum::{
    body::{boxed, Full, StreamBody},
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
use crawler::{Crawler, CrawlerQuery, CrawlerRecording};
use hls::{HlsQuery, HlsServer};
use http::HeaderValue;
use log::{
    log_db::{LogDbHandle, LogQuery},
    Logger,
};
use monitor::{MonitorDeleteError, MonitorManager};
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
        s.templater.logger().log(LogEntry {
            level: LogLevel::Info,
            source: "app".parse().unwrap(),
            monitor_id: None,
            message: msg.parse().unwrap(),
        });
    };

    let Some(template) = s.templater.get_template(path) else {
        log(format!("handle_templates: get template for path: {path}"));
        return (StatusCode::INTERNAL_SERVER_ERROR, "template does not exist").into_response();
    };

    let (is_admin, token) = match s.auth.validate_request(&headers).await {
        Some(valid_login) => (valid_login.is_admin, valid_login.token),
        None => (false, "".to_owned()),
    };

    let data = s
        .templater
        .get_data(path.to_string(), is_admin, token)
        .await;

    match template.render(data) {
        Ok(content) => {
            let body = boxed(Full::from(content));
            Response::builder()
                .header(header::CONTENT_TYPE, "text/html; charset=UTF-8")
                .body(body)
                .unwrap()
        }
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
            let body = boxed(Full::from(content.clone()));
            let mime = mime_guess::from_path(path).first_or_octet_stream();
            Response::builder()
                .header(header::CONTENT_TYPE, mime.as_ref())
                .body(body)
                .unwrap()
        }
        None => (StatusCode::NOT_FOUND, "404").into_response(),
    }
}

pub async fn hls_handler(
    Path(path): Path<String>,
    State(hls_server): State<Arc<HlsServer>>,
    method: Method,
    req_headers: HeaderMap,
    query: Query<HlsQuery>,
) -> Response {
    let mut headers = HeaderMap::new();
    headers.insert(
        "Server",
        HeaderValue::from_str("server name TODO:").unwrap(),
    );
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
                    .body(format!("parse path: {}", e))
                    .unwrap(),
            )
                .into_response()
        }
    };

    let Ok(Some(muxer)) = hls_server.muxer_by_name(muxer_name).await else {
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
        let body = StreamBody::new(stream);
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
        Ok((path, "".to_owned()))
    }
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
        Ok(_) => (StatusCode::OK, "".to_string()),
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

#[derive(Clone)]
pub struct RecordingQueryHandlerState {
    pub logger: Arc<Logger>,
    pub crawler: Arc<Crawler>,
}

pub async fn recording_query_handler(
    State(s): State<RecordingQueryHandlerState>,
    query: Query<CrawlerQuery>,
) -> Result<Json<Vec<CrawlerRecording>>, StatusCode> {
    match s.crawler.recordings_by_query(&query.0).await {
        Ok(v) => Ok(Json(v)),
        Err(e) => {
            s.logger.log(LogEntry {
                level: LogLevel::Error,
                source: "app".parse().unwrap(),
                monitor_id: None,
                message: format!("crawler: could not process recording query: {}", e)
                    .parse()
                    .unwrap(),
            });
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
    let q = query.0;
    ws.on_upgrade(move |mut socket| async move {
        let mut feed = s.logger.subscribe();

        loop {
            let entry = match feed.recv().await {
                Ok(entry) => entry,
                Err(RecvError::Closed) => return,
                Err(RecvError::Lagged(_)) => continue,
            };

            // Filter level.
            if let Some(levels) = &q.levels {
                if !levels.contains(&entry.level) {
                    continue;
                }
            }

            // Filter source.
            if let Some(sources) = &q.sources {
                if !sources.contains(&entry.source) {
                    continue;
                }
            }

            // Filter monitor id.
            if let Some(monitors) = &q.monitors {
                let Some(monitor_id) = &entry.monitor_id else {
                    continue
                };
                if !monitors.contains(monitor_id) {
                    continue;
                }
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

            use axum::extract::ws::Message;
            if let Err(e) = socket.send(Message::Text(entry_json)).await {
                if e.to_string() == "IO error: Broken pipe (os error 32)" {
                    return;
                }
                s.logger.log(LogEntry {
                    level: LogLevel::Error,
                    source: "app".parse().unwrap(),
                    monitor_id: None,
                    message: format!("log feed: {e}").parse().unwrap(),
                });
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
        Ok(_) => (StatusCode::OK, "".to_owned()),
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

pub async fn recording_thumbnail_handler(
    State(recordings_dir): State<PathBuf>,
    Path(rec_id): Path<RecordingId>,
) -> Response {
    // Make sure json file exist.
    let mut path = recordings_dir.join(rec_id.as_full_path());
    path.set_extension("json");
    let Ok(mut path) = tokio::fs::canonicalize(path).await else {
        return (StatusCode::BAD_REQUEST, "canonicalize").into_response();
    };

    // Check if thumbnail exists.
    path.set_extension("jpeg");
    let Ok(path) = tokio::fs::canonicalize(path).await else {
        return (StatusCode::BAD_REQUEST, "canonicalize").into_response();
    };

    let path_is_safe = path.starts_with(&recordings_dir);
    if !path_is_safe {
        return (StatusCode::BAD_REQUEST, "path traversal").into_response();
    };

    let file = match tokio::fs::OpenOptions::new().read(true).open(path).await {
        Ok(v) => v,
        Err(e) => return (StatusCode::NOT_FOUND, format!("open file: {}", e)).into_response(),
    };

    let stream = ReaderStream::new(file);
    let body = StreamBody::new(stream);

    (([(header::CONTENT_TYPE, "image/jpeg")]), body).into_response()
}

#[derive(Clone)]
pub struct RecordingVideoState {
    pub recordings_dir: PathBuf,
    pub video_cache: Arc<Mutex<VideoCache>>,
    pub logger: DynLogger,
}

pub async fn recording_video_handler(
    State(state): State<RecordingVideoState>,
    Path(rec_id): Path<RecordingId>,
    headers: HeaderMap,
) -> Response {
    // Make sure json file exist.
    let mut path = state.recordings_dir.join(rec_id.as_full_path());
    path.set_extension("json");

    let Ok(path) = tokio::fs::canonicalize(path).await else {
        return (StatusCode::BAD_REQUEST, "canonicalize").into_response();
    };

    let path_is_safe = path.starts_with(&state.recordings_dir);
    if !path_is_safe {
        return (StatusCode::BAD_REQUEST, "path traversal").into_response();
    };

    let video = match new_video_reader(path, &Some(state.video_cache)).await {
        Ok(v) => v,
        Err(e) => {
            state.logger.log(LogEntry {
                level: LogLevel::Error,
                source: "app".parse().unwrap(),
                monitor_id: None,
                message: format!("video request: {}", e).parse().unwrap(),
            });
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
