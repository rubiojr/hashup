package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rubiojr/hashup/internal/types"
)

func BenchmarkSqliteStore(b *testing.B) {
	// Create a temporary directory for the benchmark
	tempDir, err := os.MkdirTemp(b.TempDir(), "hashup-benchmark-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a temporary database file path
	dbPath := filepath.Join(tempDir, "bench.db")

	// Create the storage instance
	storage, err := NewSqliteStorage(dbPath)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	currentTime := time.Now()

	// Prepare a sample file to store
	sampleFile := &types.ScannedFile{
		Path:      "/path/to/benchmark/file.txt",
		Size:      1024,
		ModTime:   currentTime,
		Hash:      "abcdef1234567890",
		Extension: "txt",
		Hostname:  "benchmark-host",
	}

	// Reset the timer to exclude setup time
	b.ResetTimer()

	// Run the benchmark
	for i := 0; i < b.N; i++ {
		// Modify the file hash for each iteration to simulate different files
		// This ensures we don't just test the "file already exists" path
		sampleFile.Hash = sampleFile.Hash[:15] + string([]byte{byte('0' + i%10)})

		_, err := storage.Store(ctx, sampleFile)
		if err != nil {
			b.Fatalf("Failed to store file: %v", err)
		}
	}
}

func BenchmarkSqliteStoreDuplicates(b *testing.B) {
	// Create a temporary directory for the benchmark
	tempDir, err := os.MkdirTemp(b.TempDir(), "hashup-benchmark-dups-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a temporary database file path
	dbPath := filepath.Join(tempDir, "bench-dups.db")

	// Create the storage instance
	storage, err := NewSqliteStorage(dbPath)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	currentTime := time.Now()

	// Prepare a sample file to store
	sampleFile := &types.ScannedFile{
		Path:      "/path/to/benchmark/file.txt",
		Size:      1024,
		ModTime:   currentTime,
		Hash:      "abcdef1234567890",
		Extension: "txt",
		Hostname:  "benchmark-host",
	}

	// Store the file once before the benchmark
	_, err = storage.Store(ctx, sampleFile)
	if err != nil {
		b.Fatalf("Failed to store initial file: %v", err)
	}

	// Reset the timer to exclude setup time
	b.ResetTimer()

	// Run the benchmark with the same file (testing duplicate handling)
	for i := 0; i < b.N; i++ {
		_, err := storage.Store(ctx, sampleFile)
		if err != nil && err != ErrFileInfoExists {
			b.Fatalf("Failed to handle duplicate file: %v", err)
		}
	}
}
