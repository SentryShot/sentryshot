use super::{LogEntryWithTime, Logger, UnixMicro};
use crate::rev_buf_reader::RevBufReader;
use bytesize::ByteSize;
use common::{
    DynLogger, LogEntry, LogLevel, LogSource, MonitorId, ParseLogLevelError, ParseLogSourceError,
    ParseMonitorIdError, ParseNonEmptyStringError, LOG_SOURCE_MAX_LENGTH, MONITOR_ID_MAX_LENGTH,
};
use csv::{deserialize_csv_option, deserialize_csv_option2};
use futures::TryFutureExt;
use serde::Deserialize;
use std::{
    cmp::Ordering,
    io::SeekFrom,
    num::NonZeroUsize,
    path::{Path, PathBuf},
    sync::Arc,
    time::Duration,
};
use thiserror::Error;
use tokio::{
    fs::File,
    io::{AsyncRead, AsyncReadExt, AsyncSeek, AsyncSeekExt, AsyncWrite, AsyncWriteExt},
    sync::{mpsc, Mutex},
};
use tokio_util::sync::CancellationToken;

// chunk {
//     file.data
//     file.msg
// }
//
// file.data {
//     version u8
//     [data]
// }
//
// data {
//     time u64
//     src [srcMaxLength; u8]
//     monitorID [idMaxLength; u8]
//     msgOffset u32
//     msgSize u16
//     level u8
// }

// 16666 minutes or 27.7 hours.
const CHUNK_DURATION: u64 = 1_000_000 * SECOND;
const SECOND: u64 = 100_000;

const CHUNK_API_VERSION: u8 = 0;
const CHUNK_ID_LENGTH: usize = 5;
const CHUNK_HEADER_LENGTH: u64 = 1;

const DATA_SIZE: usize = 47;

#[allow(clippy::module_name_repetitions)]
pub struct LogDbHandle(Mutex<LogDb>);

impl LogDbHandle {
    pub async fn save_log_testing(&self, entry: LogEntryWithTime) {
        #[allow(clippy::unwrap_used)]
        self.save_log(entry).await.unwrap();
    }

    async fn save_log(&self, entry: LogEntryWithTime) -> Result<(), SaveLogError> {
        self.0.lock().await.save_log(entry).await
    }

    pub async fn query(&self, q: LogQuery) -> Result<Vec<LogEntryWithTime>, QueryLogsError> {
        self.0.lock().await.query(q).await
    }

    async fn prune(&self) -> Result<(), PurgeError> {
        self.0.lock().await.prune().await
    }

    // Saves logs from the logger into the database.
    pub async fn save_logs(&self, token: CancellationToken, logger: Arc<Logger>) {
        let mut feed = logger.subscribe();
        loop {
            tokio::select! {
                () = token.cancelled() => return,
                log = feed.recv() => {
                    let Ok(log) = log else {
                        return
                    };
                    if let Err(e) = self.save_log(log.clone()).await {
                        eprintln!("could not save log: {} {}", log.message, e);
                    }
                }
            }
        }
    }

    // Prunes logs every hour.
    pub async fn prune_loop(&self, token: CancellationToken, logger: DynLogger) {
        const MINUTE: u64 = 60;
        const HOUR: u64 = 60 * MINUTE;
        loop {
            tokio::select! {
                () = token.cancelled() => return,
                () = tokio::time::sleep(Duration::from_secs(HOUR)) => {
                    if let Err(e) = self.prune().await {
                        logger.log(LogEntry::new(
                            LogLevel::Error,
                            "app",
                            None,
                            format!("could not purge logs: {e}"),
                        ));
                    }
                }
            }
        }
    }
}

pub struct LogDb {
    log_dir: PathBuf,
    encoder: Option<ChunkEncoder>,

    // Keep track of the previous entry time to ensure
    // that the next entry will have a later time.
    prev_entry_time: UnixMicro,

    // The database will use up to 1% of total disk space or `min_disk_usage`.
    disk_space: ByteSize,
    min_disk_usage: ByteSize,

    _shutdown_complete: mpsc::Sender<()>,
}

#[derive(Debug, Error)]
pub enum NewLogDbError {
    #[error("make log directory: {0} {1}")]
    MakeLogDir(String, std::io::Error),
}

impl LogDb {
    #[allow(clippy::new_ret_no_self)]
    pub fn new(
        shutdown_complete: mpsc::Sender<()>,
        log_dir: PathBuf,
        disk_space: ByteSize,
        min_disk_usage: ByteSize,
    ) -> Result<LogDbHandle, NewLogDbError> {
        std::fs::create_dir_all(&log_dir)
            .map_err(|e| NewLogDbError::MakeLogDir(log_dir.to_string_lossy().to_string(), e))?;

        Ok(LogDbHandle(Mutex::new(Self {
            log_dir,
            encoder: None,
            prev_entry_time: UnixMicro::new(0),
            disk_space,
            min_disk_usage,
            _shutdown_complete: shutdown_complete,
        })))
    }

    async fn save_log(&mut self, mut entry: LogEntryWithTime) -> Result<(), SaveLogError> {
        let chunk_id = time_to_id(entry.time)?;

        let encoder = if let Some(encoder) = &mut self.encoder {
            if chunk_id == encoder.chunk_id {
                encoder
            } else {
                let (encoder, prev_entry_time) =
                    ChunkEncoder::new(self.log_dir.clone(), chunk_id).await?;
                self.prev_entry_time = prev_entry_time;
                self.encoder.insert(encoder)
            }
        } else {
            let (encoder, prev_entry_time) =
                ChunkEncoder::new(self.log_dir.clone(), chunk_id).await?;
            self.prev_entry_time = prev_entry_time;
            self.encoder.insert(encoder)
        };

        if entry.time <= self.prev_entry_time {
            entry.time = self
                .prev_entry_time
                .checked_add(UnixMicro::new(1))
                .ok_or(SaveLogError::IncrementPrevTime)?;
        }

        encoder.encode(&entry).await?;

        self.prev_entry_time = entry.time;

        Ok(())
    }

    // Query logs in database.
    async fn query(&self, mut q: LogQuery) -> Result<Vec<LogEntryWithTime>, QueryLogsError> {
        let chunk_ids = self.list_chunks_before(q.time).await?;

        let mut entries = Vec::new();
        //for i := len(chunkIDs) - 1; i >= 0; i-- {
        for chunk_id in chunk_ids.iter().rev() {
            if let Err(e) = self.query_chunk(&q, &mut entries, chunk_id).await {
                eprintln!("log store warning: {e}");
            }
            // Time is only relevant for the first iteration.
            q.time = None;
        }

        Ok(entries)
    }

