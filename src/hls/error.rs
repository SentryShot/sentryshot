#![allow(clippy::module_name_repetitions)]

use common::time::{DtsOffset, UnixH264};
use thiserror::Error;

#[derive(Debug, Error)]
pub enum PartHlsQueryError {
    #[error("both or neither msn and part must be present")]
    BothOrNeitherMsnAndPart,
}

#[derive(Debug, Error)]
pub enum GenerateInitError {
    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),

    #[error("{0}")]
    Mp4(#[from] mp4::Mp4Error),
}

#[derive(Debug, Error)]
pub enum GeneratePartError {
    #[error("from int: {0}")]
    TryFromInt(#[from] std::num::TryFromIntError),

    #[error("generate traf: {0}")]
    GenerateTraf(#[from] GenerateTrafError),

    #[error("mp4: {0}")]
    Mp4(#[from] mp4::Mp4Error),
}

#[derive(Debug, Error)]
pub enum GenerateTrafError {
    #[error("from int: {0} {1}")]
    TryFromInt(String, std::num::TryFromIntError),

    #[error("dts {0:?} {1:?}")]
    Dts(UnixH264, DtsOffset),

    #[error("sub")]
    Sub,
}

#[derive(Debug, Error)]
pub enum PartFinalizeError {
    #[error("generate part: {0}")]
    GeneratePart(#[from] GeneratePartError),

    #[error("get part duration")]
    Duration,
}

#[derive(Debug, Error)]
pub enum FullPlaylistError {
    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),

    #[error("duration overflowing")]
    DurationOverflowing,
}

#[derive(Debug, Error)]
pub enum SegmentFinalizeError {
    #[error("mp4: {0}")]
    Mp4(#[from] mp4::Mp4Error),

    #[error("part finalize: {0}")]
    PartFinalize(#[from] PartFinalizeError),

    #[error("calculate duration")]
    CalculateDuration,
}

#[derive(Debug, Error)]
pub enum PartWriteH264Error {
    #[error("reached maximum segment size")]
    MaximumSegmentSize,

    #[error("{0}")]
    TryFrom(#[from] std::num::TryFromIntError),

    #[error("{0}")]
    Mp4(#[from] mp4::Mp4Error),

    #[error("part finalize: {0}")]
    PartFinalize(#[from] PartFinalizeError),

    #[error("get duration")]
    Duration,
}

#[derive(Debug, Error)]
pub enum SegmenterWriteH264Error {
    #[error("mp4: {0}")]
    Mp4(#[from] mp4::Mp4Error),

    #[error("write h264: {0}")]
    WriteH264(#[from] PartWriteH264Error),

    #[error("finalize segment: {0}")]
    SegmenterFinalize(#[from] SegmentFinalizeError),

    #[error("adjust part duration: {0}")]
    AdjustPartDuration(#[from] AdjustPartDurationError),

    #[error("calculate sample duration")]
    ComputeSampleDuration,

    #[error("switch segment")]
    SwitchSegment,

    #[error("dts")]
    Dts,
}

#[derive(Debug, Error)]
pub enum AdjustPartDurationError {
    #[error("error")]
    Error,
}

#[derive(Debug, Error)]
pub enum ParseParamsError {
    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),
}

#[derive(Debug, Error)]
pub enum CreateSegmenterError {
    #[error("first sample is not an IDR")]
    NotIdr,

    #[error("Dts is not zero")]
    DtsNotZero,
}
