package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rubiojr/hashup/internal/log"
	"github.com/rubiojr/hashup/internal/store"
	"github.com/rubiojr/hashup/internal/util"
	"github.com/urfave/cli/v2"

	_ "github.com/mattn/go-sqlite3"
)

func runStore(clictx *cli.Context) error {
	cfg, err := util.LoadConfigFromCLI(clictx)
	if err != nil {
		return err
	}

	useTLS := cfg.Main.ClientKey != ""

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize statistics
	stats := store.NewProcessStats()
	stats.StartStatsPrinters(ctx, cfg.Store.StatsInterval)

	opts := []store.NATSListenerOption{
		store.WithStats(stats),
		store.WithNatsStream(cfg.Main.NatsStream),
		store.WithNatsSubject(cfg.Main.NatsSubject),
		store.WithNatsURL(cfg.Main.NatsServerURL),
	}

	if useTLS {
		opts = append(opts, store.WithClientCert(cfg.Main.ClientCert))
		opts = append(opts, store.WithClientKey(cfg.Main.ClientKey))
		opts = append(opts, store.WithCACert(cfg.Main.CACert))
	}

	storage, err := store.NewSqliteStorage(cfg.Store.DBPath)
	if err != nil {
		return err
	}

	listener, err := store.NewNatsListener(cfg.Main.EncryptionKey, storage, opts...)
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
		err := listener.Listen(ctx)
		if err != nil {
			log.Errorf("error listening for files: %v", err)
		}
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
