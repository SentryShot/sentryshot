// SPDX-License-Identifier: GPL-2.0-or-later

mod cache;

pub use cache::VodCache;
use common::{
    MonitorId,
    recording::{RecordingId, RecordingIdError},
    time::{Duration, HOUR, UnixH264, UnixNano},
};
use pin_project::pin_project;
use recdb::{CrawlerError, RecDb, RecDbQuery, RecordingResponse};
use recording::{GenerateMp4Error, ReadMetaError, Sample, generate_mp4, read_meta};
use serde::Deserialize;
use std::{
    future::Future,
    io::SeekFrom,
    num::NonZeroUsize,
    path::PathBuf,
    pin::{Pin, pin},
    sync::Arc,
    task::{Context, Poll},
};
use thiserror::Error;
use tokio::{
    io::{AsyncRead, AsyncSeek, BufReader},
    task::JoinHandle,
};

#[derive(Clone, Deserialize, Hash, PartialEq, Eq)]
pub struct VodQuery {
    #[serde(rename = "monitor-id")]
    pub monitor_id: MonitorId,
    pub start: UnixNano,
    pub end: UnixNano,

    #[serde(rename = "cache-id")]
    cache_id: u32,
}

#[derive(Debug, PartialEq, Eq)]
struct QueryResult {
    meta: Vec<u8>,
    meta_size: usize,
    size: usize,
    recs: Vec<Rec>,
}

#[pin_project]
#[derive(Debug)]
pub struct VodReader {
    r: Arc<QueryResult>,

    #[pin]
    file_state: FileState,
    pos: usize,
}

#[derive(Debug, Error)]
pub enum CreateVodReaderError {
    #[error("duration is negative")]
    NegativeDuration,

    #[error("max duration is 12 hours, it's easy to extend if anyone wants it")]
    MaxDuration,

