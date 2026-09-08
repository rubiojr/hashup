package cache

import (
	"os"
	"path/filepath"
	"sync"
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

func TestFileCacheStatsAreSafeUnderConcurrentWrites(t *testing.T) {
	fileCache := NewFileCache(1, "")
	var workers sync.WaitGroup
	for range 1000 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			fileCache.MarkFileProcessed("file", "hash")
		}()
	}
	workers.Wait()

	assert.Equal(t, int64(1000), fileCache.GetStats().Additions)
}
