package storage

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewVideoReader(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "x")
	metaPath := path + ".meta"
	mdatPath := path + ".mdat"

	testMeta := []byte{
		0,    // Version.
		0, 7, // Video sps size.
		103, 0, 0, 0, 172, 217, 0, // Video sps.
		0, 3, // Video pps size.
		2, 3, 4, // Video pps.
		0, 4, // Audio config size.
		20, 10, 0, 0, // Audio Config.
		0, 0, 0, 0, 0, 0, 0, 0, // Start time.

		// Sample.
		0,                        // Flags.
		0, 0, 0, 0, 0, 0, 0, 0x0, // PTS.
		0, 0, 0, 0, 0, 0, 0, 0, // DTS.
		0, 0, 0, 0, 0, 0, 0, 0, // Next dts.
		0, 0, 0, 0, // Offset.
		0, 0, 0, 0, // Size.
	}

	err := os.WriteFile(metaPath, testMeta, 0o600)
	require.NoError(t, err)
	err = os.WriteFile(mdatPath, []byte{0, 0, 0, 0}, 0o600)
	require.NoError(t, err)

	recording, err := NewVideoReader(path)
	require.NoError(t, err)
	defer recording.Close()

	n, err := new(bytes.Buffer).ReadFrom(recording)
	require.NoError(t, err)
	require.Greater(t, n, int64(1000))
}

func TestVideoReader(t *testing.T) {
	meta := bytes.NewReader([]byte{0, 1, 2, 3, 4})
	mdat := &mockReadSeekCloser{
		reader: bytes.NewReader([]byte{5, 6, 7, 8, 9}),
	}
	r := VideoReader{
		meta:     meta,
		mdat:     mdat,
		mdatSize: 5,
		metaSize: 5,
	}

	// Size.
	size, err := r.Seek(0, io.SeekEnd)
	require.NoError(t, err)
	require.Equal(t, int64(10), size)

	// Read within meta.
	abs, err := r.Seek(-8, io.SeekCurrent)
	require.NoError(t, err)
	require.Equal(t, int64(2), abs)

	buf1 := make([]byte, 3)
	n, err := r.Read(buf1)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, []byte{2, 3, 4}, buf1)

	// Read across border.
	abs, err = r.Seek(3, io.SeekStart)
	require.NoError(t, err)
	require.Equal(t, int64(3), abs)

	buf2 := make([]byte, 4)
	n, err = r.Read(buf2)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, []byte{3, 4, 5, 6}, buf2)

	// Read within mdat.
	abs, err = r.Seek(6, io.SeekStart)
	require.NoError(t, err)
	require.Equal(t, int64(6), abs)

	buf3 := make([]byte, 4)
	n, err = r.Read(buf3)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, []byte{6, 7, 8, 9}, buf3)

	// EOF.
	_, err = r.Read(buf3)
	require.ErrorIs(t, err, io.EOF)

	// Invalid whence.
	_, err = r.Seek(0, -1)
	require.ErrorIs(t, err, errInvalidWhence)

	// Negative  position.
	_, err = r.Seek(-1, io.SeekStart)
	require.ErrorIs(t, err, errNegativePosition)
}

type mockReadSeekCloser struct {
	reader *bytes.Reader
}

func (r *mockReadSeekCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *mockReadSeekCloser) Seek(offset int64, whence int) (int64, error) {
	return r.reader.Seek(offset, whence)
}

func (r *mockReadSeekCloser) Close() error {
	return nil
}
