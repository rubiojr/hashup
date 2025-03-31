package main

import (
	"database/sql"
	"fmt"

	"github.com/dustin/go-humanize"
	_ "github.com/mattn/go-sqlite3"
	"github.com/urfave/cli/v2"
)

func commandHosts() *cli.Command {
	return &cli.Command{
		Name:  "hosts",
		Usage: "List available hosts",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "db",
				Usage:    "Database path",
				Value:    "",
				Required: false,
			},
		},
		Action: func(c *cli.Context) error {
			db, err := dbConn(c.String("db"))
			if err != nil {
				return fmt.Errorf("failed to open database: %v", err)
			}
			defer db.Close()

			return listHosts(db)
		},
	}
}

func listHosts(db *sql.DB) error {
	query := `
		SELECT host, COUNT(*) as count, SUM(file_size) AS total_size
		FROM file_info
		GROUP BY host
	`

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query database: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var count, sum int
		var host string

		err := rows.Scan(&host, &count, &sum)
		if err != nil {
			return fmt.Errorf("failed to scan row: %v", err)
		}

		// humanize sum
		humanizedSum := humanize.Bytes(uint64(sum))
		fmt.Printf(fmt.Sprintf("%-30s %-10d %-10s\n", host, count, humanizedSum))
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating over rows: %v", err)
	}

	return nil
}
