package main

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/rubiojr/hashub/cmd/hs/types"
)

func dbConn(path string) (*sql.DB, error) {
	dbPath := path
	var err error
	if path == "" {
		dbPath, err = getDBPath()
		if err != nil {
			return nil, err
		}
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	return db, nil
}

func getDBPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %v", err)
	}
	dbDir := filepath.Join(homeDir, ".local", "share", "hashub")
	return filepath.Join(dbDir, "hashub.db"), nil
}

func calculateXXHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	hash := xxhash.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to calculate hash: %v", err)
	}

	return fmt.Sprintf("%x", hash.Sum64()), nil
}

func dbSearch(db *sql.DB, query string, extensions []string, limit int) ([]*types.FileResult, error) {
	query = strings.Replace(query, " ", "%", -1)
	sqlQuery := `
		SELECT file_path, file_size, modified_date, host, extension, file_hash
		FROM file_info
		WHERE (file_path LIKE ? OR file_hash LIKE ?)
	`

	if len(extensions) > 0 {
		placeholders := make([]string, len(extensions))
		for i := range extensions {
			placeholders[i] = "?"
		}
		sqlQuery += fmt.Sprintf(" AND extension IN (%s)", strings.Join(placeholders, ","))
	}

	sqlQuery += `
		ORDER BY modified_date DESC
	`

	var args []any
	args = append(args, "%"+query+"%", "%"+query+"%")

	if len(extensions) > 0 {
		for _, ext := range extensions {
			args = append(args, strings.TrimSpace(ext))
		}
	}

	sqlQuery += fmt.Sprintf("LIMIT %d", limit)

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
