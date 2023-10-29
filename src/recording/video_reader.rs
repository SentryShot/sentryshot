// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{
    mp4_muxer::{generate_mp4, GenerateMp4Error},
    video::{MetaReader, NewMetaReaderError, ReadAllSamplesError},
};
use pin_project::pin_project;
use std::{
    collections::HashMap,
    io::{self, SeekFrom},
    path::{Path, PathBuf},
    pin::Pin,
    sync::Arc,
    task::{Context, Poll},
};
use thiserror::Error;
use tokio::{
    io::{AsyncRead, AsyncSeek, ReadBuf},
    sync::Mutex,
};

#[pin_project]
pub struct VideoReader<RS1, RS2>
where
    RS1: AsyncRead + AsyncSeek,
    RS2: AsyncRead + AsyncSeek,
{
    #[pin]
    meta: RS1,

    #[pin]
    mdat: RS2,

    meta_size: u64,
    mdat_size: u64,
    pos: u64, // current reading index

    last_modified: std::time::SystemTime,
    read_state: ReadState,
}

impl<RS1, RS2> VideoReader<RS1, RS2>
where
    RS1: AsyncRead + AsyncSeek,
    RS2: AsyncRead + AsyncSeek,
{
    pub fn last_modified(&self) -> std::time::SystemTime {
        self.last_modified
    }

    pub fn size(&self) -> u64 {
        self.meta_size + self.mdat_size
    }
}

