package main

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/rqlite/gorqlite"
	"github.com/rubiojr/hashub/internal/errmsg"
	pnats "github.com/rubiojr/hashub/internal/processors/nats"
)

func TestProcessFile(t *testing.T) {
	processor, err := pnats.NewNATSProcessor(os.Getenv("HASHUB_NATS_URL"), "hashub.files", time.Second)
	if err != nil {
		t.Fatalf("Failed to create RQlite processor: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		info     os.FileInfo
		hostname string
		hash     string
	}{
		{
			name:     "Valid file",
			path:     "testdata/basics/hello.txt",
			hash:     "5215e13b207d6d8c",
			hostname: "test-host",
		},
		{
			name:     "Valid file",
			path:     "testdata/basics/dir/foo.txt",
			hash:     "7f3aaa4c3842a787",
			hostname: "test-host",
		},
	}

	// Verify the file was processed correctly
	conn, err := gorqlite.Open(rqliteTestURL)
	if err != nil {
		t.Fatalf("Failed to open rqlite connection: %v", err)
	}
	defer conn.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := mustFileInfo(tt.path)
			err := processor.Process(tt.path, info, tt.hostname)
			if err != nil {
				t.Fatalf("processFile() error")
			}

			// Query the database to verify the file info was inserted
			query := "SELECT file_path, file_hash FROM file_info WHERE file_path = ? AND host = ?"
			rows, err := conn.QueryOneParameterized(
				gorqlite.ParameterizedStatement{
					Query:     query,
					Arguments: []interface{}{tt.path, tt.hostname},
				},
			)
			if err != nil {
				t.Errorf("Failed to query file_info: %v", err)
				return
			}

			var path, hash string
			if !rows.Next() {
				t.Error("No rows returned from query")
				return
			}
			err = rows.Scan(&path, &hash)
			if err != nil {
				t.Errorf("Failed to scan row: %v", err)
				return
			}

			if path != tt.path {
				t.Errorf("File path in database = %v, want %v", path, tt.path)
			}

			if hash != tt.hash {
				t.Errorf("File hash in database = %v, want %v", hash, tt.hash)
			}
		})
	}

	query := "SELECT count(*) FROM file_info"
	rows, err := conn.QueryOne(query)
	if err != nil {
		t.Errorf("Failed to query file_info: %v", err)
		return
	}

	rows.Next()
	var count int
	err = rows.Scan(&count)
	if err != nil {
		t.Errorf("Failed to scan row: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected 2 row, got %d", rows.RowNumber())
	}

	t.Run("symlink", func(t *testing.T) {
		fi := mustFileInfo("testdata/basics/null")
		if fi == nil {
			t.Errorf("Failed to get FileInfo")
		}
		err = processor.Process("testdata/basics/null", fi, "test-host")
		if !errors.Is(err, errmsg.ErrNotRegularFile) {
			t.Errorf("Should have failed to process file: %v", fi.Name())
		}

	})

	t.Run("directory", func(t *testing.T) {
		fi := mustFileInfo("testdata/basics")
		if fi == nil {
			t.Errorf("Failed to get FileInfo")
		}
		err = processor.Process("testdata/basics", fi, "test-host")
		if !errors.Is(err, errmsg.ErrNotRegularFile) {
			t.Errorf("Should have failed to process file: %v", fi.Name())
		}

	})
}

func TestScanDirectory(t *testing.T) {
	cleanup := startRqlite(t)
	defer cleanup()

	processor, err := processors.NewRQliteProcessor(rqliteTestURL)
	if err != nil {
		t.Fatalf("Failed to create RQlite processor: %v", err)
	}

	count, err := scanDirectory(processor, "testdata/basics", rqliteTestURL, []string{}, true, NewPool(10), make(chan int64, 10))
	if err != nil {
		t.Errorf("Failed to scan directory: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected 2 row, got %d", count)
	}
}

func mustFileInfo(path string) os.FileInfo {
	fi, err := os.Stat(path)
	if err != nil {
		panic(err)
	}
	return fi
}
