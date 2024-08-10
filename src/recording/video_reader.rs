// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{
    mp4_muxer::{generate_mp4, GenerateMp4Error},
    video::{read_meta, ReadMetaError},
    VideoCache,
};
use pin_project::pin_project;
use std::{
    io::{self, SeekFrom},
    path::{Path, PathBuf},
    pin::Pin,
    sync::Arc,
    task::{Context, Poll},
};
use thiserror::Error;
use tokio::{
    io::{AsyncRead, AsyncSeek, BufReader, ReadBuf},
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

    meta_size: usize,
    mdat_size: usize,
    pos: usize, // current reading index

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
        u64::try_from(self.meta_size + self.mdat_size).expect("u64 fit usize")
    }
}

#[derive(Debug, Error)]
pub enum CreateVideoReaderError {
    #[error("read video metadata: {0}")]
    ReadVideoMetadat(#[from] ReadVideoMetadataError),

    #[error("open mdat: {0}")]
    OpenMdat(#[from] std::io::Error),

    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),
}

#[allow(clippy::module_name_repetitions)]
pub async fn new_video_reader(
    recording_path: PathBuf,
    cache_id: u32,
    cache: &Option<Arc<Mutex<VideoCache>>>,
) -> Result<VideoReader<MetaCursor, tokio::fs::File>, CreateVideoReaderError> {
    use CreateVideoReaderError::*;
    let mut meta_path = recording_path.clone();
    meta_path.set_extension("meta");

    let mut mdat_path = recording_path.clone();
    mdat_path.set_extension("mdat");

    let meta = {
        if let Some(cache) = cache {
            let mut cache = cache.lock().await;
            if let Some(v) = cache.get((&recording_path, cache_id)) {
                v
            } else {
                let meta = Arc::new(read_video_metadata(&meta_path).await?);
                cache.add((recording_path, cache_id), meta.clone());
                meta
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
        meta_size: meta.buf.len(),
        mdat_size: usize::try_from(meta.mdat_size).expect("usize fit u32"),
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
    #[allow(clippy::too_many_lines)]
    fn poll_read(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &mut ReadBuf<'_>,
    ) -> Poll<io::Result<()>> {
        let mut this = self.as_mut().project();
        loop {
            match this.read_state {
                ReadState::State1 => {
                    if *this.pos >= *this.meta_size + *this.mdat_size {
                        // EOF.
                        return Poll::Ready(Ok(()));
                    }

                    if *this.pos <= *this.meta_size {
                        // Read starts within meta.
                        this.meta.as_mut().start_seek(SeekFrom::Start(
                            u64::try_from(*this.pos).expect("u64 fit usize"),
                        ))?;
                        *this.read_state = ReadState::State2;
                        continue;
                    }
                    // Read starts within mdat.
                    let mdat_pos = *this.pos - *this.meta_size;
                    this.mdat.as_mut().start_seek(SeekFrom::Start(
                        u64::try_from(mdat_pos).expect("u64 fit usize"),
                    ))?;
                    *this.read_state = ReadState::State6;
                    continue;
                }
                ReadState::State2 => {
                    match this.meta.as_mut().poll_complete(cx) {
                        Poll::Ready(res) => {
                            res?;
                            if buf.remaining() <= *this.meta_size - *this.pos {
                                // Only read within meta.
                                *this.read_state = ReadState::State3;
                                continue;
                            }

                            // Read within meta and continue across border.
                            *this.read_state = ReadState::State4;
                            continue;
                        }
                        Poll::Pending => return Poll::Pending,
                    }
                }
                ReadState::State3 => {
                    let prev_buf_len = buf.filled().len();
                    match this.meta.as_mut().poll_read(cx, buf) {
                        Poll::Ready(res) => {
                            res?;
                            let n = buf.filled().len() - prev_buf_len;
                            *this.pos += n;
                            *this.read_state = ReadState::State1;
                            return Poll::Ready(Ok(()));
                        }
                        Poll::Pending => return Poll::Pending,
                    }
                }
                ReadState::State4 => {
                    let prev_buf_len = buf.filled().len();
                    match this.meta.as_mut().poll_read(cx, buf) {
                        Poll::Ready(res) => {
                            res?;
                            let n = buf.filled().len() - prev_buf_len;
                            *this.pos += n;
                            // Poll ready and continue across border.
                            *this.read_state = ReadState::State5;
                            return Poll::Ready(Ok(()));
                        }
                        Poll::Pending => return Poll::Pending,
                    }
                }
                ReadState::State5 => {
                    let prev_buf_len = buf.filled().len();
                    match this.mdat.as_mut().poll_read(cx, buf) {
                        Poll::Ready(res) => {
                            res?;

                            let n = buf.filled().len() - prev_buf_len;
                            *this.pos += n;
                            *this.read_state = ReadState::State1;
                            return Poll::Ready(Ok(()));
                        }
                        Poll::Pending => return Poll::Pending,
                    }
                }
                ReadState::State6 => {
                    match this.mdat.as_mut().poll_complete(cx) {
                        Poll::Ready(res) => {
                            res?;
                            // Read starts within mdat, read mdat.
                            *this.read_state = ReadState::State7;
                            continue;
                        }
                        Poll::Pending => return Poll::Pending,
                    }
                }
                ReadState::State7 => {
                    let prev_buf_len = buf.filled().len();
                    match this.mdat.as_mut().poll_read(cx, buf) {
                        Poll::Ready(res) => {
                            res?;
                            let n = buf.filled().len() - prev_buf_len;
                            *this.pos += n;
                            *this.read_state = ReadState::State1;
                            return Poll::Ready(Ok(()));
                        }
                        Poll::Pending => return Poll::Pending,
                    }
                }
            }
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
        Poll::Ready(Ok(u64::try_from(self.pos).expect("u64 fit usize")))
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
                self.pos = usize::try_from(n)
                    .map_err(|e| std::io::Error::new(std::io::ErrorKind::InvalidInput, e))?;
                return Ok(n);
            }
            SeekFrom::End(n) => (self.meta_size + self.mdat_size, n),
            SeekFrom::Current(n) => (self.pos, n),
        };

        #[allow(
            clippy::cast_sign_loss,
            clippy::cast_possible_truncation,
            clippy::as_conversions
        )]
        let new_pos = if offset >= 0 {
            base_pos.checked_add(offset as usize)
        } else {
            base_pos.checked_sub((offset.wrapping_neg()) as usize)
        };
        match new_pos {
            Some(n) => {
                self.pos = n;
                Ok(u64::try_from(self.pos).expect("usize fit u64"))
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

    #[error("read meta: {0}")]
    ReadMeta(#[from] ReadMetaError),

    #[error("generate mp4: {0}")]
    GenerateMp4(#[from] GenerateMp4Error),
}

async fn read_video_metadata(meta_path: &Path) -> Result<VideoMetadata, ReadVideoMetadataError> {
    use ReadVideoMetadataError::*;
    let metadata = tokio::fs::metadata(meta_path).await.map_err(Metadata)?;

    let meta_size = metadata.len();
    let last_modified = metadata.modified().map_err(LastModified)?;
    //let mod_time = metadata.modified();
    //modTime := metaStat.ModTime()

    let mut meta = BufReader::new(
        tokio::fs::OpenOptions::new()
            .read(true)
            .open(meta_path)
            .await
            .map_err(OpenFile)?,
    );

    let (header, samples) = read_meta(&mut meta, meta_size).await?;
    let params = header.params();

    let mut meta_buf = Vec::new();
    let mdat_size = generate_mp4(&mut meta_buf, header.start_time, samples.iter(), &params).await?;

    Ok(VideoMetadata {
        buf: meta_buf,
        mdat_size,
        last_modified,
    })
}

fn new_meta_cursor(inner: Arc<VideoMetadata>) -> MetaCursor {
    MetaCursor { inner, pos: 0 }
}

// Copy of std::io::Cursor
pub struct MetaCursor {
    inner: Arc<VideoMetadata>,
    pos: u64,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) struct VideoMetadata {
    pub(crate) buf: Vec<u8>,
    pub(crate) mdat_size: u32,
    pub(crate) last_modified: std::time::SystemTime,
}

impl AsyncRead for MetaCursor {
    #[allow(clippy::cast_possible_truncation, clippy::as_conversions)]
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
    #[allow(clippy::as_conversions)]
    fn seek(&mut self, style: std::io::SeekFrom) -> std::io::Result<u64> {
        let (base_pos, offset) = match style {
            SeekFrom::Start(n) => {
                self.pos = n;
                return Ok(n);
            }
            SeekFrom::End(n) => (self.inner.buf.len() as u64, n),
            SeekFrom::Current(n) => (self.pos, n),
        };

        #[allow(clippy::cast_sign_loss)]
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

#[allow(clippy::unwrap_used)]
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

        let test_meta = &[
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

        let mut video = new_video_reader(path, 0, &None).await.unwrap();

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
}
