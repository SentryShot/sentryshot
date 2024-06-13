// SPDX-License-Identifier: GPL-2.0-or-later

use common::{
    time::{DtsOffset, DurationH264, UnixH264},
    PartFinalized, VideoSample,
};
use std::{io::SeekFrom, sync::Arc};
use thiserror::Error;
use tokio::io::{AsyncRead, AsyncReadExt, AsyncSeek, AsyncSeekExt, AsyncWrite, AsyncWriteExt};

// Sample flags.
const FLAG_RANDOM_ACCESS_PRESENT: u8 = 0b1000_0000;

const SAMPLE_SIZE_U8: u8 = 25;
#[allow(clippy::as_conversions)]
pub const SAMPLE_SIZE: usize = SAMPLE_SIZE_U8 as usize;

// Sample .
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct Sample {
    pub random_access_present: bool,

    pub pts: UnixH264,         // Presentation time in 90khz since the unix epoch.
    pub dts_offset: DtsOffset, // Display time offset.
    pub duration: DurationH264,
    pub data_size: u32,
    pub data_offset: u32,
}

impl Sample {
    pub fn from_bytes(b: &[u8; SAMPLE_SIZE]) -> Self {
        let flags = b[0];
        Self {
            random_access_present: flags & FLAG_RANDOM_ACCESS_PRESENT != 0,
            pts: UnixH264::new(i64::from_be_bytes([
                b[1], b[2], b[3], b[4], b[5], b[6], b[7], b[8],
            ])),
            dts_offset: DtsOffset::new(i32::from_be_bytes([b[9], b[10], b[11], b[12]])),
            duration: DurationH264::new(u32::from_be_bytes([b[13], b[14], b[15], b[16]]).into()),
            data_offset: u32::from_be_bytes([b[17], b[18], b[19], b[20]]),
            data_size: u32::from_be_bytes([b[21], b[22], b[23], b[24]]),
        }
        /*Self {
            random_access_present: flags & FLAG_RANDOM_ACCESS_PRESENT != 0,
            pts: i64::from_be_bytes(b[1..9].try_into().unwrap()),
            cts: i32::from_be_bytes(b[9..13].try_into().unwrap()),
            duration: u32::from_be_bytes(b[13..17].try_into().unwrap()),
            data_offset: u32::from_be_bytes(b[17..21].try_into().unwrap()),
            data_size: u32::from_be_bytes(b[21..25].try_into().unwrap()),
        }*/
    }

    pub fn encode(&self) -> Result<Vec<u8>, std::num::TryFromIntError> {
        let mut flags: u8 = 0;
        if self.random_access_present {
            flags |= FLAG_RANDOM_ACCESS_PRESENT;
        }

        let mut out = Vec::with_capacity(SAMPLE_SIZE);

        out.push(flags);
        out.extend_from_slice(&self.pts.to_be_bytes());
        out.extend_from_slice(&self.dts_offset.to_be_bytes());
        out.extend_from_slice(&self.duration.as_u32()?.to_be_bytes());
        out.extend_from_slice(&self.data_offset.to_be_bytes());
        out.extend_from_slice(&self.data_size.to_be_bytes());
        Ok(out)
    }

    pub fn dts(&self) -> Option<UnixH264> {
        self.pts.checked_sub(self.dts_offset.into())
    }
}

// Writes videos in our custom format.
#[allow(clippy::module_name_repetitions)]
pub struct VideoWriter<'a, W: AsyncWrite + Unpin> {
    meta: &'a mut W, // Output file.
    mdat: &'a mut W, // Output file.

    mdat_pos: u32,
}

#[derive(Debug, Error)]
pub enum NewVideoWriterError {
    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),

    #[error("write: {0}")]
    Write(#[from] std::io::Error),
}

#[derive(Debug, Error)]
pub enum WriteSampleError {
    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),

    #[error("write: {0}")]
    Write(std::io::Error),

    #[error("flush: {0}")]
    Flush(std::io::Error),

    #[error("sub")]
    Sub,
}

