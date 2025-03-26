package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/urfave/cli/v2"
)

func startEmbeddedNATSServer(c *cli.Context) error {
	opts := &server.Options{}

	//opts.Port, _ = strconv.Atoi(c.String("port"))
	//opts.HTTPPort, _ = strconv.Atoi(c.String("http-port"))

	// Configure debug and trace options
	opts.Debug = c.Bool("debug")
	opts.Trace = c.Bool("trace")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	opts.ConfigFile = c.String("config-file")
	if opts.ConfigFile == "" {
		opts.ConfigFile = filepath.Join(homeDir, ".config", "hashup", "nats.conf")
	}

	// Set up the data directory for JetStream if specified
	dataDir := c.String("data-dir")
	if dataDir == "" {
		dataDir = filepath.Join(homeDir, ".local", "share", "hashup", "nats")
	}

	if dataDir != "" {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return fmt.Errorf("failed to create data directory: %v", err)
		}

		opts.StoreDir = dataDir
	}

	opts.JetStream = true

	// Create and start the NATS server
	ns, err := server.NewServer(opts)
	if err != nil {
		return fmt.Errorf("failed to create NATS server: %v", err)
	}

	// Configure logging
	ns.ConfigureLogger()

	// Start the server
	go ns.Start()

	if !ns.ReadyForConnections(10 * time.Second) {
		return fmt.Errorf("NATS server failed to start in time")
	}

	// Print server info
	fmt.Printf("NATS server is running on port %d\n", opts.Port)
	if opts.HTTPPort > 0 {
		fmt.Printf("HTTP monitoring available on port %d\n", opts.HTTPPort)
	}
	if opts.JetStream {
		fmt.Printf("JetStream is enabled with storage in: %s\n", opts.StoreDir)
	}

	// Setup signal handler for graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Press Ctrl+C to stop the server")

	// Wait for signal
	<-signalChan

	fmt.Println("\nShutting down NATS server...")
	ns.Shutdown()

	return nil
}