    #[error("query recordings: {0}")]
    QueryRecordings(#[from] CrawlerError),

    #[error("sub")]
    Sub,

    #[error("add")]
    Add,

    #[error("parse recording id: {0}")]
    ParseRecordingId(#[from] RecordingIdError),

    #[error("read metadata: {0}")]
    Metadata(std::io::Error),

    #[error("open file: {0}")]
    OpenFile(std::io::Error),

    #[error("mismatched params")]
    MismatchedParams,

    #[error("read meta: {0}")]
    ReadMeta(#[from] ReadMetaError),

    #[error("dts")]
    Dts,

    #[error("end")]
    End,

    #[error("generate mp4: {0}")]
    GenerateMp4(#[from] GenerateMp4Error),
}

impl VodReader {
    pub async fn new(
        recdb: &RecDb,
        cache: &VodCache,
        q: VodQuery,
    ) -> Result<Option<Self>, CreateVodReaderError> {
        let r = {
            if let Some(r) = cache.get(&q).await {
                r
            } else {
                let Some(r) = execute_query(recdb, &q).await? else {
                    return Ok(None);
                };
                cache.add(q, r.clone()).await;
                r
            }
        };

        Ok(Some(Self {
            r,
            file_state: FileState::Close(CloseReadState::State1),
            pos: 0,
        }))
    }

    pub fn size(&self) -> u64 {
        u64::try_from(self.r.size).expect("u64 fit usize")
    }
}

#[allow(clippy::too_many_lines)]
async fn execute_query(
    recdb: &RecDb,
    q: &VodQuery,
) -> Result<Option<Arc<QueryResult>>, CreateVodReaderError> {
    use CreateVodReaderError::*;

    let duration = q.end - q.start;
    if duration.is_negative() {
        return Err(NegativeDuration);
    }
    if duration > UnixNano::new(HOUR * 12) {
        return Err(MaxDuration);
    }

    // Find first recording by seeking backwards.
    let end_minus_1 = q
        .start
        .checked_sub(Duration::from_secs(1).into())
        .ok_or(Sub)?;
    let mut recordings = recdb
        .recordings_by_query(&RecDbQuery {
            recording_id: RecordingId::from_nanos(end_minus_1, &q.monitor_id)?,
            end: None,
            limit: NonZeroUsize::new(1).expect("nonzero"),
            reverse: false,
            monitors: vec![q.monitor_id.to_string()],
            include_data: false,
        })
        .await?;

    let first_rec_id = match recordings.first() {
        Some(v) => v.id().clone(),
        None => RecordingId::zero(),
    };

    // Find all matching recorings by querying from the first recording.
    let end_plus_1 = q
        .end
        .checked_add(Duration::from_secs(1).into())
        .ok_or(Add)?;
    recordings.extend(
        recdb
            .recordings_by_query(&RecDbQuery {
                recording_id: first_rec_id.clone(),
                end: Some(RecordingId::from_nanos(end_plus_1, &q.monitor_id)?),
                limit: NonZeroUsize::new(100_000).expect("nonzero"),
                reverse: true,
                monitors: vec![q.monitor_id.to_string()],
                include_data: false,
            })
            .await?,
    );

    if recordings.is_empty() {
        return Ok(None);
    }

    let mut recs = Vec::new();
    let mut params = None;

    for rec in &recordings {
        let RecordingResponse::Finalized(rec) = rec else {
            continue;
        };

        let Some(meta_path) = recdb.recording_file_by_ext(&rec.id, "meta").await else {
            continue;
        };
        let Some(mdat_path) = recdb.recording_file_by_ext(&rec.id, "mdat").await else {
            continue;
        };

        let meta_size = tokio::fs::metadata(&meta_path)
            .await
            .map_err(Metadata)?
            .len();

        let mut meta = BufReader::new(
            tokio::fs::OpenOptions::new()
                .read(true)
                .open(meta_path)
                .await
                .map_err(OpenFile)?,
        );

        let (header, samples) = read_meta(&mut meta, meta_size).await?;
        params = Some(header.params());

        let samples: Vec<_> = samples
            .into_iter()
            .map(|s| {
                let dts = UnixNano::from(s.dts().ok_or(Dts)?);
                let end = UnixNano::from(s.end().ok_or(End)?);
                Ok((s, q.start <= dts && end <= q.end))
            })
            // I don't like fallible iterators.
            .collect::<Result<Vec<(Sample, bool)>, CreateVodReaderError>>()?
            .into_iter()
            .filter_map(|(v, keep)| keep.then_some(v))
            // Skip until first IDR.
            .skip_while(|v| !v.random_access_present)
            .collect();

        if let Some(first) = samples.first() {
            let data_start = usize::try_from(first.data_offset).expect("usize fit u32");
            let data_size: usize = samples
                .iter()
                .map(|v| usize::try_from(v.data_size).expect("u32 fit usize"))
                .sum();

            recs.push(RecPartWithSamples {
                rec: Rec {
                    mdat_path: mdat_path.clone(),
                    data_start,
                    size: data_size,
                    start: 0,
                    end: 0,
                },
                samples: samples.clone(),
            });
        }
    }

    let mut samples: Vec<_> = recs.iter_mut().flat_map(|v| &mut v.samples).collect();

    // Shift first sample to start time.
    let Some(first) = samples.first_mut() else {
        return Ok(None);
    };
    first.pts = q.start.into();

    // Pad durations to fill any gaps.
    for i in 1..samples.len() {
        let s0_dts = samples[i - 1].dts().ok_or(Dts)?;
        let s1_dts = samples[i].dts().ok_or(Dts)?;
        let diff = s1_dts - s0_dts;
        samples[i - 1].duration = diff.into();
    }
    let last = samples.last_mut().expect("should exist");
    last.duration = (UnixH264::from(q.end) - last.pts).into();
    assert_eq!(last.end().ok_or(End)?, q.end.into());

    let mut meta = Vec::new();
    let mdat_size = usize::try_from(
        generate_mp4(
            &mut meta,
            q.start.into(),
            recs.iter().flat_map(|v| &v.samples),
            &params.expect("should be Some"),
        )
        .await?,
    )
    .expect("u32 fit usize");
    let meta_size = meta.len();

    // Calculate recordings offsets.
    let mut pos = meta_size;
    let mut recs: Vec<_> = recs.into_iter().map(|v| v.rec).collect();
    for rec in &mut recs {
        rec.start = pos;
        rec.end = pos + rec.size;
        pos += rec.size;
    }

    Ok(Some(Arc::new(QueryResult {
        meta: meta.clone(),
        meta_size: meta.len(),
        size: meta_size + mdat_size,
        recs,
    })))
}

#[allow(clippy::too_many_lines)]
impl AsyncRead for VodReader {
    fn poll_read(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &mut tokio::io::ReadBuf<'_>,
    ) -> std::task::Poll<std::io::Result<()>> {
        loop {
            let mut this = self.as_mut().project();
            match this.file_state.as_mut().project() {
                FileStateProj::Close(mut read_state) => match read_state.as_mut().project() {
                    CloseReadStateProj::State1 => {
                        // Position is within meta.
                        if *this.pos < this.r.meta_size {
                            let meta_remaining = self.r.meta_size - self.pos;
                            let amt = std::cmp::min(meta_remaining, buf.remaining());

                            buf.put_slice(&self.r.meta[self.pos..][..amt]);
                            self.pos += amt;
                            return Poll::Ready(Ok(()));
                        }

                        // Find recording at the position.
                        let Some(i) = this.r.recs.iter().position(|rec| *this.pos < rec.end) else {
                            // EOF.
                            return Poll::Ready(Ok(()));
                        };
                        let rec = &this.r.recs[i];
                        assert!(rec.start <= *this.pos);

                        let file_pos = *this.pos - rec.start + rec.data_start;
                        let remaining = rec.end - *this.pos;
                        let amt = std::cmp::min(remaining, buf.remaining());

                        *read_state = CloseReadState::State4(i, file_pos, amt);
                        continue;
                    }
                    CloseReadStateProj::State4(i, file_pos, amt) => {
                        let mdat_path = this.r.recs[*i].mdat_path.clone();
                        let open_fut = tokio::task::spawn_blocking(move || {
                            std::fs::OpenOptions::new().read(true).open(mdat_path)
                        });
                        *read_state = CloseReadState::State5(open_fut, *i, *file_pos, *amt);
                        continue;
                    }
                    CloseReadStateProj::State5(open_fut, i, file_pos, amt) => {
                        //
                        match open_fut.poll(cx) {
                            Poll::Ready(res) => {
                                let file = tokio::fs::File::from_std(res??);
                                if *file_pos != 0 {
                                    let state = OpenReadState::State6(*file_pos, *amt);
                                    *this.file_state = FileState::Open(state, file, 0, *i);
                                    continue;
                                }

                                *this.file_state =
                                    FileState::Open(OpenReadState::State3(*amt), file, 0, *i);
                                continue;
                            }
                            Poll::Pending => return Poll::Pending,
                        }
                    }
                },
                FileStateProj::Open(mut read_state, open_file, open_file_pos, open_file_index) => {
                    match read_state.as_mut().project() {
                        OpenReadStateProj::State1 => {
                            // Position is within meta.
                            if *this.pos < this.r.meta_size {
                                let meta_remaining = self.r.meta_size - self.pos;
                                let amt = std::cmp::min(meta_remaining, buf.remaining());

                                buf.put_slice(&self.r.meta[self.pos..][..amt]);
                                self.pos += amt;
                                return Poll::Ready(Ok(()));
                            }

                            // Find recording at the position.
                            let Some(i) = this.r.recs.iter().position(|rec| *this.pos < rec.end)
                            else {
                                // EOF.
                                return Poll::Ready(Ok(()));
                            };
                            let rec = &this.r.recs[i];
                            assert!(rec.start <= *this.pos);

                            let file_pos = *this.pos - rec.start + rec.data_start;
                            let remaining = rec.end - *this.pos;
                            let amt = std::cmp::min(remaining, buf.remaining());

                            // The right file is open and still has data.
                            if *open_file_index == i
                                && *this.pos != rec.end
                                && *open_file_pos != file_pos
                            {
                                let file_pos = u64::try_from(file_pos).expect("u64 fit usize");
                                open_file.start_seek(SeekFrom::Start(file_pos))?;

                                *read_state = OpenReadState::State2(amt);
                                continue;
                            }
                            *this.file_state =
                                FileState::Close(CloseReadState::State4(i, file_pos, amt));
                            continue;
                        }
                        OpenReadStateProj::State2(amt) => {
                            //
                            match open_file.poll_complete(cx) {
                                Poll::Ready(res) => {
                                    res?;
                                    *read_state = OpenReadState::State3(*amt);
                                    continue;
                                }
                                Poll::Pending => return Poll::Pending,
                            }
                        }
                        OpenReadStateProj::State3(amt) => {
                            let prev_buf_len = buf.filled().len();
                            match open_file.poll_read(cx, buf) {
                                Poll::Ready(res) => {
                                    res?;

                                    let n = std::cmp::min(*amt, buf.filled().len() - prev_buf_len);

                                    // Trim overfilling.
                                    buf.set_filled(prev_buf_len + n);

                                    *open_file_pos += n;
                                    *this.pos += n;

                                    *read_state = OpenReadState::State1;
                                    return Poll::Ready(Ok(()));
                                }
                                Poll::Pending => return Poll::Pending,
                            }
                        }
                        OpenReadStateProj::State6(file_pos, amt) => {
                            let file_pos = u64::try_from(*file_pos).expect("u64 fit usize");
                            open_file.start_seek(SeekFrom::Start(file_pos))?;
                            *read_state = OpenReadState::State7(*amt);
                            continue;
                        }
                        OpenReadStateProj::State7(amt) => {
                            //
                            match open_file.poll_complete(cx) {
                                Poll::Ready(res) => {
                                    res?;
                                    *read_state = OpenReadState::State3(*amt);
                                    continue;
                                }
                                Poll::Pending => return Poll::Pending,
                            }
                        }
                    }
                }
            }
        }
    }
}

#[derive(Debug)]
#[pin_project(project = FileStateProj)]
enum FileState {
    Close(#[pin] CloseReadState),
    Open(#[pin] OpenReadState, #[pin] tokio::fs::File, usize, usize),
}

#[derive(Debug)]
#[pin_project(project = CloseReadStateProj)]
enum CloseReadState {
    State1,
    State4(usize, usize, usize),
    State5(#[pin] OpenFut, usize, usize, usize),
}

#[derive(Debug)]
#[pin_project(project = OpenReadStateProj)]
enum OpenReadState {
    State1,
    State2(usize),
    State3(usize),
    State6(usize, usize),
    State7(usize),
}

type OpenFut = JoinHandle<Result<std::fs::File, std::io::Error>>;

impl AsyncSeek for VodReader {
    fn start_seek(mut self: Pin<&mut Self>, position: SeekFrom) -> std::io::Result<()> {
        match position {
            SeekFrom::Start(pos) => {
                self.pos = usize::try_from(pos)
                    .map_err(|e| std::io::Error::new(std::io::ErrorKind::InvalidInput, e))?;
                Ok(())
            }
            _ => unimplemented!(),
        }
    }

    fn poll_complete(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<std::io::Result<u64>> {
        Poll::Ready(Ok(u64::try_from(self.pos).expect("u64 fit usize")))
    }
}

/*
impl Read for VodReader {
    fn read(&mut self, buf: &mut [u8]) -> std::io::Result<usize> {
        // STATE 1.
        // Position is within meta.
        if self.pos < self.meta_size {
            let meta_remaining = self.meta_size - self.pos;
            let amt = std::cmp::min(meta_remaining, buf.len());
            buf[..amt].clone_from_slice(&self.meta[self.pos..][..amt]);
            self.pos += amt;
            return Ok(amt);
        }

        // Find recording at the position.
        let Some(i) = self.recs.iter().position(|rec| self.pos < rec.end) else {
            // EOF.
            return Ok(0);
        };
        let rec = &self.recs[i];
        assert!(rec.start <= self.pos);

        let file_pos = self.pos - rec.start + rec.data_start;
        let remaining = rec.end - self.pos;
        let amt = std::cmp::min(remaining, buf.len());

        if let Some(f) = &mut self.open_file {
            // File already open and still have data.
            if f.index == i && self.pos != rec.end {
                if f.pos != file_pos {
                    // STATE 2.
                    f.file.seek(SeekFrom::Start(file_pos as u64)).unwrap();
                }
                // STATE 3.
                let n = std::io::Read::read(&mut f.file, &mut buf[..amt])?;
                f.pos += n;
                self.pos += n;
                return Ok(n);
            }
        }

        // STATE 4 AND 5.
        let mut file = std::fs::OpenOptions::new()
            .read(true)
            .open(&rec.mdat_path)
            .unwrap();
        if file_pos != 0 {
            STATE 6 and 7.
            file.seek(SeekFrom::Start(file_pos as u64)).unwrap();
        }

        // Set open_file and go to state 3.
        let n = std::io::Read::read(&mut file, &mut buf[..amt])?;
        self.pos += n;

        self.open_file = Some(OpenFile {
            index: i,
            file,
            pos: n,
        });

        return Ok(n);
    }
}

impl Seek for VodReader {
    fn seek(&mut self, pos: SeekFrom) -> std::io::Result<u64> {
        let SeekFrom::Start(pos) = pos else {
            unimplemented!()
        };
        self.pos = pos as usize;
        return Ok(pos);
    }
}
*/

struct RecPartWithSamples {
    rec: Rec,
    samples: Vec<Sample>,
}

#[derive(Debug, PartialEq, Eq)]
struct Rec {
    mdat_path: PathBuf,
    data_start: usize,
    size: usize,
    start: usize,
    end: usize,
}

#[cfg(test)]
#[allow(clippy::unwrap_used, clippy::as_conversions)]
mod tests {
    use super::*;
    use common::{
        DummyLogger, DummyStorage, PaddedBytes, VideoSample,
        recording::RecordingData,
        time::{DtsOffset, DurationH264, HOUR, MINUTE, SECOND, UnixH264, UnixNano},
    };
    use pretty_assertions::assert_eq;
    use pretty_hex::pretty_hex;
    use recdb::RecDb;
    use recording::{MetaHeader, VideoWriter};
    use std::{path::Path, sync::Arc};
    use tempfile::TempDir;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};

    #[tokio::test]
    async fn test_vod_simple1() {
        let year_2000: UnixH264 = UnixNano::new(946_684_800 * SECOND).into();
        let start_time: UnixH264 = year_2000 + UnixNano::new(10 * MINUTE).into();

        let (_tmp_dir, rec_db) = single_recording(start_time).await;

        let query = VodQuery {
            monitor_id: "x".to_owned().try_into().unwrap(),
            start: start_time.into(),
            end: UnixNano::from(start_time + UnixH264::new(7)) + UnixNano::new(1),
            cache_id: 0,
        };
        let got = new_vod_reader_read_all(&rec_db, query).await;

        #[rustfmt::skip]
        let want: Vec<u8> = [
            test_mp4(
                [0, 0, 2, 0x88],
                [0, 0, 0, 0],
                [0, 0, 2, 0x14],
                [0, 0, 1, 0xb0],
                [0, 0, 0, 0x7],
                [0, 0, 1, 0x5b],
                [0, 0, 1, 0x1b],
            ).as_slice(),
            &[
                0, 0, 0, 0x20, b's', b't', b't', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 2, // Entry count.
                0, 0, 0, 1, // Entry1 sample count.
                0, 0, 0, 4, // Entry1 sample delta.
                0, 0, 0, 3, // Entry2 sample count.
                0, 0, 0, 1, // Entry2 sample delta.
                0, 0, 0, 0x18, b's', b't', b's', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 2, // Entry count.
                0, 0, 0, 1, // Entry1.
                0, 0, 0, 3, // Entry2.
                0, 0, 0, 0x18, b'c', b't', b't', b's',
                1, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 4, // Entry1 sample count.
                0, 0, 0, 0, // Entry1 sample offset
                0, 0, 0, 0x1c, b's', b't', b's', b'c',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 first chunk.
                0, 0, 0, 4, // Entry1 samples per chunk.
                0, 0, 0, 1, // Entry1 sample description index.
                0, 0, 0, 0x24, b's', b't', b's', b'z',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 0, // Sample size.
                0, 0, 0, 4, // Sample count.
                0, 0, 0, 1, // Entry1 size.
                0, 0, 0, 1, // Entry2 size.
                0, 0, 0, 1, // Entry3 size.
                0, 0, 0, 1, // Entry4 size.
                0, 0, 0, 0x14, b's', b't', b'c', b'o',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 2, 0xa4, // Chunk offset1.
                //
                0, 0, 0, 0x0c, b'm', b'd', b'a', b't',
                1, 2, 3, 4 // Data.
            ]
        ].into_iter().flatten().copied().collect();
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }

    #[tokio::test]
    async fn test_vod_simple2() {
        let year_2000: UnixH264 = UnixNano::new(946_684_800 * SECOND).into();
        let start_time: UnixH264 = year_2000 + UnixNano::new(10 * MINUTE).into();

        let (_tmp_dir, rec_db) = single_recording(start_time).await;

        let query = VodQuery {
            monitor_id: "x".to_owned().try_into().unwrap(),
            start: (start_time + UnixH264::new(4)).into(), // Second sample.
            end: UnixNano::from(start_time + UnixH264::new(7)) + UnixNano::new(1),
            cache_id: 0,
        };
        let got = new_vod_reader_read_all(&rec_db, query).await;

        #[rustfmt::skip]
        let want: Vec<u8> = [
            test_mp4(
                [0, 0, 2, 0x7c],
                [0, 0, 0, 0],
                [0, 0, 2, 0x8],
                [0, 0, 1, 0xa4],
                [0, 0, 0, 4],
                [0, 0, 1, 0x4f],
                [0, 0, 1, 0xf],
            ).as_slice(),
            &[
                0, 0, 0, 0x20, b's', b't', b't', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 2, // Entry count.
                0, 0, 0, 1, // Entry1 sample count.
                0, 0, 0, 3, // Entry1 sample delta.
                0, 0, 0, 1, // Entry2 sample count.
                0, 0, 0, 1, // Entry2 sample delta.
                0, 0, 0, 0x14, b's', b't', b's', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1.
                0, 0, 0, 0x18, b'c', b't', b't', b's',
                1, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 2, // Entry1 sample count.
                0, 0, 0, 0, // Entry1 sample offset
                0, 0, 0, 0x1c, b's', b't', b's', b'c',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 first chunk.
                0, 0, 0, 2, // Entry1 samples per chunk.
                0, 0, 0, 1, // Entry1 sample description index.
                0, 0, 0, 0x1c, b's', b't', b's', b'z',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 0, // Sample size.
                0, 0, 0, 2, // Sample count.
                0, 0, 0, 1, // Entry1 size.
                0, 0, 0, 1, // Entry2 size.
                0, 0, 0, 0x14, b's', b't', b'c', b'o',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 2, 0x98, // Chunk offset1.
                //
                0, 0, 0, 0x0a, b'm', b'd', b'a', b't',
                3, 4 // Data.
            ]
        ].into_iter().flatten().copied().collect();
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }

    #[tokio::test]
    async fn test_vod_simple3() {
        let year_2000: UnixH264 = UnixNano::new(946_684_800 * SECOND).into();
        let start_time: UnixH264 = year_2000 + UnixNano::new(10 * MINUTE).into();

        let (_tmp_dir, rec_db) = single_recording(start_time).await;

        let query = VodQuery {
            monitor_id: "x".to_owned().try_into().unwrap(),
            start: (start_time + UnixH264::new(5)).into(), // Third sample.
            end: (start_time + UnixH264::new(1_000_000)).into(),
            cache_id: 0,
        };
        let got = new_vod_reader_read_all(&rec_db, query).await;

        #[rustfmt::skip]
        let want: Vec<u8> = [
            test_mp4(
                [0, 0, 2,    0x7c],
                [0, 0, 0x2b, 0x67],
                [0, 0, 2,    0x8],
                [0, 0, 1,    0xa4],
                [0, 0xf, 0x42, 0x3b],
                [0, 0, 1, 0x4f],
                [0, 0, 1, 0xf],
            ).as_slice(),
            &[
                0, 0, 0, 0x20, b's', b't', b't', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 2, // Entry count.
                0, 0, 0, 1, // Entry1 sample count.
                0, 0, 0, 2, // Entry1 sample delta.
                0, 0, 0, 1, // Entry2 sample count.
                0, 0xf, 0x42, 0x39, // Entry2 sample delta.
                0, 0, 0, 0x14, b's', b't', b's', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1.
                0, 0, 0, 0x18, b'c', b't', b't', b's',
                1, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 2, // Entry1 sample count.
                0, 0, 0, 0, // Entry1 sample offset
                0, 0, 0, 0x1c, b's', b't', b's', b'c',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 first chunk.
                0, 0, 0, 2, // Entry1 samples per chunk.
                0, 0, 0, 1, // Entry1 sample description index.
                0, 0, 0, 0x1c, b's', b't', b's', b'z',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 0, // Sample size.
                0, 0, 0, 2, // Sample count.
                0, 0, 0, 1, // Entry1 size.
                0, 0, 0, 1, // Entry2 size.
                0, 0, 0, 0x14, b's', b't', b'c', b'o',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 2, 0x98, // Chunk offset1.
                //
                0, 0, 0, 0x0a, b'm', b'd', b'a', b't',
                3, 4 // Data.
            ]
        ].into_iter().flatten().copied().collect();
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }

    #[tokio::test]
    async fn test_vod_simple4() {
        let year_2000: UnixH264 = UnixNano::new(946_684_800 * SECOND).into();
        let start_time: UnixH264 = year_2000 + UnixNano::new(10 * MINUTE).into();

        let (_tmp_dir, rec_db) = single_recording(start_time).await;

        let query = VodQuery {
            monitor_id: "x".to_owned().try_into().unwrap(),
            start: (start_time + UnixH264::new(6)).into(), // Last sample.
            end: (start_time + UnixH264::new(1_000_000)).into(),
            cache_id: 0,
        };
        assert!(
            VodReader::new(&rec_db, &VodCache::new(), query)
                .await
                .unwrap()
                .is_none()
        );
    }

    #[tokio::test]
    async fn test_vod_simple5() {
        let year_2000: UnixH264 = UnixNano::new(946_684_800 * SECOND).into();
        let start_time: UnixH264 = year_2000 + UnixNano::new(10 * MINUTE).into();

        let (_tmp_dir, rec_db) = single_recording(start_time + UnixH264::new(90000)).await;

        let query = VodQuery {
            monitor_id: "x".to_owned().try_into().unwrap(),
            start: start_time.into(),
            end: (start_time + UnixH264::new(SECOND)).into(),
            cache_id: 0,
        };
        let got = new_vod_reader_read_all(&rec_db, query).await;

        #[rustfmt::skip]
        let want: Vec<u8> = [
            test_mp4(
                [0, 0, 2, 0x90],
                [0, 0xa9, 0x8a, 0xc7],
                [0, 0, 2, 0x1c],
                [0, 0, 1, 0xb8],
                [0x3b, 0x9a, 0xc9, 0xff],
                [0, 0, 1, 0x63],
                [0, 0, 1, 0x23],
            ).as_slice(),
            &[
                0, 0, 0, 0x28, b's', b't', b't', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 3, // Entry count.
                0, 0, 0, 1, // Entry1 sample count.
                0, 1, 0x5f, 0x94, // Entry1 sample delta.
                0, 0, 0, 2, // Entry2 sample count.
                0, 0, 0, 1, // Entry2 sample delta.
                0, 0, 0, 1, // Entry3 sample count.
                0x3b, 0x99, 0x6a, 0x69, // Entry3 sample delta.
                0, 0, 0, 0x18, b's', b't', b's', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 2, // Entry count.
                0, 0, 0, 1, // Entry1.
                0, 0, 0, 3, // Entry2.
                0, 0, 0, 0x18, b'c', b't', b't', b's',
                1, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 4, // Entry1 sample count.
                0, 0, 0, 0, // Entry1 sample offset
                0, 0, 0, 0x1c, b's', b't', b's', b'c',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 first chunk.
                0, 0, 0, 4, // Entry1 samples per chunk.
                0, 0, 0, 1, // Entry1 sample description index.
                0, 0, 0, 0x24, b's', b't', b's', b'z',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 0, // Sample size.
                0, 0, 0, 4, // Sample count.
                0, 0, 0, 1, // Entry1 size.
                0, 0, 0, 1, // Entry2 size.
                0, 0, 0, 1, // Entry3 size.
                0, 0, 0, 1, // Entry4 size.
                0, 0, 0, 0x14, b's', b't', b'c', b'o',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 2, 0xac, // Chunk offset1.
                //
                0, 0, 0, 0x0c, b'm', b'd', b'a', b't',
                1, 2, 3, 4 // Data.
            ]
        ].into_iter().flatten().copied().collect();
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }

    #[tokio::test]
    async fn test_vod_simple_end() {
        let year_2000: UnixH264 = UnixNano::new(946_684_800 * SECOND).into();
        let start_time: UnixH264 = year_2000 + UnixNano::new(10 * MINUTE).into();

        let (_tmp_dir, rec_db) = single_recording(start_time).await;

        let query = VodQuery {
            monitor_id: "x".to_owned().try_into().unwrap(),
            start: start_time.into(),
            end: UnixNano::from(start_time + UnixH264::new(6)) + UnixNano::new(1), // Third sample.
            cache_id: 0,
        };
        let got = new_vod_reader_read_all(&rec_db, query).await;

        #[rustfmt::skip]
        let want: Vec<u8> = [
            test_mp4(
                [0, 0, 2, 0x84],
                [0, 0, 0, 0],
                [0, 0, 2, 0x10],
                [0, 0, 1, 0xac],
                [0, 0, 0, 6],
                [0, 0, 1, 0x57],
                [0, 0, 1, 0x17],
            ).as_slice(),
            &[
                0, 0, 0, 0x20, b's', b't', b't', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 2, // Entry count.
                0, 0, 0, 1, // Entry1 sample count.
                0, 0, 0, 4, // Entry1 sample delta.
                0, 0, 0, 2, // Entry2 sample count.
                0, 0, 0, 1, // Entry2 sample delta.
                0, 0, 0, 0x18, b's', b't', b's', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 2, // Entry count.
                0, 0, 0, 1, // Entry1.
                0, 0, 0, 3, // Entry2.
                0, 0, 0, 0x18, b'c', b't', b't', b's',
                1, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 3, // Entry1 sample count.
                0, 0, 0, 0, // Entry1 sample offset
                0, 0, 0, 0x1c, b's', b't', b's', b'c',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 first chunk.
                0, 0, 0, 3, // Entry1 samples per chunk.
                0, 0, 0, 1, // Entry1 sample description index.
                0, 0, 0, 0x20, b's', b't', b's', b'z',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 0, // Sample size.
                0, 0, 0, 3, // Sample count.
                0, 0, 0, 1, // Entry1 size.
                0, 0, 0, 1, // Entry2 size.
                0, 0, 0, 1, // Entry3 size.
                0, 0, 0, 0x14, b's', b't', b'c', b'o',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 2, 0xa0, // Chunk offset1.
                //
                0, 0, 0, 0x0b, b'm', b'd', b'a', b't',
                1, 2, 3 // Data.
            ]
        ].into_iter().flatten().copied().collect();
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }

    #[tokio::test]
    async fn test_vod_negative_duration() {
        let year_2000: UnixH264 = UnixNano::new(946_684_800 * SECOND).into();
        let start_time: UnixH264 = year_2000 + UnixNano::new(10 * MINUTE).into();

        let (_tmp_dir, rec_db) = single_recording(start_time).await;

        let query = VodQuery {
            monitor_id: "x".to_owned().try_into().unwrap(),
            start: (start_time + UnixH264::new(20)).into(),
            end: start_time.into(),
            cache_id: 0,
        };
        VodReader::new(&rec_db, &VodCache::new(), query)
            .await
            .unwrap_err();
    }

    #[tokio::test]
    async fn test_vod_max_duration() {
        let year_2000: UnixH264 = UnixNano::new(946_684_800 * SECOND).into();
        let start_time: UnixH264 = year_2000 + UnixNano::new(10 * MINUTE).into();

        let (_tmp_dir, rec_db) = single_recording(start_time).await;

        let query = VodQuery {
            monitor_id: "x".to_owned().try_into().unwrap(),
            start: start_time.into(),
            end: UnixNano::from(start_time) + UnixNano::new(HOUR * 13),
            cache_id: 0,
        };
        let result = VodReader::new(&rec_db, &VodCache::new(), query).await;
        assert!(matches!(result, Err(CreateVodReaderError::MaxDuration)));
    }

    async fn new_test_recdb(path: &Path) -> RecDb {
        RecDb::new(DummyLogger::new(), path.to_path_buf(), DummyStorage::new()).await
    }

    async fn single_recording(start_time: UnixH264) -> (TempDir, RecDb) {
        let temp_dir = TempDir::new().unwrap();
        let mut rec_db = new_test_recdb(temp_dir.path()).await;

        save_recording(
            &mut rec_db,
            start_time,
            start_time + UnixH264::new(7),
            vec![
                VideoSample {
                    pts: start_time + UnixH264::new(3),
                    dts_offset: DtsOffset::new(0),
                    avcc: Arc::new(PaddedBytes::new(vec![0x1])),
                    random_access_present: true,
                    duration: DurationH264::new(1),
                },
                VideoSample {
                    pts: start_time + UnixH264::new(4),
                    avcc: Arc::new(PaddedBytes::new(vec![0x2])),
                    duration: DurationH264::new(1),
                    ..Default::default()
                },
                VideoSample {
                    pts: start_time + UnixH264::new(5),
                    dts_offset: DtsOffset::new(0),
                    avcc: Arc::new(PaddedBytes::new(vec![0x3])),
                    random_access_present: true,
                    duration: DurationH264::new(1),
                },
                VideoSample {
                    pts: start_time + UnixH264::new(6),
                    avcc: Arc::new(PaddedBytes::new(vec![0x4])),
                    duration: DurationH264::new(1),
                    ..Default::default()
                },
            ],
        )
        .await;
        (temp_dir, rec_db)
    }

    #[tokio::test]
    async fn test_vod_two() {
        let year_2000: UnixH264 = UnixNano::new(946_684_800 * SECOND).into();
        let start_time = year_2000 + UnixNano::new(10 * MINUTE).into() + UnixH264::new(89998);

        let (_tmp_dir, rec_db) = two_recordings(start_time).await;

        let query = VodQuery {
            monitor_id: "x".to_owned().try_into().unwrap(),
            start: start_time.into(),
            end: (start_time + UnixH264::new(16)).into(),
            cache_id: 0,
        };
        let got = new_vod_reader_read_all(&rec_db, query).await;

        #[rustfmt::skip]
        let want: Vec<u8> = [
            test_mp4(
                [0, 0, 2, 0x90],
                [0, 0, 0, 0],
                [0, 0, 2, 0x1c],
                [0, 0, 1, 0xb8],
                [0, 0, 0, 0x10],
                [0, 0, 1, 0x63],
                [0, 0, 1, 0x23],
            ).as_slice(),
            &[
                0, 0, 0, 0x28, b's', b't', b't', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 3, // Entry count.
                0, 0, 0, 1, // Entry1 sample count.
                0, 0, 0, 2, // Entry1 sample delta.
                0, 0, 0, 2, // Entry2 sample count.
                0, 0, 0, 1, // Entry2 sample delta.
                0, 0, 0, 1, // Entry3 sample count.
                0, 0, 0, 0xc, // Entry3 sample delta.
                0, 0, 0, 0x18, b's', b't', b's', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 2, // Entry count.
                0, 0, 0, 1, // Entry1.
                0, 0, 0, 3, // Entry2.
                0, 0, 0, 0x18, b'c', b't', b't', b's',
                1, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 4, // Entry1 sample count.
                0, 0, 0, 0, // Entry1 sample offset
                0, 0, 0, 0x1c, b's', b't', b's', b'c',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 first chunk.
                0, 0, 0, 4, // Entry1 samples per chunk.
                0, 0, 0, 1, // Entry1 sample description index.
                0, 0, 0, 0x24, b's', b't', b's', b'z',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 0, // Sample size.
                0, 0, 0, 4, // Sample count.
                0, 0, 0, 1, // Entry1 size.
                0, 0, 0, 1, // Entry2 size.
                0, 0, 0, 1, // Entry3 size.
                0, 0, 0, 1, // Entry4 size.
                0, 0, 0, 0x14, b's', b't', b'c', b'o',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 2, 0xac, // Chunk offset1.
                //
                0, 0, 0, 0x0c, b'm', b'd', b'a', b't',
                1, 2, 3, 4 // Data.
            ]
        ].into_iter().flatten().copied().collect();
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }

    async fn two_recordings(start_time: UnixH264) -> (TempDir, RecDb) {
        let temp_dir = TempDir::new().unwrap();
        let mut rec_db = new_test_recdb(temp_dir.path()).await;

        save_recording(
            &mut rec_db,
            start_time,
            start_time + UnixH264::new(2),
            vec![
                VideoSample {
                    pts: start_time,
                    dts_offset: DtsOffset::new(0),
                    avcc: Arc::new(PaddedBytes::new(vec![0x1])),
                    random_access_present: true,
                    duration: DurationH264::new(1),
                },
                VideoSample {
                    pts: start_time + UnixH264::new(1),
                    avcc: Arc::new(PaddedBytes::new(vec![0x2])),
                    duration: DurationH264::new(1),
                    ..Default::default()
                },
            ],
        )
        .await;
        save_recording(
            &mut rec_db,
            start_time + UnixH264::new(2),
            start_time + UnixH264::new(4),
            vec![
                VideoSample {
                    pts: start_time + UnixH264::new(2),
                    dts_offset: DtsOffset::new(0),
                    avcc: Arc::new(PaddedBytes::new(vec![0x3])),
                    random_access_present: true,
                    duration: DurationH264::new(1),
                },
                VideoSample {
                    pts: start_time + UnixH264::new(3),
                    avcc: Arc::new(PaddedBytes::new(vec![0x4])),
                    duration: DurationH264::new(1),
                    ..Default::default()
                },
            ],
        )
        .await;
        (temp_dir, rec_db)
    }

    #[tokio::test]
    #[allow(clippy::too_many_lines)]
    async fn test_vod_gap() {
        let year_2000: UnixH264 = UnixNano::new(946_684_800 * SECOND).into();
        let start_time = year_2000 + UnixNano::new(10 * MINUTE).into() + UnixH264::new(89998);

        let temp_dir = TempDir::new().unwrap();
        let mut rec_db = new_test_recdb(temp_dir.path()).await;

        save_recording(
            &mut rec_db,
            start_time,
            start_time + UnixH264::new(2),
            vec![
                VideoSample {
                    pts: start_time,
                    dts_offset: DtsOffset::new(0),
                    avcc: Arc::new(PaddedBytes::new(vec![0x1])),
                    random_access_present: true,
                    duration: DurationH264::new(1),
                },
                VideoSample {
                    pts: start_time + UnixH264::new(1),
                    avcc: Arc::new(PaddedBytes::new(vec![0x2])),
                    duration: DurationH264::new(1),
                    ..Default::default()
                },
            ],
        )
        .await;
        save_recording(
            &mut rec_db,
            start_time + UnixH264::new(12),
            start_time + UnixH264::new(14),
            vec![
                VideoSample {
                    pts: start_time + UnixH264::new(12),
                    dts_offset: DtsOffset::new(0),
                    avcc: Arc::new(PaddedBytes::new(vec![0x3])),
                    random_access_present: true,
                    duration: DurationH264::new(1),
                },
                VideoSample {
                    pts: start_time + UnixH264::new(13),
                    avcc: Arc::new(PaddedBytes::new(vec![0x4])),
                    duration: DurationH264::new(1),
                    ..Default::default()
                },
            ],
        )
        .await;

        let query = VodQuery {
            monitor_id: "x".to_owned().try_into().unwrap(),
            start: start_time.into(),
            end: (start_time + UnixH264::new(16)).into(),
            cache_id: 0,
        };
        let got = new_vod_reader_read_all(&rec_db, query).await;

        #[rustfmt::skip]
        let want: Vec<u8> = [
            test_mp4(
                [0, 0, 2, 0x98],
                [0, 0, 0, 0],
                [0, 0, 2, 0x24],
                [0, 0, 1, 0xc0],
                [0, 0, 0, 0x10],
                [0, 0, 1, 0x6b],
                [0, 0, 1, 0x2b],
            ).as_slice(),
            &[
                0, 0, 0, 0x30, b's', b't', b't', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 4, // Entry count.
                0, 0, 0, 1, // Entry1 sample count.
                0, 0, 0, 2, // Entry1 sample delta.
                0, 0, 0, 1, // Entry2 sample count.
                0, 0, 0, 0xb, // Entry2 sample delta.
                0, 0, 0, 1, // Entry3 sample count.
                0, 0, 0, 1, // Entry3 sample delta.
                0, 0, 0, 1, // Entry4 sample count.
                0, 0, 0, 2, // Entry4 sample delta.
                0, 0, 0, 0x18, b's', b't', b's', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 2, // Entry count.
                0, 0, 0, 1, // Entry1.
                0, 0, 0, 3, // Entry2.
                0, 0, 0, 0x18, b'c', b't', b't', b's',
                1, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 4, // Entry1 sample count.
                0, 0, 0, 0, // Entry1 sample offset
                0, 0, 0, 0x1c, b's', b't', b's', b'c',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 first chunk.
                0, 0, 0, 4, // Entry1 samples per chunk.
                0, 0, 0, 1, // Entry1 sample description index.
                0, 0, 0, 0x24, b's', b't', b's', b'z',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 0, // Sample size.
                0, 0, 0, 4, // Sample count.
                0, 0, 0, 1, // Entry1 size.
                0, 0, 0, 1, // Entry2 size.
                0, 0, 0, 1, // Entry3 size.
                0, 0, 0, 1, // Entry4 size.
                0, 0, 0, 0x14, b's', b't', b'c', b'o',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 2, 0xb4, // Chunk offset1.
                //
                0, 0, 0, 0x0c, b'm', b'd', b'a', b't',
                1, 2, 3, 4 // Data.
            ]
        ].into_iter().flatten().copied().collect();
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }

    #[tokio::test]
    async fn test_vod_multiple() {
        let year_2000: UnixH264 = UnixNano::new(946_684_800 * SECOND).into();
        let start_time: UnixH264 = year_2000 + UnixNano::new(10 * MINUTE).into();

        let (_tmp_dir, rec_db) = multiple_recordings(start_time).await;
        let query = VodQuery {
            monitor_id: "x".to_owned().try_into().unwrap(),
            start: (start_time + UnixNano::new(SECOND * 10).into()).into(),
            end: UnixNano::from(start_time + UnixNano::new(SECOND * 10).into() + UnixH264::new(1))
                + UnixNano::new(1),
            cache_id: 0,
        };
        let got = new_vod_reader_read_all(&rec_db, query).await;

        #[rustfmt::skip]
        let want: Vec<u8> = [
            test_mp4(
                [0, 0, 2, 0x70],
                [0, 0, 0, 0],
                [0, 0, 1, 0xfc],
                [0, 0, 1, 0x98],
                [0, 0, 0, 1],
                [0, 0, 1, 0x43],
                [0, 0, 1, 3],
            ).as_slice(),
            &[
                0, 0, 0, 0x18, b's', b't', b't', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 sample count.
                0, 0, 0, 1, // Entry1 sample delta.
                0, 0, 0, 0x14, b's', b't', b's', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1.
                0, 0, 0, 0x18, b'c', b't', b't', b's',
                1, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 sample count.
                0, 0, 0, 0, // Entry1 sample offset
                0, 0, 0, 0x1c, b's', b't', b's', b'c',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 first chunk.
                0, 0, 0, 1, // Entry1 samples per chunk.
                0, 0, 0, 1, // Entry1 sample description index.
                0, 0, 0, 0x18, b's', b't', b's', b'z',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 0, // Sample size.
                0, 0, 0, 1, // Sample count.
                0, 0, 0, 1, // Entry1 size.
                0, 0, 0, 0x14, b's', b't', b'c', b'o',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 2, 0x8c, // Chunk offset1.
                //
                0, 0, 0, 9, b'm', b'd', b'a', b't',
                2, // Data.
            ]
        ].into_iter().flatten().copied().collect();
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }

    #[tokio::test]
    async fn test_vod_multiple2() {
        let year_2000: UnixH264 = UnixNano::new(946_684_800 * SECOND).into();
        let start_time: UnixH264 = year_2000 + UnixNano::new(10 * MINUTE).into();

        let (_tmp_dir, rec_db) = multiple_recordings(start_time).await;
        let query = VodQuery {
            monitor_id: "x".to_owned().try_into().unwrap(),
            start: (start_time + UnixNano::new(SECOND * 9).into()).into(),
            end: (start_time + UnixNano::new(SECOND * 11).into()).into(),
            cache_id: 0,
        };
        let got = new_vod_reader_read_all(&rec_db, query).await;

        #[rustfmt::skip]
        let want: Vec<u8> = [
            test_mp4(
                [0, 0, 2, 0x70],
                [0, 0, 7, 0xd0],
                [0, 0, 1, 0xfc],
                [0, 0, 1, 0x98],
                [0, 2, 0xbf, 0x20],
                [0, 0, 1, 0x43],
                [0, 0, 1, 3],
            ).as_slice(),
            &[
                0, 0, 0, 0x18, b's', b't', b't', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 sample count.
                0, 2, 0xbf, 0x20, // Entry1 sample delta.
                0, 0, 0, 0x14, b's', b't', b's', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1.
                0, 0, 0, 0x18, b'c', b't', b't', b's',
                1, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 sample count.
                0, 0, 0, 0, // Entry1 sample offset
                0, 0, 0, 0x1c, b's', b't', b's', b'c',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 first chunk.
                0, 0, 0, 1, // Entry1 samples per chunk.
                0, 0, 0, 1, // Entry1 sample description index.
                0, 0, 0, 0x18, b's', b't', b's', b'z',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 0, // Sample size.
                0, 0, 0, 1, // Sample count.
                0, 0, 0, 1, // Entry1 size.
                0, 0, 0, 0x14, b's', b't', b'c', b'o',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 2, 0x8c, // Chunk offset1.
                //
                0, 0, 0, 9, b'm', b'd', b'a', b't',
                2, // Data.
            ]
        ].into_iter().flatten().copied().collect();
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }

    #[tokio::test]
    async fn test_vod_multiple3() {
        let year_2000: UnixH264 = UnixNano::new(946_684_800 * SECOND).into();
        let start_time: UnixH264 = year_2000 + UnixNano::new(10 * MINUTE).into();

        let (_tmp_dir, rec_db) = multiple_recordings(start_time).await;
        let query = VodQuery {
            monitor_id: "x".to_owned().try_into().unwrap(),
            start: (start_time + UnixNano::new(SECOND * 8).into()).into(),
            end: (start_time + UnixNano::new(SECOND * 12).into()).into(),
            cache_id: 0,
        };
        let got = new_vod_reader_read_all(&rec_db, query).await;

        #[rustfmt::skip]
        let want: Vec<u8> = [
            test_mp4(
                [0, 0, 2, 0x70],
                [0, 0, 0xf, 0xa0],
                [0, 0, 1, 0xfc],
                [0, 0, 1, 0x98],
                [0, 5, 0x7e, 0x40],
                [0, 0, 1, 0x43],
                [0, 0, 1, 3],
            ).as_slice(),
            &[
                0, 0, 0, 0x18, b's', b't', b't', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 sample count.
                0, 5, 0x7e, 0x40, // Entry1 sample delta.
                0, 0, 0, 0x14, b's', b't', b's', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1.
                0, 0, 0, 0x18, b'c', b't', b't', b's',
                1, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 sample count.
                0, 0, 0, 0, // Entry1 sample offset
                0, 0, 0, 0x1c, b's', b't', b's', b'c',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 first chunk.
                0, 0, 0, 1, // Entry1 samples per chunk.
                0, 0, 0, 1, // Entry1 sample description index.
                0, 0, 0, 0x18, b's', b't', b's', b'z',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 0, // Sample size.
                0, 0, 0, 1, // Sample count.
                0, 0, 0, 1, // Entry1 size.
                0, 0, 0, 0x14, b's', b't', b'c', b'o',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 2, 0x8c, // Chunk offset1.
                //
                0, 0, 0, 9, b'm', b'd', b'a', b't',
                2, // Data.
            ]
        ].into_iter().flatten().copied().collect();
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }

    async fn multiple_recordings(start_time: UnixH264) -> (TempDir, RecDb) {
        let temp_dir = TempDir::new().unwrap();
        let mut rec_db = new_test_recdb(temp_dir.path()).await;

        let rec1 = start_time;
        save_recording(
            &mut rec_db,
            rec1,
            rec1 + UnixH264::new(1),
            vec![VideoSample {
                pts: rec1,
                dts_offset: DtsOffset::new(0),
                avcc: Arc::new(PaddedBytes::new(vec![0x1])),
                random_access_present: true,
                duration: DurationH264::new(1),
            }],
        )
        .await;
        let rec2 = start_time + UnixNano::new(SECOND * 10).into();
        println!("rec2 {}", UnixNano::from(rec2));
        save_recording(
            &mut rec_db,
            rec2,
            rec2 + UnixH264::new(1),
            vec![VideoSample {
                pts: rec2,
                dts_offset: DtsOffset::new(0),
                avcc: Arc::new(PaddedBytes::new(vec![0x2])),
                random_access_present: true,
                duration: DurationH264::new(1),
            }],
        )
        .await;
        let rec3 = start_time + UnixNano::new(SECOND * 20).into();
        save_recording(
            &mut rec_db,
            rec3,
            rec3 + UnixH264::new(1),
            vec![VideoSample {
                pts: rec3,
                dts_offset: DtsOffset::new(0),
                avcc: Arc::new(PaddedBytes::new(vec![0x3])),
                random_access_present: true,
                duration: DurationH264::new(1),
            }],
        )
        .await;
        (temp_dir, rec_db)
    }

    #[tokio::test]
    async fn test_shift_start() {
        let year_2000: UnixH264 = UnixNano::new(946_684_800 * SECOND).into();
        let start_time = year_2000 + UnixNano::new(10 * MINUTE).into();

        let temp_dir = TempDir::new().unwrap();
        let mut rec_db = new_test_recdb(temp_dir.path()).await;

        save_recording(
            &mut rec_db,
            start_time + UnixH264::new(10),
            start_time + UnixH264::new(12),
            vec![
                VideoSample {
                    pts: start_time + UnixH264::new(10),
                    dts_offset: DtsOffset::new(0),
                    avcc: Arc::new(PaddedBytes::new(vec![0x1])),
                    random_access_present: true,
                    duration: DurationH264::new(1),
                },
                VideoSample {
                    pts: start_time + UnixH264::new(11),
                    avcc: Arc::new(PaddedBytes::new(vec![0x2])),
                    duration: DurationH264::new(1),
                    ..Default::default()
                },
            ],
        )
        .await;

        let query = VodQuery {
            monitor_id: "x".to_owned().try_into().unwrap(),
            start: start_time.into(),
            end: UnixNano::from(start_time + UnixH264::new(12)) + UnixNano::new(1),
            cache_id: 0,
        };
        let got = new_vod_reader_read_all(&rec_db, query).await;

        #[rustfmt::skip]
        let want: Vec<u8> = [
            test_mp4(
                [0, 0, 2, 0x7c],
                [0, 0, 0, 0],
                [0, 0, 2, 8],
                [0, 0, 1, 0xa4],
                [0, 0, 0, 0x0c],
                [0, 0, 1, 0x4f],
                [0, 0, 1, 0x0f],
            ).as_slice(),
            &[
                0, 0, 0, 0x20, b's', b't', b't', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 2, // Entry count.
                0, 0, 0, 1, // Entry1 sample count.
                0, 0, 0, 0xb, // Entry1 sample delta.
                0, 0, 0, 1, // Entry2 sample count.
                0, 0, 0, 1, // Entry2 sample delta.
                0, 0, 0, 0x14, b's', b't', b's', b's',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1.
                0, 0, 0, 0x18, b'c', b't', b't', b's',
                1, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 2, // Entry1 sample count.
                0, 0, 0, 0, // Entry1 sample offset
                0, 0, 0, 0x1c, b's', b't', b's', b'c',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 0, 1, // Entry1 first chunk.
                0, 0, 0, 2, // Entry1 samples per chunk.
                0, 0, 0, 1, // Entry1 sample description index.
                0, 0, 0, 0x1c, b's', b't', b's', b'z',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 0, // Sample size.
                0, 0, 0, 2, // Sample count.
                0, 0, 0, 1, // Entry1 size.
                0, 0, 0, 1, // Entry2 size.
                0, 0, 0, 0x14, b's', b't', b'c', b'o',
                0, 0, 0, 0, // FullBox.
                0, 0, 0, 1, // Entry count.
                0, 0, 2, 0x98, // Chunk offset1.
                //
                0, 0, 0, 0x0a, b'm', b'd', b'a', b't',
                1, 2,  // Data.
            ]
        ].into_iter().flatten().copied().collect();
        assert_eq!(pretty_hex(&want), pretty_hex(&got));
    }

    async fn save_recording(
        rec_db: &mut RecDb,
        start_time: UnixH264,
        end_time: UnixH264,
        samples: Vec<VideoSample>,
    ) {
        let rec = rec_db
            .new_recording("x".to_owned().try_into().unwrap(), start_time)
            .await
            .unwrap();

        let mut meta = rec.new_file("meta").await.unwrap();
        let mut mdat = rec.new_file("mdat").await.unwrap();
        let header = MetaHeader {
            start_time,
            width: 640,
            height: 480,
            extra_data: vec![0x33],
        };

        let mut w = VideoWriter::new(&mut *meta, &mut *mdat, header)
            .await
            .unwrap();
        for sample in samples {
            w.write_sample(&sample).await.unwrap();
        }

        let data = RecordingData {
            start: start_time.into(),
            end: end_time.into(),
            events: Vec::new(),
        };

        let json = serde_json::to_vec_pretty(&data).unwrap();

        let mut data_file = rec.new_file("json").await.unwrap();
        data_file.write_all(&json).await.unwrap();
        data_file.flush().await.unwrap();
    }

    #[rustfmt::skip]
    fn test_mp4(
        moov_size: [u8; 4],
        duration: [u8; 4],
        track_size: [u8; 4],
        mdia_size: [u8; 4],
        duration2: [u8; 4],
        minf_size: [u8; 4],
        stbl_size: [u8; 4],
    ) -> Vec<u8> {
        [&[
            0, 0, 0, 0x14, b'f', b't', b'y', b'p',
            b'i', b's', b'o', b'4',
            0, 0, 2, 0, // Minor version.
            b'i', b's', b'o', b'4',
            //
        ], &moov_size[..], &[ b'm', b'o', b'o', b'v',
            0, 0, 0, 0x6c, b'm', b'v', b'h', b'd',
            0, 0, 0, 0, // Fullbox.
            0, 0, 0, 0, // Creation time.
            0, 0, 0, 0, // Modification time.
            0, 0, 3, 0xe8, // Timescale.

        ], &duration[..], &[
            0, 1, 0, 0, // Rate.
            1, 0, // Volume.
            0, 0, // Reserved.
            0, 0, 0, 0, 0, 0, 0, 0, // Reserved2.
            0, 1, 0, 0, 0, 0, 0, 0, 0, // Matrix.
            0, 0, 0, 0, 0, 0, 0, 0, 1,
            0, 0, 0, 0, 0, 0, 0, 0, 0,
            0, 0, 0, 0, 0, 0x40, 0, 0, 0,
            0, 0, 0, 0, 0, 0, // Pre-defined.
            0, 0, 0, 0, 0, 0,
            0, 0, 0, 0, 0, 0,
            0, 0, 0, 0, 0, 0,
            0, 0, 0, 2, // Next track ID.
            //
            /* Video trak */
            ], &track_size[..], &[b't', b'r', b'a', b'k',
            0, 0, 0, 0x5c, b't', b'k', b'h', b'd',
            0, 0, 0, 3, // Fullbox.
            0, 0, 0, 0, // Creation time.
            0, 0, 0, 0, // Modification time.
            0, 0, 0, 1, // Track ID.
            0, 0, 0, 0, // Reserved0.
            ], &duration[..], &[
            0, 0, 0, 0, 0, 0, 0, 0, // Reserved1.
            0, 0, // Layer.
            0, 0, // Alternate group.
            0, 0, // Volume.
            0, 0, // Reserved2.
            0, 1, 0, 0, 0, 0, 0, 0, 0, // Matrix.
            0, 0, 0, 0, 0, 0, 0, 0, 1,
            0, 0, 0, 0, 0, 0, 0, 0, 0,
            0, 0, 0, 0, 0, 0x40, 0, 0, 0,
            2, 0x80, 0, 0, // Width.
            1, 0xe0, 0, 0, // Height.
            ], &mdia_size[..], &[b'm', b'd', b'i', b'a',
            0, 0, 0, 0x20, b'm', b'd', b'h', b'd',
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 0, // Creation time.
            0, 0, 0, 0, // Modification time.
            0, 1, 0x5f, 0x90, // Time scale.
            ], &duration2[..], &[
            0x55, 0xc4, // Language.
            0, 0, // Predefined.
            0, 0, 0, 0x2d, b'h', b'd', b'l', b'r',
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 0, // Predefined.
            b'v', b'i', b'd', b'e', // Handler type.
            0, 0, 0, 0, // Reserved.
            0, 0, 0, 0,
            0, 0, 0, 0,
            b'V', b'i', b'd', b'e', b'o', b'H', b'a', b'n', b'd', b'l', b'e', b'r', 0,
            ], &minf_size[..], &[b'm', b'i', b'n', b'f',
            0, 0, 0, 0x14, b'v', b'm', b'h', b'd',
            0, 0, 0, 0, // FullBox.
            0, 0, // Graphics mode.
            0, 0, 0, 0, 0, 0, // OpColor.
            0, 0, 0, 0x24, b'd', b'i', b'n', b'f',
            0, 0, 0, 0x1c, b'd', b'r', b'e', b'f',
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 1, // Entry count.
            0, 0, 0, 0xc, b'u', b'r', b'l', b' ',
            0, 0, 0, 1, // FullBox.
            ], &stbl_size[..], &[b's', b't', b'b', b'l',
            0, 0, 0, 0x6f, b's', b't', b's', b'd',
            0, 0, 0, 0, // FullBox.
            0, 0, 0, 1, // Entry count.
            0, 0, 0, 0x5f, b'a', b'v', b'c', b'1',
            0, 0, 0, 0, 0, 0, // Reserved.
            0, 1, // Data reference index.
            0, 0, // Predefined.
            0, 0, // Reserved.
            0, 0, 0, 0, // Predefined2.
            0, 0, 0, 0,
            0, 0, 0, 0,
            2, 0x80, // Width.
            1, 0xe0, // Height.
            0, 0x48, 0, 0, // Horizresolution
            0, 0x48, 0, 0, // Vertresolution
            0, 0, 0, 0, // Reserved2.
            0, 1, // Frame count.
            0, 0, 0, 0, 0, 0, 0, 0, // Compressor name.
            0, 0, 0, 0, 0, 0, 0, 0,
            0, 0, 0, 0, 0, 0, 0, 0,
            0, 0, 0, 0, 0, 0, 0, 0,
            0, 0x18, // Depth.
            0xff, 0xff, // Predefined3.
            0, 0, 0, 0x09, b'a', b'v', b'c', b'C',
            0x33,    // Extradata.
        ]].into_iter().flatten().copied().collect()
    }

    async fn new_vod_reader_read_all(rec_db: &RecDb, query: VodQuery) -> Vec<u8> {
        let mut out = Vec::new();
        let mut reader = VodReader::new(rec_db, &VodCache::new(), query)
            .await
            .unwrap()
            .unwrap();
        reader.read_to_end(&mut out).await.unwrap();
        assert_eq!(out.len() as u64, reader.size());
        out
    }
}
