// SPDX-License-Identifier: GPL-2.0-or-later

mod mp4_muxer;
mod video;
mod video_reader;

pub use video::{Header, NewVideoWriterError, VideoWriter, WriteSampleError};
pub use video_reader::{new_video_reader, NewVideoReaderError, VideoCache};
