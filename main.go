package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"filippo.io/age"
	"github.com/rubiojr/hashup/internal/api"
	"github.com/rubiojr/hashup/internal/config"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

//go:embed configs
var configFiles embed.FS

func main() {
	app := &cli.App{
		Name:  "hashup",
		Usage: "File inventory tool",
		Commands: []*cli.Command{
			{
				Name:  "keygen",
				Usage: "Generate encryption keys",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "config",
						Usage: "Path to the configuration file",
					},
				},
				Action: func(c *cli.Context) error {
					out := os.Stdout
					k, err := age.GenerateX25519Identity()
					if err != nil {
						return fmt.Errorf("internal error: %v", err)
					}

					if !term.IsTerminal(int(out.Fd())) {
						fmt.Fprintf(os.Stderr, "Public key: %s\n", k.Recipient())
					}

					fmt.Fprintf(out, "# created: %s\n", time.Now().Format(time.RFC3339))
					fmt.Fprintf(out, "# public key: %s\n", k.Recipient())
					fmt.Fprintf(out, "%s\n", k)

					return nil
				},
			},
			{
				Name:  "api",
				Usage: "Serve index API",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "address",
						Usage: "Address to listen on",
						Value: "localhost:8448",
					},
					&cli.IntFlag{
						Name:  "limit",
						Usage: "Maximum number of results to return",
						Value: 100,
					},
					&cli.StringFlag{
						Name:  "config",
						Usage: "Path to the configuration file",
					},
				},
				Action: func(c *cli.Context) error {
					cfgPath := c.String("config")
					if cfgPath == "" {
						cfgDir, err := config.DefaultConfigDir()
						if err != nil {
							return err
						}
						cfgPath = filepath.Join(cfgDir, "config.toml")
					}
					return api.Serve(cfgPath, c.String("address"))
				},
			},
			{
				Name:    "scan",
				Aliases: []string{"i"},
				Usage:   "Scan files recursively",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "config",
						Usage: "Path to the configuration file",
						Value: "",
					},
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
					&cli.StringFlag{
						Name:  "client-cert",
						Usage: "TLS client key",
					},
					&cli.StringFlag{
						Name:  "client-key",
						Usage: "TLS client cert",
					},
					&cli.StringFlag{
						Name:  "ca-cert",
						Usage: "TLS CA cert",
					},
					&cli.StringFlag{
						Name:  "every",
						Usage: "Run the scanner regularly. Interval specified in seconds(s), minutes(m) or hours(h)",
					},
				},
				Action: func(c *cli.Context) error {
					if c.Bool("debug") {
						os.Setenv("HASHUP_DEBUG", "1")
					}
					if c.String("every") != "" {
						return runEvery(c)
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
						Name:  "config",
						Usage: "Path to the configuration file",
						Value: "",
					},
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
					&cli.StringFlag{
						Name:  "client-cert",
						Usage: "TLS client key",
					},
					&cli.StringFlag{
						Name:  "client-key",
						Usage: "TLS client cert",
					},
					&cli.StringFlag{
						Name:  "ca-cert",
						Usage: "TLS CA cert",
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
			{
				Name:  "version",
				Usage: "HashUp version",
				Flags: []cli.Flag{},
				Action: func(c *cli.Context) error {
					if bi, ok := debug.ReadBuildInfo(); ok {
						fmt.Println(bi.Main.Version)
					}
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
}
