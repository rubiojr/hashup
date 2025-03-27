package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rubiojr/hashup/internal/config"
)

func setupConfig(force bool) error {
	configDir, err := config.DefaultConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %v", err)
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	if err := writeEmbeddedFile("configs/nats.conf", filepath.Join(configDir, "nats.conf"), force); err != nil {
		return err
	}

	if err := writeEmbeddedFile("configs/hashup.toml", filepath.Join(configDir, "config.toml"), force); err != nil {
		return err
	}

	dataDir := config.DefaultNATSDataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create NATS data directory: %v", err)
	}

	dbDir := config.DefaultDBDir()
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %v", err)
	}

	fmt.Println("Configuration files written successfully:")
	fmt.Printf("  Config directory: %s\n", configDir)
	fmt.Printf("  NATS config: %s\n", filepath.Join(configDir, "nats.conf"))
	fmt.Printf("  HashUp config: %s\n", filepath.Join(configDir, "config.toml"))
	fmt.Printf("  NATS data directory: %s\n", dataDir)
	fmt.Printf("  Database directory: %s\n", dbDir)

	return nil
}

func writeEmbeddedFile(embeddedPath, targetPath string, force bool) error {
	// Check if file already exists
	if !force {
		if _, err := os.Stat(targetPath); err == nil {
			return fmt.Errorf("file already exists: %s (use --force to overwrite)", targetPath)
		}
	}

	// Open embedded file
	file, err := configFiles.Open(embeddedPath)
	if err != nil {
		return fmt.Errorf("failed to open embedded file %s: %v", embeddedPath, err)
	}
	defer file.Close()

	// Create target file
	targetFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", targetPath, err)
	}
	defer targetFile.Close()

	// Copy contents
	if _, err := io.Copy(targetFile, file); err != nil {
		return fmt.Errorf("failed to write file %s: %v", targetPath, err)
	}

	return nil
}
