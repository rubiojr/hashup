package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

//go:embed configs
var configFiles embed.FS

func main() {
	app := &cli.App{
		Name:  "hashup",
		Usage: "File inventory tool",
		Commands: []*cli.Command{
			{
				Name:    "scan",
				Aliases: []string{"i"},
				Usage:   "Scan files recursively",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "nats-url",
						Usage:   "NATS URL",
						EnvVars: []string{"HASHUP_NATS_URL"},
					},
					&cli.BoolFlag{
						Name:  "debug",
						Value: false,
						Usage: "HASHUP_DEBUG",
					},
					&cli.StringFlag{
						Name:  "ignore-file",
						Value: "",
						Usage: "List of files to ignore when scanning",
					},
					&cli.BoolFlag{
						Name:  "ignore-hidden",
						Value: true,
						Usage: "Do not scann hidden files and directories",
					},
					&cli.IntFlag{
						Name:  "concurrency",
						Usage: "Number of concurrent workers",
					},
					&cli.StringFlag{
						Name:    "encryption-key",
						Usage:   "Key to use for encryption (if empty, a random key is generated)",
						EnvVars: []string{"HASHUP_ENCRYPTION_KEY"},
					},
				},
				Action: func(c *cli.Context) error {
					if c.Bool("debug") {
						os.Setenv("HASHUP_DEBUG", "1")
					}
					return runScanner(c)
				},
			},
			{
				Name:    "store",
				Aliases: []string{"s"},
				Usage:   "Start NATS consumer to store file metadata in the database",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "nats-url",
						Aliases: []string{"n"},
						Usage:   "NATS server URL",
						EnvVars: []string{"HASHUP_NATS_URL"},
					},
					&cli.StringFlag{
						Name:    "stream",
						Usage:   "Stream to subscribe to",
						EnvVars: []string{"HASHUP_NATS_STREAM"},
					},
					&cli.StringFlag{
						Name:    "subject",
						Usage:   "Subject to subscribe to",
						EnvVars: []string{"HASHUP_NATS_SUBJECT"},
					},
					&cli.StringFlag{
						Name:  "filter-host",
						Usage: "Only store files from the given host",
					},
					&cli.StringFlag{
						Name:    "db-path",
						Aliases: []string{"d"},
						Usage:   "Override default database path",
						EnvVars: []string{"HASHUP_DB_PATH"},
					},
					&cli.StringFlag{
						Name:    "encryption-key",
						Usage:   "Encryption key to decrypt messages",
						EnvVars: []string{"HASHUP_ENCRYPTION_KEY"},
					},
					&cli.BoolFlag{
						Name:    "debug",
						Usage:   "Debug mode",
						EnvVars: []string{"HASHUP_DEBUG"},
					},
					&cli.IntFlag{
						Name:    "stats-interval",
						Usage:   "Interval in seconds to print statistics (0 to disable)",
						EnvVars: []string{"HASHUP_STATS_INTERVAL"},
					},
				},
				Action: func(c *cli.Context) error {
					if c.Bool("debug") {
						os.Setenv("HASHUP_DEBUG", "1")
					}
					return runStore(c)
				},
			},
			{
				Name:  "nats",
				Usage: "Start embedded NATS server",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:  "port",
						Usage: "Port to listen on",
					},
					&cli.IntFlag{
						Name:  "http-port",
						Usage: "HTTP monitoring port (0 to disable)",
					},
					&cli.StringFlag{
						Name:  "config",
						Usage: "Path to the configuration file",
						Value: "",
					},
					&cli.StringFlag{
						Name:  "data-dir",
						Usage: "Directory to store NATS data",
						Value: "",
					},
					&cli.BoolFlag{
						Name:  "jetstream",
						Value: true,
						Usage: "Enable JetStream",
					},
					&cli.BoolFlag{
						Name:  "debug",
						Value: false,
						Usage: "Enable debug logging",
					},
					&cli.BoolFlag{
						Name:  "trace",
						Value: false,
						Usage: "Enable trace logging",
					},
				},
				Action: func(c *cli.Context) error {
					return startEmbeddedNATSServer(c)
				},
			},
			{
				Name:  "setup",
				Usage: "Setup initial configuration files",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "force",
						Usage: "Overwrite existing configuration files",
						Value: false,
					},
				},
				Action: func(c *cli.Context) error {
					return setupConfig(c.Bool("force"))
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
}
