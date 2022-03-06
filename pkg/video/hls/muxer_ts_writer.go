package hls

import (
	"context"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/mpegts"
)

type muxerTSWriter struct {
	innerMuxer     *mpegts.Muxer
	currentSegment *MuxerTSSegment
}

func newMuxerTSWriter(
	videoTrack gortsplib.Track,
	audioTrack gortsplib.Track) (*muxerTSWriter, error) {
	w := &muxerTSWriter{}

	w.innerMuxer = mpegts.NewMuxer(context.Background(), w)

	if videoTrack != nil {
		err := w.innerMuxer.AddElementaryStream(mpegts.PMTElementaryStream{
			ElementaryPID: 256,
			StreamType:    mpegts.StreamTypeH264Video,
		})
		if err != nil {
			return nil, err
		}
	}

	if audioTrack != nil {
		err := w.innerMuxer.AddElementaryStream(mpegts.PMTElementaryStream{
			ElementaryPID: 257,
			StreamType:    mpegts.StreamTypeAACAudio,
		})
		if err != nil {
			return nil, err
		}
	}

	if videoTrack != nil {
		w.innerMuxer.SetPCRPID(256)
	} else {
		w.innerMuxer.SetPCRPID(257)
	}

	return w, nil
}

func (mt *muxerTSWriter) Write(p []byte) (int, error) {
	return mt.currentSegment.write(p)
}

func (mt *muxerTSWriter) WriteByte(c byte) error {
	_, err := mt.Write([]byte{c})
	return err
}

func (mt *muxerTSWriter) WriteData(d *mpegts.MuxerData) (int, error) {
	return mt.innerMuxer.WriteData(d)
}
