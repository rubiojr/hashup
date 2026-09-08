package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rubiojr/hashup/internal/cache"
	"github.com/rubiojr/hashup/internal/processors"
	"github.com/rubiojr/hashup/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCache struct {
	processed map[string]bool
}

func (c *testCache) IsFileProcessed(path, hash string) bool {
	return c.processed[path+hash]
}

func (c *testCache) MarkFileProcessed(path, hash string) {
	c.processed[path+hash] = true
}

func (*testCache) Save() error {
	return nil
}

type recordingProcessor struct {
	calls int
	err   error
}

func (p *recordingProcessor) Process(string, types.ScannedFile) error {
	p.calls++
	return p.err
}

func TestScanDirectory(t *testing.T) {
	// Create a test context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get the absolute path to testdata/basics
	testDir := "testdata/basics"

	// Create a channel processor to collect the scanned files
	chanProcessor := processors.NewChanProcessor()
	processedFiles := make(map[string]types.ScannedFile)

	// Start a goroutine to read from the channel processor
	done := make(chan struct{})
	go func() {
		defer close(done)
		for file := range chanProcessor.Ch {
			processedFiles[file.Path] = file
		}
	}()

	// Create the directory scanner
	options := []Option{
		WithScanningConcurrency(1),
		WithIgnoreHidden(false), // To test both hidden and non-hidden files
		WithCache(&cache.NoopCache{}),
	}
	dirScanner := NewDirectoryScanner(testDir, options...)

	// Run the scanner
	count, err := dirScanner.ScanDirectory(ctx, chanProcessor)
	assert.NoError(t, err)

	// There should be at least 3 files (hello.txt, dir/foo.txt, .hiddenfile)
	assert.GreaterOrEqual(t, count, int64(3))

	// Close the processor's channel after scanning
	close(chanProcessor.Ch)

	// Wait for all files to be processed
	<-done

	// Verify the expected files were processed
	helloPath := filepath.Join(testDir, "hello.txt")
	fooDirPath := filepath.Join(testDir, "dir", "foo.txt")
	hiddenFilePath := filepath.Join(testDir, ".hiddenfile")

	// Verify hello.txt was processed
	helloFile, exists := processedFiles[helloPath]
	assert.True(t, exists, "hello.txt should be processed")
	if exists {
		assert.Equal(t, int64(12), helloFile.Size) // "hello world\n" = 12 bytes
		assert.Equal(t, "txt", helloFile.Extension)

		// Verify the file hash is consistent
		fileInfo, err := os.Stat(helloPath)
		assert.NoError(t, err)
		assert.Equal(t, fileInfo.Size(), helloFile.Size)
	}

	// Verify dir/foo.txt was processed
	fooFile, exists := processedFiles[fooDirPath]
	assert.True(t, exists, "dir/foo.txt should be processed")
	if exists {
		assert.Equal(t, int64(4), fooFile.Size) // "bar\n" = 4 bytes
		assert.Equal(t, "txt", fooFile.Extension)
	}

	// If we're not ignoring hidden files, verify .hiddenfile was processed
	if !dirScanner.ignoreHidden {
		hiddenFile, exists := processedFiles[hiddenFilePath]
		assert.True(t, exists, ".hiddenfile should be processed")
		if exists {
			assert.Equal(t, "", hiddenFile.Extension)
			assert.Equal(t, int64(0), hiddenFile.Size) // empty file
		}
	}

	// Make sure we have the hostname set for all files
	hostname, err := os.Hostname()
	assert.NoError(t, err)

	for _, file := range processedFiles {
		assert.Equal(t, hostname, file.Hostname)
		assert.NotEmpty(t, file.Hash, "File hash should not be empty")
	}
}

func TestScanDirectoryRetriesFailedProcessing(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "retry.txt"), []byte("retry"), 0600))
	fileCache := &testCache{processed: make(map[string]bool)}

	failing := &recordingProcessor{err: errors.New("processing failed")}
	_, err := NewDirectoryScanner(dir, WithCache(fileCache)).ScanDirectory(context.Background(), failing)
	require.NoError(t, err)
	require.Equal(t, 1, failing.calls)

	succeeding := &recordingProcessor{}
	_, err = NewDirectoryScanner(dir, WithCache(fileCache)).ScanDirectory(context.Background(), succeeding)
	require.NoError(t, err)
	assert.Equal(t, 1, succeeding.calls, "failed files must be retried on the next scan")

	cached := &recordingProcessor{}
	_, err = NewDirectoryScanner(dir, WithCache(fileCache)).ScanDirectory(context.Background(), cached)
	require.NoError(t, err)
	assert.Zero(t, cached.calls, "successfully processed files should remain cached")
}
