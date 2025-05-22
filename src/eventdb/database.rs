// SPDX-License-Identifier: GPL-2.0-or-later

use common::{
    ArcMsgLogger, Event, FILE_MODE, LogLevel,
    monitor::CreateEventDbError,
    time::{MINUTE, SECOND, UnixNano},
};
use pin_project::pin_project;
use serde::Deserialize;
use std::{
    cmp::Ordering,
    collections::{
        BTreeMap, VecDeque,
        btree_map::Entry::{Occupied, Vacant},
    },
    future::Future,
    io::SeekFrom,
    num::NonZeroUsize,
    path::{Path, PathBuf},
    task::Poll,
};
use thiserror::Error;
use tokio::{
    fs::File,
    io::{AsyncRead, AsyncReadExt, AsyncSeek, AsyncSeekExt, AsyncWrite, AsyncWriteExt},
    runtime::Handle,
    sync::{mpsc, oneshot},
};

use crate::buf_seek_reader::BufSeekReader;

// chunk {
//     file.data
//     file.payload
// }
//
// file.data {
//     magic_bytes: [u8; 27]
//     version u8
//     [data]
// }
//
// data { // 14 bytes.
//     time u64
//     payloadOffset u32
//     payloadSize u16
// }
//
//
//
//                       BUFFERS
//
//         write_event()            query()
//               │                    ▲
//               ▼                    │
//        reorder buffer──────────────┤
//               │                    │
//               │                    │
//               ├───►writer cache────┼
//               │                    │
//               ▼                    │
//    encoder write buffers   decoder read buffers
//                   │           ▲
//                   └──►files───┘
//
//

struct Chunk;

impl Chunk {
    // 16666 minutes or 27.7 hours.
    const DURATION: i64 = 100_000 * SECOND;

    // The chunk identifier is the first N digits of a Unix timestamp.
    const ID_LENGTH: usize = 5;

    const API_VERSION: u8 = 0;
}

// 8 + 4 + 2
const DATA_SIZE: usize = 14;
#[cfg(test)]
const DATA_SIZE2: u64 = 14;

struct ChunkHeader([u8; Self::LENGTH2]);

impl ChunkHeader {
    const MAGIC_BYTES: [u8; 27] = *b"SentryShot\0eventdb\0\0\x89\x85\x80\x85\0\0v";
    const LENGTH: u64 = 28;
    const LENGTH2: usize = 28;

    async fn from_reader<R: AsyncRead + Unpin>(mut r: R) -> Result<Self, std::io::Error> {
        let mut header: [u8; Self::LENGTH2] = [0; Self::LENGTH2];
        r.read_exact(&mut header).await?;
        Ok(Self(header))
    }

    fn magic_bytes(&self) -> [u8; Self::MAGIC_BYTES.len()] {
        self.0[..Self::MAGIC_BYTES.len()]
            .try_into()
            .expect("size should match")
    }

    fn version(&self) -> u8 {
        u8::from_be_bytes([self.0[ChunkHeader::LENGTH2 - 1]])
    }

    async fn write_header<W: AsyncWrite + Unpin>(mut writer: W) -> Result<(), std::io::Error> {
        let header = [
            ChunkHeader::MAGIC_BYTES.as_slice(),
            &Chunk::API_VERSION.to_be_bytes(),
        ]
        .concat();
        writer.write_all(&header).await
    }
}

#[derive(Clone)]
pub struct Database {
    logger: ArcMsgLogger,
    db_dir: PathBuf,
    tx: mpsc::Sender<DatabaseRequest>,
}

enum DatabaseRequest {
    WriteEvent {
        event: Event,
        _res: oneshot::Sender<()>, // Only used for synchronization in tests.
    },
    QueryCacheAndBuffer {
        query: EventQuery,
        entries: Vec<Event>,
        res: oneshot::Sender<Vec<Event>>,
    },
}

impl Database {
    pub async fn new(
        shutdown_complete: mpsc::Sender<()>,
        logger: ArcMsgLogger,
        db_dir: PathBuf,
        cache_capacity: usize,
        write_buf_capacify: usize,
        disable_reordering: bool,
    ) -> Result<Self, CreateEventDbError> {
        assert!(cache_capacity >= write_buf_capacify);
        common::create_dir_all2(Handle::current(), db_dir.clone())
            .await
            .map_err(|e| CreateEventDbError::CreateDir(db_dir.clone(), e))?;

        let (tx, mut rx) = mpsc::channel(1);
        let logger2 = logger.clone();

        let mut writer = DbWriter::new(
            logger.clone(),
            db_dir.clone(),
            cache_capacity,
            write_buf_capacify,
        );

        // Entries are queued here for 10 seconds.
        let mut reorder_buf = ReorderBuffer::new(logger.clone(), disable_reordering);

        tokio::spawn(async move {
            let _shutdown_complete = shutdown_complete;
            loop {
                tokio::select! {
                    req = rx.recv() => {
                        let Some(req) = req else { // Channel closed.
                            for event in reorder_buf.drain() {
                                if let Err(e) = writer.write_entry(event).await {
                                    logger2.log(LogLevel::Error, &format!("failed to write event: {e}"));
                                }
                            }

                            if let Err(e) = writer.flush_and_cancel().await {
                                logger2.log(LogLevel::Error, &format!("failed flush database: {e}"));
                            };
                            return;
                        };
                        match req {
                            DatabaseRequest::WriteEvent{ event, _res } => {
                                // Drop events with a time after one minute in the future.
                                let now = UnixNano::now();
                                if now + UnixNano::new(MINUTE) < event.time {
                                    continue
                                }
                                reorder_buf.insert_deduplicate_time(event);
                                reorder_buf.write_events(now, &mut writer).await;
                            },
                            DatabaseRequest::QueryCacheAndBuffer {mut query, mut entries, res} => {
                                writer.query_cache(&mut query, &mut entries);
                                reorder_buf.query(&mut query, &mut entries);
                                _ = res.send(entries);
                            },
                        }
                    },
                    () = reorder_buf.sleep() => {
                        reorder_buf.write_events(UnixNano::now(), &mut writer).await;
                    }
                }
            }
        });

        Ok(Self { logger, db_dir, tx })
    }

    pub async fn write_event(&self, event: Event) {
        let (res, rx) = oneshot::channel();
        let request = DatabaseRequest::WriteEvent { event, _res: res };
        if self.tx.send(request).await.is_err() {
            // Cancelled.
            return;
        }
        _ = rx.await;
    }

