package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/urfave/cli/v2"
)

// Config represents the overall application configuration
type Config struct {
	Main    MainConfig    `toml:"main"`
	Store   StoreConfig   `toml:"store"`
	Indexer IndexerConfig `toml:"indexer"`
}

// MainConfig represents the main configuration section
type MainConfig struct {
	NatsServerURL string `toml:"nats_server_url"`
	EncryptionKey string `toml:"encryption_key"`
	NatsStream    string `toml:"nats_stream"`
	NatsSubject   string `toml:"nats_subject"`
}

// StoreConfig represents the store configuration section
type StoreConfig struct {
	StatsInterval int    `toml:"stats_interval"`
	DBPath        string `toml:"db_path"`
}

// IndexerConfig represents the indexer configuration section
type IndexerConfig struct {
	IndexingInterval    int `toml:"indexing_interval"`
	IndexingConcurrency int `toml:"indexing_concurrency"`
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() Config {
	return Config{
		Main: MainConfig{
			NatsServerURL: "http://localhost:4222",
			EncryptionKey: "",
			NatsStream:    "HASHUP",
			NatsSubject:   "FILES",
		},
		Store: StoreConfig{
			StatsInterval: 30,
			DBPath:        DefaultDBPath(),
		},
		Indexer: IndexerConfig{
			IndexingInterval:    3600, // 1 hour in seconds
			IndexingConcurrency: 5,
		},
	}
}

// LoadConfig loads the configuration from the specified file path
func LoadConfig(path string) (*Config, error) {
	config := DefaultConfig()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found")
	}

	_, err := toml.DecodeFile(path, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to decode config file: %v", err)
	}

	return &config, nil
}

func LoadConfigFromCLI(ctx *cli.Context) (*Config, error) {
	cfg, err := LoadDefaultConfig()
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

	return cfg, nil
}

// LoadDefaultConfig loads the configuration from the default path
func LoadDefaultConfig() (*Config, error) {
	configDir, err := DefaultConfigDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(configDir, "config.toml")
	return LoadConfig(configPath)
}

// SaveConfig saves the configuration to the specified file path
func SaveConfig(config *Config, path string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %v", err)
	}
	defer file.Close()

	encoder := toml.NewEncoder(file)
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to encode config: %v", err)
	}

	return nil
}

// SaveDefaultConfig saves the configuration to the default path
func SaveDefaultConfig(config *Config) error {
	configDir, err := DefaultConfigDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "config.toml")
	return SaveConfig(config, configPath)
}

// DefaultConfigDir returns the configuration directory path
func DefaultConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %v", err)
	}

	return filepath.Join(homeDir, ".config", "hashup"), nil
}

// DefaultDBPath returns the default database path
func DefaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return filepath.Join(home, ".local", "share", "hashup", "hashup.db")
}

// DefaultDBDir returns the default database directory path
func DefaultDBDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return filepath.Join(home, ".local", "share", "hashup")
}

// DefaultNATSDataDir returns the default NATS data directory path
func DefaultNATSDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return filepath.Join(home, ".local", "share", "hashup", "nats")
}
