// SPDX-License-Identifier: GPL-2.0-or-later

#![allow(clippy::unused_async)]

#[cfg(test)]
mod test;

use axum::{
    body::Body,
    extract::{Path, Query, State, WebSocketUpgrade},
    http::{header, HeaderMap, Method, StatusCode, Uri},
    response::{IntoResponse, Response},
    Extension, Json,
};
use common::{
    monitor::{ArcMonitorManager, MonitorConfig, MonitorConfigs, MonitorDeleteError},
    recording::RecordingId,
    AccountId, AccountSetRequest, AccountsMap, ArcAuth, ArcLogger, AuthAccountDeleteError,
    AuthenticatedUser, ILogger, LogEntry, LogLevel, MonitorId,
};
use hls::{HlsQuery, HlsServer};
use http::HeaderValue;
use log::{
    log_db::{entry_matches_query, LogDb, LogQuery},
    slow_poller::{self, PollQuery, SlowPoller},
    Logger,
};
use monitor_groups::ArcMonitorGroups;
use mp4_streamer::{Mp4Streamer, PlayReponse, Range, StartSessionReponse};
use recdb::{DeleteRecordingError, RecDb, RecDbQuery, RecordingResponse};
use recording::{new_video_reader, VideoCache};
use rust_embed::EmbeddedFiles;
use serde::Deserialize;
use std::{path::PathBuf, sync::Arc};
use thiserror::Error;
use tokio::sync::{broadcast::error::RecvError, Mutex};
use tokio_util::io::ReaderStream;
use vod::{CreateVodReaderError, VodCache, VodQuery, VodReader};
use web::{serve_mp4_content, Mp4StreamerRangeHeader, Templater};

