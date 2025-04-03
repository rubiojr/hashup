package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	_ "github.com/mattn/go-sqlite3"
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

		results, err := hsdb.Search(db, query, []string{}, 100)
		if err != nil {
			statusJSON(http.StatusInternalServerError, err, w, r)
			return
		}

		render.JSON(w, r, results)
	})
}
