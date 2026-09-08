package util

import (
	"flag"
	"path/filepath"
	"testing"

	"github.com/rubiojr/hashup/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestLoadConfigFromCLIAppliesSubject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, config.SaveConfig(config.DefaultConfig(), path))
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.String("config", "", "")
	set.String("subject", "", "")
	require.NoError(t, set.Set("config", path))
	require.NoError(t, set.Set("subject", "LAPTOP_FILES"))

	cfg, err := LoadConfigFromCLI(cli.NewContext(&cli.App{}, set, nil))

	require.NoError(t, err)
	assert.Equal(t, "LAPTOP_FILES", cfg.Main.NatsSubject)
}
