package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rubiojr/hashup/internal/config"
	"github.com/rubiojr/hashup/internal/log"
	"github.com/rubiojr/hashup/internal/store"
	"github.com/urfave/cli/v2"

	_ "github.com/mattn/go-sqlite3"
)

func runStore(clictx *cli.Context) error {
	cfg, err := config.LoadConfigFromCLI(clictx)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize statistics
	stats := store.NewProcessStats()
	stats.StartStatsPrinters(ctx, cfg.Store.StatsInterval)

	opts := []store.Option{
		store.WithStats(stats),
		store.WithNatsStream(cfg.Main.NatsStream),
		store.WithNatsSubject(cfg.Main.NatsSubject),
		store.WithNatsURL(cfg.Main.NatsServerURL),
	}
	st, err := store.NewSqliteStore(cfg.Store.DBPath, cfg.Main.EncryptionKey, opts...)
	if err != nil {
		return err
	}

	log.Printf("Listening for files on %s...\n", cfg.Main.NatsSubject)
	log.Printf("Saving data to %s\n", cfg.Store.DBPath)
	if cfg.Store.StatsInterval > 0 {
		log.Printf("Statistics will be printed every %d seconds\n", cfg.Store.StatsInterval)
	}

	// Wait for interrupt signal
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		st.Listen(ctx)
		signalCh <- syscall.SIGINT
	}()

	<-signalCh

	// Print final stats before exiting
	fmt.Println("\n\nFinal statistics:")
	stats.PrintStats()

	cancel()
	log.Debug("Shutting down...")

	return nil
}
