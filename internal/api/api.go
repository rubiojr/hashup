package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rubiojr/hashup/cmd/hs/types"
	"github.com/rubiojr/hashup/internal/config"
	hsdb "github.com/rubiojr/hashup/internal/db"
)

func Serve(cfgPath string, addr string) error {
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("Failed to load config: %v", err)
	}
	dbPath := cfg.Store.DBPath

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(render.SetContentType(render.ContentTypeJSON))
	r.Get("/search", searchHandler(dbPath))
	http.ListenAndServe(addr, r)

	http.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
	})

	return nil
}

type Client struct {
	client    *http.Client
	serverURL string
}

func NewClient(serverURL string) *Client {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			IdleConnTimeout: 90 * time.Second,
			Dial: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).Dial,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}
	return &Client{client: client, serverURL: serverURL}
}

func (c *Client) Search(query string) ([]*types.FileResult, error) {
	// Build the URL with query parameters
	urlStr := fmt.Sprintf("%s/search?q=%s", c.serverURL, url.QueryEscape(query))

	// Create request
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		// Try to read error message
		body, _ := io.ReadAll(resp.Body)

		// Parse error response if possible
		var errorResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Error != "" {
			return nil, fmt.Errorf("server returned error: %s (status: %d)", errorResp.Error, resp.StatusCode)
		}

		return nil, fmt.Errorf("server returned non-OK status: %d", resp.StatusCode)
	}

	// Parse response body
	var results []*types.FileResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return results, nil
}

func statusJSON(code int, err error, w http.ResponseWriter, r *http.Request) {
	if err != nil {
		w.WriteHeader(code)
		render.JSON(w, r, map[string]string{
			"status": "error",
			"error":  err.Error(),
			"code":   strconv.Itoa(code),
		})
		return
	}

	render.JSON(w, r, map[string]string{
		"status": "ok",
		"code":   strconv.Itoa(code),
	})
}

func searchHandler(dbPath string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			statusJSON(http.StatusInternalServerError, err, w, r)
			return
		}
		defer db.Close()

		query := r.URL.Query().Get("q")
		if query == "" {
			statusJSON(http.StatusBadRequest, errors.New("q query parameter is required"), w, r)
			return
		}

		ext := r.URL.Query().Get("ext")
		exts := []string{}
		if ext != "" {
			exts = append(exts, ext)
		}

		host := r.URL.Query().Get("host")
		hosts := []string{}
		if host != "" {
			hosts = append(hosts, host)
		}

		results, err := hsdb.Search(db, query, exts, hosts, 100)
		if err != nil {
			statusJSON(http.StatusInternalServerError, err, w, r)
			return
		}

		render.JSON(w, r, results)
	})
}
