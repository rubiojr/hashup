package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rubiojr/hashup/internal/config"
	"github.com/rubiojr/hashup/internal/log"
	"github.com/rubiojr/hashup/internal/processors/nats"
	"github.com/rubiojr/hashup/internal/scanner"
	"github.com/urfave/cli/v2"
)

func runScanner(clictx *cli.Context) error {
	cfg, err := config.LoadConfigFromCLI(clictx)
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}
	encryptionKey := cfg.Main.EncryptionKey
	if encryptionKey == "" {
		return fmt.Errorf("encryption key is required")
	}

	natsServerURL := cfg.Main.NatsServerURL
	if natsServerURL == "" {
		return fmt.Errorf("nats server url is required")
	}

	rootDir := "./"
	if clictx.Args().Len() > 0 {
		rootDir = clictx.Args().Get(0)
	}

	if !clictx.Bool("debug") {
		log.SetOutput(io.Discard)
	}

	var fileCount int64
	// Count the number of files to be indexed
	go func() {
		tnow := time.Now()
		counter := FileCounter(rootDir)
		select {
		case fileCount = <-counter.Chan:
			elapsed := time.Since(tnow)
			log.Printf("Counted %d files in %s\n", fileCount, elapsed)
		}
	}()

	var ignoreList []string
	if clictx.String("ignore-file") != "" {
		var err error
		ignoreList, err = readIgnoreList(clictx.String("ignore-file"))
		if err != nil {
			return fmt.Errorf("failed to read ignore list: %v", err)
		}
	}

	scannerOpts := []scanner.Option{
		scanner.WithIgnoreList(ignoreList),
		scanner.WithIgnoreHidden(clictx.Bool("ignore-hidden")),
	}
	scanner := scanner.NewDirectoryScanner(rootDir, scannerOpts...)

	var pCounter int64
	go func() {
		for range scanner.CounterChan() {
			pCounter++
			if fileCount != 0 {
				percent := float64(pCounter) / float64(fileCount) * 100
				fmt.Printf("Scanned [%d/%d] files (%.0f%%)\r", pCounter, fileCount, percent)
			} else {
				fmt.Printf("Scanned %d files\r", pCounter)
			}
		}
	}()

	var processorOpts []nats.Option
	statsChan := make(chan nats.Stats, 1000)
	var processedFiles int64
	var skippedFiles int64
	var queuedFiles int64
	processorOpts = append(
		processorOpts,
		nats.WithEncryptionKey(encryptionKey),
		nats.WithStatsChannel(statsChan),
	)

	go func() {
		for stats := range statsChan {
			processedFiles++
			skippedFiles += int64(stats.SkippedFiles)
			queuedFiles += int64(stats.QueuedFiles)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	processor, err := nats.NewNATSProcessor(
		ctx,
		natsServerURL,
		cfg.Main.NatsStream,
		cfg.Main.NatsSubject,
		time.Second,
		processorOpts...,
	)
	if err != nil {
		return fmt.Errorf("failed to create NATS processor: %v", err)
	}
	defer processor.Close()

	signalCh := make(chan os.Signal, 1)
	go func() {
		startTime := time.Now()
		fmt.Printf("Starting directory scan in %s...\n", rootDir)

		count, err := scanner.ScanDirectory(ctx, processor)
		if err != nil {
			log.Errorf("error scanning directory: %v", err)
		}
		elapsed := time.Since(startTime)
		fmt.Printf("Completed scanning %d files in %q in %v\r", count, rootDir, elapsed)
		cancel()
		signalCh <- syscall.SIGINT
	}()

	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	<-signalCh
	log.Printf("Shutting down...")
	cancel()
	fmt.Printf(
		"Processed %d files, skipped %d files, queued %d files\n",
		processedFiles,
		skippedFiles,
		queuedFiles,
	)
	return nil
}

func readIgnoreList(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var ignoreList []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			if strings.HasPrefix(line, "~/") {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return nil, err
				}
				line = "^" + filepath.Join(homeDir, line[2:])
			}
			ignoreList = append(ignoreList, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return ignoreList, nil
}
