// Video storage format.
//
// Requirements.
//   1. Data must remain valid in case of system failure.
//   2. Data must be readable as soon as it is written.
//   3. Must support querying arbitrary time ranges in sublinear time (binary search).
//   4. Media data should be stored continuously on disk.
//   5. Must support B-frames.
//
//
// Audio is currently not supported.
// The data is stored in two files:  
//
// <recordingID>.mdat: File with continuous chunks of raw media data.
//   [u8]
//
// <recordingID>.meta: File that contains all metadata required to generate moof.
//   version: u8,
//   startTimeNS: i64,
//   width: u16,
//   height: u16,
//   extradata_size: u16,
//   extradata: [u8],
//   samples: [sampleV0],
//
//
// sampleV0 { // 25 bytes.
//   flags: u8 { random_access_present },
//   pts: i64,        // Absolute Unix nanosecond timestamp.
//   dts_offset: i32, // pts - dts
//   duration: u32,
//
//   // Offset in <recordingID>.mdat where the media data is stored.
//   offset: u32,
//   size: u32,
// }
