package main

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/dustin/go-humanize"
	_ "github.com/mattn/go-sqlite3"
	"github.com/urfave/cli/v2"
)

func commandLargeFiles() *cli.Command {
	return &cli.Command{
		Name:  "large-files",
		Usage: "List large files",
		Flags: []cli.Flag{
			&cli.Int64Flag{
				Name:     "threshold",
				Usage:    "threshold for large files in bytes",
				Value:    1000000000, // 1GB default
				Required: false,
			},
		},
		Action: func(c *cli.Context) error {
			dbPath, err := getDBPath()
			if err != nil {
				return err
			}

			// Connect to SQLite database
			db, err := sql.Open("sqlite3", dbPath)
			if err != nil {
				return fmt.Errorf("failed to open database: %v", err)
			}
			defer db.Close()

			return largeFiles(db, c.Int64("threshold"))
		},
	}
}

func largeFiles(db *sql.DB, threshold int64) error {
	query := `
		SELECT file_path, file_size, host, file_hash
		FROM file_info
		WHERE file_size > ?
		ORDER BY file_size DESC
	`

	rows, err := db.Query(query, threshold)
	if err != nil {
		return fmt.Errorf("failed to query database: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var path string
		var size int64
		var host string
		var hash string

		err := rows.Scan(&path, &size, &host, &hash)
		if err != nil {
			return fmt.Errorf("failed to scan row: %v", err)
		}

		// humanize size
		humanizedSize := humanize.Bytes(uint64(size))
		ellipsizedPath := filepath.Base(path)
		if len(ellipsizedPath) > 50 {
			ellipsizedPath = ellipsizedPath[:47] + "..."
		}
		fmt.Printf("%-8s %-12s %-8s %s\n", humanizedSize, host, hash[:10], ellipsizedPath)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating over rows: %v", err)
	}

	return nil
}
