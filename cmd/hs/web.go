package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rubiojr/hashup/cmd/hs/types"
	"github.com/rubiojr/hashup/internal/config"
	"github.com/rubiojr/hashup/internal/templates"
	"github.com/urfave/cli/v2"
)

type SearchResponse struct {
	Count   int                `json:"count"`
	Query   string             `json:"query"`
	Results []types.FileResult `json:"results"`
}

func healthHandlers() {
	http.HandleFunc("/health/nats", func(w http.ResponseWriter, r *http.Request) {
		config, err := config.LoadDefaultConfig()
		if err != nil {
			http.Error(w, fmt.Errorf("Error loading config: %w", err).Error(), http.StatusInternalServerError)
			return
		}

		_, err = nats.Connect(config.Main.NatsServerURL)
		if err != nil {
			http.Error(w, fmt.Errorf("Error connecting to NATS server: %w", err).Error(), http.StatusInternalServerError)
		}
		fmt.Fprintf(w, "NATS server is healthy")
	})
}

func fileStatsHandler() {
	http.HandleFunc("/stats/files", func(w http.ResponseWriter, r *http.Request) {
		dbConn, err := dbConn("")
		if err != nil {
			http.Error(w, fmt.Errorf("failed to get database connection: %v", err).Error(), http.StatusInternalServerError)
			return
		}
		defer dbConn.Close()

		stats, err := fileStats(dbConn, "file_size", true, "")
		if err != nil {
			http.Error(w, fmt.Errorf("failed to get file stats: %v", err).Error(), http.StatusInternalServerError)
			return
		}

		jsonData, err := jsonStats(stats, "", 10)
		if err != nil {
			http.Error(w, fmt.Errorf("failed to generate JSON stats: %v", err).Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "%s", jsonData)
	})
}

func natsStreamInfoHandler() error {
	http.HandleFunc("/stats/nats/stream/info", func(w http.ResponseWriter, r *http.Request) {
		config, err := config.LoadDefaultConfig()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		nc, err := nats.Connect(config.Main.NatsServerURL)
		if err != nil {
			http.Error(w, fmt.Errorf("Error connecting to NATS server: %w", err).Error(), http.StatusInternalServerError)
		}

		js, _ := jetstream.New(nc)
		if err != nil {
			http.Error(w, fmt.Errorf("Error creating JetStream management interface: %w", err).Error(), http.StatusInternalServerError)
			return
		}

		stream, err := js.Stream(ctx, config.Main.NatsStream)
		if err != nil {
			http.Error(w, fmt.Errorf("Error creating stream: %w", err).Error(), http.StatusInternalServerError)
			return
		}
		streamInfo, err := stream.Info(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		clusterInfo := streamInfo.Cluster
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		info := struct {
			StreamName    string `json:"stream_name"`
			ClusterName   string `json:"cluster_name"`
			ClusterLeader string `json:"cluster_leader"`
			Messages      int64  `json:"messages"`
			Bytes         int64  `json:"bytes"`
			ConsumerCount int64  `json:"consumer_count"`
		}{
			StreamName:    streamInfo.Config.Name,
			ClusterName:   clusterInfo.Name,
			ClusterLeader: clusterInfo.Leader,
			Messages:      int64(streamInfo.State.Msgs),
			Bytes:         int64(streamInfo.State.Bytes),
			ConsumerCount: int64(streamInfo.State.Consumers),
		}

		jsonState, err := json.Marshal(info)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "%s", jsonState)
	})
	return nil
}

func serveWeb(c *cli.Context) {
	addr := c.String("address")
	extensions := strings.Split(c.String("extensions"), ",")

	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	dbPath, err := getDBPath()
	if err != nil {
		panic(err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		templates.Index().Render(r.Context(), w)
	})

	http.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		results, err := dbSearch(db, query, extensions, 5)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			fmt.Println(err)
			return
		}

		templates.Results(results).Render(r.Context(), w)
	})

	log.Printf("Server started at %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func commandServeWeb() *cli.Command {
	return &cli.Command{
		Name:  "web",
		Usage: "Serve web search",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "host",
				Usage:    "Filter by host",
				Value:    "",
				Required: false,
			},
			&cli.StringFlag{
				Name:  "address",
				Usage: "Address to listen on",
				Value: ":8449",
			},
			&cli.StringFlag{
				Name:  "extensions",
				Usage: "Comma separated list of file extensions to include in search results",
				Value: "",
			},
			&cli.IntFlag{
				Name:  "limit",
				Usage: "Maximum number of results to return",
				Value: 100,
			},
		},
		Action: func(c *cli.Context) error {
			err := natsStreamInfoHandler()
			if err != nil {
				return fmt.Errorf("Error handling NATS stream info: %w", err)
			}
			healthHandlers()
			handleAPI(c)
			fileStatsHandler()
			serveWeb(c)
			return nil
		},
	}
}