    pub async fn query(
        &self,
        mut query: EventQuery,
    ) -> Result<Option<Vec<Event>>, QueryEventsError> {
        if query.start >= query.end {
            use QueryEventsError::*;
            return Err(StartGreaterOrEqualEnd(query.start, query.end));
        }

        let mut entries = Vec::new();
        self.new_reader().query_db(&mut query, &mut entries).await?;

        let (res, rx) = oneshot::channel();
        let request = DatabaseRequest::QueryCacheAndBuffer {
            query,
            entries,
            res,
        };
        if self.tx.send(request).await.is_err() {
            // Cancelled.
            return Ok(None);
        }
        Ok(Some(rx.await.expect("actor should respond")))
    }

    fn new_reader(&self) -> EventDbReader {
        EventDbReader {
            logger: self.logger.clone(),
            db_dir: self.db_dir.clone(),
        }
    }
}

struct ReorderBuffer {
    logger: ArcMsgLogger,
    inner: BTreeMap<UnixNano, Event>,

    disable_reordering: bool, // Testing.
}

impl ReorderBuffer {
    fn new(logger: ArcMsgLogger, disable_reordering: bool) -> Self {
        Self {
            logger,
            inner: BTreeMap::new(),
            disable_reordering,
        }
    }

    fn insert_deduplicate_time(&mut self, mut event: Event) {
        // Increment time until the time no longer a duplicate.
        loop {
            match self.inner.entry(event.time) {
                Vacant(v) => {
                    v.insert(event);
                    break;
                }
                Occupied(_) => event.time += UnixNano::new(1),
            }
        }
    }

    async fn write_events(&mut self, now: UnixNano, writer: &mut DbWriter) {
        let time = now - UnixNano::new(10 * SECOND);
        while let Some(entry) = self.inner.first_entry() {
            if entry.get().time < time || self.disable_reordering {
                if let Err(e) = writer.write_entry(entry.remove_entry().1).await {
                    self.logger
                        .log(LogLevel::Error, &format!("failed to write event: {e}"));
                }
            } else {
                break;
            }
        }
    }

    // Sleeps until first item is older than 10 seconds.
    fn sleep(&self) -> ReorderSleep {
        let first_item = self.inner.first_key_value().map(|v| *v.0);
        ReorderSleep(
            calculate_reorder_sleep_duration(UnixNano::now(), first_item).map(tokio::time::sleep),
        )
    }

    fn query(&self, q: &mut EventQuery, entries: &mut Vec<Event>) {
        for entry in self.inner.values() {
            if entry.time < q.start {
                continue;
            }
            if q.end <= entry.time {
                return;
            }
            if entries.len() >= q.limit.get() {
                return;
            }
            entries.push(entry.clone());
            q.start = entry.time + UnixNano::new(1);
        }
    }

    fn drain(self) -> Vec<Event> {
        self.inner.into_values().collect()
    }
}

fn calculate_reorder_sleep_duration(
    now: UnixNano,
    first_item: Option<UnixNano>,
) -> Option<std::time::Duration> {
    let duration = (first_item? + UnixNano::new(11 * SECOND)).sub(now)?;
    if !duration.is_positive() {
        return Some(std::time::Duration::from_nanos(0));
    }
    Some(
        duration
            .as_std()
            .expect("positive i64 should always be valid u64"),
    )
}

