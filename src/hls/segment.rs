use crate::{
    error::{PartWriteH264Error, SegmentFinalizeError},
    part::{MuxerPart, PartFinalized, PartsReader},
    playlist::Playlist,
    types::IdCounter,
};
use async_trait::async_trait;
use common::{
    SegmentImpl, VideoSample,
    time::{DurationH264, UnixH264, UnixNano},
};
use std::{mem, sync::Arc};
use tokio::io::AsyncRead;

#[allow(clippy::struct_field_names, clippy::module_name_repetitions)]
pub struct Segment {
    id: u64,
    muxer_id: u16,
    start_dts: UnixH264,
    muxer_start_time: UnixNano,
    segment_max_size: u64,
    playlist: Arc<Playlist>,

    name: String,
    size: u64,
    parts: Vec<Arc<PartFinalized>>,
    current_part: MuxerPart,
}

impl Segment {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        id: u64,
        muxer_id: u16,
        start_dts: UnixH264,
        muxer_start_time: UnixNano,
        segment_max_size: u64,
        playlist: Arc<Playlist>,
        part_id_counter: &mut IdCounter,
    ) -> Self {
        let first_part_id = part_id_counter.next_id();
        Self {
            id,
            muxer_id,
            start_dts,
            muxer_start_time,
            segment_max_size,
            playlist,
            name: format!("seg{id}"),
            size: 0,
            parts: Vec::new(),
            current_part: MuxerPart::new(first_part_id, muxer_start_time),
        }
    }

    pub fn start_dts(&self) -> UnixH264 {
        self.start_dts
    }

    pub async fn write_h264(
        &mut self,
        sample: VideoSample,
        adjusted_part_duration: DurationH264,
        part_id_counter: &mut IdCounter,
    ) -> Result<(), PartWriteH264Error> {
        use crate::error::PartWriteH264Error::*;
        let size = u64::try_from(sample.avcc.len())?;

        if (self.size + size) > self.segment_max_size {
            return Err(MaximumSegmentSize);
        }

        self.current_part.write_h264(sample);
        self.size += size;

        // switch part
        if self.current_part.duration().ok_or(Duration)? >= adjusted_part_duration {
            let current_part = mem::replace(
                &mut self.current_part,
                MuxerPart::new(part_id_counter.next_id(), self.muxer_start_time),
            );
            let finalized_part = Arc::new(current_part.finalize()?);

            self.parts.push(finalized_part.clone());
            self.playlist.part_finalized(finalized_part).await;
        }

        Ok(())
    }

    // Retuns None if cancelled.
    pub async fn finalize(
        mut self,
        next_video_sample_dts: UnixH264,
    ) -> Result<Option<SegmentFinalized>, SegmentFinalizeError> {
        let finalized_part = Arc::new(self.current_part.finalize()?);

        if finalized_part.rendered_content.is_some() {
            if self
                .playlist
                .part_finalized(finalized_part.clone())
                .await
                .is_none()
            {
                return Ok(None);
            }
            self.parts.push(finalized_part);
        }

        Ok(Some(SegmentFinalized::new(
            self.id,
            self.muxer_id,
            self.start_dts,
            self.name,
            self.parts,
            next_video_sample_dts
                .checked_sub(self.start_dts)
                .ok_or(SegmentFinalizeError::CalculateDuration)?
                .into(),
        )))
    }
}

#[derive(Debug)]
#[allow(clippy::module_name_repetitions)]
pub struct SegmentFinalized {
    id: u64,
    muxer_id: u16,
    start_time: UnixH264,
    //pub start_dts: i64,
    //muxer_start_time: i64,
    //playlist: Arc<Playlist>,
    name: String,
    //size: u64,
    parts: Vec<Arc<PartFinalized>>,
    duration: DurationH264,
}

impl SegmentFinalized {
    #[must_use]
    pub fn new(
        id: u64,
        muxer_id: u16,
        start_time: UnixH264,
        name: String,
        parts: Vec<Arc<PartFinalized>>,
        duration: DurationH264,
    ) -> Self {
        Self {
            id,
            muxer_id,
            start_time,
            name,
            parts,
            duration,
        }
    }

    #[must_use]
    pub fn name(&self) -> &str {
        &self.name
    }

    #[must_use]
    pub fn parts(&self) -> &Vec<Arc<PartFinalized>> {
        &self.parts
    }

    #[must_use]
    pub fn reader(&self) -> Box<dyn AsyncRead + Send + Unpin> {
        Box::new(PartsReader::new(self.parts.clone()))
    }
}

#[async_trait]
impl SegmentImpl for SegmentFinalized {
    #[must_use]
    fn id(&self) -> u64 {
        self.id
    }

    #[must_use]
    fn muxer_id(&self) -> u16 {
        self.muxer_id
    }

    fn frames(&self) -> Box<dyn Iterator<Item = &VideoSample> + Send + '_> {
        Box::new(self.parts.iter().flat_map(|v| v.video_samples.iter()))
    }

    #[must_use]
    fn duration(&self) -> DurationH264 {
        self.duration
    }

    #[must_use]
    fn start_time(&self) -> UnixH264 {
        self.start_time
    }
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use bytes::Bytes;
    use tokio::io::AsyncReadExt;

    fn new_test_part(content: Vec<u8>) -> Arc<PartFinalized> {
        Arc::new(PartFinalized {
            id: 0,
            is_independent: false,
            video_samples: Arc::new(Vec::new()),
            rendered_content: Some(Bytes::from(content)),
            rendered_duration: DurationH264::new(0),
        })
    }

    async fn read_n(reader: &mut PartsReader, n: usize) -> Vec<u8> {
        let mut buf = Vec::with_capacity(n);
        reader.read_buf(&mut buf).await.unwrap();
        buf
    }

    #[tokio::test]
    async fn test_parts_reader() {
        let parts = vec![
            new_test_part(vec![0, 1, 2, 3]),
            new_test_part(vec![4, 5, 6]),
            new_test_part(vec![7, 8]),
            new_test_part(vec![9]),
        ];
        let mut reader = PartsReader::new(parts);

        assert_eq!(vec![0, 1, 2], read_n(&mut reader, 3).await);
        assert_eq!(vec![3, 4, 5, 6], read_n(&mut reader, 4).await);
        assert_eq!(vec![7, 8, 9], read_n(&mut reader, 99).await);
    }
}
