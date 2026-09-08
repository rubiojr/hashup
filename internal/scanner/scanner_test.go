package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
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
	mu       sync.Mutex
	calls    int
	err      error
	messages []types.ScannedFile
}

func (p *recordingProcessor) Process(_ string, message types.ScannedFile) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.messages = append(p.messages, message)
	return p.err
}

func (p *recordingProcessor) Messages() []types.ScannedFile {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]types.ScannedFile(nil), p.messages...)
}

func TestScanDirectory(t *testing.T) {
	// Create a test context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get the absolute path to testdata/basics
	testDir := "testdata/basics"
	absTestDir, err := filepath.Abs(testDir)
	require.NoError(t, err)

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
	helloPath := filepath.Join(absTestDir, "hello.txt")
	fooDirPath := filepath.Join(absTestDir, "dir", "foo.txt")
	hiddenFilePath := filepath.Join(absTestDir, ".hiddenfile")

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

func TestScanDirectoryCacheNamespaceAndForce(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0600))
	fileCache := &testCache{processed: make(map[string]bool)}

	first := &recordingProcessor{}
	_, err := NewDirectoryScanner(dir, WithCache(fileCache), WithCacheNamespace("destination-a")).ScanDirectory(context.Background(), first)
	require.NoError(t, err)
	require.Equal(t, 1, first.calls)

	sameDestination := &recordingProcessor{}
	_, err = NewDirectoryScanner(dir, WithCache(fileCache), WithCacheNamespace("destination-a")).ScanDirectory(context.Background(), sameDestination)
	require.NoError(t, err)
	assert.Zero(t, sameDestination.calls)

	newDestination := &recordingProcessor{}
	_, err = NewDirectoryScanner(dir, WithCache(fileCache), WithCacheNamespace("destination-b")).ScanDirectory(context.Background(), newDestination)
	require.NoError(t, err)
	assert.Equal(t, 1, newDestination.calls)

	forced := &recordingProcessor{}
	_, err = NewDirectoryScanner(dir, WithCache(fileCache), WithCacheNamespace("destination-b"), WithForce(true)).ScanDirectory(context.Background(), forced)
	require.NoError(t, err)
	assert.Equal(t, 1, forced.calls)
}

func TestScanDirectoryPublishesAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0600))
	cwd, err := os.Getwd()
	require.NoError(t, err)
	relativeDir, err := filepath.Rel(cwd, dir)
	require.NoError(t, err)

	processor := &recordingProcessor{}
	_, err = NewDirectoryScanner(relativeDir, WithCache(&cache.NoopCache{})).ScanDirectory(context.Background(), processor)
	require.NoError(t, err)
	messages := processor.Messages()
	require.Len(t, messages, 1)
	assert.Equal(t, file, messages[0].Path)
}

func TestScanDirectoryIgnoresMatchingDirectory(t *testing.T) {
	dir := t.TempDir()
	ignoredDir := filepath.Join(dir, "ignored")
	require.NoError(t, os.Mkdir(ignoredDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(ignoredDir, "ignored.txt"), []byte("ignored"), 0600))
	keptFile := filepath.Join(dir, "kept.txt")
	require.NoError(t, os.WriteFile(keptFile, []byte("kept"), 0600))

	processor := &recordingProcessor{}
	_, err := NewDirectoryScanner(
		dir,
		WithIgnoreList([]string{regexp.QuoteMeta(ignoredDir) + "$"}),
		WithCache(&cache.NoopCache{}),
	).ScanDirectory(context.Background(), processor)
	require.NoError(t, err)

	messages := processor.Messages()
	require.Len(t, messages, 1)
	assert.Equal(t, keptFile, messages[0].Path)
}

func TestScanDirectoryRejectsInvalidIgnorePattern(t *testing.T) {
	scanner := NewDirectoryScanner(t.TempDir(), WithIgnoreList([]string{"["}), WithCache(&cache.NoopCache{}))

	_, err := scanner.ScanDirectory(context.Background(), &recordingProcessor{})

	assert.ErrorContains(t, err, "invalid ignore pattern")
}

func TestScanDirectoryHonorsConcurrency(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"one.txt", "two.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0600))
	}
	processor := &blockingProcessor{
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(processor.release) }) }
	t.Cleanup(release)

	done := make(chan error, 1)
	go func() {
		_, err := NewDirectoryScanner(
			dir,
			WithScanningConcurrency(1),
			WithCache(&cache.NoopCache{}),
		).ScanDirectory(context.Background(), processor)
		done <- err
	}()

	select {
	case <-processor.started:
	case <-time.After(time.Second):
		t.Fatal("first file did not start processing")
	}
	select {
	case <-processor.started:
		t.Fatal("more than one file processed concurrently")
	case <-time.After(100 * time.Millisecond):
	}

	release()
	require.NoError(t, <-done)
	assert.Equal(t, int32(1), processor.maximum.Load())
}

func TestScanDirectoryRejectsNonpositiveConcurrency(t *testing.T) {
	scanner := NewDirectoryScanner(t.TempDir(), WithScanningConcurrency(0), WithCache(&cache.NoopCache{}))

	_, err := scanner.ScanDirectory(context.Background(), &recordingProcessor{})

	assert.ErrorContains(t, err, "concurrency must be greater than zero")
}

type blockingProcessor struct {
	current atomic.Int32
	maximum atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (p *blockingProcessor) Process(string, types.ScannedFile) error {
	current := p.current.Add(1)
	for current > p.maximum.Load() && !p.maximum.CompareAndSwap(p.maximum.Load(), current) {
	}
	p.started <- struct{}{}
	<-p.release
	p.current.Add(-1)
	return nil
}

func TestScanDirectoryDoesNotRequireProgressReader(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0600))
	scanner := NewDirectoryScanner(dir, WithCache(&cache.NoopCache{}))
	progress := scanner.CounterChan()
	done := make(chan error, 1)
	go func() {
		_, err := scanner.ScanDirectory(context.Background(), &recordingProcessor{})
		done <- err
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		// Release the blocked sender so the test process can shut down cleanly.
		<-progress
		t.Fatal("scanner blocked without a progress reader")
	}
}

func TestDirectoryScannerDefaultsToNoopCache(t *testing.T) {
	scanner := NewDirectoryScanner(t.TempDir())
	assert.IsType(t, &cache.NoopCache{}, scanner.cache)
}
