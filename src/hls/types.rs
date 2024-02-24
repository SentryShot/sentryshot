use common::TrackParameters;
use retina::codec::VideoParameters;
use std::convert::TryFrom;

use crate::error::ParseParamsError;

// 14496-12_2015 8.3.2.3
// track_ID is an integer that uniquely identifies this track
// over the entire life‐time of this presentation.
// Track IDs are never re‐used and cannot be zero.
pub const VIDEO_TRACK_ID: u32 = 1;

pub struct IdCounter(u64);

impl IdCounter {
    pub fn new(initial: u64) -> Self {
        Self(initial)
    }

    pub fn next_id(&mut self) -> u64 {
        let id = self.0;
        self.0 += 1;
        id
    }
}

pub struct MuxerIdCounter(u16);

impl MuxerIdCounter {
    pub fn new() -> Self {
        Self(0)
    }

    pub fn next_id(&mut self) -> u16 {
        let id = self.0;
        self.0 = self.0.wrapping_add(1);
        id
    }
}

pub fn track_params_from_video_params(
    params: &VideoParameters,
) -> Result<TrackParameters, ParseParamsError> {
    let (width, height) = params.pixel_dimensions();
    Ok(TrackParameters {
        width: u16::try_from(width)?,
        height: u16::try_from(height)?,
        codec: params.rfc6381_codec().to_owned(),
        extra_data: params.extra_data().to_owned(),
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_id_counter() {
        let mut counter = IdCounter::new(0);
        assert_eq!(0, counter.next_id());
        assert_eq!(1, counter.next_id());
        assert_eq!(2, counter.next_id());
    }

    #[test]
    fn test_muxer_id_counter() {
        let mut counter = MuxerIdCounter::new();
        assert_eq!(0, counter.next_id());
        assert_eq!(1, counter.next_id());
        assert_eq!(2, counter.next_id());
    }
}
