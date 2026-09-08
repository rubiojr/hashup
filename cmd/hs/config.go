package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/urfave/cli/v2"
)

const hsConfigMetadataKey = "hs-config"

type hsConfig struct {
	Main hsMainConfig `toml:"main"`
}

type hsMainConfig struct {
	APIServerURL string `toml:"api_server_url"`
}

func defaultHSConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, ".config", "hashup", "hs.toml"), nil
}

func loadHSConfig(path string, required bool) (*hsConfig, error) {
	config := &hsConfig{}
	if _, err := toml.DecodeFile(path, config); err != nil {
		if !required && errors.Is(err, os.ErrNotExist) {
			return config, nil
		}
		return nil, fmt.Errorf("load hs config %q: %w", path, err)
	}
	return config, nil
}

func configureHS(c *cli.Context) error {
	path := c.String("config")
	required := path != ""
	if path == "" {
		var err error
		path, err = defaultHSConfigPath()
		if err != nil {
			return err
		}
	}

	config, err := loadHSConfig(path, required)
	if err != nil {
		return err
	}
	c.App.Metadata[hsConfigMetadataKey] = config
	return nil
}

func configuredAPIServerURL(c *cli.Context) string {
	if serverURL := c.String("server-url"); serverURL != "" {
		return serverURL
	}
	config, _ := c.App.Metadata[hsConfigMetadataKey].(*hsConfig)
	if config == nil {
		return ""
	}
	return config.Main.APIServerURL
}
