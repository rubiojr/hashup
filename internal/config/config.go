package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/urfave/cli/v2"
)

// Config represents the overall application configuration
type Config struct {
	Main    MainConfig    `toml:"main"`
	Store   StoreConfig   `toml:"store"`
	Scanner ScannerConfig `toml:"scanner"`
	Path    string
}

// MainConfig represents the main configuration section
type MainConfig struct {
	NatsServerURL string `toml:"nats_server_url"`
	EncryptionKey string `toml:"encryption_key"`
	NatsStream    string `toml:"nats_stream"`
	NatsSubject   string `toml:"nats_subject"`
	ClientCert    string `toml:"client_cert"`
	ClientKey     string `toml:"client_key"`
	CACert        string `toml:"ca_cert"`
}

// StoreConfig represents the store configuration section
type StoreConfig struct {
	StatsInterval int    `toml:"stats_interval"`
	DBPath        string `toml:"db_path"`
}

// ScannerConfig represents the scanner configuration section
type ScannerConfig struct {
	ScanningInterval    int    `toml:"scanning_interval"`
	ScanningConcurrency int    `toml:"scanning_concurrency"`
	CachePath           string `toml:"cache_path"`
}

func (c Config) NormalizePath(file string) string {
	if file == "" {
		return ""
	}

	if strings.HasPrefix(file, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			panic(err)
		}
		file = filepath.Join(homeDir, file[1:])
	}

	if filepath.IsAbs(file) {
		return file
	}

	return filepath.Join(filepath.Dir(c.Path), file)
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	cdir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	return &Config{
		Path: filepath.Join(cdir, "config.toml"),
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
		Scanner: ScannerConfig{
			ScanningInterval:    3600, // 1 hour in seconds
			ScanningConcurrency: 5,
			CachePath:           DefaultCachePath(),
		},
	}
}

// LoadConfig loads the configuration from the specified file path
func LoadConfig(path string) (*Config, error) {
	config := DefaultConfig()
	config.Path = path

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found")
	}

	_, err := toml.DecodeFile(path, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to decode config file: %v", err)
	}

	config.Main.ClientKey = config.NormalizePath(config.Main.ClientKey)
	config.Main.ClientCert = config.NormalizePath(config.Main.ClientCert)
	config.Main.CACert = config.NormalizePath(config.Main.CACert)
	config.Store.DBPath = config.NormalizePath(config.Store.DBPath)

	return config, nil
}

func LoadConfigFromCLI(ctx *cli.Context) (*Config, error) {
	var cfg *Config
	var err error
	if ctx.String("config") != "" {
		cfg, err = LoadConfig(ctx.String("config"))
	} else {
		cfg, err = LoadDefaultConfig()
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

func DefaultCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic("Failed to get user home directory")
	}

	dir := filepath.Join(home, ".cache", "hashup")

	err = os.MkdirAll(dir, 0755)
	if err != nil {
		panic("Failed to create cache directory")
	}

	return filepath.Join(dir, "cache")
}
