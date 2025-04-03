package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rubiojr/hashup/cmd/hs/types"
	"github.com/rubiojr/hashup/internal/api"
	hsdb "github.com/rubiojr/hashup/internal/db"
	"github.com/urfave/cli/v2"
)

func commandSearch() *cli.Command {
	return &cli.Command{
		Name:    "search",
		Aliases: []string{"s"},
		Usage:   "Search for files by filename",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:     "limit",
				Usage:    "Number of results to return",
				Value:    100,
				Required: false,
			},
			&cli.StringFlag{
				Name:     "db",
				Usage:    "Database path",
				Value:    "",
				Required: false,
			},
			&cli.StringFlag{
				Name:     "host",
				Usage:    "Filter by host",
				Value:    "",
				Required: false,
			},
			&cli.StringFlag{
				Name:     "tag",
				Usage:    "Filter by tag",
				Required: false,
			},
			&cli.StringFlag{
				Name:     "extension",
				Usage:    "Filter by file extension",
				Value:    "",
				Required: false,
			},
			&cli.StringFlag{
				Name:     "server-url",
				Usage:    "HashUp API server URL",
				Required: false,
			},
		},
		Action: func(c *cli.Context) error {
			hostFilter := c.String("host")
			extFilter := c.String("extension")
			serverURL := c.String("server-url")

			filename := c.Args().Get(0)
			if c.NArg() == 0 {
				return fmt.Errorf("filename argument is required")
			}

			if serverURL != "" {
				return searchServer(serverURL, filename)
			}

			if c.String("tag") != "" {
				return searchByTag(c)
			}

			if hostFilter != "" {
				return searchByHost(c, filename)
			}

			if extFilter != "" {
				return searchByExt(c, filename)
			}
			return searchFiles(c, filename)
		},
	}
}

func searchByTag(c *cli.Context) error {
	tag := c.String("tag")
	limit := c.String("limit")
	db, err := dbConn(c.String("db"))
	defer db.Close()

	query := `
		SELECT file_path, file_size, modified_date, host, extension, file_hash
		FROM file_info
		JOIN file_tags ON file_info.id = file_tags.file_id
		WHERE file_tags.tags LIKE ?
		LIMIT ?
	`

	rows, err := db.Query(query, "%"+tag+"%", limit)
	if err != nil {
		return fmt.Errorf("failed to query database: %v", err)
	}
	defer rows.Close()

	return printRows(rows)
}

func searchByExt(c *cli.Context, filename string) error {
	ext := c.String("ext")
	limit := c.Int("limit")
	db, err := dbConn(c.String("db"))
	if err != nil {
		return fmt.Errorf("failed to get database connection: %v", err)
	}
	defer db.Close()

	r, err := hsdb.Search(db, filename, []string{ext}, []string{}, limit)
	for _, result := range r {
		printFileResult(result)
	}

	return nil
}

func searchByHost(c *cli.Context, filename string) error {
	host := c.String("host")
	limit := c.Int("limit")
	db, err := dbConn(c.String("db"))
	if err != nil {
		return fmt.Errorf("failed to get database connection: %v", err)
	}
	defer db.Close()

	r, err := hsdb.Search(db, filename, []string{}, []string{host}, limit)
	for _, result := range r {
		printFileResult(result)
	}
	return nil
}

func searchFiles(c *cli.Context, filename string) error {
	db, err := dbConn(c.String("db"))
	if err != nil {
		return fmt.Errorf("failed to get database connection: %v", err)
	}
	defer db.Close()

	r, err := hsdb.Search(db, filename, []string{}, []string{}, 100)
	for _, result := range r {
		printFileResult(result)
	}

	return nil
}

func printRows(rows *sql.Rows) error {
	for rows.Next() {
		var filePath string
		var fileSize int64
		var modifiedDate string
		var host, extension, file_hash string

		err := rows.Scan(&filePath, &fileSize, &modifiedDate, &host, &extension, &file_hash)
		if err != nil {
			return fmt.Errorf("failed to scan row: %v", err)
		}

		fmt.Printf("File Path: %s\n", filePath)
		fmt.Printf("File Size: %d bytes\n", fileSize)
		fmt.Printf("Modified Date: %s\n", modifiedDate)
		fmt.Printf("Host: %s\n", host)
		fmt.Printf("Extension: %s\n", extension)
		fmt.Printf("Hash: %s\n", file_hash)
		fmt.Println(strings.Repeat("-", 40))
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating over rows: %v", err)
	}

	return nil
}

func searchServer(serverURL string, filename string) error {
	client := api.NewClient(serverURL)
	r, err := client.Search(filename)
	if err != nil {
		return fmt.Errorf("failed to search server: %v", err)
	}

	for _, result := range r {
		printFileResult(result)
	}

	return nil
}

func printFileResult(result *types.FileResult) {
	fmt.Printf("File Path: %s\n", result.FilePath)
	fmt.Printf("File Size: %d bytes\n", result.FileSize)
	fmt.Printf("Modified Date: %s\n", result.ModifiedDate)
	fmt.Printf("Host: %s\n", result.Host)
	fmt.Printf("Extension: %s\n", result.Extension)
	fmt.Printf("Hash: %s\n", result.FileHash)
	fmt.Println(strings.Repeat("-", 40))
}
