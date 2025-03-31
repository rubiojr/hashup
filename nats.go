package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/urfave/cli/v2"
)

func startEmbeddedNATSServer(c *cli.Context) error {
	opts := &server.Options{}

	opts.Debug = c.Bool("debug")
	opts.Trace = c.Bool("trace")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	conf := c.String("config")
	if conf == "" {
		conf = filepath.Join(homeDir, ".config", "hashup", "nats.conf")
	}
	opts.ConfigFile = conf
	err = opts.ProcessConfigFile(conf)
	if err != nil {
		return fmt.Errorf("failed to process config file: %v", err)
	}

	if c.Int("port") != 0 {
		opts.Port = c.Int("port")
	}

	if c.Int("http-port") != 0 {
		opts.HTTPPort = c.Int("http-port")
	}

	dataDir := c.String("data-dir")
	if dataDir != "" {
		opts.StoreDir = dataDir
	}

	if opts.StoreDir == "" {
		opts.StoreDir = filepath.Join(homeDir, ".local", "share", "hashup", "nats")
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %v", err)
	}

	// This is required for HashUp
	opts.JetStream = true

	ns, err := server.NewServer(opts)
	if err != nil {
		return fmt.Errorf("failed to create NATS server: %v", err)
	}

	ns.ConfigureLogger()

	go ns.Start()

	if !ns.ReadyForConnections(10 * time.Second) {
		return fmt.Errorf("NATS server failed to start in time")
	}

	fmt.Printf("NATS server is running on port %d\n", opts.Port)
	if opts.HTTPPort > 0 {
		fmt.Printf("HTTP monitoring available on port %d\n", opts.HTTPPort)
	}
	if opts.JetStream {
		fmt.Printf("JetStream is enabled with storage in: %s\n", opts.StoreDir)
	}

	fmt.Println("Press Ctrl+C to stop the server")
	ns.WaitForShutdown()

	fmt.Println("\nShutting down NATS server...")

	return nil
}
