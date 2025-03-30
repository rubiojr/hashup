package test

import (
	"database/sql"
	"os"
	"testing"

	hsdb "github.com/rubiojr/hashup/internal/db"
)

func TempDB(t *testing.T) *sql.DB {
	dir := t.TempDir()
	tmpFile, err := os.CreateTemp(dir, "hashup-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temporary file: %v", err)
	}
	// Open a SQLite database connection
	db, err := sql.Open("sqlite3", tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	_, err = db.Exec(hsdb.Schema)
	if err != nil {
		t.Fatalf("Failed to create tables: %v", err)
	}

	return db
}
