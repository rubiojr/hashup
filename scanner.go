package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
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
		if c.Context.Err() != nil {
			return joinContextError(err, c.Context.Err())
		}
		fmt.Fprintf(os.Stderr, "failed to run scanner: %v\n", err)
	}
	if err := c.Context.Err(); err != nil {
		return err
	}

	ticker := time.NewTicker(d)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.Context.Err(); err != nil {
				return err
			}
			err := scan(c)
			if err != nil {
				if c.Context.Err() != nil {
					return joinContextError(err, c.Context.Err())
				}
				fmt.Fprintf(os.Stderr, "failed to run scanner: %v\n", err)
			}
			if err := c.Context.Err(); err != nil {
				return err
			}
		case <-c.Context.Done():
			return c.Context.Err()
		}
	}
}

func joinContextError(err, contextErr error) error {
	if errors.Is(err, contextErr) {
		return err
	}
	return errors.Join(err, contextErr)
}

func runScannerCommand(c *cli.Context) error {
	ctx, stop := signal.NotifyContext(c.Context, os.Interrupt, syscall.SIGTERM)
	defer stop()
	c.Context = ctx
	go func() {
		<-ctx.Done()
		stop()
	}()
	if c.String("every") != "" {
		return runEvery(c)
	}
	return runScanner(c)
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

	fileCache := cache.NewFileCache(100, cfg.Scanner.CachePath)
	defer fileCache.Close()
	scannerOpts := []scanner.Option{
		scanner.WithIgnoreList(ignoreList),
		scanner.WithIgnoreHidden(clictx.Bool("ignore-hidden")),
		scanner.WithCache(fileCache),
		scanner.WithCacheNamespace(cacheNamespace),
		scanner.WithForce(clictx.Bool("force")),
		scanner.WithScanningConcurrency(concurrency),
	}
	scanner := scanner.NewDirectoryScanner(rootDir, scannerOpts...)

	var processorOpts []nats.Option
	processorOpts = append(
		processorOpts,
		nats.WithEncryptionKey(encryptionKey),
	)

	if cfg.Main.ClientKey != "" {
		processorOpts = append(processorOpts,
			nats.WithClientKey(cfg.Main.ClientKey),
			nats.WithClientCert(cfg.Main.ClientCert),
			nats.WithCACert(cfg.Main.CACert),
		)
	}

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
		return fmt.Errorf("failed to create NATS processor: %w", err)
	}

	progressChan := scanner.CounterChan()
	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		for count := range progressChan {
			fmt.Printf("Scanned %d files\r", count)
		}
	}()

	startTime := time.Now()
	fmt.Printf("Starting directory scan in %s...\n", rootDir)
	count, scanErr := scanner.ScanDirectory(ctx, processor)
	closeErr := processor.Close()
	<-progressDone
	cancel()
	if scanErr != nil {
		log.Errorf("error scanning directory: %v", scanErr)
	}
	elapsed := time.Since(startTime)
	cacheStats := fileCache.GetStats()
	processorStats := processor.Stats()
	fmt.Printf("\rCompleted scanning %d files in %q in %v\r\n", count, rootDir, elapsed)
	fmt.Printf(
		"Cache hits %d, attempted %d files, failed %d files, queued %d files\n",
		cacheStats.Hits,
		processorStats.AttemptedFiles,
		processorStats.FailedFiles,
		processorStats.QueuedFiles,
	)

	return errors.Join(scanErr, closeErr)
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
		rawURL = strings.TrimSpace(rawURL)
		if !strings.Contains(rawURL, "://") {
			rawURL = "nats://" + rawURL
		}
		serverURL, err := url.Parse(rawURL)
		if err != nil {
			return "", fmt.Errorf("parse NATS URL: %w", err)
		}
		if serverURL.User != nil {
			serverURL.User = url.User(serverURL.User.Username())
		}
		serverURLs[i] = serverURL.String()
	}

	identity := strings.Join([]string{
		"hashup-scanner-v2",
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
