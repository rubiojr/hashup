package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rubiojr/hashup/cmd/hs/types"
	"github.com/rubiojr/hashup/internal/log"
)

//go:embed hashup.sql
var Schema string

// Open a SQLite database with appropriate pragmas
func OpenDatabase(dbPath string) (*sql.DB, error) {
	log.Debugf("Opening database %s", dbPath)
	pragmas := []string{
		"_foreign_keys=ON",                    // Enable foreign key constraints
		"_journal_mode=WAL",                   // Use WAL mode for better concurrency
		fmt.Sprintf("_busy_timeout=%d", 5000), // Set busy timeout
		"_cache_size=-20000",                  // Use 20MB page cache (negative value = kilobytes)
		"_synchronous=NORMAL",                 // Ensure full synchronous mode
	}
	plist := ""
	for _, pragma := range pragmas {
		plist += fmt.Sprintf("&%s", pragma)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %v", err)
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

func Search(db *sql.DB, query string, extensions []string, limit int) ([]*types.FileResult, error) {
	query = strings.Replace(query, " ", "%", -1)
	sqlQuery := `
		SELECT file_path, file_size, modified_date, host, extension, file_hash
		FROM file_info
		WHERE (file_path LIKE ? OR file_hash LIKE ?)
	`

	var args []any
	args = append(args, "%"+query+"%", "%"+query+"%")

	if len(extensions) > 0 {
		placeholders := make([]string, len(extensions))
		for i := range extensions {
			placeholders[i] = "?"
		}
		sqlQuery += fmt.Sprintf(" AND extension IN (%s)", strings.Join(placeholders, ","))
		for _, ext := range extensions {
			args = append(args, strings.TrimSpace(ext))
		}
	}

	sqlQuery += `
		ORDER BY modified_date DESC
	`

	sqlQuery += fmt.Sprintf("LIMIT %d", limit)
	fmt.Println(sqlQuery)

	rows, err := db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("Database error: %v", err)
	}
	defer rows.Close()

	var results []*types.FileResult
	for rows.Next() {
		var result types.FileResult
		err := rows.Scan(
			&result.FilePath,
			&result.FileSize,
			&result.ModifiedDate,
			&result.Host,
			&result.Extension,
			&result.FileHash,
		)
		if err != nil {
			return nil, fmt.Errorf("Error scanning row: %v", err)
		}
		results = append(results, &result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("Error iterating over rows: %v", err)
	}

	return results, nil
}
