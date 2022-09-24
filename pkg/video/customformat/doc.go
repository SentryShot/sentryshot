// Package customformat reads and writes videos in a custom format.
package customformat

// Custom format for storing videos.
// Requirements.
//   1. Data must remain valid in case of a system failure.
//   2. Samples should be readable as soon as they are written.
//   3. Must support B-frames.
//
//
//
// <recordingID>.mdat: File with continuous chunks of raw media data.
//   []byte
//
// <recordingID>.meta: File that contains all metadata required to generate mp4.
//   version         uint8
//   videoSPSSize    uint16
//   videoSPS        []byte
//   videoPPSSize    uint16
//   videoPPS        []byte
//   audioConfigSize uint16
//   audioConfig     []byte
//   startTimeNS     int64
//   samples         []sampleV0
//
//
// sampleV0 { // 33 bytes. timestamps are in UnixNano format.
//   flags uint8 { isAudioSample, isSyncSample }
//   pts   int64
//   dts   int64
//
//   // nextPTS if isAudioSample else nextDTS
//   Next int64
//
//   // Offset in video.mdat where the actual data is stored.
//   offset uint32
//   size uint32
// }
