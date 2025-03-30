package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rubiojr/hashup/internal/cache"
	"github.com/rubiojr/hashup/internal/processors"
	"github.com/rubiojr/hashup/internal/types"
	"github.com/stretchr/testify/assert"
)

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
