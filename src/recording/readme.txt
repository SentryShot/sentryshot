// customformat reads and writes videos in a custom format.

// Custom format for storing videos.
// Requirements.
//   1. Data must remain valid in case of a system failure.
//   2. Samples should be readable as soon as they are written.
//   3. Must support B-frames.
//
//
//
// <recordingID>.mdat: File with continuous chunks of raw media data.
//   [u8]
//
// <recordingID>.meta: File that contains all metadata required to generate mp4.
//   version: u8,
//   startTimeNS: i64,
//   width: u16,
//   height: u16,
//   extradata_size: u16,
//   extradata: [u8],
//   samples: [sampleV0],
//
//
// sampleV0 { // 25 bytes. i64 timestamps are in UnixNano format.
//   flags: u8 { random_access_present },
//   pts: i64,
//   dts_offset: i32,
//   duration: u32,
//
//   // Offset in video.mdat where the actual data is stored.
//   offset: u32,
//   size: u32,
// }