    async fn query_chunk(
        &self,
        q: &LogQuery,
        entries: &mut Vec<LogEntryWithTime>,
        chunk_id: &String,
    ) -> Result<(), QueryChunkError> {
        let mut decoder = ChunkDecoder::new(&self.log_dir, chunk_id).await?;

        let entry_index = {
            if let Some(time) = q.time {
                decoder.search(time).await?
            } else {
                decoder.last_index() + 1
            }
        };

        for i in (0..entry_index).rev() {
            // Limit check.
            if let Some(limit) = q.limit {
                if entries.len() >= limit.get() {
                    break;
                }
            }

            let entry = match decoder.decode(i).await {
                Ok((v, _)) => v,
                Err(e @ DecodeError::RecoverableDecodeEntry(..)) => {
                    let (data_path, _) = chunk_id_to_paths(&self.log_dir, chunk_id);
                    eprintln!("log store warning: {data_path:?} {e}");
                    continue;
                }
                Err(e) => return Err(QueryChunkError::Decode(e)),
            };

            if !q.entry_matches_filter(&entry) {
                continue;
            }

            match time_to_id(entry.time) {
                Ok(entry_chunk_id) => {
                    if entry_chunk_id != *chunk_id {
                        continue;
                    }
                }
                Err(_) => continue,
            }

            entries.push(entry);
        }

        Ok(())
    }

    async fn list_chunks_before(
        &self,
        time: Option<UnixMicro>,
    ) -> Result<Vec<String>, ListChunksBeforeError> {
        let chunks = self.list_chunks().await?;
        let Some(time) = time else {
            return Ok(chunks);
        };

        let before_id = time_to_id(time)?;

        let mut filtered = Vec::new();
        for chunk in chunks {
            if let Ordering::Greater = chunk.cmp(&before_id) {
                continue;
            }
            filtered.push(chunk);
            /* if strings.Compare(chunk, beforeID) <= 0 {
                filtered.push(chunk);
            }*/
        }
        Ok(filtered)
    }

    async fn list_chunks(&self) -> Result<Vec<String>, std::io::Error> {
        let log_dir = self.log_dir.clone();

        tokio::task::spawn_blocking(|| {
            let mut chunks = Vec::new();
            for file in std::fs::read_dir(log_dir)? {
                let file = file?;
                let name = file
                    .file_name()
                    .into_string()
                    .unwrap_or_else(|_| String::new());

                let is_data_file = Path::new(&name)
                    .extension()
                    .map_or(false, |ext| ext.eq_ignore_ascii_case("data"));
                if name.len() < CHUNK_ID_LENGTH + 5 || !is_data_file {
                    continue;
                }
                chunks.push(name[..CHUNK_ID_LENGTH].to_owned());
            }
            chunks.sort();

            Ok(chunks)
        })
        .await
        .expect("join")
    }

    // Prunes a single chunk if needed.
    async fn prune(&self) -> Result<(), PurgeError> {
        use PurgeError::*;
        let dir_size = dir_size(self.log_dir.clone()).await?;

        if dir_size <= ByteSize(self.disk_space.as_u64() / 100) || dir_size <= self.min_disk_usage {
            return Ok(());
        }

        let chunks = self.list_chunks().await.map_err(ListChunks)?;
        let Some(chunk_to_remove) = chunks.first() else {
            // No chunks.
            return Ok(());
        };

        let (data_path, msg_path) = chunk_id_to_paths(&self.log_dir, chunk_to_remove);

        tokio::fs::remove_file(&data_path)
            .await
            .map_err(|e| RemoveDataFile(data_path.to_string_lossy().to_string(), e))?;
        tokio::fs::remove_file(&msg_path)
            .await
            .map_err(|e| RemoveMsgFile(msg_path.to_string_lossy().to_string(), e))?;

        Ok(())
    }
}

