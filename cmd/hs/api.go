package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/urfave/cli/v2"
)

func commandServe() *cli.Command {
	return &cli.Command{
		Name:  "api",
		Usage: "Serve index API",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "host",
				Usage:    "Filter by host",
				Value:    "",
				Required: false,
			},
			&cli.StringFlag{
				Name:  "extensions",
				Usage: "Comma separated list of file extensions to include in search results",
				Value: "",
			},
			&cli.StringFlag{
				Name:  "address",
				Usage: "Address to listen on",
				Value: "localhost:8448",
			},
			&cli.IntFlag{
				Name:  "limit",
				Usage: "Maximum number of results to return",
				Value: 100,
			},
		},
		Action: func(c *cli.Context) error {
			return serveAPI(c)
		},
	}
}

func handleAPI(c *cli.Context) error {
	extensions := strings.Split(c.String("extensions"), ",")

	dbPath, err := getDBPath()
	if err != nil {
		return fmt.Errorf("Failed to get database path: %v", err)
	}

	http.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Handling search request")
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to open database: %v", err), http.StatusInternalServerError)
			fmt.Println(err)
			return
		}
		defer db.Close()

		if r.Method != http.MethodGet {
			http.Error(w, "Only GET method is allowed", http.StatusMethodNotAllowed)
			return
		}

		query := r.URL.Query().Get("q")
		if query == "" {
			http.Error(w, "Missing query parameter 'q'", http.StatusBadRequest)
			return
		}

		results, err := dbSearch(db, query, extensions, 100)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to search files: %v", err), http.StatusInternalServerError)
			fmt.Println(err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"query":   query,
			"results": results,
			"count":   len(results),
		})
	})

	return nil
}

func serveAPI(c *cli.Context) error {
	address := c.String("address")
	handleAPI(c)
	log.Printf("Server started at %s\n", address)
	return http.ListenAndServe(address, nil)
}
