package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rubiojr/hashup/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestStore(t *testing.T) {
	ctx := context.Background()

	// Create store instance
	s, err := NewSqliteStorage(filepath.Join(t.TempDir(), "hashup.db"))
	assert.NoError(t, err)
	db := s.db

	modTime := time.Now().Add(-24 * time.Hour) // A day ago
	fileMsg1 := &types.ScannedFile{
		Path:      "/path/to/file1.txt",
		Size:      1024,
		ModTime:   modTime,
		Hash:      "abcdef1234567890",
		Extension: "txt",
		Hostname:  "test-host",
	}

	t.Run("Save a new file", func(t *testing.T) {
		written, err := s.Store(ctx, fileMsg1)
		if err != nil {
			t.Errorf("Failed to save file: %v", err)
		}
		assert.True(t, written.Both())

		// Verify the file was saved
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM file_info").Scan(&count)
		if err != nil {
			t.Errorf("Failed to count rows: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 row in file_info, got %d", count)
		}

	})

	t.Run("Save the same file again", func(t *testing.T) {
		written, err := s.Store(ctx, fileMsg1)
		if err != nil {
			t.Errorf("Failed to save file again: %v", err)
		}
		assert.True(t, written.Clean(), fmt.Errorf("%+v", written))

		// Verify no additional record was created
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM file_info").Scan(&count)
		if err != nil {
			t.Errorf("Failed to count rows: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected still 1 row in file_info, got %d", count)
		}
	})

	t.Run("Save same file with update size", func(t *testing.T) {
		fileMsg1.Size = 2048
		// Hash will be different due to different content
		fileMsg1.Hash = "abcdef1234567891"
		written, err := s.Store(ctx, fileMsg1)
		if err != nil {
			t.Errorf("Failed to update file: %v", err)
		}
		assert.True(t, written.Both())

		// Verify the file size was updated
		var fileSize int64
		err = db.QueryRow("SELECT file_size FROM file_info WHERE file_hash = ?", fileMsg1.Hash).Scan(&fileSize)
		if err != nil {
			t.Errorf("Failed to fetch updated file: %v", err)
		}
		if fileSize != 2048 {
			t.Errorf("Expected updated file_size=2048, got %d", fileSize)
		}

		// Verify an additional record was created
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM file_info").Scan(&count)
		if err != nil {
			t.Errorf("Failed to count rows: %v", err)
		}
		if count != 2 {
			t.Errorf("Expected still 2 row in file_info, got %d", count)
		}
	})

	t.Run("Save a file with the same hash but different path", func(t *testing.T) {
		fileMsg2 := &types.ScannedFile{
			Path:      "/path/to/file2.txt",
			Size:      1024,
			ModTime:   modTime,
			Hash:      "abcdef1234567890", // Same hash as fileMsg1
			Extension: "txt",
			Hostname:  "test-host",
		}

		written, err := s.Store(ctx, fileMsg2)
		assert.NoError(t, err)
		assert.False(t, written.FileHash)
		assert.True(t, written.FileInfo)

		// Verify there are now two file_info records but still one hash record
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM file_info").Scan(&count)
		if err != nil {
			t.Errorf("Failed to count file_info rows: %v", err)
		}
		if count != 3 {
			t.Errorf("Expected 3 rows in file_info, got %d", count)
		}

		err = db.QueryRow("SELECT COUNT(*) FROM file_hashes").Scan(&count)
		if err != nil {
			t.Errorf("Failed to count file_hashes rows: %v", err)
		}
		if count != 2 {
			t.Errorf("Expected 2 row in file_hashes, got %d", count)
		}
	})

	t.Run("Save a completely new file with a different hash", func(t *testing.T) {
		fileMsg3 := &types.ScannedFile{
			Path:      "/path/to/file3.docx",
			Size:      4096,
			ModTime:   modTime,
			Hash:      "9876543210fedcba", // Different hash
			Extension: "docx",
			Hostname:  "test-host",
		}

		written, err := s.Store(ctx, fileMsg3)
		assert.NoError(t, err)
		assert.True(t, written.Both())

		// Verify there are now three file_info records and two hash records
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM file_info").Scan(&count)
		if err != nil {
			t.Errorf("Failed to count file_info rows: %v", err)
		}
		if count != 4 {
			t.Errorf("Expected 4 rows in file_info, got %d", count)
		}

		err = db.QueryRow("SELECT COUNT(*) FROM file_hashes").Scan(&count)
		if err != nil {
			t.Errorf("Failed to count file_hashes rows: %v", err)
		}
		if count != 3 {
			t.Errorf("Expected 3 rows in file_hashes, got %d", count)
		}

	})
}