pub async fn template_handler(
    Extension(user): Extension<AuthenticatedUser>,
    State(templater): State<Arc<Templater<'_>>>,
    uri: Uri,
) -> Response {
    let path = uri.to_string();
    let path = path.strip_prefix('/').unwrap_or(&path);

    let log = |msg: String| {
        templater
            .logger()
            .log(LogEntry::new(LogLevel::Info, "app", None, msg));
    };

    let Some(template) = templater.get_template(path) else {
        log(format!("handle_templates: get template for path: {path}"));
        return (StatusCode::INTERNAL_SERVER_ERROR, "template does not exist").into_response();
    };

    let data = templater
        .get_data(path.to_owned(), user.is_admin, user.stored_token)
        .await;

    match template.render(data).to_string() {
        Ok(content) => (
            [
                (header::CONTENT_TYPE, "text/html; charset=UTF-8"),
                (header::CACHE_CONTROL, "private, no-cache"),
            ],
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
    headers: HeaderMap,
    State(assets_and_etag): State<(EmbeddedFiles, String)>,
) -> Response {
    let (assets, etag) = assets_and_etag;

    if let Some(if_none_match) = headers.get(header::IF_NONE_MATCH) {
        if let Ok(if_none_match) = if_none_match.to_str() {
            if if_none_match == etag {
                return StatusCode::NOT_MODIFIED.into_response();
            }
        }
    }

    match assets.get(path.as_str()) {
        Some(content) => {
            let body = Body::from(content.clone());
            let mime = mime_guess::from_path(path).first_or_octet_stream();
            (
                [(header::CONTENT_TYPE, mime.as_ref()), (header::ETAG, &etag)],
                body,
            )
                .into_response()
        }
        None => (StatusCode::NOT_FOUND, "404").into_response(),
    }
}

#[allow(clippy::unwrap_used)]
pub async fn hls_handler(
    Path(path): Path<String>,
    State(hls_server): State<HlsServer>,
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
        return (StatusCode::NOT_FOUND, headers, "muxer not found").into_response();
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

#[derive(Clone)]
pub struct VodHandlerState {
    pub logger: Arc<Logger>,
    pub recdb: Arc<RecDb>,
    pub cache: VodCache,
}

pub async fn vod_handler(
    State(state): State<VodHandlerState>,
    query: Query<VodQuery>,
    headers: HeaderMap,
) -> Response {
    use CreateVodReaderError::*;
    let monitor_id = query.0.monitor_id.clone();
    let reader = match VodReader::new(&state.recdb, &state.cache, query.0).await {
        Ok(Some(v)) => v,
        Ok(None) => return (StatusCode::NOT_FOUND, "no video found").into_response(),
        Err(e @ (NegativeDuration | MaxDuration)) => {
            return (StatusCode::BAD_REQUEST, e.to_string()).into_response();
        }
        Err(e) => {
            state.logger.log(LogEntry::new(
                LogLevel::Error,
                "app",
                Some(monitor_id),
                format!("vod handler: {e}"),
            ));
            return (StatusCode::INTERNAL_SERVER_ERROR, "error printed to logs").into_response();
        }
    };
    serve_mp4_content(&Method::GET, &headers, None, reader.size(), reader).await
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
    pub id: AccountId,
}

pub async fn account_delete_handler(
    State(auth): State<ArcAuth>,
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
    State(auth): State<ArcAuth>,
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

pub async fn accounts_handler(State(auth): State<ArcAuth>) -> Json<AccountsMap> {
    Json(auth.accounts().await)
}

pub async fn account_my_token_handler(Extension(user): Extension<AuthenticatedUser>) -> Response {
    user.stored_token.into_response()
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
    pub auth: ArcAuth,
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

            if !entry_matches_query(&entry, &q) {
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

            if let Err(e) = socket.send(Message::Text(entry_json.into())).await {
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

pub async fn log_slow_poll_handler(
    State(poller): State<SlowPoller>,
    query: Query<PollQuery>,
) -> Response {
    match poller.slow_poll(query.0).await {
        slow_poller::Response::Ok(v) => Json::from(v).into_response(),
        slow_poller::Response::TooManyConncetions => StatusCode::TOO_MANY_REQUESTS.into_response(),
        slow_poller::Response::Cancelled => StatusCode::INTERNAL_SERVER_ERROR.into_response(),
    }
}

pub async fn log_query_handler(State(log_db): State<LogDb>, query: Query<LogQuery>) -> Response {
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
    State(monitor_manager): State<ArcMonitorManager>,
    query: Query<MonitorIdQuery>,
) -> (StatusCode, String) {
    match monitor_manager.monitor_delete(query.id.clone()).await {
        Ok(()) => (StatusCode::OK, String::new()),
        Err(e @ MonitorDeleteError::NotExist(_)) => (StatusCode::NOT_FOUND, e.to_string()),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()),
    }
}

pub async fn monitor_put_handler(
    State(monitor_manager): State<ArcMonitorManager>,
    Json(payload): Json<MonitorConfig>,
) -> Response {
    tokio::spawn(async move {
        match monitor_manager.monitor_set(payload).await {
            Ok(created) => {
                if created {
                    StatusCode::CREATED.into_response()
                } else {
                    StatusCode::OK.into_response()
                }
            }
            Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response(),
        }
    })
    .await
    .expect("join")
}

pub async fn monitor_restart_handler(
    State(monitor_manager): State<ArcMonitorManager>,
    query: Query<MonitorIdQuery>,
) -> Response {
    if let Err(e) = monitor_manager.monitor_restart(query.id.clone()).await {
        return (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response();
    };

    StatusCode::OK.into_response()
}

pub async fn monitors_handler(
    State(monitor_manager): State<ArcMonitorManager>,
) -> Json<MonitorConfigs> {
    Json(monitor_manager.monitor_configs().await.clone())
}

pub async fn monitor_groups_get_handler(
    State(monitor_groups): State<ArcMonitorGroups>,
) -> Json<monitor_groups::Groups> {
    Json(monitor_groups.get().await)
}

pub async fn monitor_groups_put_handler(
    State(monitor_groups): State<ArcMonitorGroups>,
    Json(payload): Json<monitor_groups::Groups>,
) -> Response {
    tokio::spawn(async move {
        match monitor_groups.set(payload).await {
            Ok(()) => StatusCode::OK.into_response(),
            Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response(),
        }
    })
    .await
    .expect("join")
}

#[derive(Debug, Deserialize)]
pub struct Mp4StreamerQuery {
    #[serde(rename = "session-id")]
    session_id: u32,

    #[serde(rename = "monitor-id")]
    monitor_id: MonitorId,

    #[serde(rename = "sub-stream")]
    sub_stream: bool,
}

pub async fn mp4_streamer_start_session_handler(
    State(mp4_streamer): State<Mp4Streamer>,
    query: Query<Mp4StreamerQuery>,
) -> Response {
    use StartSessionReponse::*;
    let result = mp4_streamer
        .start_session(query.monitor_id.clone(), query.sub_stream, query.session_id)
        .await;
    match result {
        Ready(start_time) => (StatusCode::OK, start_time.to_string()).into_response(),
        SessionAlreadyExist => (StatusCode::BAD_REQUEST, "session already exists").into_response(),
        NotReady | MuxerNotExist | StreamerCancelled | MuxerCancelled => {
            StatusCode::NOT_FOUND.into_response()
        }
    }
}

pub async fn mp4_streamer_play_handler(
    State(mp4_streamer): State<Mp4Streamer>,
    query: Query<Mp4StreamerQuery>,
    range: Mp4StreamerRangeHeader,
) -> Response {
    use PlayReponse::*;
    let range = match range {
        Mp4StreamerRangeHeader::Start(start) => Range::Start(start),
    };
    let result = mp4_streamer
        .play(
            query.monitor_id.clone(),
            query.sub_stream,
            query.session_id,
            range,
        )
        .await;
    match result {
        Ready(ready) => {
            let mut headers = HeaderMap::new();
            headers.insert(header::ACCEPT_RANGES, HeaderValue::from_static("bytes"));
            headers.insert(header::CONTENT_TYPE, HeaderValue::from_static("video/mp4"));
            /*headers.insert(
                header::CONTENT_LENGTH,
                HeaderValue::from_str(&(ready.length).to_string()).unwrap(),
            );*/

            headers.insert(
                header::CONTENT_RANGE,
                HeaderValue::from_str(&format!("bytes {}-{}/*", ready.start, ready.end()))
                    .expect("valid header string"),
            );
            (
                StatusCode::PARTIAL_CONTENT,
                headers,
                Body::from_stream(ready.body),
            )
                .into_response()
        }
        NotImplemented(err) => (StatusCode::INTERNAL_SERVER_ERROR, err).into_response(),
        FramesExpired => StatusCode::GONE.into_response(),
        SessionNotExist => (
            StatusCode::PRECONDITION_REQUIRED,
            "start a session with /api/mp4-streamer/start-session first",
        )
            .into_response(),
        NotReady | MuxerNotExist | StreamerCancelled | MuxerCancelled => {
            StatusCode::NOT_FOUND.into_response()
        }
    }
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

    (
        ([
            (header::CONTENT_TYPE, "image/jpeg"),
            (header::CACHE_CONTROL, "max-age=31536000, immutable"),
        ]),
        body,
    )
        .into_response()
}

#[derive(Clone)]
pub struct RecordingVideoState {
    pub rec_db: Arc<RecDb>,
    pub video_cache: Arc<Mutex<VideoCache>>,
    pub logger: ArcLogger,
}

#[derive(Debug, Deserialize)]
pub struct RecordingVideoQuery {
    #[serde(default, rename = "cache-id")]
    cache_id: u32,
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

    let video = match new_video_reader(path, query.cache_id, &Some(state.video_cache)).await {
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
        Some(video.last_modified()),
        video.size(),
        video,
    )
    .await
}
