package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	hsdb "github.com/rubiojr/hashup/internal/db"
	"github.com/urfave/cli/v2"
)

func commandAdmin() *cli.Command {
	return &cli.Command{
		Name:  "admin",
		Usage: "Administrative commands",
		Subcommands: []*cli.Command{
			{
				Name:  "recreate-db",
				Usage: "Re-create the database",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "force",
						Aliases: []string{"f"},
						Usage:   "Force recreation without confirmation",
						Value:   false,
					},
					&cli.StringFlag{
						Name:  "db-path",
						Usage: "Override default database path",
						Value: "",
					},
				},
				Action: func(c *cli.Context) error {
					return recreateDatabase(c.Bool("force"), c.String("db-path"))
				},
			},
			{
				Name:  "delete-host",
				Usage: "Delete all files from a specific host",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "host",
						Aliases:  []string{"H"},
						Usage:    "Hostname to delete files from",
						Required: true,
					},
					&cli.BoolFlag{
						Name:    "force",
						Aliases: []string{"f"},
						Usage:   "Force deletion without confirmation",
						Value:   false,
					},
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Show what would be deleted without actually deleting",
						Value: false,
					},
				},
				Action: func(c *cli.Context) error {
					dbPath, err := getDBPath()
					if err != nil {
						return err
					}

					return deleteFilesByHost(
						dbPath,
						c.String("host"),
						c.Bool("force"),
						c.Bool("dry-run"),
					)
				},
			},
		},
	}
}

func recreateDatabase(force bool, dbPathOverride string) error {
	// Get the database path
	dbPath := dbPathOverride
	var err error
	if dbPath == "" {
		dbPath, err = getDBPath()
		if err != nil {
			return err
		}
	}

	// Check if the database file exists
	_, err = os.Stat(dbPath)
	if err == nil {
		// File exists, ask for confirmation unless forced
		if !force {
			fmt.Printf("Warning: This will delete all data in %s\n", dbPath)
			fmt.Print("Are you sure you want to continue? (y/N): ")
			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				fmt.Println("Database recreation aborted.")
				return nil
			}
		}

		// Delete the existing database file
		err = os.Remove(dbPath)
		if err != nil {
			return fmt.Errorf("failed to remove existing database: %v", err)
		}
		fmt.Printf("Removed existing database: %s\n", dbPath)
	} else if !os.IsNotExist(err) {
		// Some other error occurred
		return fmt.Errorf("error checking database file: %v", err)
	}

	// Ensure the directory exists
	dbDir := filepath.Dir(dbPath)
	err = os.MkdirAll(dbDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create database directory: %v", err)
	}

	// Create a new database
	db, err := hsdb.OpenDatabase(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}
	defer db.Close()

	fmt.Printf("Successfully created new database at %s\n", dbPath)
	return nil
}

func deleteFilesByHost(dbPath, host string, force, dryRun bool) error {
	// Connect to the database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}
	defer db.Close()

	// Count how many files will be deleted
	var fileCount int
	err = db.QueryRow("SELECT COUNT(*) FROM file_info WHERE host = ?", host).Scan(&fileCount)
	if err != nil {
		return fmt.Errorf("failed to count files: %v", err)
	}

	if fileCount == 0 {
		fmt.Printf("No files found for host '%s'\n", host)
		return nil
	}

	// In dry-run mode, just show what would be deleted
	if dryRun {
		fmt.Printf("Would delete %d files from host '%s'\n", fileCount, host)

		// Show some sample files that would be deleted
		rows, err := db.Query(`
			SELECT file_path, file_size, file_hash
			FROM file_info
			WHERE host = ?
			LIMIT 5
		`, host)
		if err != nil {
			return fmt.Errorf("failed to query sample files: %v", err)
		}
		defer rows.Close()

		fmt.Println("\nSample files that would be deleted:")
		for rows.Next() {
			var path string
			var size int64
			var hash string
			if err := rows.Scan(&path, &size, &hash); err != nil {
				return fmt.Errorf("failed to scan row: %v", err)
			}
			fmt.Printf("  %s (size: %d, hash: %s)\n", path, size, hash)
		}

		if fileCount > 5 {
			fmt.Printf("  ... and %d more\n", fileCount-5)
		}

		return nil
	}

	// Ask for confirmation unless forced
	if !force {
		fmt.Printf("Warning: This will delete %d files for host '%s'\n", fileCount, host)
		fmt.Print("Are you sure you want to continue? (y/N): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Deletion aborted.")
			return nil
		}
	}

	// Begin a transaction
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// First, get the IDs of files we need to delete from other tables
	rows, err := tx.Query("SELECT id FROM file_info WHERE host = ?", host)
	if err != nil {
		return fmt.Errorf("failed to query file IDs: %v", err)
	}

	var fileIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("failed to scan file ID: %v", err)
		}
		fileIDs = append(fileIDs, id)
	}
	rows.Close()

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating over rows: %v", err)
	}

	// Now delete related records from dependent tables
	if len(fileIDs) > 0 {
		// Delete from file_tags
		for _, id := range fileIDs {
			_, err = tx.Exec("DELETE FROM file_tags WHERE file_id = ?", id)
			if err != nil {
				return fmt.Errorf("failed to delete from file_tags: %v", err)
			}
		}

		// Delete from file_notes
		for _, id := range fileIDs {
			_, err = tx.Exec("DELETE FROM file_notes WHERE file_id = ?", id)
			if err != nil {
				return fmt.Errorf("failed to delete from file_notes: %v", err)
			}
		}
	}

	// Delete the file records
	result, err := tx.Exec("DELETE FROM file_info WHERE host = ?", host)
	if err != nil {
		return fmt.Errorf("failed to delete files: %v", err)
	}

	// Get the number of rows affected
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %v", err)
	}

	// Cleanup orphaned hashes (optional, but keeps the database clean)
	_, err = tx.Exec(`
		DELETE FROM file_hashes
		WHERE id NOT IN (
			SELECT DISTINCT hash_id FROM file_info
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to clean up orphaned hashes: %v", err)
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	fmt.Printf("Successfully deleted %d files from host '%s'\n", rowsAffected, host)
	return nil
}

func createTables(db *sql.DB) error {
	_, err := db.Exec(hsdb.Schema)
	return err
}
