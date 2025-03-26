package main

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/urfave/cli/v2"
)

func commandTag() *cli.Command {
	return &cli.Command{
		Name:  "tag",
		Usage: "Tag a file in the database",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:     "tags",
				Aliases:  []string{"t"},
				Usage:    "Tags to add (comma-separated)",
				Required: true,
			},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() == 0 {
				return fmt.Errorf("file path argument is required")
			}

			filePath := c.Args().Get(0)
			tags := c.StringSlice("tags")

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

			return tagFile(db, filePath, tags)
		},
	}
}

func tagFile(db *sql.DB, filePath string, tags []string) error {
	// order tags
	sort.Strings(tags)

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	// Calculate file hash
	fileHash, err := calculateXXHash(filePath)
	if err != nil {
		return err
	}

	// Begin transaction
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Find the file_id using the hash
	var fileID int
	err = tx.QueryRow(`
		SELECT file_info.id
		FROM file_info
		WHERE file_hash = ? AND host = ? LIMIT 1
	`, fileHash, hostname).Scan(&fileID)

	if err == sql.ErrNoRows {
		return fmt.Errorf("file %s not found in database", fileHash)
	} else if err != nil {
		return fmt.Errorf("failed to query database: %v", err)
	}

	// Check if tags already exist for this file
	var tagID int
	err = tx.QueryRow(`
		SELECT id
		FROM file_tags
		WHERE file_id = ? LIMIT 1
	`, fileID).Scan(&tagID)

	if err == sql.ErrNoRows {
		// No tags exist yet, insert new tags
		_, err = tx.Exec(`
			INSERT INTO file_tags (file_id, tags) VALUES (?, ?)
		`, fileID, strings.Join(tags, ","))

		if err != nil {
			return fmt.Errorf("failed to insert new tags: %v", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to query existing tags: %v", err)
	} else {
		// Update existing tags
		_, err = tx.Exec(`
			UPDATE file_tags SET tags = ? WHERE id = ?
		`, strings.Join(tags, ","), tagID)

		if err != nil {
			return fmt.Errorf("failed to update tags: %v", err)
		}
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	fmt.Printf("Added tags %v to %s\n", tags, filePath)
	return nil
}
