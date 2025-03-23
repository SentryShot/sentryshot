// SPDX-License-Identifier: GPL-2.0-or-later

#![allow(clippy::module_name_repetitions)]

use common::time::{DtsOffset, UnixH264};
use thiserror::Error;

#[derive(Debug, Error)]
pub enum GenerateInitError {
    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),

    #[error("{0}")]
    Mp4(#[from] mp4::Mp4Error),
}

#[derive(Debug, Error)]
pub enum GenerateMoofError {
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
    GeneratePart(#[from] GenerateMoofError),

    #[error("get part duration")]
    Duration,
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
pub enum WriteFrameError {
    #[error("mp4: {0}")]
    Mp4(#[from] mp4::Mp4Error),

    #[error("write h264: {0}")]
    WriteH264(#[from] PartWriteH264Error),

    #[error("generate mp4 boxes: {0}")]
    GenerateMp4Boxes(#[from] GenerateMoofError),

    #[error("calculate sample duration")]
    ComputeFrameDuration,

    #[error("switch segment")]
    SwitchSegment,

    #[error("dts")]
    Dts,
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

    #[error("generate init: {0}")]
    GenerateInit(#[from] GenerateInitError),
}