impl<'a, W: AsyncWrite + Unpin> VideoWriter<'a, W> {
    // Creates a new writer and writes the header.
    pub async fn new(
        meta: &'a mut W,
        mdat: &'a mut W,
        header: Header,
    ) -> Result<VideoWriter<'a, W>, NewVideoWriterError> {
        meta.write_all(&header.marshal()?).await?;
        Ok(Self {
            meta,
            mdat,
            mdat_pos: 0,
        })
    }

    // Writes HLS parts in the custom format to the output files.
    pub async fn write_parts(
        &mut self,
        parts: &Vec<Arc<PartFinalized>>,
    ) -> Result<(), WriteSampleError> {
        use WriteSampleError::*;

        for part in parts {
            for sample in part.video_samples.iter() {
                self.write_sample(sample).await?;
            }
        }
        self.mdat.flush().await.map_err(Flush)?;
        self.meta.flush().await.map_err(Flush)?;
        Ok(())
    }

    // Writes a single sample in the custom format to the output files.
    pub async fn write_sample(&mut self, sample: &VideoSample) -> Result<(), WriteSampleError> {
        use WriteSampleError::*;

        let s = Sample {
            random_access_present: sample.random_access_present,
            pts: sample.pts,
            dts_offset: sample.dts_offset,
            duration: sample.duration,
            data_offset: self.mdat_pos,
            data_size: u32::try_from(sample.avcc.len())?,
        };

        self.mdat.write_all(&sample.avcc).await.map_err(Write)?;
        self.mdat_pos += u32::try_from(sample.avcc.len())?;

        self.meta.write_all(&s.encode()?).await.map_err(Write)?;

        Ok(())
    }
}

// Reads a single meta file.
pub struct MetaReader<T: AsyncRead + AsyncSeek + Unpin> {
    file: T,

    header_size: u64,
    //file_size: usize,
    sample_count: usize,
}

#[derive(Debug, Error)]
pub enum NewMetaReaderError {
    #[error("unmarshal header {0}")]
    UnmarshalHeader(#[from] HeaderFromReaderError),

    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),
}

#[derive(Debug, Error)]
pub enum ReadAllSamplesError {
    #[error("seek: {0}")]
    Seek(std::io::Error),

    #[error("read: {0}")]
    Read(std::io::Error),
}

impl<T: AsyncRead + AsyncSeek + Unpin> MetaReader<T> {
    pub async fn new(mut file: T, file_size: u64) -> Result<(Self, Header), NewMetaReaderError> {
        let header = Header::from_reader(&mut file).await?;
        let header_size = u64::try_from(header.size())?;

        Ok((
            Self {
                file,
                header_size,
                //file_size,
                sample_count: usize::try_from(
                    (file_size - header_size) / u64::from(SAMPLE_SIZE_U8),
                )?,
            },
            header,
        ))
    }

    // Reads and returns all samples in the file.
    pub async fn read_all_samples(&mut self) -> Result<Vec<Sample>, ReadAllSamplesError> {
        // Seek to end of the header.
        use ReadAllSamplesError::*;
        self.file
            .seek(SeekFrom::Start(self.header_size))
            .await
            .map_err(Seek)?;

        let mut buf = [0; SAMPLE_SIZE];
        let mut samples = vec![Sample::default(); self.sample_count];
        for sample in &mut samples {
            self.file.read_exact(&mut buf).await.map_err(Read)?;
            *sample = Sample::from_bytes(&buf);
        }

        Ok(samples)
    }
}

// Recording meta file header.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Header {
    pub start_time: UnixH264,
    pub width: u16,
    pub height: u16,
    pub extra_data: Vec<u8>,
}

#[derive(Debug, Error)]
pub enum HeaderFromReaderError {
    #[error("old version, upgrade with this script: TODO")]
    OldVersion,

    #[error("unsupported version")]
    UnsupportedVersion,

    #[error("read: {0}")]
    Read(#[from] std::io::Error),

    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),
}

impl Header {
    pub async fn from_reader<R: AsyncRead + Unpin>(
        r: &mut R,
    ) -> Result<Self, HeaderFromReaderError> {
        let mut api_version = [0];
        r.read_exact(&mut api_version).await?;
        if api_version[0] == 0 {
            return Err(HeaderFromReaderError::OldVersion);
        } else if api_version[0] != 1 {
            return Err(HeaderFromReaderError::UnsupportedVersion);
        }

        // Start time.
        let mut start_time = [0; 8];
        r.read_exact(&mut start_time).await?;
        let start_time = UnixH264::new(i64::from_be_bytes(start_time));

        // Width.
        let mut width = [0; 2];
        r.read_exact(&mut width).await?;
        let width = u16::from_be_bytes(width);

        // Height.
        let mut height = [0; 2];
        r.read_exact(&mut height).await?;
        let height = u16::from_be_bytes(height);

        // Extra data.
        let mut size_buf = [0; 2];
        r.read_exact(&mut size_buf).await?;

        let size = u16::from_be_bytes(size_buf);

        let mut extra_data = vec![0; size.into()];
        r.read_exact(&mut extra_data).await?;

        Ok(Header {
            start_time,
            width,
            height,
            extra_data,
        })
    }

    // Marshaled size.
    #[must_use]
    pub fn size(&self) -> usize {
        15 + self.extra_data.len()
    }