#[derive(Debug, Error)]
enum QueryChunkError {
    #[error("new chunk decoder: {0}")]
    NewChunkDecoder(#[from] NewChunkDecoderError),

    #[error("search: {0}")]
    Search(#[from] DecoderSearchError),

    #[error("decode: {0}")]
    Decode(#[from] DecodeError),
}

#[derive(Debug, Error)]
pub enum ListChunksBeforeError {
    #[error("list chunks: {0}")]
    ListChunks(#[from] std::io::Error),

    #[error("time to id: {0}")]
    TimeToId(#[from] TimeToIdError),
}

#[derive(Debug, Error)]
pub enum QueryLogsError {
    #[error("list chunks before: {0}")]
    ListChunksBefore(#[from] ListChunksBeforeError),
}

#[derive(Debug, Error)]
enum SaveLogError {
    #[error("{0}")]
    TimeToId(#[from] TimeToIdError),

    #[error("new chunk encoder: {0}")]
    NewChunkEncoder(#[from] NewChunkEncoderError),

    #[error("increment prev time")]
    IncrementPrevTime,

    #[error("{0}")]
    Encode(#[from] EncodeError),
}

#[derive(Debug, Error)]
enum PurgeError {
    #[error("dir size: {0}")]
    DirSize(#[from] DirSizeError),

    #[error("list chunks: {0}")]
    ListChunks(std::io::Error),

    #[error("remove data file: {0} {1}")]
    RemoveDataFile(String, std::io::Error),

    #[error("remove msg file: {0} {1}")]
    RemoveMsgFile(String, std::io::Error),
}

#[derive(Default, Deserialize)]
pub struct LogQuery {
    #[serde(default)]
    #[serde(deserialize_with = "deserialize_csv_option2")]
    pub levels: Vec<LogLevel>,

    #[serde(default)]
    #[serde(deserialize_with = "deserialize_csv_option")]
    pub sources: Vec<LogSource>,

    pub time: Option<UnixMicro>,

    #[serde(default)]
    #[serde(deserialize_with = "deserialize_csv_option")]
    pub monitors: Vec<MonitorId>,

    pub limit: Option<NonZeroUsize>,
}

impl LogQuery {
    #[must_use]
    pub fn entry_matches_filter(&self, entry: &LogEntryWithTime) -> bool {
        level_in_levels(entry.level, &self.levels)
            && source_in_souces(&entry.source, &self.sources)
            && monitor_id_in_monitor_ids(&entry.monitor_id, &self.monitors)
    }
}

// Returns true if level is in levels or if levels is empty.
fn level_in_levels(level: LogLevel, levels: &[LogLevel]) -> bool {
    levels.is_empty() || levels.contains(&level)
}

// Returns true if source is in sources or if sources is empty.
fn source_in_souces(source: &LogSource, sources: &[LogSource]) -> bool {
    sources.is_empty() || sources.contains(source)
}

// Returns true if monitor_id is in monitors or if monitor_ids is empty.
fn monitor_id_in_monitor_ids(monitor_id: &Option<MonitorId>, monitors_ids: &[MonitorId]) -> bool {
    let Some(monitor_id) = monitor_id else {
        return true;
    };
    monitors_ids.is_empty() || monitors_ids.contains(monitor_id)
}

#[derive(Debug, Error)]
enum DirSizeError {
    #[error("read dir: {0}")]
    ReadDir(std::io::Error),

    #[error("dir entry: {0}")]
    DirEntry(std::io::Error),

    #[error("metadata: {0}")]
    Metadata(std::io::Error),

    #[error("add")]
    Add,
}

async fn dir_size(path: PathBuf) -> Result<ByteSize, DirSizeError> {
    use DirSizeError::*;

    tokio::task::spawn_blocking(|| {
        let mut total: u64 = 0;
        for file in std::fs::read_dir(path).map_err(ReadDir)? {
            let file = file.map_err(DirEntry)?;
            let metadata = file.metadata().map_err(Metadata)?;
            total = total.checked_add(metadata.len()).ok_or(Add)?;
        }
        Ok(ByteSize(total))
    })
    .await
    .expect("join")
}

fn chunk_id_to_paths(log_dir: &Path, chunk_id: &str) -> (PathBuf, PathBuf) {
    let data_path = log_dir.join(chunk_id.to_owned() + ".data");
    let msg_path = log_dir.join(chunk_id.to_owned() + ".msg");
    (data_path, msg_path)
}

struct ChunkDecoder {
    n_entries: usize,
    data_file: RevBufReader<File>,
    msg_file: RevBufReader<File>,
}

#[derive(Debug, Error)]
enum NewChunkDecoderError {
    #[error("open data file: {0}")]
    OpenDataFile(std::io::Error),

    #[error("read version: {0}")]
    ReadVersion(std::io::Error),

    #[error("unknown chunk api version")]
    UnknownChunkVersion,

    #[error("data file metadata: {0}")]
    DataFileMetadata(std::io::Error),

    #[error("open msg file: {0} {1}")]
    OpenMsgFile(PathBuf, std::io::Error),

    #[error("calculate entries: {0}")]
    CalculateEntries(#[from] CalculateEntriesError),
}

impl ChunkDecoder {
    async fn new(log_dir: &Path, chunk_id: &str) -> Result<Self, NewChunkDecoderError> {
        use NewChunkDecoderError::*;
        let (data_path, msg_path) = chunk_id_to_paths(log_dir, chunk_id);

        let mut data_file = tokio::fs::OpenOptions::new()
            .read(true)
            .open(data_path)
            .await
            .map_err(OpenDataFile)?;

        let mut version = vec![0; 1];
        data_file
            .read_exact(&mut version)
            .await
            .map_err(ReadVersion)?;

        if version[0] != 0 {
            return Err(UnknownChunkVersion);
        }

        let data_file_size = data_file.metadata().await.map_err(DataFileMetadata)?.len();

        let msg_file = tokio::fs::OpenOptions::new()
            .read(true)
            .open(&msg_path)
            .await
            .map_err(|e| OpenMsgFile(msg_path, e))?;

        let msg_file = RevBufReader::new(msg_file);
        let data_file = RevBufReader::new(data_file);

        Ok(Self {
            msg_file,
            data_file,
            n_entries: calculate_n_entries(data_file_size)?,
        })
    }

    fn last_index(&self) -> usize {
        self.n_entries - 1
    }

    // Binary search.
    async fn search(&mut self, time: UnixMicro) -> Result<usize, DecoderSearchError> {
        let (mut l, mut r) = (0, self.n_entries - 1);
        while l <= r {
            let i = (l + r) / 2;
            let (entry, _) = self.decode(i).await?;

            match entry.time.cmp(&time) {
                Ordering::Less => l = i + 1,
                Ordering::Equal => return Ok(i),
                Ordering::Greater => r = i - 1,
            }
        }
        Ok(l)
    }

    async fn decode(&mut self, index: usize) -> Result<(LogEntryWithTime, u32), DecodeError> {
        use DecodeError::*;
        let index = u64::try_from(index)?;
        let data_size_u64 = u64::try_from(DATA_SIZE)?;
        let entry_pos: u64 = CHUNK_HEADER_LENGTH
            .checked_add(index.checked_mul(data_size_u64).ok_or(Mul)?)
            .ok_or(Add)?;

        self.data_file
            .seek(SeekFrom::Start(entry_pos))
            .await
            .map_err(Seek)?;

        let mut raw_entry = [0; DATA_SIZE];
        self.data_file
            .read_exact(&mut raw_entry)
            .await
            .map_err(Read)?;

        decode_entry(&raw_entry, &mut self.msg_file)
            .await
            .map_err(|e| RecoverableDecodeEntry(index, entry_pos, e))
    }
}

#[derive(Error, Debug)]
enum CalculateDataEndError {
    #[error("{0}")]
    CalculateEntries(#[from] CalculateEntriesError),

    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),

    #[error("add")]
    Add,

    #[error("mul")]
    Mul,
}

fn calculate_data_end(size: u64) -> Result<u64, CalculateDataEndError> {
    use CalculateDataEndError::*;
    let n_entries = calculate_n_entries(size)?;

    CHUNK_HEADER_LENGTH
        .checked_add(u64::try_from(n_entries.checked_mul(DATA_SIZE).ok_or(Mul)?)?)
        .ok_or(Add)
}

#[derive(Debug, Error)]
enum DecodeError {
    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),

    #[error("add")]
    Add,

    #[error("mul")]
    Mul,

    #[error("seek")]
    Seek(std::io::Error),

    #[error("read: {0}")]
    Read(std::io::Error),

    #[error("decode entry: index:{0} pos:{1} error: {2}")]
    RecoverableDecodeEntry(u64, u64, RecoverableDecodeEntryError),
}

#[derive(Debug, Error)]
enum DecoderSearchError {
    #[error("decode: {0}")]
    Decode(#[from] DecodeError),
}

#[derive(Debug, Error)]
enum CalculateEntriesError {
    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),

    #[error("sub")]
    Sub,

    #[error("div")]
    Div,
}

fn calculate_n_entries(size: u64) -> Result<usize, CalculateEntriesError> {
    use CalculateEntriesError::*;
    // (size - chunkHeaderLength) / dataSize
    Ok(usize::try_from(
        size.checked_sub(CHUNK_HEADER_LENGTH)
            .ok_or(Sub)?
            .checked_div(u64::try_from(DATA_SIZE)?)
            .ok_or(Div)?,
    )?)
}

#[derive(Debug, Error)]
enum NewChunkEncoderError {
    #[error("open file: {0}")]
    OpenFile(std::io::Error),

    #[error("write file: {0}")]
    WriteFile(std::io::Error),

    #[error("flush: {0}")]
    Flush(std::io::Error),

    #[error("open data file: {0}")]
    OpenDataFile(std::io::Error),

    #[error("seek to data end: {0}")]
    SeekToDataEnd(std::io::Error),

    #[error("open msg file: {0}")]
    OpenMsgFile(std::io::Error),

    #[error("seek to msg end: {0}")]
    SeekToMsgEnd(std::io::Error),

    #[error("new chunk decoder: {0}")]
    NewChunkDecoder(#[from] NewChunkDecoderError),

    #[error("decode: '{0}' {1}")]
    Decode(PathBuf, DecodeError),

    #[error("calculate data end: {0}")]
    CalculateDataEnd(#[from] CalculateDataEndError),

    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),
}

struct ChunkEncoder {
    chunk_id: String,
    data_file: File,
    msg_file: File,
    msg_pos: u32,
}

impl ChunkEncoder {
    async fn new(
        log_dir: PathBuf,
        chunk_id: String,
    ) -> Result<(Self, UnixMicro), NewChunkEncoderError> {
        use NewChunkEncoderError::*;
        let (data_path, msg_path) = chunk_id_to_paths(&log_dir, &chunk_id);

        let mut data_end = CHUNK_HEADER_LENGTH;
        let data_file_size = get_file_size(&data_path).await;
        let mut prev_entry_time = UnixMicro::new(0);
        let mut msg_pos = 0;
        if data_file_size == 0 {
            let mut file = tokio::fs::OpenOptions::new()
                .create(true)
                .truncate(true)
                .write(true)
                .open(&data_path)
                .await
                .map_err(OpenFile)?;

            file.write_all(&CHUNK_API_VERSION.to_be_bytes())
                .await
                .map_err(WriteFile)?;

            file.flush().await.map_err(Flush)?;
        } else {
            let mut decoder = ChunkDecoder::new(&log_dir, &chunk_id).await?;

            // Find the first valid entry from the end.
            // Treat file as empty if no valid entry is found.
            let last_index = decoder.last_index();
            for i in (0..=last_index).rev() {
                let (last_entry, msg_offset) = match decoder.decode(i).await {
                    Ok(v) => v,
                    Err(e @ DecodeError::RecoverableDecodeEntry(..)) => {
                        eprintln!("log store warning: {data_path:?} {e}");
                        continue;
                    }
                    Err(e) => return Err(NewChunkEncoderError::Decode(data_path.clone(), e)),
                };

                prev_entry_time = last_entry.time;
                data_end = calculate_data_end(data_file_size)?;
                msg_pos = msg_offset + u32::try_from(last_entry.message.len())? + 1;
            }
        }

        let mut data_file = tokio::fs::OpenOptions::new()
            .read(true)
            .write(true)
            .open(data_path)
            .await
            .map_err(OpenDataFile)?;

        data_file
            .seek(SeekFrom::Start(data_end))
            .await
            .map_err(SeekToDataEnd)?;

        let mut msg_file = tokio::fs::OpenOptions::new()
            .create(true)
            .read(true)
            .write(true)
            .open(msg_path)
            .await
            .map_err(OpenMsgFile)?;

        msg_file
            .seek(SeekFrom::Start(msg_pos.into()))
            .await
            .map_err(SeekToMsgEnd)?;

        Ok((
            Self {
                chunk_id,
                data_file,
                msg_file,
                msg_pos,
            },
            prev_entry_time,
        ))
    }

    async fn encode(&mut self, entry: &LogEntryWithTime) -> Result<(), EncodeError> {
        let mut buf = Vec::with_capacity(DATA_SIZE);
        encode_entry(&mut buf, entry, &mut self.msg_file, &mut self.msg_pos).await?;

        self.data_file
            .write_all(&buf)
            .await
            .map_err(EncodeError::Write)?;

        self.data_file.flush().await.map_err(EncodeError::Flush)?;

        Ok(())
    }
}

#[derive(Debug, Error)]
enum EncodeError {
    #[error("encode entry: {0}")]
    EncodeEntry(#[from] EncodeEntryError),

    #[error("write: {0}")]
    Write(std::io::Error),

    #[error("flush: {0}")]
    Flush(std::io::Error),
}

#[derive(Debug, Error)]
enum EncodeEntryError {
    #[error("write: {0}")]
    Write(#[from] std::io::Error),

    #[error("flush: {0}")]
    Flush(std::io::Error),

    #[error("{0}")]
    TryIntError(#[from] std::num::TryFromIntError),

    #[error("add")]
    Add,
}

async fn encode_entry(
    buf: &mut (impl AsyncWrite + Unpin),
    entry: &LogEntryWithTime,
    msg_file: &mut (impl AsyncWrite + Unpin),
    msg_offset: &mut u32,
) -> Result<(), EncodeEntryError> {
    use EncodeEntryError::*;
    let src_length = entry.source.len();

    let id_length = {
        if let Some(monitor_id) = &entry.monitor_id {
            monitor_id.len()
        } else {
            0
        }
    };

    // Write message and newline.
    msg_file.write_all(entry.message.as_bytes()).await?;
    msg_file.write_all(&[b'\n']).await?;
    msg_file.flush().await.map_err(EncodeEntryError::Flush)?;

    // Time.
    buf.write_all(entry.time.to_be_bytes().as_slice()).await?;

    // Source.
    buf.write_all(entry.source.as_bytes()).await?;
    buf.write_all(&b" ".repeat(LOG_SOURCE_MAX_LENGTH - src_length))
        .await?;

    // Monitor ID.
    if let Some(monitor_id) = &entry.monitor_id {
        buf.write_all(monitor_id.as_bytes()).await?;
    }
    buf.write_all(&b" ".repeat(MONITOR_ID_MAX_LENGTH - id_length))
        .await?;

    // Message offset and size.
    buf.write_all(&msg_offset.to_be_bytes()).await?;
    buf.write_all(&u16::try_from(entry.message.len())?.to_be_bytes())
        .await?;

    // Level.
    buf.write_all(&entry.level.as_u8().to_be_bytes()).await?;

    // *msg_offset += entry.message.len() + 1
    *msg_offset = msg_offset
        .checked_add(u32::try_from(entry.message.len())?)
        .ok_or(Add)?
        .checked_add(1)
        .ok_or(Add)?;
    Ok(())
}

#[derive(Debug, Error)]
enum RecoverableDecodeEntryError {
    #[error("{0}")]
    TryFromSlice(#[from] std::array::TryFromSliceError),

    #[error("{0}")]
    FromUtf8(#[from] std::string::FromUtf8Error),

    #[error("seek: {0}")]
    Seek(std::io::Error),

    #[error("read: {0}")]
    Read(std::io::Error),

    #[error("parse monitor id: {0}")]
    ParseMonitorId(#[from] ParseMonitorIdError),

    #[error("parse log source: {0}")]
    ParseLogSource(#[from] ParseLogSourceError),

    #[error("parse log level: {0}")]
    ParseLogLevel(#[from] ParseLogLevelError),

    #[error("parse log message: {0}")]
    ParseLogMessage(#[from] ParseNonEmptyStringError),
}

async fn decode_entry<T: AsyncRead + AsyncSeek + Unpin>(
    buf: &[u8; DATA_SIZE],
    msg_file: &mut T,
) -> Result<(LogEntryWithTime, u32), RecoverableDecodeEntryError> {
    use RecoverableDecodeEntryError::*;

    let time = u64::from_be_bytes(buf[..8].try_into()?);
    let source = String::from_utf8(buf[8..16].to_owned())?;
    let monitor_id = String::from_utf8(buf[16..40].to_owned())?;
    let monitor_id = monitor_id.trim();
    let msg_offset = u32::from_be_bytes(buf[40..44].try_into()?);
    let msg_size = u16::from_be_bytes(buf[44..46].try_into()?);
    let level = buf[46].to_owned();

    msg_file
        .seek(SeekFrom::Start(msg_offset.into()))
        .await
        .map_err(Seek)?;

    let mut msg_buf = vec![0; msg_size.into()];
    msg_file.read_exact(&mut msg_buf).map_err(Read).await?;

    let monitor_id = {
        if monitor_id.is_empty() {
            None
        } else {
            Some(monitor_id.to_owned().try_into()?)
        }
    };

    Ok((
        LogEntryWithTime {
            time: UnixMicro::new(time),
            source: source.trim().to_owned().try_into()?,
            monitor_id,
            level: LogLevel::try_from(level)?,
            message: String::from_utf8(msg_buf)?.try_into()?,
        },
        msg_offset,
    ))
}

#[derive(Debug, Error)]
pub enum TimeToIdError {
    #[error("invalid time")]
    InvalidTime,
}

// Returns the first x digits in a UnixMilli timestamp as String.
// Output is padded with zeros if needed.
fn time_to_id(time: UnixMicro) -> Result<String, TimeToIdError> {
    let shifted = *time / CHUNK_DURATION;
    let padded = format!("{shifted:0>CHUNK_ID_LENGTH$}");
    if padded.len() > CHUNK_ID_LENGTH {
        return Err(TimeToIdError::InvalidTime);
    }
    Ok(padded)
}

async fn get_file_size(path: &Path) -> u64 {
    let Ok(metadata) = tokio::fs::metadata(path).await else {
        return 0;
    };
    metadata.len()
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use common::NonEmptyString;
    use pretty_assertions::assert_eq;
    use std::io::Cursor;
    use tempfile::tempdir;
    use test_case::test_case;

    fn new_test_db(log_dir: &Path) -> LogDbHandle {
        let (shutdown_complete_tx, _) = mpsc::channel::<()>(1);
        LogDb::new(
            shutdown_complete_tx,
            log_dir.to_owned(),
            ByteSize(0),
            ByteSize(0),
        )
        .unwrap()
    }

    fn src(s: &'static str) -> LogSource {
        s.try_into().unwrap()
    }
    fn m_id(s: &str) -> MonitorId {
        s.to_owned().try_into().unwrap()
    }
    fn msg(s: &str) -> NonEmptyString {
        s.to_owned().try_into().unwrap()
    }

    fn msg1() -> LogEntryWithTime {
        LogEntryWithTime {
            level: LogLevel::Error,
            source: src("s1"),
            monitor_id: Some(m_id("m1")),
            message: msg("msg1"),
            time: UnixMicro::new(4000),
        }
    }
    fn msg2() -> LogEntryWithTime {
        LogEntryWithTime {
            level: LogLevel::Warning,
            source: src("s1"),
            monitor_id: None,
            message: msg("msg2"),
            time: UnixMicro::new(3000),
        }
    }
    fn msg3() -> LogEntryWithTime {
        LogEntryWithTime {
            level: LogLevel::Info,
            source: src("s2"),
            monitor_id: Some(m_id("m2")),
            message: msg("msg3"),
            time: UnixMicro::new(2000),
        }
    }
    /*msg4 := Log{
        level: LogLevel::Debug,
        source:   "s2",
        message:   "msg4",
        time:  1000,
    }*/

    #[test_case(
        LogQuery{
            levels: vec![LogLevel::Warning],
            sources: vec![src("s1")],
            ..Default::default()
        },
        &[msg2()];
        "single level"
    )]
    #[test_case(
        LogQuery{
            levels: vec![LogLevel::Error, LogLevel::Warning],
            sources: vec![src("s1")],
            ..Default::default()
        },
        &[msg1(), msg2()];
        "multiple levels"
    )]
    #[test_case(
        LogQuery{
            levels:  vec![LogLevel::Error, LogLevel::Info],
            sources: vec![src("s1")],
            ..Default::default()
        },
        &[msg1()];
        "single source"
    )]
    #[test_case(
        LogQuery{
            levels: vec![LogLevel::Error, LogLevel::Info],
            sources: vec![src("s1"), src("s2")],
            ..Default::default()
        },
        &[msg1(), msg3()];
        "multiple sources"
    )]
    #[test_case(
        LogQuery{
            levels: vec![LogLevel::Error, LogLevel::Info],
            sources: vec![src("s1"), src("s2")],
            monitors: vec![m_id("m1")],
            ..Default::default()
        },
        &[msg1()];
        "single monitor"
    )]
    #[test_case(
        LogQuery{
            levels: vec![LogLevel::Error, LogLevel::Info],
            sources: vec![src("s1"), src("s2")],
            monitors: vec![m_id("m1"), m_id("m2")],
            ..Default::default()
        },
        &[msg1(), msg3()];
        "multiple monitors"
    )]
    #[test_case(
        LogQuery{
            levels: vec![LogLevel::Error, LogLevel::Warning, LogLevel::Info, LogLevel::Debug],
            sources: vec![src("s1"), src("s2")],
            ..Default::default()
        },
        &[msg1(), msg2(), msg3()];
        "all"
    )]
    #[test_case(
        LogQuery{..Default::default()},
        &[msg1(), msg2(), msg3()];
        "none"
    )]
    #[test_case(
        LogQuery{
            levels: vec![LogLevel::Error, LogLevel::Warning, LogLevel::Info, LogLevel::Debug],
            sources: vec![src("s1"), src("s2")],
            limit: Some(NonZeroUsize::new(2).unwrap()),
            ..Default::default()
        },
        &[msg1(), msg2()];
        "limit"
    )]
    #[test_case(
        LogQuery{
            levels: vec![LogLevel::Info],
            limit: Some(NonZeroUsize::new(1).unwrap()),
            ..Default::default()
        },
        &[msg3()];
        "limit2"
    )]
    #[test_case(
        LogQuery{
            levels: vec![LogLevel::Error, LogLevel::Warning, LogLevel::Info, LogLevel::Debug],
            sources: vec![src("s1"), src("s2")],
            time: Some(UnixMicro::new(4000)),
            ..Default::default()
        },
        &[msg2(), msg3()];
        "exactTime"
    )]
    #[test_case(
        LogQuery{
            levels: vec![LogLevel::Error, LogLevel::Warning, LogLevel::Info, LogLevel::Debug],
            sources: vec![src("s1"), src("s2")],
            time: Some(UnixMicro::new(3500)),
            ..Default::default()
        },
        &[msg2(), msg3()];
        "time"
    )]
    #[tokio::test]
    async fn test_log_db_query(input: LogQuery, want: &[LogEntryWithTime]) {
        let temp_dir = tempdir().unwrap();
        let db = new_test_db(temp_dir.path());

        // Populate database.
        db.save_log(msg3()).await.unwrap();
        db.save_log(msg2()).await.unwrap();
        db.save_log(msg1()).await.unwrap();

        let got = db.query(input).await.unwrap();
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_log_db_query_no_entries() {
        let temp_dir = tempdir().unwrap();
        let db = new_test_db(temp_dir.path());

        let entries = db.query(empty_query()).await.unwrap();
        assert_eq!(0, entries.len());
    }

    fn new_test_entry(time: u64) -> LogEntryWithTime {
        LogEntryWithTime {
            level: LogLevel::Error,
            source: src("x"),
            monitor_id: None,
            time: UnixMicro::new(time),
            message: time.to_string().try_into().unwrap(),
        }
    }

    fn empty_query() -> LogQuery {
        LogQuery {
            ..Default::default()
        }
    }

    #[tokio::test]
    async fn test_log_db_write_and_read() {
        let msg1 = new_test_entry(1);
        let msg2 = new_test_entry(2);
        let msg3 = new_test_entry(3);

        let temp_dir = tempdir().unwrap();
        let db = new_test_db(temp_dir.path());

        db.save_log(msg1.clone()).await.unwrap();
        let entries = db.query(empty_query()).await.unwrap();
        assert_eq!(msg1, entries[0]);

        db.save_log(msg2.clone()).await.unwrap();
        let entries = db.query(empty_query()).await.unwrap();
        assert_eq!(vec![msg2.clone(), msg1.clone()], entries);

        db.save_log(msg3.clone()).await.unwrap();
        let entries = db.query(empty_query()).await.unwrap();
        assert_eq!(vec![msg3, msg2, msg1], entries);
    }

    #[tokio::test]
    async fn test_log_db_multiple_chunks() {
        let msg1 = new_test_entry(1);
        let msg2 = new_test_entry(CHUNK_DURATION);
        let msg3 = new_test_entry(CHUNK_DURATION * 2);

        let temp_dir = tempdir().unwrap();
        let db = new_test_db(temp_dir.path());

        db.save_log(msg1.clone()).await.unwrap();
        db.save_log(msg2.clone()).await.unwrap();
        db.save_log(msg3.clone()).await.unwrap();

        let entries = db.query(empty_query()).await.unwrap();

        assert_eq!(vec![msg3, msg2, msg1], entries);
    }

    fn new_test_entry2(time: u64, message: &str) -> LogEntryWithTime {
        LogEntryWithTime {
            level: LogLevel::Error,
            source: src("x"),
            monitor_id: None,
            time: UnixMicro::new(time),
            message: msg(message),
        }
    }

    #[tokio::test]
    async fn test_log_db_recover_msg_pos() {
        let msg1 = new_test_entry2(1, "a");
        let msg2 = new_test_entry2(2, "b");

        let temp_dir = tempdir().unwrap();

        let db = new_test_db(temp_dir.path());
        db.save_log(msg1.clone()).await.unwrap();

        let db = new_test_db(temp_dir.path());
        db.save_log(msg2.clone()).await.unwrap();

        let want = vec![msg2, msg1];
        let got = db.query(empty_query()).await.unwrap();
        assert_eq!(want, got);

        let file_want = b"a\nb\n".to_vec();
        let file_got = std::fs::read(temp_dir.path().join("00000.msg")).unwrap();
        assert_eq!(file_want, file_got);
    }

    #[tokio::test]
    async fn test_empty_entry() {
        let temp_dir = tempdir().unwrap();

        let entry1 = new_test_entry2(1, "good1");
        let entry2 = new_test_entry2(2, "bad");
        let entry3 = new_test_entry2(3, "good2");

        let db = new_test_db(temp_dir.path());
        db.save_log(entry1.clone()).await.unwrap();
        db.save_log(entry2).await.unwrap();

        // Overwrite second entry with zeros.
        let mut file = tokio::fs::OpenOptions::new()
            .write(true)
            .open(temp_dir.path().join("00000.data"))
            .await
            .unwrap();
        file.seek(SeekFrom::Start(48)).await.unwrap();
        file.write_all(&[0].repeat(48)).await.unwrap();
        file.flush().await.unwrap();

        let db = new_test_db(temp_dir.path());

        let got = db.query(empty_query()).await.unwrap();
        assert_eq!([entry1.clone()].as_slice(), got.as_slice());

        db.save_log(entry3.clone()).await.unwrap();
        let got2 = db.query(empty_query()).await.unwrap();
        assert_eq!([entry3, entry1].as_slice(), got2.as_slice());
    }

    #[tokio::test]
    async fn test_empty_entry2() {
        let temp_dir = tempdir().unwrap();

        let entry1 = new_test_entry2(1, "bad");
        let entry2 = new_test_entry2(2, "good");

        let db = new_test_db(temp_dir.path());
        db.save_log(entry1.clone()).await.unwrap();

        // Overwrite first entry with zeros.
        let mut file = tokio::fs::OpenOptions::new()
            .write(true)
            .open(temp_dir.path().join("00000.data"))
            .await
            .unwrap();
        file.write_all(&[0].repeat(48)).await.unwrap();
        file.flush().await.unwrap();

        let db = new_test_db(temp_dir.path());

        assert!(db.query(empty_query()).await.unwrap().is_empty());

        db.save_log(entry2.clone()).await.unwrap();
        let got = db.query(empty_query()).await.unwrap();
        assert_eq!([entry2].as_slice(), got.as_slice());
    }

    #[tokio::test]
    async fn test_log_db_order() {
        let temp_dir = tempdir().unwrap();

        let db = new_test_db(temp_dir.path());
        db.save_log(new_test_entry(100)).await.unwrap();

        let db = new_test_db(temp_dir.path());
        db.save_log(new_test_entry(90)).await.unwrap();
        db.save_log(new_test_entry(120)).await.unwrap();
        db.save_log(new_test_entry(0)).await.unwrap();

        let want = vec![
            new_test_entry2(121, "0"),
            new_test_entry(120),
            new_test_entry2(101, "90"),
            new_test_entry(100),
        ];
        let got = db.query(empty_query()).await.unwrap();
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_log_db_search() {
        let temp_dir = tempdir().unwrap();
        let db = new_test_db(temp_dir.path());

        let msg1 = new_test_entry(1);
        let msg2 = new_test_entry(2);
        let msg3 = new_test_entry(3);
        let msg4 = new_test_entry(4);
        let msg5 = new_test_entry(CHUNK_DURATION);
        let msg6 = new_test_entry(CHUNK_DURATION + 1);
        let msg7 = new_test_entry(CHUNK_DURATION + 2);
        let msg8 = new_test_entry(CHUNK_DURATION * 2);
        let msg9 = new_test_entry(CHUNK_DURATION * 2 + 1);

        db.save_log(msg1.clone()).await.unwrap();
        db.save_log(msg2.clone()).await.unwrap();
        db.save_log(msg3.clone()).await.unwrap();
        db.save_log(msg4.clone()).await.unwrap();
        db.save_log(msg5.clone()).await.unwrap();
        db.save_log(msg6.clone()).await.unwrap();
        db.save_log(msg7.clone()).await.unwrap();
        db.save_log(msg8.clone()).await.unwrap();
        db.save_log(msg9.clone()).await.unwrap();

        #[rustfmt::skip]
        let cases = vec![
            (0,              vec![&msg9, &msg8, &msg7, &msg6, &msg5, &msg4, &msg3, &msg2, &msg1]),
            (*msg9.time+1, vec![&msg9, &msg8, &msg7, &msg6, &msg5, &msg4, &msg3, &msg2, &msg1]),
            (*msg9.time,   vec![&msg8, &msg7, &msg6, &msg5, &msg4, &msg3, &msg2, &msg1]),
            (*msg8.time,   vec![&msg7, &msg6, &msg5, &msg4, &msg3, &msg2, &msg1]),
            (*msg8.time-1, vec![&msg7, &msg6, &msg5, &msg4, &msg3, &msg2, &msg1]),
            (*msg7.time+1, vec![&msg7, &msg6, &msg5, &msg4, &msg3, &msg2, &msg1]),
            (*msg7.time,   vec![&msg6, &msg5, &msg4, &msg3, &msg2, &msg1]),
            (*msg6.time,   vec![&msg5, &msg4, &msg3, &msg2, &msg1]),
            (*msg5.time,   vec![&msg4, &msg3, &msg2, &msg1]),
            (*msg5.time-1, vec![&msg4, &msg3, &msg2, &msg1]),
            (*msg4.time+1, vec![&msg4, &msg3, &msg2, &msg1]),
            (*msg4.time,   vec![&msg3, &msg2, &msg1]),
            (*msg3.time,   vec![&msg2, &msg1]),
            (*msg2.time,   vec![&msg1]),
            (*msg1.time,   vec![]),
        ];

        for (input_time, want) in cases {
            let time = {
                if input_time == 0 {
                    None
                } else {
                    Some(UnixMicro::new(input_time))
                }
            };
            let got = db
                .query(LogQuery {
                    time,
                    ..Default::default()
                })
                .await
                .unwrap();
            let want: Vec<LogEntryWithTime> = want
                .into_iter()
                .map(std::borrow::ToOwned::to_owned)
                .collect();
            assert_eq!(want, got);
        }
    }

    #[tokio::test]
    async fn test_new_store_mkdir() {
        let (shutdown_complete_tx, mut shutdown_complete_rx) = mpsc::channel::<()>(1);
        let temp_dir = tempdir().unwrap();

        let new_dir = temp_dir.path().join("test");
        assert!(std::fs::metadata(&new_dir).is_err());

        LogDb::new(
            shutdown_complete_tx,
            new_dir.clone(),
            ByteSize(0),
            ByteSize(0),
        )
        .unwrap();

        std::fs::metadata(new_dir).unwrap();

        let _ = shutdown_complete_rx.recv().await;
    }

    fn test_entry() -> LogEntryWithTime {
        LogEntryWithTime {
            level: LogLevel::Debug,
            source: src("abcdefgh"),
            monitor_id: Some(m_id("aabbccddeeffgghhiijjkkll")),
            message: msg("a"),
            time: UnixMicro::new(5),
        }
    }

    #[tokio::test]
    async fn test_log_entry_encode() {
        let mut buf = Vec::with_capacity(DATA_SIZE);
        let mut msg_buf = Cursor::new(Vec::new());
        let mut msg_pos = 0;

        encode_entry(&mut buf, &test_entry(), &mut msg_buf, &mut msg_pos)
            .await
            .unwrap();

        let want = vec![
            0, 0, 0, 0, 0, 0, 0, 5, // Time.
            b'a', b'b', b'c', b'd', b'e', b'f', b'g', b'h', // Src.
            b'a', b'a', b'b', b'b', b'c', b'c', b'd', b'd', // Monitor ID.
            b'e', b'e', b'f', b'f', b'g', b'g', b'h', b'h', //
            b'i', b'i', b'j', b'j', b'k', b'k', b'l', b'l', //
            0, 0, 0, 0, // Message offset.
            0, 1,  // Message size.
            48, // Level.
        ];

        assert_eq!(want, buf);
        assert_eq!(vec![b'a', b'\n'], msg_buf.into_inner());
        assert_eq!(
            test_entry().message.len() + 1,
            usize::try_from(msg_pos).unwrap()
        );
    }

    #[tokio::test]
    async fn test_log_entry_decode() {
        let mut buf = Cursor::new(Vec::new());
        let mut msg_buf = Cursor::new(Vec::new());
        let mut msg_pos: u32 = 10;

        msg_buf.seek(SeekFrom::Start(10)).await.unwrap();

        encode_entry(&mut buf, &test_entry(), &mut msg_buf, &mut msg_pos)
            .await
            .unwrap();

        let buf: [u8; DATA_SIZE] = buf.into_inner().try_into().unwrap();

        let (entry, msg_offset) = decode_entry(&buf, &mut msg_buf).await.unwrap();
        assert_eq!(test_entry(), entry);
        assert_eq!(10, msg_offset);
    }

    #[test_case(UnixMicro::new(0), "00000"; "a")]
    #[test_case(UnixMicro::new(1_000_334_455_000_111), "10003"; "b")]
    #[test_case(UnixMicro::new(1_122_334_455_000_111), "11223"; "c")]
    #[test_case(UnixMicro::new(CHUNK_DURATION - 1), "00000"; "d")]
    #[test_case(UnixMicro::new(CHUNK_DURATION), "00001"; "e")]
    #[test_case(UnixMicro::new(CHUNK_DURATION + 1), "00001"; "f")]
    fn test_time_to_id(input: UnixMicro, output: &str) {
        assert_eq!(output, time_to_id(input).unwrap());
    }

    #[test]
    fn test_time_to_id_error() {
        assert!(matches!(
            time_to_id(UnixMicro::new(12_345_678_901_234_567)),
            Err(TimeToIdError::InvalidTime)
        ));
    }

    #[tokio::test]
    async fn test_new_chunk_encoder_version_error() {
        let log_dir = tempdir().unwrap();
        let chunk_id = "0";

        std::fs::write(log_dir.path().join("0.data"), [255]).unwrap();

        assert!(matches!(
            ChunkEncoder::new(log_dir.path().to_owned(), chunk_id.to_owned()).await,
            Err(NewChunkEncoderError::NewChunkDecoder(
                NewChunkDecoderError::UnknownChunkVersion
            ))
        ));
    }

    #[tokio::test]
    async fn test_new_chunk_decoder_version_error() {
        let log_dir = tempdir().unwrap();
        let chunk_id = "0";

        std::fs::write(log_dir.path().join("0.data"), [255]).unwrap();

        assert!(matches!(
            ChunkDecoder::new(log_dir.path(), chunk_id).await,
            Err(NewChunkDecoderError::UnknownChunkVersion)
        ));
    }

    #[tokio::test]
    async fn test_log_prune() {
        let temp_dir = tempdir().unwrap();
        let log_dir = temp_dir.path();

        let (shutdown_complete_tx, mut shutdown_complete_rx) = mpsc::channel::<()>(1);
        let db = LogDb::new(
            shutdown_complete_tx,
            log_dir.to_owned(),
            ByteSize::kb(10),
            ByteSize(0),
        )
        .unwrap();

        write_test_chunk(log_dir, "00000");
        write_test_chunk(log_dir, "11111");

        let want = vec![
            "00000.data".to_owned(),
            "00000.msg".to_owned(),
            "11111.data".to_owned(),
            "11111.msg".to_owned(),
        ];
        assert_eq!(want, list_files(log_dir));

        db.prune().await.unwrap();

        let want = vec!["11111.data".to_owned(), "11111.msg".to_owned()];
        assert_eq!(want, list_files(log_dir));

        drop(db);
        _ = shutdown_complete_rx.recv().await;
    }

    #[tokio::test]
    async fn test_log_prune_disk_space() {
        let temp_dir = tempdir().unwrap();
        let log_dir = temp_dir.path();

        let (shutdown_complete_tx, mut shutdown_complete_rx) = mpsc::channel::<()>(1);
        let db = LogDb::new(
            shutdown_complete_tx,
            log_dir.to_owned(),
            ByteSize::kb(10),
            ByteSize(0),
        )
        .unwrap();

        write_test_chunk(log_dir, "00000");
        write_test_chunk(log_dir, "11111");
        assert_eq!(2, chunk_count(log_dir).await);

        db.prune().await.unwrap();
        db.prune().await.unwrap();
        assert_eq!(1, chunk_count(log_dir).await);

        drop(db);
        _ = shutdown_complete_rx.recv().await;
    }

    #[tokio::test]
    async fn test_log_prune_min_disk_usage() {
        let temp_dir = tempdir().unwrap();
        let log_dir = temp_dir.path();

        let (shutdown_complete_tx, mut shutdown_complete_rx) = mpsc::channel::<()>(1);
        let db = LogDb::new(
            shutdown_complete_tx,
            log_dir.to_owned(),
            ByteSize(0),
            ByteSize(100),
        )
        .unwrap();

        write_test_chunk(log_dir, "00000");
        write_test_chunk(log_dir, "11111");
        assert_eq!(2, chunk_count(log_dir).await);

        db.prune().await.unwrap();
        db.prune().await.unwrap();
        assert_eq!(1, chunk_count(log_dir).await);

        drop(db);
        _ = shutdown_complete_rx.recv().await;
    }

    #[tokio::test]
    async fn test_log_prune_no_files() {
        let temp_dir = tempdir().unwrap();
        let log_dir = temp_dir.path();

        let (shutdown_complete_tx, mut shutdown_complete_rx) = mpsc::channel::<()>(1);
        let db = LogDb::new(
            shutdown_complete_tx,
            log_dir.to_owned(),
            ByteSize(0),
            ByteSize(0),
        )
        .unwrap();

        assert_eq!(0, chunk_count(log_dir).await);
        db.prune().await.unwrap();

        drop(db);
        _ = shutdown_complete_rx.recv().await;
    }
    /*
        t.Run("diskSpaceErr", func(t *testing.T) {
            stubError := errors.New("stub")
            stubGetDiskSpace := func() (int64, error) {
                return 0, stubError
            }
            logDir := t.TempDir()
            s := Store{
                logDir:       logDir,
                getDiskSpace: stubGetDiskSpace,
                minDiskUsage: 0,
            }
            writeTestChunk(t, logDir, "00000")

            err := s.purge()
            require.ErrorIs(t, err, stubError)
        })
    */

    // Each chunk is 100 bytes.
    fn write_test_chunk(log_dir: &Path, chunk_id: &str) {
        let (data_path, msg_path) = chunk_id_to_paths(log_dir, chunk_id);
        std::fs::write(data_path, [0].repeat(50)).unwrap();
        std::fs::write(msg_path, [0].repeat(50)).unwrap();
    }

    async fn chunk_count(log_dir: &Path) -> usize {
        let (shutdown_complete, _) = mpsc::channel::<()>(1);
        let db = LogDb {
            log_dir: log_dir.to_owned(),
            encoder: None,
            prev_entry_time: UnixMicro::new(0),
            disk_space: ByteSize(0),
            min_disk_usage: ByteSize(0),
            _shutdown_complete: shutdown_complete,
        };
        db.list_chunks().await.unwrap().len()
    }

    fn list_files(path: &Path) -> Vec<String> {
        let files = std::fs::read_dir(path).unwrap();

        let mut file_names = Vec::new();
        for file in files {
            let file = file.unwrap();
            file_names.push(file.file_name().to_string_lossy().to_string());
        }
        file_names.sort();
        file_names
    }
}
