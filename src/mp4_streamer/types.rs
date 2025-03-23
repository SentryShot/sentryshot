// SPDX-License-Identifier: GPL-2.0-or-later

use common::TrackParameters;
use retina::codec::VideoParameters;
use std::convert::TryFrom;

use crate::error::ParseParamsError;

// 14496-12_2015 8.3.2.3
// track_ID is an integer that uniquely identifies this track
// over the entire life‐time of this presentation.
// Track IDs are never re‐used and cannot be zero.
pub const VIDEO_TRACK_ID: u32 = 1;

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
