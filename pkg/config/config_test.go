package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/assert"

	"github.com/rubiojr/hashup/pkg/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()

	homeDir, _ := os.UserHomeDir()
	expectedDBPath := filepath.Join(homeDir, ".local", "share", "hashup", "hashup.db")
	expectedCachePath := filepath.Join(homeDir, ".cache", "hashup", "cache")

	assert.Equal(t, "http://localhost:4222", cfg.Main.NatsServerURL)
	assert.Equal(t, "HASHUP", cfg.Main.NatsStream)
	assert.Equal(t, "FILES", cfg.Main.NatsSubject)
	assert.Equal(t, 30, cfg.Store.StatsInterval)
	assert.Equal(t, expectedDBPath, cfg.Store.DBPath)
	assert.Equal(t, 3600, cfg.Scanner.ScanningInterval)
	assert.Equal(t, 5, cfg.Scanner.ScanningConcurrency)
	assert.Equal(t, expectedCachePath, cfg.Scanner.CachePath)
}

func TestNormalizePath(t *testing.T) {
	homeDir, _ := os.UserHomeDir()
	cfg := config.Config{Path: "/some/config/path/config.toml"}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Empty path", "", ""},
		{"Relative path", "relative/path", "/some/config/path/relative/path"},
		{"Absolute path", "/absolute/path", "/absolute/path"},
		{"Home tilde path", "~/something", filepath.Join(homeDir, "something")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cfg.NormalizePath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test_config.toml")

	testCfg := config.DefaultConfig()
	testCfg.Main.NatsServerURL = "nats://testserver:4222"
	testCfg.Store.StatsInterval = 60

	f, err := os.Create(configPath)
	assert.NoError(t, err)
	defer f.Close()

	err = toml.NewEncoder(f).Encode(testCfg)
	assert.NoError(t, err)
	f.Close()

	// Test loading the config
	cfg, err := config.LoadConfig(configPath)
	assert.NoError(t, err)
	assert.Equal(t, "nats://testserver:4222", cfg.Main.NatsServerURL)
	assert.Equal(t, 60, cfg.Store.StatsInterval)

	// Test loading non-existent config
	_, err = config.LoadConfig(filepath.Join(tempDir, "nonexistent.toml"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config file not found")
}

func TestSaveConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")

	cfg := config.DefaultConfig()
	cfg.Main.NatsServerURL = "nats://customserver:4222"

	err := config.SaveConfig(cfg, configPath)
	assert.NoError(t, err)

	// Verify saved config can be loaded correctly
	loadedCfg, err := config.LoadConfig(configPath)
	assert.NoError(t, err)
	assert.Equal(t, "nats://customserver:4222", loadedCfg.Main.NatsServerURL)
}

func TestDefaultPaths(t *testing.T) {
	homeDir, _ := os.UserHomeDir()

	configDir, err := config.DefaultConfigDir()
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(homeDir, ".config", "hashup"), configDir)

	dbPath := config.DefaultDBPath()
	assert.Equal(t, filepath.Join(homeDir, ".local", "share", "hashup", "hashup.db"), dbPath)

	dbDir := config.DefaultDBDir()
	assert.Equal(t, filepath.Join(homeDir, ".local", "share", "hashup"), dbDir)

	natsDir := config.DefaultNATSDataDir()
	assert.Equal(t, filepath.Join(homeDir, ".local", "share", "hashup", "nats"), natsDir)

	cachePath := config.DefaultCachePath()
	assert.Equal(t, filepath.Join(homeDir, ".cache", "hashup", "cache"), cachePath)
}
