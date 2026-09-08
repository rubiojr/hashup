package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileCacheOnlySavesExplicitly(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cache.bin")
	fileCache := NewFileCache(1, cachePath)
	fileCache.MarkFileProcessed("file", "hash")

	assert.Never(t, func() bool {
		_, err := os.Stat(cachePath)
		return err == nil
	}, 100*time.Millisecond, 10*time.Millisecond)

	require.NoError(t, fileCache.Save())
	_, err := os.Stat(cachePath)
	require.NoError(t, err)
}
