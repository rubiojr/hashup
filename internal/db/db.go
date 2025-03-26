package db

import (
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed hashub.sql
var Schema string

// Open a SQLite database with appropriate pragmas
func OpenDatabase(dbPath string) (*sql.DB, error) {
	pragmas := []string{
		"_foreign_keys=ON",                 // Enable foreign key constraints
		"_journal_mode=WAL",                // Use WAL mode for better concurrency
		fmt.Sprintf("_busy_timeout=%d", 5), // Set busy timeout
		"_cache_size=-20000",               // Use 20MB page cache (negative value = kilobytes)
	}
	plist := ""
	for _, pragma := range pragmas {
		plist += fmt.Sprintf("&%s", pragma)
	}
	dsn := fmt.Sprintf("file:%s?mode=rwc%s", dbPath, plist)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	if err := CreateTables(db); err != nil {
		return db, fmt.Errorf("failed to create tables: %v", err)
	}

	return db, nil
}

func CreateTables(db *sql.DB) error {
	_, err := db.Exec(Schema)
	return err
}
