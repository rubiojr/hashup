package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestLoadHSConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hs.toml")
	require.NoError(t, os.WriteFile(path, []byte("[main]\napi_server_url = \"https://hashup.example.com\""), 0600))

	config, err := loadHSConfig(path, true)

	require.NoError(t, err)
	assert.Equal(t, "https://hashup.example.com", config.Main.APIServerURL)
}

func TestLoadHSConfigAllowsMissingDefault(t *testing.T) {
	config, err := loadHSConfig(filepath.Join(t.TempDir(), "missing.toml"), false)

	require.NoError(t, err)
	assert.Empty(t, config.Main.APIServerURL)
}

func TestLoadHSConfigRequiresExplicitPath(t *testing.T) {
	_, err := loadHSConfig(filepath.Join(t.TempDir(), "missing.toml"), true)

	assert.ErrorContains(t, err, "load hs config")
}

func TestConfiguredAPIServerURLPrecedence(t *testing.T) {
	tests := []struct {
		name        string
		flagURL     string
		configURL   string
		expectedURL string
	}{
		{name: "flag", flagURL: "https://flag.example.com", configURL: "https://config.example.com", expectedURL: "https://flag.example.com"},
		{name: "config", configURL: "https://config.example.com", expectedURL: "https://config.example.com"},
		{name: "local database fallback", expectedURL: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := flag.NewFlagSet("test", flag.ContinueOnError)
			set.String("server-url", "", "")
			require.NoError(t, set.Set("server-url", tt.flagURL))
			app := &cli.App{Metadata: map[string]any{
				hsConfigMetadataKey: &hsConfig{Main: hsMainConfig{APIServerURL: tt.configURL}},
			}}
			ctx := cli.NewContext(app, set, nil)

			assert.Equal(t, tt.expectedURL, configuredAPIServerURL(ctx))
		})
	}
}