#[derive(Debug, Error)]
pub enum NewVideoReaderError {
    #[error("read video metadata: {0}")]
    ReadVideoMetadat(#[from] ReadVideoMetadataError),

    #[error("open mdat: {0}")]
    OpenMdat(#[from] std::io::Error),

    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),
}

pub async fn new_video_reader(
    recording_path: PathBuf,
    cache: &Option<Arc<Mutex<VideoCache>>>,
) -> Result<VideoReader<MetaCursor, tokio::fs::File>, NewVideoReaderError> {
    use NewVideoReaderError::*;
    let mut meta_path = recording_path.to_owned();
    meta_path.set_extension("meta");

    let mut mdat_path = recording_path.to_owned();
    mdat_path.set_extension("mdat");

    let meta = {
        if let Some(cache) = cache {
            let mut cache = cache.lock().await;
            match cache.get(&recording_path) {
                Some(v) => v,
                None => {
                    let meta = Arc::new(read_video_metadata(&meta_path).await?);
                    cache.add(recording_path, meta.clone());
                    meta
                }
            }
        } else {
            Arc::new(read_video_metadata(&meta_path).await?)
        }
    };

    let mdat = tokio::fs::OpenOptions::new()
        .read(true)
        .open(mdat_path)
        .await
        .map_err(OpenMdat)?;

    Ok(VideoReader {
        mdat,
        meta_size: u64::try_from(meta.buf.len())?,
        mdat_size: meta.mdat_size.into(),
        pos: 0,
        last_modified: meta.last_modified,
        meta: new_meta_cursor(meta),
        read_state: ReadState::State1,
    })
}

impl<RS1, RS2> AsyncRead for VideoReader<RS1, RS2>
where
    RS1: AsyncRead + AsyncSeek,
    RS2: AsyncRead + AsyncSeek,
{
    fn poll_read(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &mut ReadBuf<'_>,
    ) -> Poll<io::Result<()>> {
        // Read starts within mdat, read mdat.
        let state7 = |mut this: Pin<&mut Self>, cx: &mut Context, buf: &mut ReadBuf| {
            let prev_buf_len = buf.filled().len();
            match this.as_mut().project().mdat.poll_read(cx, buf) {
                Poll::Ready(res) => {
                    res?;
                    let n = buf.filled().len() - prev_buf_len;
                    *this.as_mut().project().pos += u64::try_from(n).unwrap();
                    *this.as_mut().project().read_state = ReadState::State1;
                    Poll::Ready(Ok(()))
                }
                Poll::Pending => Poll::Pending,
            }
        };

        // Read starts within mdat, seek mdat.
        let state6 = |mut this: Pin<&mut Self>, cx: &mut Context, buf: &mut ReadBuf| {
            match this.as_mut().project().mdat.poll_complete(cx) {
                Poll::Ready(res) => {
                    res?;
                    // Read starts within mdat, read mdat.
                    *this.as_mut().project().read_state = ReadState::State7;
                    state7(this, cx, buf)
                }
                Poll::Pending => Poll::Pending,
            }
        };

        // Continue across border.
        let state5 = |mut this: Pin<&mut Self>, cx: &mut Context, buf: &mut ReadBuf| {
            let prev_buf_len = buf.filled().len();
            match this.as_mut().project().mdat.poll_read(cx, buf) {
                Poll::Ready(res) => {
                    res?;

                    let n = buf.filled().len() - prev_buf_len;
                    *this.as_mut().project().pos += u64::try_from(n).unwrap();
                    *this.as_mut().project().read_state = ReadState::State1;
                    Poll::Ready(Ok(()))
                }
                Poll::Pending => Poll::Pending,
            }
        };

        // Read within meta and continue across border.
        let state4 = |mut this: Pin<&mut Self>, cx: &mut Context, buf: &mut ReadBuf| {
            let prev_buf_len = buf.filled().len();
            match this.as_mut().project().meta.poll_read(cx, buf) {
                Poll::Ready(res) => {
                    res?;
                    let n = buf.filled().len() - prev_buf_len;
                    *this.as_mut().project().pos += u64::try_from(n).unwrap();
                    // Poll ready and continue across border.
                    *this.as_mut().project().read_state = ReadState::State5;
                    Poll::Ready(Ok(()))
                }
                Poll::Pending => Poll::Pending,
            }
        };

        // Only read within meta.
        let state3 = |mut this: Pin<&mut Self>, cx: &mut Context, buf: &mut ReadBuf| {
            let prev_buf_len = buf.filled().len();
            match this.as_mut().project().meta.poll_read(cx, buf) {
                Poll::Ready(res) => {
                    res?;
                    let n = buf.filled().len() - prev_buf_len;
                    *this.as_mut().project().pos += u64::try_from(n).unwrap();
                    *this.as_mut().project().read_state = ReadState::State1;
                    Poll::Ready(Ok(()))
                }
                Poll::Pending => Poll::Pending,
            }
        };

        let state2 = |mut this: Pin<&mut Self>, cx: &mut Context, buf: &mut ReadBuf| {
            match this.as_mut().project().meta.poll_complete(cx) {
                Poll::Ready(res) => {
                    res?;
                    let pos = *this.as_mut().project().pos;
                    let meta_size = *this.as_mut().project().meta_size;
                    if u64::try_from(buf.remaining()).unwrap() <= meta_size - pos {
                        // Only read within meta.
                        *this.as_mut().project().read_state = ReadState::State3;
                        return state3(this, cx, buf);
                    }

                    // Read within meta and continue across border.
                    *this.as_mut().project().read_state = ReadState::State4;
                    state4(this, cx, buf)
                }
                Poll::Pending => Poll::Pending,
            }
        };

        let state1 = |mut this: Pin<&mut Self>, cx: &mut Context, buf: &mut ReadBuf| {
            let pos = *this.as_mut().project().pos;
            let meta_size = *this.as_mut().project().meta_size;
            let mdat_size = *this.as_mut().project().mdat_size;

            // EOF.
            if pos >= meta_size + mdat_size {
                return Poll::Ready(Ok(()));
            }

            if pos <= meta_size {
                // Read starts within meta.
                this.as_mut()
                    .project()
                    .meta
                    .start_seek(SeekFrom::Start(pos))?;
                *this.as_mut().project().read_state = ReadState::State2;
                state2(this, cx, buf)
            } else {
                // Read starts within mdat.
                let mdat_pos = pos - meta_size;
                this.as_mut()
                    .project()
                    .mdat
                    .start_seek(SeekFrom::Start(mdat_pos))?;
                *this.as_mut().project().read_state = ReadState::State6;
                state6(this, cx, buf)
            }
        };

        match self.as_mut().project().read_state {
            ReadState::State1 => state1(self, cx, buf),
            ReadState::State2 => state2(self, cx, buf),
            ReadState::State3 => state3(self, cx, buf),
            ReadState::State4 => state4(self, cx, buf),
            ReadState::State5 => state5(self, cx, buf),
            ReadState::State6 => state6(self, cx, buf),
            ReadState::State7 => state7(self, cx, buf),
        }
    }
}

enum ReadState {
    State1,
    State2,
    State3,
    State4,
    State5,
    State6,
    State7,
}

// Sync version.
/*impl<RS1: Read + Seek, RS2: Read + Seek> Read for VideoReader<RS1, RS2> {
    fn read(&mut self, buf: &mut [u8]) -> io::Result<usize> {
        // STATE 1.

        // EOF.
        if self.pos >= self.meta_size + self.mdat_size {
            return Ok(0);
        }

        // Read starts within meta.
        if self.pos <= self.meta_size {
            self.meta.seek(SeekFrom::Start(self.pos))?; // STATE 2.

            // Read within meta.
            if u64::try_from(buf.len()).unwrap() <= self.meta_size - self.pos {
                let n = self.meta.read(buf)?; // STATE 3.
                self.pos += u64::try_from(n).unwrap();
                return Ok(n);
            }

            // Read across border.
            let n = self.meta.read(buf)?; // STATE 4.
            let n2 = self.mdat.read(&mut buf[n..])?; // STATE 5.
            self.pos += u64::try_from(n + n2).unwrap();
            return Ok(n + n2);
        }

        // Read within mdat.
        self.mdat.seek(SeekFrom::Start(self.pos - self.meta_size))?; // STATE 6.
        let n = self.mdat.read(buf)?; // STATE 7.
        self.pos += u64::try_from(n).unwrap();

        Ok(n)
    }
}*/

impl<RS1, RS2> AsyncSeek for VideoReader<RS1, RS2>
where
    RS1: AsyncRead + AsyncSeek + Unpin,
    RS2: AsyncRead + AsyncSeek + Unpin,
{
    fn start_seek(mut self: Pin<&mut Self>, pos: SeekFrom) -> io::Result<()> {
        io::Seek::seek(&mut *self, pos).map(drop)
    }

    fn poll_complete(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<io::Result<u64>> {
        Poll::Ready(Ok(self.pos))
    }
}

#[derive(Debug, Error)]
pub enum SeekError {
    #[error("invalid seek to a negative or overflowing position")]
    Invalid,
}

impl<RS1, RS2> std::io::Seek for VideoReader<RS1, RS2>
where
    RS1: AsyncRead + AsyncSeek,
    RS2: AsyncRead + AsyncSeek,
{
    fn seek(&mut self, style: SeekFrom) -> io::Result<u64> {
        let (base_pos, offset) = match style {
            SeekFrom::Start(n) => {
                self.pos = n;
                return Ok(n);
            }
            SeekFrom::End(n) => (self.meta_size + self.mdat_size, n),
            SeekFrom::Current(n) => (self.pos, n),
        };

        let new_pos = if offset >= 0 {
            base_pos.checked_add(offset as u64)
        } else {
            base_pos.checked_sub((offset.wrapping_neg()) as u64)
        };
        match new_pos {
            Some(n) => {
                self.pos = n;
                Ok(self.pos)
            }
            None => Err(std::io::Error::new(
                std::io::ErrorKind::InvalidInput,
                SeekError::Invalid,
            )),
        }
    }
}

#[derive(Debug, Error)]
pub enum ReadVideoMetadataError {
    #[error("metadata: {0}")]
    Metadata(std::io::Error),

    #[error("last modified: {0}")]
    LastModified(std::io::Error),

    #[error("open meta file: {0}")]
    OpenFile(std::io::Error),

    #[error("new meta reader: {0}")]
    NewMetaReader(#[from] NewMetaReaderError),

    #[error("read all samples:{0}")]
    ReadAllSamples(#[from] ReadAllSamplesError),

    #[error("generate mp4: {0}")]
    GenerateMp4(#[from] GenerateMp4Error),
}

async fn read_video_metadata(meta_path: &Path) -> Result<VideoMetadata, ReadVideoMetadataError> {
    use ReadVideoMetadataError::*;
    let metadata = std::fs::metadata(meta_path).map_err(Metadata)?;

    let meta_size = metadata.len();
    let last_modified = metadata.modified().map_err(LastModified)?;
    //let mod_time = metadata.modified();
    //modTime := metaStat.ModTime()

    let mut meta = tokio::fs::OpenOptions::new()
        .read(true)
        .open(meta_path)
        .await
        .map_err(OpenFile)?;

    let (mut meta_reader, header) = MetaReader::new(&mut meta, meta_size).await?;

    let params = header.params();

    let samples = meta_reader.read_all_samples().await?;

    let (meta_buf, mdat_size) = {
        tokio::task::spawn_blocking(move || -> Result<(Vec<u8>, u32), ReadVideoMetadataError> {
            let mut meta_buf = Vec::new();
            let mdat_size = generate_mp4(&mut meta_buf, header.start_time, samples, &params)?;
            Ok((meta_buf, mdat_size))
        })
        .await
        .unwrap()?
    };

    Ok(VideoMetadata {
        buf: meta_buf,
        mdat_size,
        last_modified,
    })
}

// VideoCache Caches the n most recent video readers.
pub struct VideoCache {
    items: HashMap<PathBuf, CacheItem>,
    age: usize,

    max_size: usize,
}

#[derive(Debug)]
struct CacheItem {
    age: usize,
    data: Arc<VideoMetadata>,
}

fn new_meta_cursor(inner: Arc<VideoMetadata>) -> MetaCursor {
    MetaCursor { inner, pos: 0 }
}

// Copy of std::io::Cursor
pub struct MetaCursor {
    inner: Arc<VideoMetadata>,
    pos: u64,
}

impl AsyncRead for MetaCursor {
    fn poll_read(
        mut self: Pin<&mut Self>,
        _: &mut Context<'_>,
        buf: &mut ReadBuf<'_>,
    ) -> Poll<io::Result<()>> {
        /*let len = self.pos.min(self.inner.buf.len() as u64);
        let remaining_slice = &self.inner.buf[(len as usize)..];

        buf.put_slice(remaining_slice);
        self.pos += remaining_slice.len() as u64;

        Poll::Ready(Ok(()))*/

        let pos = self.pos;
        let slice: &[u8] = &self.inner.buf;

        // The position could technically be out of bounds, so don't panic...
        if pos > slice.len() as u64 {
            return Poll::Ready(Ok(()));
        }

        let start = pos as usize;
        let amt = std::cmp::min(slice.len() - start, buf.remaining());
        // Add won't overflow because of pos check above.
        let end = start + amt;
        buf.put_slice(&slice[start..end]);
        self.pos = end as u64;

        Poll::Ready(Ok(()))
    }
}

/*
impl Read for MetaCursor {
    fn read(&mut self, buf: &mut [u8]) -> io::Result<usize> {
        let len = self.pos.min(self.inner.buf.len() as u64);
        let mut remaining_slice = &self.inner.buf[(len as usize)..];

        let n = Read::read(&mut remaining_slice, buf)?;
        self.pos += n as u64;
        Ok(n)
    }
}*/

impl AsyncSeek for MetaCursor {
    fn start_seek(mut self: Pin<&mut Self>, pos: SeekFrom) -> io::Result<()> {
        io::Seek::seek(&mut *self, pos).map(drop)
    }

    fn poll_complete(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<io::Result<u64>> {
        Poll::Ready(Ok(self.get_mut().pos))
    }
}

impl std::io::Seek for MetaCursor {
    fn seek(&mut self, style: std::io::SeekFrom) -> std::io::Result<u64> {
        let (base_pos, offset) = match style {
            SeekFrom::Start(n) => {
                self.pos = n;
                return Ok(n);
            }
            SeekFrom::End(n) => (self.inner.buf.len() as u64, n),
            SeekFrom::Current(n) => (self.pos, n),
        };

        let new_pos = if offset >= 0 {
            base_pos.checked_add(offset as u64)
        } else {
            base_pos.checked_sub((offset.wrapping_neg()) as u64)
        };
        match new_pos {
            Some(n) => {
                self.pos = n;
                Ok(self.pos)
            }
            None => Err(std::io::Error::new(
                std::io::ErrorKind::InvalidInput,
                SeekError::Invalid,
            )),
        }
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
struct VideoMetadata {
    buf: Vec<u8>,
    mdat_size: u32,
    last_modified: std::time::SystemTime,
}

const VIDEO_CACHE_SIZE: usize = 10;

impl VideoCache {
    // NewVideoCache creates a video cache.
    pub fn new() -> Self {
        Self {
            items: HashMap::new(),
            age: 0,
            max_size: VIDEO_CACHE_SIZE,
        }
    }

    // add item to the cache.
    fn add(&mut self, key: PathBuf, video: Arc<VideoMetadata>) {
        // Ignore duplicate keys.
        if self.items.contains_key(&key) {
            return;
        }

        self.age += 1;

        if self.items.len() >= self.max_size {
            // Delete the oldest item.
            let (key, _) = self.items.iter().min_by_key(|(_, v)| v.age).unwrap();
            self.items.remove(&key.to_owned());
        }

        self.items.insert(
            key,
            CacheItem {
                age: self.age,
                data: video,
            },
        );
    }

    // Get item by key and update its age if it exists.
    fn get(&mut self, key: &Path) -> Option<Arc<VideoMetadata>> {
        for (item_key, item) in self.items.iter_mut() {
            if item_key == key {
                self.age += 1;
                item.age = self.age;
                return Some(item.data.clone());
            }
        }
        None
    }
}

impl Default for VideoCache {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use std::{io::Cursor, time::UNIX_EPOCH};

    use super::*;
    use pretty_assertions::assert_eq;
    use tempfile::tempdir;
    use tokio::io::{AsyncReadExt, AsyncSeekExt};

    #[tokio::test]
    async fn test_new_video_reader() {
        let temp_dir = tempdir().unwrap();

        let path = temp_dir.path().join("x");
        let meta_path = temp_dir.path().join("x.meta");
        let mdat_path = temp_dir.path().join("x.mdat");

        let test_meta = vec![
            1, // Version.
            0, 0, 0, 0, 0, 0, 0, 0, // Start time.
            7, 0x80, // Width.
            4, 0x38, // Height.
            0, 2, // Extra data size.
            0, 1, // Extra data.
            //
            // Sample.
            0, // Flags.
            0, 0, 0, 0, 0, 0, 0, 0x0, // PTS.
            0, 0, 0, 0, 0, 0, 0, 0, // DTS.
            0, 0, 0, 0, 0, 0, 0, 0, // Next dts.
            0, 0, 0, 0, // Offset.
            0, 0, 0, 0, // Size.
        ];

        std::fs::write(meta_path, test_meta).unwrap();
        std::fs::write(mdat_path, [0, 0, 0, 0]).unwrap();

        let mut video = new_video_reader(path, &None).await.unwrap();

        // Read 1000 bytes.
        let mut buf = vec![0; 100];
        video.read_exact(&mut buf).await.unwrap();
        //panic!("a {}", n);
        //n, err := new(bytes.Buffer).ReadFrom(video)
        //require.Greater(t, n, int64(1000))*/
    }

    #[tokio::test]
    async fn test_video_reader() {
        let mut r = VideoReader {
            meta: Box::pin(Cursor::new(vec![0, 1, 2, 3, 4])),
            mdat: Box::pin(Cursor::new(vec![5, 6, 7, 8, 9])),
            meta_size: 5,
            mdat_size: 5,
            pos: 0,
            last_modified: UNIX_EPOCH,
            read_state: ReadState::State1,
        };

        // Size.
        assert_eq!(10, r.seek(SeekFrom::End(0)).await.unwrap());

        // Read within meta.
        assert_eq!(2, r.seek(SeekFrom::Current(-8)).await.unwrap());

        let mut buf = [0, 0, 0];
        r.read_exact(&mut buf).await.unwrap();
        assert_eq!([2, 3, 4], buf);

        // Read across border.
        println!("border1");
        assert_eq!(3, r.seek(SeekFrom::Start(3)).await.unwrap());

        let mut buf = [0, 0, 0, 0];
        println!("border2");
        r.read_exact(&mut buf).await.unwrap();
        assert_eq!([3, 4, 5, 6], buf);

        // Read within mdat.
        println!("within mdat1");
        assert_eq!(6, r.seek(SeekFrom::Start(6)).await.unwrap());

        let mut buf = [0, 0, 0, 0];
        println!("within mdat2");
        r.read_exact(&mut buf).await.unwrap();
        assert_eq!([6, 7, 8, 9].to_vec(), buf.to_vec());

        // EOF.
        assert!(r.read_exact(&mut buf).await.is_err());
    }

    fn empty_metadata() -> Arc<VideoMetadata> {
        Arc::new(VideoMetadata {
            buf: Vec::new(),
            mdat_size: 0,
            last_modified: UNIX_EPOCH,
        })
    }

    #[test]
    fn test_video_reader_cache() {
        let mut cache = VideoCache::new();
        cache.max_size = 3;

        // Fill cache.
        cache.add(PathBuf::from("A"), empty_metadata());
        cache.add(PathBuf::from("B"), empty_metadata());
        cache.add(PathBuf::from("C"), empty_metadata());

        // Add item and check if "A" was removed.
        cache.add(PathBuf::from("D"), empty_metadata());
        assert!(cache.get(Path::new("A")).is_none());

        // Get "B" to make it the newest item.
        cache.get(Path::new("B"));

        // Add item and check if "C" was removed instead of "B".
        let e = Arc::new(VideoMetadata {
            buf: Vec::new(),
            mdat_size: 9999,
            last_modified: UNIX_EPOCH,
        });
        cache.add(PathBuf::from("E"), e.clone());
        assert!(cache.get(Path::new("C")).is_none());

        // Add item and check if "D" was removed instead of "B".
        cache.add(PathBuf::from("F"), empty_metadata());
        assert!(cache.get(Path::new("D")).is_none());

        // Add item and check if "B" was removed.
        cache.add(PathBuf::from("G"), empty_metadata());
        assert!(cache.get(Path::new("B")).is_none());

        // Check if duplicate keys are ignored.
        cache.add(PathBuf::from("G"), empty_metadata());
        let e2 = cache.get(Path::new("E")).unwrap();
        assert_eq!(e, e2);
    }
}