    pub fn marshal(&self) -> Result<Vec<u8>, std::num::TryFromIntError> {
        const API_VERSION: u8 = 1;

        let mut out = Vec::with_capacity(self.size());

        out.push(API_VERSION);

        // Start time.
        out.extend_from_slice(&self.start_time.to_be_bytes());

        // Width.
        out.extend_from_slice(&self.width.to_be_bytes());

        // Height.
        out.extend_from_slice(&self.height.to_be_bytes());

        // Extra data.
        let extra_data_size = u16::try_from(self.extra_data.len())?;
        out.extend_from_slice(&extra_data_size.to_be_bytes());
        out.extend_from_slice(&self.extra_data);

        Ok(out)
    }

    #[must_use]
    pub fn params(&self) -> TrackParameters {
        TrackParameters {
            width: self.width,
            height: self.height,
            extra_data: self.extra_data.clone(),
        }
    }
}

#[derive(Debug)]
pub struct TrackParameters {
    pub width: u16,
    pub height: u16,
    pub extra_data: Vec<u8>,
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use common::time::{DtsOffset, DurationH264, UnixH264};
    use pretty_assertions::assert_eq;
    use pretty_hex::pretty_hex;
    use sentryshot_padded_bytes::PaddedBytes;
    use std::{io::Cursor, sync::Arc};

    #[tokio::test]
    async fn test_video() {
        let mut meta = Vec::new();
        let mut mdat = Vec::new();

        let test_header = Header {
            start_time: UnixH264::new(1_000_000_000),
            width: 1920,
            height: 1080,
            extra_data: vec![0, 1],
        };

        let mut w = VideoWriter::new(&mut meta, &mut mdat, test_header.clone())
            .await
            .unwrap();

        let parts = vec![Arc::new(PartFinalized {
            video_samples: Arc::new(vec![
                VideoSample {
                    pts: UnixH264::new(100_000_000_000_000_000),
                    dts_offset: DtsOffset::new(-1_000_000_000),
                    random_access_present: true,
                    avcc: Arc::new(PaddedBytes::new(vec![3, 4])),
                    duration: DurationH264::new(1_000_000_000),
                },
                VideoSample {
                    pts: UnixH264::new(300_000_000_000_000_000),
                    dts_offset: DtsOffset::new(-1_000_000_000),
                    random_access_present: false,
                    avcc: Arc::new(PaddedBytes::new(vec![5, 6, 7])),
                    duration: DurationH264::new(1_000_000_000),
                },
            ]),
            ..Default::default()
        })];
        w.write_parts(&parts).await.unwrap();

        #[rustfmt::skip]
        let want_meta = vec![
            1, // Version.
            0, 0, 0, 0, 0x3b, 0x9a, 0xca, 0, // Start time.
            7, 0x80, // Width.
            4, 0x38, // Height.
            0, 2, // Extra data size.
            0, 1, // Extra data.
            //
            // Sample 1.
            0b1000_0000, // Flags.
            1, 0x63, 0x45, 0x78, 0x5d, 0x8a, 0, 0, // PTS.
            0xc4, 0x65, 0x36, 0, // DTS offset.
            0x3b, 0x9a, 0xca, 0, // Duration.
            0, 0, 0, 0, // Offset.
            0, 0, 0, 2, // Size.
            //
            // Sample 2.
            0b0000_0000, // Flags.
            4, 0x29, 0xd0, 0x69, 0x18, 0x9e, 0, 0, // PTS.
            0xc4, 0x65, 0x36, 0, // Dts offset.
            0x3b, 0x9a, 0xca, 0, // Duration.
            0, 0, 0, 2, // Offset.
            0, 0, 0, 3, // Size.
        ];
        let want_meta_len = u64::try_from(want_meta.len()).unwrap();

        let want_mdat = vec![3, 4, 5, 6, 7];

        assert_eq!(pretty_hex(&want_meta), pretty_hex(&meta));
        assert_eq!(want_mdat, mdat);

        let (mut r, header) = MetaReader::new(Cursor::new(want_meta), want_meta_len)
            .await
            .unwrap();
        assert_eq!(test_header, header);

        let want_samples = vec![
            Sample {
                random_access_present: true,
                pts: UnixH264::new(100_000_000_000_000_000),
                dts_offset: DtsOffset::new(-1_000_000_000),
                duration: DurationH264::new(1_000_000_000),
                data_size: 2,
                data_offset: 0,
            },
            Sample {
                random_access_present: false,
                pts: UnixH264::new(300_000_000_000_000_000),
                dts_offset: DtsOffset::new(-1_000_000_000),
                duration: DurationH264::new(1_000_000_000),
                data_size: 3,
                data_offset: 2,
            },
        ];

        let samples = r.read_all_samples().await.unwrap();
        assert_eq!(want_samples, samples);
    }
}
