use crate::{
    error::{PartWriteH264Error, SegmentFinalizeError},
    part::MuxerPart,
    playlist::Playlist,
    types::IdCounter,
};
use common::{
    time::{DurationH264, UnixH264},
    PartFinalized, SegmentFinalized, VideoSample,
};
use std::{mem, sync::Arc};

#[allow(clippy::struct_field_names)]
pub struct Segment {
    id: u64,
    start_time: UnixH264,
    start_dts: DurationH264,
    muxer_start_time: i64,
    segment_max_size: u64,
    playlist: Arc<Playlist>,

    name: String,
    size: u64,
    parts: Vec<Arc<PartFinalized>>,
    current_part: MuxerPart,
}

impl Segment {
    pub fn new(
        id: u64,
        start_time: UnixH264,
        start_dts: DurationH264,
        muxer_start_time: i64,
        segment_max_size: u64,
        playlist: Arc<Playlist>,
        part_id_counter: &mut IdCounter,
    ) -> Self {
        let first_part_id = part_id_counter.next_id();
        Self {
            id,
            start_time,
            start_dts,
            muxer_start_time,
            segment_max_size,
            playlist,
            name: format!("seg{id}"),
            size: 0,
            parts: Vec::new(),
            current_part: MuxerPart::new(muxer_start_time, first_part_id),
        }
    }

    pub fn start_dts(&self) -> DurationH264 {
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
                MuxerPart::new(self.muxer_start_time, part_id_counter.next_id()),
            );
            let finalized_part = Arc::new(current_part.finalize()?);

            self.parts.push(finalized_part.clone());
            self.playlist.part_finalized(finalized_part).await?;
        }

        Ok(())
    }

    pub async fn finalize(
        mut self,
        next_video_sample_dts: DurationH264,
    ) -> Result<SegmentFinalized, SegmentFinalizeError> {
        let finalized_part = Arc::new(self.current_part.finalize()?);

        if finalized_part.rendered_content.is_some() {
            self.playlist.part_finalized(finalized_part.clone()).await?;
            self.parts.push(finalized_part);
        }

        Ok(SegmentFinalized::new(
            self.id,
            self.start_time,
            self.name,
            self.parts,
            next_video_sample_dts
                .checked_sub(self.start_dts)
                .ok_or(SegmentFinalizeError::CalculateDuration)?,
        ))
    }
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use bytes::Bytes;
    use common::PartsReader;
    use tokio::io::AsyncReadExt;

    fn new_test_part(content: Vec<u8>) -> Arc<PartFinalized> {
        Arc::new(PartFinalized {
            muxer_start_time: 0,
            id: 0,
            is_independent: false,
            video_samples: Arc::new(Vec::new()),
            rendered_content: Some(Bytes::from(content)),
            rendered_duration: DurationH264::from(0),
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
