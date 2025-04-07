package util

import (
	"fmt"
	"io"
	"os"

	"github.com/cespare/xxhash/v2"
	"github.com/rubiojr/hashup/pkg/config"
	"github.com/urfave/cli/v2"
)

// ComputeFileHash opens a file, streams its contents through an xxhash hasher,
// and returns the computed 64-bit hash in hexadecimal string format.
func ComputeFileHash(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := xxhash.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	// Convert the 64-bit hash to hexadecimal.
	return fmt.Sprintf("%016x", hasher.Sum64()), nil
}

func LoadConfigFromCLI(ctx *cli.Context) (*config.Config, error) {
	var cfg *config.Config
	var err error
	if ctx.String("config") != "" {
		cfg, err = config.LoadConfig(ctx.String("config"))
	} else {
		cfg, err = config.LoadDefaultConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load default config: %v", err)
	}

	encryptionKey := ctx.String("encryption-key")
	if encryptionKey != "" {
		cfg.Main.EncryptionKey = encryptionKey
	}

	dbPath := ctx.String("db-path")
	if dbPath != "" {
		cfg.Store.DBPath = dbPath
	}

	statsInterval := ctx.Int("stats-interval")
	if statsInterval != 0 {
		cfg.Store.StatsInterval = statsInterval
	}

	natsServer := ctx.String("nats-url")
	if natsServer != "" {
		cfg.Main.NatsServerURL = natsServer
	}

	streamName := ctx.String("stream")
	if streamName != "" {
		cfg.Main.NatsStream = streamName
	}

	clientCert := ctx.String("client-cert")
	if clientCert != "" {
		cfg.Main.ClientCert = cfg.NormalizePath(clientCert)
	}

	clientKey := ctx.String("client-key")
	if clientKey != "" {
		cfg.Main.ClientKey = cfg.NormalizePath(clientKey)
	}

	caCert := ctx.String("ca-cert")
	if caCert != "" {
		cfg.Main.CACert = cfg.NormalizePath(caCert)
	}

	return cfg, nil
}