// Future will always return pending if this is None.
#[pin_project]
struct ReorderSleep(#[pin] Option<tokio::time::Sleep>);

impl Future for ReorderSleep {
    type Output = ();

    fn poll(
        mut self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<Self::Output> {
        match self.as_mut().project().0.as_pin_mut() {
            Some(v) => v.poll(cx),
            None => Poll::Pending,
        }
    }
}

pub(crate) struct DbWriter {
    logger: ArcMsgLogger,
    db_dir: PathBuf,
    encoder: Option<ChunkEncoder>,

    // Keep track of the previous entry time to ensure
    // that the next entry will have a later time.
    prev_entry_time: UnixNano,

    cache: VecDeque<Event>,
    cache_capacity: usize,
    write_buf_capacify: usize,
}

impl DbWriter {
    fn new(
        logger: ArcMsgLogger,
        db_dir: PathBuf,
        cache_capacity: usize,
        write_buf_capacify: usize,
    ) -> Self {
        Self {
            logger,
            db_dir,
            encoder: None,
            prev_entry_time: UnixNano::new(0),
            cache: VecDeque::with_capacity(cache_capacity),
            cache_capacity,
            write_buf_capacify,
        }
    }

    async fn flush_and_cancel(mut self) -> Result<(), EncoderFlushError> {
        if let Some(encoder) = self.encoder.take() {
            encoder.flush().await?;
        }
        Ok(())
    }

    async fn write_entry(&mut self, mut entry: Event) -> Result<(), SaveEntryError> {
        // Get encoder.
        let chunk_id = time_to_id(entry.time)?;
        let encoder = if let Some(encoder) = self.encoder.take() {
            // If entry belongs to chunk tied to this encoder.
            if chunk_id == encoder.chunk_id {
                encoder
            } else {
                // Flush and replace encoder.
                if let Err(e) = encoder.flush().await {
                    self.logger
                        .log(LogLevel::Error, &format!("swap encoder: flush: {e}"));
                }
                let (encoder, prev_entry_time) = self.new_chunk_encoder(chunk_id).await?;
                // This should only be true if the system clock was rewined at some point.
                if self.prev_entry_time < prev_entry_time {
                    self.prev_entry_time = prev_entry_time;
                }
                encoder
            }
        } else {
            // Encoder was None.
            let (encoder, prev_entry_time) = self.new_chunk_encoder(chunk_id).await?;
            self.prev_entry_time = prev_entry_time;
            encoder
        };

        if entry.time <= self.prev_entry_time {
            entry.time = self.prev_entry_time + UnixNano::new(1);
        }

        self.encoder = Some(encoder.write_entry(&entry).await?);
        self.cache_push(entry.clone());
        self.prev_entry_time = entry.time;

        Ok(())
    }

    async fn new_chunk_encoder(
        &self,
        chunk_id: String,
    ) -> Result<(ChunkEncoder, UnixNano), NewChunkEncoderError> {
        ChunkEncoder::new(
            self.logger.clone(),
            self.db_dir.clone(),
            chunk_id,
            self.write_buf_capacify,
        )
        .await
    }

    fn cache_push(&mut self, entry: Event) {
        if self.cache_capacity == 0 {
            return;
        }
        assert!(self.cache.len() <= self.cache_capacity);
        if self.cache.len() == self.cache_capacity {
            self.cache.pop_front();
        }
        self.cache.push_back(entry);
    }

    fn query_cache(&self, q: &mut EventQuery, entries: &mut Vec<Event>) {
        for entry in &self.cache {
            if entry.time < q.start {
                continue;
            }
            if q.end <= entry.time {
                return;
            }
            if entries.len() >= q.limit.get() {
                return;
            }
            entries.push(entry.clone());
            q.start = entry.time + UnixNano::new(1);
        }
    }
}

struct EventDbReader {
    logger: ArcMsgLogger,
    db_dir: PathBuf,
}

impl EventDbReader {
    async fn query_db(
        &self,
        q: &mut EventQuery,
        entries: &mut Vec<Event>,
    ) -> Result<(), QueryEventsError> {
        if entries.len() >= q.limit.get() {
            return Ok(());
        }

        let after_id = time_to_id(q.start)?;

        let mut first_chunk = true;
        for chunk_id in list_chunks(self.db_dir.clone()).await? {
            if chunk_id < after_id {
                continue;
            }
            if let Err(e) = self.query_chunk(q, first_chunk, entries, &chunk_id).await {
                self.logger
                    .log(LogLevel::Warning, &format!("query chunk: {e}"));
            }
            first_chunk = false;
        }
        Ok(())
    }

    async fn query_chunk(
        &self,
        q: &mut EventQuery,
        first_chunk: bool,
        entries: &mut Vec<Event>,
        chunk_id: &String,
    ) -> Result<(), QueryChunkError> {
        use QueryChunkError::*;
        let mut decoder = ChunkDecoder::new(&self.db_dir, chunk_id).await?;

        let entry_index = if first_chunk && decoder.n_entries != 0 {
            decoder.search(q.start).await.map_err(Search)?
        } else {
            0
        };

        for i in entry_index..decoder.n_entries {
            // Limit check.
            if entries.len() >= q.limit.get() {
                break;
            }

            let (data_path, _) = chunk_id_to_paths(&self.db_dir, chunk_id);
            let entry = match decoder.decode_lazy(i).await {
                Ok((v, _)) => v,
                Err(e) => return Err(Decode(e)),
            };
            if q.end <= entry.time {
                break;
            }

            let entry = match entry.finalize().await {
                Ok(v) => v,
                Err(e) => {
                    self.logger
                        .log(LogLevel::Warning, &format!("finalize: {data_path:?} {e}"));
                    continue;
                }
            };

            match time_to_id(entry.time) {
                Ok(entry_chunk_id) => {
                    if entry_chunk_id != *chunk_id {
                        continue;
                    }
                }
                Err(e) => {
                    self.logger
                        .log(LogLevel::Warning, &format!("time to id: {data_path:?} {e}"));
                    continue;
                }
            }

            q.start = entry.time + UnixNano::new(1);
            entries.push(entry);
        }

        Ok(())
    }
}

pub(crate) async fn list_chunks(db_dir: PathBuf) -> Result<Vec<String>, std::io::Error> {
    tokio::task::spawn_blocking(|| {
        let mut chunks = Vec::new();
        for file in std::fs::read_dir(db_dir)? {
            let file = file?;
            let name = file
                .file_name()
                .into_string()
                .unwrap_or_else(|_| String::new());

            let is_data_file = Path::new(&name)
                .extension()
                .is_some_and(|ext| ext.eq_ignore_ascii_case("data"));
            if name.len() < Chunk::ID_LENGTH + 5 || !is_data_file {
                continue;
            }
            chunks.push(name[..Chunk::ID_LENGTH].to_owned());
        }
        chunks.sort();

        Ok(chunks)
    })
    .await
    .expect("join")
}

#[derive(Debug, Error)]
enum QueryChunkError {
    #[error("new chunk decoder: {0}")]
    NewChunkDecoder(#[from] CreateChunkDecoderError),

    #[error("search: {0}")]
    Search(DecodeError),

    #[error("decode: {0}")]
    Decode(DecodeError),
}

#[derive(Debug, Error)]
pub enum QueryEventsError {
    #[error("start greater or equal end: start={0} end={1}")]
    StartGreaterOrEqualEnd(UnixNano, UnixNano),

    #[error("list chunks: {0}")]
    ListChunks(#[from] std::io::Error),

    #[error("time to id: {0}")]
    TimeToId(#[from] TimeToIdError),
}

#[derive(Debug, Error)]
pub enum SaveEntryError {
    #[error("{0}")]
    TimeToId(#[from] TimeToIdError),

    #[error("new chunk encoder: {0}")]
    NewChunkEncoder(#[from] NewChunkEncoderError),

    #[error("{0}")]
    Encode(#[from] EncodeError),
}

#[derive(Deserialize)]
pub struct EventQuery {
    pub start: UnixNano,
    pub end: UnixNano,
    pub limit: NonZeroUsize,
}

fn chunk_id_to_paths(db_dir: &Path, chunk_id: &str) -> (PathBuf, PathBuf) {
    let data_path = db_dir.join(chunk_id.to_owned() + ".data");
    let payload_path = db_dir.join(chunk_id.to_owned() + ".payload");
    (data_path, payload_path)
}

struct ChunkDecoder {
    n_entries: usize,
    data_file: BufSeekReader<File>,
    payload_file: BufSeekReader<File>,
}

#[derive(Debug, Error)]
pub enum CreateChunkDecoderError {
    #[error("open data file: {0}")]
    OpenDataFile(std::io::Error),

    #[error("read header: {0}")]
    ReadHeader(std::io::Error),

    #[error("mismatched magic bytes")]
    MismatchedMagicBytes,

    #[error("unknown chunk api version")]
    UnknownChunkVersion,

    #[error("data file metadata: {0}")]
    DataFileMetadata(std::io::Error),

    #[error("open payload file: {0} {1}")]
    OpenPayloadFile(PathBuf, std::io::Error),

    #[error("calculate entries: {0}")]
    CalculateEntries(#[from] CalculateEntriesError),
}

impl ChunkDecoder {
    async fn new(db_dir: &Path, chunk_id: &str) -> Result<Self, CreateChunkDecoderError> {
        use CreateChunkDecoderError::*;
        let (data_path, payload_path) = chunk_id_to_paths(db_dir, chunk_id);

        let mut data_file = tokio::fs::OpenOptions::new()
            .read(true)
            .open(data_path)
            .await
            .map_err(OpenDataFile)?;

        let header = ChunkHeader::from_reader(&mut data_file)
            .await
            .map_err(ReadHeader)?;

        if header.magic_bytes() != ChunkHeader::MAGIC_BYTES {
            return Err(MismatchedMagicBytes);
        }

        if header.version() != Chunk::API_VERSION {
            return Err(UnknownChunkVersion);
        }

        let data_file_size = data_file.metadata().await.map_err(DataFileMetadata)?.len();

        let payload_file = tokio::fs::OpenOptions::new()
            .read(true)
            .open(&payload_path)
            .await
            .map_err(|e| OpenPayloadFile(payload_path, e))?;

        let data_file = BufSeekReader::with_capacity(1024 * 32, data_file);
        let payload_file = BufSeekReader::with_capacity(1024 * 32, payload_file);

        Ok(Self {
            n_entries: calculate_n_entries(data_file_size)?,
            data_file,
            payload_file,
        })
    }

    // Returns None if there are no entries.
    fn last_index(&self) -> Option<usize> {
        self.n_entries.checked_sub(1)
    }

    // Binary search.
    async fn search(&mut self, time: UnixNano) -> Result<usize, DecodeError> {
        assert!(self.n_entries != 0);

        let (mut l, mut r) = (0, self.n_entries - 1);
        while l <= r {
            let i = (l + r) / 2;
            let (entry, _) = self.decode_lazy(i).await?;
            // Special case for zeroed entries.
            if *entry.time == 0 {
                if r == 0 {
                    break;
                }
                r -= 1;
                continue;
            }

            match entry.time.cmp(&time) {
                Ordering::Less => l = i + 1,
                Ordering::Equal => return Ok(i),
                Ordering::Greater => {
                    if i == 0 {
                        return Ok(0);
                    }
                    r = i - 1;
                }
            }
        }
        Ok(l)
    }

    async fn decode_lazy(
        &mut self,
        index: usize,
    ) -> Result<(LazyDbEntry<BufSeekReader<File>>, u32), DecodeError> {
        use DecodeError::*;
        let index = u64::try_from(index)?;
        let data_size_u64 = u64::try_from(DATA_SIZE)?;
        let entry_pos: u64 = ChunkHeader::LENGTH
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

        Ok(decode_entry_lazy(&raw_entry, &mut self.payload_file))
    }
}

#[derive(Debug)]
struct LazyDbEntry<'a, T: AsyncRead + AsyncSeek + Unpin> {
    time: UnixNano,

    payload_file: &'a mut T,
    payload_offset: u32,
    payload_size: u16,
}

impl<T: AsyncRead + AsyncSeek + Unpin> LazyDbEntry<'_, T> {
    async fn finalize(self) -> Result<Event, RecoverableDecodeError> {
        use RecoverableDecodeError::*;
        self.payload_file
            .seek(SeekFrom::Start(self.payload_offset.into()))
            .await
            .map_err(Seek)?;

        let mut payload_buf = vec![0; self.payload_size.into()];
        self.payload_file
            .read_exact(&mut payload_buf)
            .await
            .map_err(Read)?;

        let detections: Event = serde_json::from_slice(&payload_buf)
            .map_err(|e| Desrialize(e, self.payload_offset, self.payload_size))?;
        Ok(detections)
    }
}

#[derive(Error, Debug)]
pub enum CalculateDataEndError {
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

    ChunkHeader::LENGTH
        .checked_add(u64::try_from(n_entries.checked_mul(DATA_SIZE).ok_or(Mul)?)?)
        .ok_or(Add)
}

#[derive(Debug, Error)]
pub enum DecodeError {
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
}

#[derive(Debug, Error)]
pub enum CalculateEntriesError {
    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),

    #[error("sub")]
    Sub,

    #[error("div")]
    Div,
}

fn calculate_n_entries(size: u64) -> Result<usize, CalculateEntriesError> {
    use CalculateEntriesError::*;
    // (size - ChunkHeader::Length) / dataSize
    Ok(usize::try_from(
        size.checked_sub(ChunkHeader::LENGTH)
            .ok_or(Sub)?
            .checked_div(u64::try_from(DATA_SIZE)?)
            .ok_or(Div)?,
    )?)
}

#[derive(Debug, Error)]
pub enum NewChunkEncoderError {
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

    #[error("open payload file: {0}")]
    OpenPayloadFile(std::io::Error),

    #[error("seek to payload end: {0}")]
    SeekToPayloadEnd(std::io::Error),

    #[error("new chunk decoder: {0}")]
    NewChunkDecoder(#[from] CreateChunkDecoderError),

    #[error("decode is chunk {0}: {1}")]
    Decode(String, DecodeError),

    #[error("calculate data end: {0}")]
    CalculateDataEnd(#[from] CalculateDataEndError),
}

struct ChunkEncoder {
    chunk_id: String,
    data_file: File,
    payload_file: File,
    payload_pos: u32,

    write_buf_capacity: usize,
    buf_count: usize,
    data_buf: Vec<u8>,
    payload_buf: Vec<u8>,
}

impl ChunkEncoder {
    // Must be flushed before being dropped.
    async fn new(
        logger: ArcMsgLogger,
        db_dir: PathBuf,
        chunk_id: String,
        write_buf_capacity: usize,
    ) -> Result<(Self, UnixNano), NewChunkEncoderError> {
        use NewChunkEncoderError::*;
        let (data_path, payload_path) = chunk_id_to_paths(&db_dir, &chunk_id);

        let mut data_end = ChunkHeader::LENGTH;
        let data_file_size = get_file_size(&data_path).await;
        let mut prev_entry_time = UnixNano::new(0);
        let mut payload_pos = 0;
        if data_file_size == 0 {
            let mut data_file = tokio::fs::OpenOptions::new()
                .create(true)
                .mode(FILE_MODE)
                .truncate(true)
                .write(true)
                .open(&data_path)
                .await
                .map_err(OpenFile)?;

            ChunkHeader::write_header(&mut data_file)
                .await
                .map_err(WriteFile)?;

            data_file.flush().await.map_err(Flush)?;
        } else {
            let mut decoder = ChunkDecoder::new(&db_dir, &chunk_id).await?;

            // Find the first valid entry from the end.
            // Treat file as empty if no valid entry is found.
            if let Some(last_index) = decoder.last_index() {
                for i in (0..=last_index).rev() {
                    let (last_entry, payload_offset) = decoder
                        .decode_lazy(i)
                        .await
                        .map_err(|e| Decode(chunk_id.clone(), e))?;

                    prev_entry_time = last_entry.time;
                    let payload_size = last_entry.payload_size;
                    if let Err(e) = last_entry.finalize().await {
                        logger.log(
                            LogLevel::Error,
                            &format!("new encoder: read entry in chunk {chunk_id}: {e}"),
                        );
                        continue;
                    }

                    data_end = calculate_data_end(data_file_size)?;
                    payload_pos = payload_offset + u32::from(payload_size) + 1;
                    break;
                }
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

        let mut payload_file = tokio::fs::OpenOptions::new()
            .mode(FILE_MODE)
            .create(true)
            .truncate(false)
            .write(true)
            .open(payload_path)
            .await
            .map_err(OpenPayloadFile)?;

        payload_file
            .seek(SeekFrom::Start(payload_pos.into()))
            .await
            .map_err(SeekToPayloadEnd)?;

        Ok((
            Self {
                chunk_id,
                data_file,
                payload_file,
                payload_pos,
                write_buf_capacity,
                buf_count: 0,
                data_buf: Vec::with_capacity(DATA_SIZE * write_buf_capacity),
                payload_buf: Vec::new(),
            },
            prev_entry_time,
        ))
    }

    async fn write_entry(mut self, entry: &Event) -> Result<Self, EncodeError> {
        let mut data_buf = Vec::new();
        let mut payload_buf = Vec::new();
        encode_entry(
            &mut data_buf,
            &mut payload_buf,
            entry,
            &mut self.payload_pos,
        )
        .await?;
        self.data_buf
            .write_all(&data_buf)
            .await
            .expect("writing to Vec should not fail");
        self.payload_buf
            .write_all(&payload_buf)
            .await
            .expect("writing to Vec should not fail");

        self.buf_count += 1;

        if self.buf_count >= self.write_buf_capacity {
            self = self.flush().await?;
        }

        Ok(self)
    }

    // Encoder should be discarded if this fails.
    async fn flush(mut self) -> Result<Self, EncoderFlushError> {
        use EncoderFlushError::*;
        if self.buf_count == 0 {
            return Ok(self);
        }
        self.buf_count = 0;

        self.payload_file
            .write_all(&self.payload_buf)
            .await
            .map_err(Write)?;

        self.payload_file.flush().await.map_err(Flush)?;

        self.data_file
            .write_all(&self.data_buf)
            .await
            .map_err(Write)?;

        self.data_file.flush().await.map_err(Flush)?;

        self.payload_buf.clear();
        self.data_buf.clear();

        Ok(self)
    }
}

#[derive(Debug, Error)]
pub enum EncodeError {
    #[error("encode entry: {0}")]
    EncodeEntry(#[from] EncodeEntryError),

    #[error("flush: {0}")]
    Flush(#[from] EncoderFlushError),
}

#[derive(Debug, Error)]
pub enum EncoderFlushError {
    #[error("write: {0}")]
    Write(std::io::Error),

    #[error("flush: {0}")]
    Flush(std::io::Error),
}

#[derive(Debug, Error)]
pub enum EncodeEntryError {
    #[error("write: {0}")]
    Write(#[from] std::io::Error),

    #[error("{0}")]
    TryIntError(#[from] std::num::TryFromIntError),

    #[error("payload too big: {0}")]
    PayloadTooBig(usize),

    #[error("add")]
    Add,
}

async fn encode_entry<W: AsyncWrite + Unpin, W2: AsyncWrite + Unpin>(
    buf: &mut W,
    payload_buf: &mut W2,
    entry: &Event,
    payload_pos: &mut u32,
) -> Result<(), EncodeEntryError> {
    use EncodeEntryError::*;

    // Write payload and newline.
    let payload = serde_json::to_vec(&entry).expect("serialization should be infallible");
    let Ok(payload_len) = u16::try_from(payload.len()) else {
        return Err(PayloadTooBig(payload.len()));
    };
    payload_buf.write_all(&payload).await?;
    payload_buf.write_all(b"\n").await?;

    // Time.
    buf.write_all(entry.time.to_be_bytes().as_slice()).await?;

    // Payload offset and length.
    buf.write_all(&payload_pos.to_be_bytes()).await?;
    buf.write_all(&payload_len.to_be_bytes()).await?;

    // *payload_pos += payload.len() + 1
    *payload_pos = payload_pos
        .checked_add(u32::try_from(payload.len() + 1)?)
        .ok_or(Add)?;
    Ok(())
}

#[derive(Debug, Error)]
pub enum RecoverableDecodeError {
    #[error("deserialize payload: {0} pos={1} size={2}")]
    Desrialize(serde_json::Error, u32, u16),

    #[error("seek: {0}")]
    Seek(std::io::Error),

    #[error("read: {0}")]
    Read(std::io::Error),
}

#[allow(clippy::unwrap_used)]
fn decode_entry_lazy<'a, T: AsyncRead + AsyncSeek + Unpin>(
    buf: &[u8; DATA_SIZE],
    payload_file: &'a mut T,
) -> (LazyDbEntry<'a, T>, u32) {
    let time = i64::from_be_bytes(buf[..8].try_into().unwrap());
    let payload_offset = u32::from_be_bytes(buf[8..12].try_into().unwrap());
    let payload_size = u16::from_be_bytes(buf[12..14].try_into().unwrap());
    (
        LazyDbEntry {
            time: UnixNano::new(time),
            payload_file,
            payload_offset,
            payload_size,
        },
        payload_offset,
    )
}

#[derive(Debug, Error)]
pub enum TimeToIdError {
    #[error("invalid time")]
    InvalidTime,
}

// Returns the first x digits in a UnixNano timestamp as String.
// Output is padded with zeros if needed.
pub(crate) fn time_to_id(time: UnixNano) -> Result<String, TimeToIdError> {
    if time.is_negative() {
        return Err(TimeToIdError::InvalidTime);
    }
    let shifted = *time / Chunk::DURATION;
    let padded = format!("{shifted:0>0$}", Chunk::ID_LENGTH);
    if padded.len() > Chunk::ID_LENGTH {
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
    use common::Region;
    use common::{Detection, DummyLogger};
    use pretty_assertions::assert_eq;
    use pretty_hex::pretty_hex;
    use std::io::Cursor;
    use tempfile::tempdir;
    use test_case::test_case;

    async fn new_test_db(db_dir: &Path) -> Database {
        new_test_db2(db_dir, 0).await
    }
    async fn new_test_db2(db_dir: &Path, cache_capacity: usize) -> Database {
        let (shutdown_complete_tx, _) = mpsc::channel::<()>(1);
        Database::new(
            shutdown_complete_tx,
            DummyLogger::new(),
            db_dir.to_owned(),
            cache_capacity,
            cache_capacity,
            true,
        )
        .await
        .unwrap()
    }

    fn det(label: &str) -> Vec<Detection> {
        vec![Detection {
            label: label.to_owned().try_into().unwrap(),
            score: 0.0,
            region: Region::default(),
        }]
    }

    fn test_event(time: UnixNano, detections: Vec<Detection>) -> Event {
        Event {
            time,
            detections,
            duration: common::time::Duration::new(0),
            source: None,
        }
    }

    fn test_event2(time: UnixNano) -> Event {
        test_event(time, Vec::new())
    }

    fn new_test_entry(time: i64) -> Event {
        test_event(UnixNano::new(time), Vec::new())
    }

    /*fn entry1() -> Event {
        test_event(UnixNano::new(1000), det("1"))
    }*/
    fn entry2() -> Event {
        test_event(UnixNano::new(2000), det("2"))
    }
    fn entry3() -> Event {
        test_event(UnixNano::new(3000), det("3"))
    }
    fn entry4() -> Event {
        test_event(UnixNano::new(4000), det("4"))
    }

    fn query_all() -> EventQuery {
        EventQuery {
            start: UnixNano::new(1),
            end: UnixNano::new(i64::MAX),
            limit: NonZeroUsize::MAX,
        }
    }

    #[test_case(
        query_all(),
        &[entry2(), entry3(), entry4()];
        "all"
    )]
    #[test_case(
        EventQuery{
            start: UnixNano::new(0),
            end: UnixNano::new(i64::MAX),
            limit: NonZeroUsize::new(2).unwrap(),
        },
        &[entry2(), entry3()];
        "limit"
    )]
    #[test_case(
        EventQuery{
            start: UnixNano::new(0),
            end: UnixNano::new(i64::MAX),
            limit: NonZeroUsize::new(1).unwrap(),
        },
        &[entry2()];
        "limit2"
    )]
    #[test_case(
        EventQuery{
            start: UnixNano::new(2000),
            end: UnixNano::new(i64::MAX),
            limit: NonZeroUsize::MAX,
        },
        &[entry2(), entry3(), entry4()];
        "exactTime"
    )]
    #[test_case(
        EventQuery{
            start: UnixNano::new(0),
            end: UnixNano::new(4000),
            limit: NonZeroUsize::MAX,
        },
        &[entry2(), entry3()];
        "exactTime2"
    )]
    #[test_case(
        EventQuery{
            start: UnixNano::new(2500),
            end: UnixNano::new(i64::MAX),
            limit: NonZeroUsize::MAX,
        },
        &[entry3(), entry4()];
        "time"
    )]
    #[test_case(
        EventQuery{
            start: UnixNano::new(0),
            end: UnixNano::new(3500),
            limit: NonZeroUsize::MAX,
        },
        &[entry2(), entry3()];
        "time2"
    )]
    #[tokio::test]
    async fn test_event_db_query(input: EventQuery, want: &[Event]) {
        let temp_dir = tempdir().unwrap();
        let db = new_test_db(temp_dir.path()).await;

        // Populate database.
        db.write_event(entry2()).await;
        db.write_event(entry3()).await;
        db.write_event(entry4()).await;

        let got = db.query(input).await.unwrap().unwrap();
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_event_db_query_no_entries() {
        let temp_dir = tempdir().unwrap();
        let db = new_test_db(temp_dir.path()).await;

        let entries = db.query(query_all()).await.unwrap().unwrap();
        assert_eq!(0, entries.len());
    }

    #[tokio::test]
    async fn test_event_db_write_and_read() {
        let entry1 = new_test_entry(1);
        let entry2 = new_test_entry(2);
        let entry3 = new_test_entry(3);

        let temp_dir = tempdir().unwrap();
        let db = new_test_db(temp_dir.path()).await;

        db.write_event(entry1.clone()).await;
        let entries = db.query(query_all()).await.unwrap().unwrap();
        assert_eq!(entry1, entries[0]);

        db.write_event(entry2.clone()).await;
        let entries = db.query(query_all()).await.unwrap().unwrap();
        assert_eq!(vec![entry1.clone(), entry2.clone()], entries);

        db.write_event(entry3.clone()).await;
        let entries = db.query(query_all()).await.unwrap().unwrap();
        assert_eq!(vec![entry1, entry2, entry3], entries);
    }

    #[tokio::test]
    async fn test_event_db_multiple_chunks() {
        let entry1 = new_test_entry(1);
        let entry2 = new_test_entry(Chunk::DURATION);
        let entry3 = new_test_entry(Chunk::DURATION * 2);

        let temp_dir = tempdir().unwrap();
        let db = new_test_db(temp_dir.path()).await;

        db.write_event(entry1.clone()).await;
        db.write_event(entry2.clone()).await;
        db.write_event(entry3.clone()).await;

        let entries = db.query(query_all()).await.unwrap().unwrap();

        assert_eq!(vec![entry1, entry2, entry3], entries);
    }

    fn new_test_entry2(time: i64, label: &str) -> Event {
        test_event(
            UnixNano::new(time),
            vec![Detection {
                label: label.to_owned().try_into().unwrap(),
                score: 0.0,
                region: Region::default(),
            }],
        )
    }

    #[tokio::test]
    async fn test_event_db_recover_write_pos() {
        let e1 = new_test_entry2(1, "a");
        let e2 = new_test_entry2(2, "b");
        let e3 = new_test_entry2(3, "c");

        let temp_dir = tempdir().unwrap();

        let db = new_test_db(temp_dir.path()).await;
        db.write_event(e1.clone()).await;
        db.write_event(e2.clone()).await;

        let db = new_test_db(temp_dir.path()).await;
        db.write_event(e3.clone()).await;

        let want = vec![e1, e2, e3];
        let got = db.query(query_all()).await.unwrap().unwrap();
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_empty_entry() {
        let temp_dir = tempdir().unwrap();

        let entry1 = new_test_entry2(2, "good1");
        let entry2 = new_test_entry2(3, "bad");
        let entry3 = new_test_entry2(4, "good2");

        let db = new_test_db(temp_dir.path()).await;
        db.write_event(entry1.clone()).await;
        db.write_event(entry2).await;

        // Overwrite second entry with zeros.
        let mut file = tokio::fs::OpenOptions::new()
            .write(true)
            .open(temp_dir.path().join("00000.data"))
            .await
            .unwrap();
        file.seek(SeekFrom::Start(ChunkHeader::LENGTH + DATA_SIZE2))
            .await
            .unwrap();
        file.write_all(&[0].repeat(DATA_SIZE)).await.unwrap();
        file.flush().await.unwrap();

        let db = new_test_db(temp_dir.path()).await;

        let got = db.query(query_all()).await.unwrap().unwrap();
        assert_eq!([entry1.clone()].as_slice(), got.as_slice());

        db.write_event(entry3.clone()).await;
        let got2 = db.query(query_all()).await.unwrap().unwrap();
        assert_eq!([entry1, entry3].as_slice(), got2.as_slice());
    }

    #[tokio::test]
    async fn test_empty_entry2() {
        let temp_dir = tempdir().unwrap();

        let entry1 = new_test_entry2(1, "bad");
        let entry2 = new_test_entry2(2, "good");

        let db = new_test_db(temp_dir.path()).await;
        db.write_event(entry1.clone()).await;

        // Overwrite first entry with zeros.
        let mut file = tokio::fs::OpenOptions::new()
            .write(true)
            .open(temp_dir.path().join("00000.data"))
            .await
            .unwrap();
        file.seek(SeekFrom::Start(28)).await.unwrap();
        file.write_all(&[0].repeat(20)).await.unwrap();
        file.flush().await.unwrap();

        let db = new_test_db(temp_dir.path()).await;

        assert!(db.query(query_all()).await.unwrap().unwrap().is_empty());

        db.write_event(entry2.clone()).await;
        let got = db.query(query_all()).await.unwrap().unwrap();
        assert_eq!([entry2].as_slice(), got.as_slice());
    }

    #[tokio::test]
    async fn test_empty_chunk() {
        let temp_dir = tempdir().unwrap();

        let entry1 = new_test_entry2(1, "missing");

        let db = new_test_db(temp_dir.path()).await;
        db.write_event(entry1.clone()).await;

        // Clear data file.
        let file_path = temp_dir.path().join("00000.data");
        tokio::fs::remove_file(&file_path).await.unwrap();

        let magic_bytes: &[u8] = &ChunkHeader::MAGIC_BYTES;
        let header = [magic_bytes, &Chunk::API_VERSION.to_be_bytes()].concat();
        tokio::fs::write(file_path, &header).await.unwrap();

        let db = new_test_db(temp_dir.path()).await;

        assert!(db.query(query_all()).await.unwrap().unwrap().is_empty());

        let entry2 = new_test_entry2(1, "good");
        db.write_event(entry2.clone()).await;
        let got = db.query(query_all()).await.unwrap().unwrap();
        assert_eq!([entry2].as_slice(), got.as_slice());
    }

    #[tokio::test]
    async fn test_event_db_order() {
        let temp_dir = tempdir().unwrap();

        let db = new_test_db(temp_dir.path()).await;
        db.write_event(new_test_entry(100)).await;

        let db = new_test_db(temp_dir.path()).await;
        db.write_event(new_test_entry(90)).await;
        db.write_event(new_test_entry(120)).await;
        db.write_event(new_test_entry(0)).await;

        let want = vec![
            new_test_entry(100),
            new_test_entry(101),
            new_test_entry(120),
            new_test_entry(121),
        ];
        let got = db.query(query_all()).await.unwrap().unwrap();
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_event_db_search() {
        let e1 = new_test_entry(1);
        let e2 = new_test_entry(2);
        let e3 = new_test_entry(3);
        let e4 = new_test_entry(4);
        let e5 = new_test_entry(Chunk::DURATION);
        let e6 = new_test_entry(Chunk::DURATION + 1);
        let e7 = new_test_entry(Chunk::DURATION + 2);
        let e8 = new_test_entry(Chunk::DURATION * 2);
        let e9 = new_test_entry(Chunk::DURATION * 2 + 1);

        #[rustfmt::skip]
        let cases: &[(i64, i64, &[&Event])] = &[
            (0,          i64::MAX,   &[&e1, &e2, &e3, &e4, &e5, &e6, &e7, &e8, &e9]),
            (0,          *e1.time,   &[]),
            (0,          *e2.time,   &[&e1]),
            (0,          *e3.time,   &[&e1, &e2]),
            (*e1.time,   *e4.time,   &[&e1, &e2, &e3]),
            (*e2.time,   *e4.time+1, &[&e2, &e3, &e4]),
            (*e3.time,   *e5.time-1, &[&e3, &e4]),
            (*e4.time,   *e5.time,   &[&e4]),
            (*e4.time+1, *e6.time,   &[&e5]),
            (*e5.time-1, *e7.time,   &[&e5, &e6]),
            (*e5.time,   *e7.time+1, &[&e5, &e6, &e7]),
            (*e6.time,   *e8.time-1, &[&e6, &e7]),
            (*e7.time,   *e8.time,   &[&e7]),
            (*e7.time+1, *e9.time,   &[&e8]),
            (*e8.time-1, *e9.time+1, &[&e8, &e9]),
            (*e8.time,   i64::MAX,   &[&e8, &e9]),
            (*e9.time,   i64::MAX,   &[&e9]),
            (*e9.time+1, i64::MAX,   &[]),
        ];

        for cache_cap in 0..10 {
            let temp_dir = tempdir().unwrap();
            let db = new_test_db2(temp_dir.path(), cache_cap).await;
            for e in [&e1, &e2, &e3, &e4, &e5, &e6, &e7, &e8, &e9] {
                db.write_event(e.clone()).await;
            }

            for (i, (start, end, want)) in cases.iter().enumerate() {
                let want: Vec<Event> = want.iter().copied().cloned().collect();
                let got = db
                    .query(EventQuery {
                        start: UnixNano::new(*start),
                        end: UnixNano::new(*end),
                        limit: NonZeroUsize::MAX,
                    })
                    .await
                    .unwrap()
                    .unwrap();
                assert_eq!(want, got, "CASE={i}");
            }
        }
    }

    #[tokio::test]
    async fn test_new_store_mkdir() {
        let (shutdown_complete_tx, mut shutdown_complete_rx) = mpsc::channel::<()>(1);
        let temp_dir = tempdir().unwrap();

        let new_dir = temp_dir.path().join("test");
        assert!(std::fs::metadata(&new_dir).is_err());

        Database::new(
            shutdown_complete_tx,
            DummyLogger::new(),
            new_dir.clone(),
            0,
            0,
            true,
        )
        .await
        .unwrap();

        std::fs::metadata(new_dir).unwrap();

        let _ = shutdown_complete_rx.recv().await;
    }

    #[tokio::test]
    async fn test_event_entry_encode() {
        let mut data_buf = Vec::with_capacity(DATA_SIZE);
        let mut payload_buf = Cursor::new(Vec::new());
        let mut payload_pos = 0;

        let entry = test_event2(UnixNano::new(5));
        encode_entry(&mut data_buf, &mut payload_buf, &entry, &mut payload_pos)
            .await
            .unwrap();

        let want = vec![
            0, 0, 0, 0, 0, 0, 0, 5, // Time.
            0, 0, 0, 0, // Message offset.
            0, 0x35, // Message size.
        ];
        let want_payload = b"{\"time\":5,\"duration\":0,\"detections\":[],\"source\":null}
";
        assert_eq!(pretty_hex(&want), pretty_hex(&data_buf));
        assert_eq!(
            pretty_hex(&want_payload),
            pretty_hex(&payload_buf.into_inner())
        );

        let payload_len = serde_json::to_vec(&entry).unwrap().len();
        assert_eq!(payload_len + 1, usize::try_from(payload_pos).unwrap());
    }

    #[tokio::test]
    async fn test_event_entry_decode() {
        let mut data_buf = Cursor::new(Vec::new());
        let mut payload_buf = Cursor::new(Vec::new());
        let mut payload_pos: u32 = 10;

        payload_buf.seek(SeekFrom::Start(10)).await.unwrap();

        let entry = test_event2(UnixNano::new(5));
        encode_entry(&mut data_buf, &mut payload_buf, &entry, &mut payload_pos)
            .await
            .unwrap();

        let buf: [u8; DATA_SIZE] = data_buf.into_inner().try_into().unwrap();

        let (got_entry, payload_pos) = decode_entry_lazy(&buf, &mut payload_buf);
        let got_entry = got_entry.finalize().await.unwrap();
        assert_eq!(entry, got_entry);
        assert_eq!(10, payload_pos);
    }

    #[test_case(UnixNano::new(0), "00000"; "a")]
    #[test_case(UnixNano::new(1_000_334_455_000_111_000), "10003"; "b")]
    #[test_case(UnixNano::new(1_122_334_455_000_111_000), "11223"; "c")]
    #[test_case(UnixNano::new(Chunk::DURATION - 1000), "00000"; "d")]
    #[test_case(UnixNano::new(Chunk::DURATION), "00001"; "e")]
    #[test_case(UnixNano::new(Chunk::DURATION + 1000), "00001"; "f")]
    fn test_time_to_id(input: UnixNano, output: &str) {
        assert_eq!(output, time_to_id(input).unwrap());
    }

    #[test]
    fn test_time_to_id_error() {
        assert!(matches!(
            time_to_id(UnixNano::new(-1)),
            Err(TimeToIdError::InvalidTime)
        ));
    }

    #[tokio::test]
    async fn test_new_chunk_encoder_version_error() {
        let db_dir = tempdir().unwrap();
        let chunk_id = "0";

        std::fs::write(
            db_dir.path().join("0.data"),
            [ChunkHeader::MAGIC_BYTES.as_slice(), &[255].repeat(30)].concat(),
        )
        .unwrap();

        assert!(matches!(
            ChunkEncoder::new(
                DummyLogger::new(),
                db_dir.path().to_owned(),
                chunk_id.to_owned(),
                0
            )
            .await,
            Err(NewChunkEncoderError::NewChunkDecoder(
                CreateChunkDecoderError::UnknownChunkVersion
            ))
        ));
    }

    #[tokio::test]
    async fn test_new_chunk_decoder_version_error() {
        let db_dir = tempdir().unwrap();
        let chunk_id = "0";

        std::fs::write(
            db_dir.path().join("0.data"),
            [ChunkHeader::MAGIC_BYTES.as_slice(), &[255].repeat(30)].concat(),
        )
        .unwrap();

        assert!(matches!(
            ChunkDecoder::new(db_dir.path(), chunk_id).await,
            Err(CreateChunkDecoderError::UnknownChunkVersion)
        ));
    }

    #[allow(clippy::unnecessary_wraps)]
    fn dur(v: i64) -> Option<std::time::Duration> {
        Some(std::time::Duration::from_nanos(u64::try_from(v).unwrap()))
    }

    #[test_case(UnixNano::new(0), None, None; "none")]
    #[test_case(UnixNano::new(SECOND),    Some(UnixNano::new(0)),        dur(10*SECOND); "zero")]
    #[test_case(UnixNano::new(SECOND),    Some(UnixNano::new(SECOND)),   dur(11*SECOND); "one")]
    #[test_case(UnixNano::new(SECOND),    Some(UnixNano::new(2*SECOND)), dur(12*SECOND); "two")]
    #[test_case(UnixNano::new(10*SECOND), Some(UnixNano::new(0)), dur(SECOND); "less")]
    #[test_case(UnixNano::new(11*SECOND), Some(UnixNano::new(0)), dur(0);      "equal")]
    #[test_case(UnixNano::new(12*SECOND), Some(UnixNano::new(0)), dur(0);      "greater")]
    #[test_case(UnixNano::new(i64::MIN), Some(UnixNano::new(0)), None; "invalid")]
    fn test_calculate_sleep_duration(
        now: UnixNano,
        first_item: Option<UnixNano>,
        want: Option<std::time::Duration>,
    ) {
        assert_eq!(want, calculate_reorder_sleep_duration(now, first_item));
    }
}
