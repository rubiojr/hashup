// hashub/cmd/hs/tags.go
package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/urfave/cli/v2"
)

func commandTags() *cli.Command {
	return &cli.Command{
		Name:  "tags",
		Usage: "List all tags",
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

			return listTags(db)
		},
	}
}

func listTags(db *sql.DB) error {
	query := `
		SELECT DISTINCT tags FROM file_tags
	`

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query database: %v", err)
	}
	defer rows.Close()

	// Use a map to store unique tags
	uniqueTags := make(map[string]struct{})

	for rows.Next() {
		var tagString string
		err := rows.Scan(&tagString)
		if err != nil {
			return fmt.Errorf("failed to scan row: %v", err)
		}

		// Split the comma-separated tags and add them to the map
		tags := strings.Split(tagString, ",")
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				uniqueTags[tag] = struct{}{}
			}
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating over rows: %v", err)
	}

	// Print all unique tags
	for tag := range uniqueTags {
		fmt.Println(tag)
	}

	return nil
}
