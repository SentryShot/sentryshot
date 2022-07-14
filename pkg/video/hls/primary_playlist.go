package hls

import (
	"bytes"
	"encoding/hex"
	"io"
	"net/http"
	"nvr/pkg/video/gortsplib"
	"strconv"
	"strings"
)

type primaryPlaylist struct {
	videoTrack *gortsplib.TrackH264
	audioTrack *gortsplib.TrackAAC
}

func newPrimaryPlaylist(
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
) *primaryPlaylist {
	return &primaryPlaylist{
		videoTrack: videoTrack,
		audioTrack: audioTrack,
	}
}

func (p *primaryPlaylist) file() *MuxerFileResponse {
	return &MuxerFileResponse{
		Status: http.StatusOK,
		Header: map[string]string{
			"Content-Type": `audio/mpegURL`,
		},
		Body: func() io.Reader {
			var codecs []string

			if p.videoTrack != nil {
				sps := p.videoTrack.SafeSPS()
				if len(sps) >= 4 {
					codecs = append(codecs, "avc1."+hex.EncodeToString(sps[1:4]))
				}
			}

			// https://developer.mozilla.org/en-US/docs/Web/Media/Formats/codecs_parameter
			if p.audioTrack != nil {
				codecs = append(
					codecs,
					"mp4a.40."+strconv.FormatInt(int64(p.audioTrack.Config.Type), 10),
				)
			}

			return bytes.NewReader([]byte("#EXTM3U\n" +
				"#EXT-X-VERSION:9\n" +
				"#EXT-X-INDEPENDENT-SEGMENTS\n" +
				"\n" +
				"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"" + strings.Join(codecs, ",") + "\"\n" +
				"stream.m3u8\n" +
				"\n"))
		}(),
	}
}
