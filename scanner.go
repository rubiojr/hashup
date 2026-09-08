package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rubiojr/hashup/internal/cache"
	hscrypto "github.com/rubiojr/hashup/internal/crypto"
	"github.com/rubiojr/hashup/internal/log"
	"github.com/rubiojr/hashup/internal/processors/nats"
	"github.com/rubiojr/hashup/internal/scanner"
	"github.com/rubiojr/hashup/internal/util"
	"github.com/rubiojr/hashup/pkg/config"
	"github.com/urfave/cli/v2"
)

func runEvery(c *cli.Context) error {
	return runEveryWith(c, runScanner)
}

func runEveryWith(c *cli.Context, scan func(*cli.Context) error) error {
	d, err := time.ParseDuration(c.String("every"))
	if err != nil {
		return fmt.Errorf("failed to parse duration: %w", err)
	}
	if d <= 0 {
		return fmt.Errorf("scan interval must be greater than zero")
	}

	if err := scan(c); err != nil {
		return err
	}

	ticker := time.NewTicker(d)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := scan(c)
			if err != nil {
				return fmt.Errorf("failed to run scanner: %w", err)
			}
		case <-c.Context.Done():
			return c.Context.Err()
		}
	}
}

func runScanner(clictx *cli.Context) error {
	cfg, err := util.LoadConfigFromCLI(clictx)
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

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	cacheNamespace, err := scannerCacheNamespace(cfg, hostname)
	if err != nil {
		return fmt.Errorf("failed to identify scanner destination: %w", err)
	}
	concurrency, err := scannerConcurrency(clictx, cfg)
	if err != nil {
		return err
	}

	var ignoreList []string
	if clictx.String("ignore-file") != "" {
		var err error
		ignoreList, err = readIgnoreList(clictx.String("ignore-file"))
		if err != nil {
			return fmt.Errorf("failed to read ignore list: %v", err)
		}
	}

	var fileCount int64
	// Count and print the number of files to be indexed
	go func() {
		tnow := time.Now()
		fileCount = <-FileCounter(clictx.Context, rootDir).Chan
		elapsed := time.Since(tnow)
		log.Debugf("Counted %d files in %s\n", fileCount, elapsed)
	}()

	scannerOpts := []scanner.Option{
		scanner.WithIgnoreList(ignoreList),
		scanner.WithIgnoreHidden(clictx.Bool("ignore-hidden")),
		scanner.WithCache(cache.NewFileCache(100, cfg.Scanner.CachePath)),
		scanner.WithCacheNamespace(cacheNamespace),
		scanner.WithForce(clictx.Bool("force")),
		scanner.WithScanningConcurrency(concurrency),
	}
	scanner := scanner.NewDirectoryScanner(rootDir, scannerOpts...)

	var pCounter int64
	counterChan := scanner.CounterChan()
	go func() {
		for {
			select {
			case <-clictx.Done():
				return
			case <-counterChan:
				pCounter++
				if fileCount != 0 {
					percent := float64(pCounter) / float64(fileCount) * 100
					fmt.Printf("Scanned [%d/%d] files (%.0f%%)\r", pCounter, fileCount, percent)
				} else {
					fmt.Printf("Scanned %d files\r", pCounter)
				}
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

	if cfg.Main.ClientKey != "" {
		processorOpts = append(processorOpts,
			nats.WithClientKey(cfg.Main.ClientKey),
			nats.WithClientCert(cfg.Main.ClientCert),
			nats.WithCACert(cfg.Main.CACert),
		)
	}

	go func() {
		for {
			select {
			case <-clictx.Done():
				return
			case stats := <-statsChan:
				processedFiles++
				skippedFiles += int64(stats.SkippedFiles)
				queuedFiles += int64(stats.QueuedFiles)
			}
		}
	}()

	ctx, cancel := context.WithCancel(clictx.Context)
	defer cancel()
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

	startTime := time.Now()
	fmt.Printf("Starting directory scan in %s...\n", rootDir)
	count, scanErr := scanner.ScanDirectory(ctx, processor)
	processor.Close()
	cancel()
	if scanErr != nil {
		log.Errorf("error scanning directory: %v", scanErr)
	}
	elapsed := time.Since(startTime)
	fmt.Printf("Completed scanning %d files in %q in %v\r\n", count, rootDir, elapsed)
	fmt.Printf(
		"Processed %d files, skipped %d files, queued %d files\n",
		processedFiles,
		skippedFiles,
		queuedFiles,
	)

	return scanErr
}

func scannerConcurrency(c *cli.Context, cfg *config.Config) (int, error) {
	concurrency := cfg.Scanner.ScanningConcurrency
	if c.IsSet("concurrency") {
		concurrency = c.Int("concurrency")
	}
	if concurrency <= 0 {
		return 0, fmt.Errorf("scanning concurrency must be greater than zero")
	}
	return concurrency, nil
}

func scannerCacheNamespace(cfg *config.Config, hostname string) (string, error) {
	recipient, err := hscrypto.DerivePublicKey(cfg.Main.EncryptionKey)
	if err != nil {
		return "", err
	}

	serverURLs := strings.Split(cfg.Main.NatsServerURL, ",")
	for i, rawURL := range serverURLs {
		serverURL, err := url.Parse(strings.TrimSpace(rawURL))
		if err != nil {
			return "", fmt.Errorf("parse NATS URL: %w", err)
		}
		if serverURL.User != nil {
			serverURL.User = url.User(serverURL.User.Username())
		}
		serverURLs[i] = serverURL.String()
	}

	identity := strings.Join([]string{
		"hashup-scanner-v1",
		strings.Join(serverURLs, ","),
		cfg.Main.NatsStream,
		cfg.Main.NatsSubject,
		recipient,
		hostname,
	}, "\x00")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(identity))), nil
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
