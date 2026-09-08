//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package scanner

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestOpenRegularFileRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe")
	require.NoError(t, unix.Mkfifo(path, 0600))
	done := make(chan error, 1)
	go func() {
		_, _, err := openRegularFile(path)
		done <- err
	}()

	select {
	case err := <-done:
		assert.ErrorContains(t, err, "not a regular file")
	case <-time.After(time.Second):
		t.Fatal("opening a FIFO blocked")
	}
}
