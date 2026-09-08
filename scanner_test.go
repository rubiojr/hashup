package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rubiojr/hashup/internal/crypto"
	"github.com/rubiojr/hashup/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestMainExitsNonzeroOnCommandError(t *testing.T) {
	if os.Getenv("HASHUP_TEST_MAIN_ERROR") == "1" {
		os.Args = []string{"hashup", "scan", "--config", os.Getenv("HASHUP_TEST_MISSING_CONFIG")}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainExitsNonzeroOnCommandError")
	cmd.Env = append(os.Environ(),
		"HASHUP_TEST_MAIN_ERROR=1",
		"HASHUP_TEST_MISSING_CONFIG="+filepath.Join(t.TempDir(), "missing.toml"),
	)
	err := cmd.Run()

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.NotZero(t, exitErr.ExitCode())
}

func TestScannerCacheNamespaceTracksDestination(t *testing.T) {
	_, privateKey, err := crypto.GenerateAgeKeyPair()
	require.NoError(t, err)
	_, otherPrivateKey, err := crypto.GenerateAgeKeyPair()
	require.NoError(t, err)

	base := &config.Config{Main: config.MainConfig{
		NatsServerURL: "tls://user:secret@server.example.com:4222",
		NatsStream:    "HASHUP",
		NatsSubject:   "FILES",
		EncryptionKey: privateKey,
	}}
	baseNamespace, err := scannerCacheNamespace(base, "laptop")
	require.NoError(t, err)

	tests := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{name: "server", mutate: func(cfg *config.Config) { cfg.Main.NatsServerURL = "tls://user:secret@other.example.com:4222" }},
		{name: "user", mutate: func(cfg *config.Config) { cfg.Main.NatsServerURL = "tls://other:secret@server.example.com:4222" }},
		{name: "stream", mutate: func(cfg *config.Config) { cfg.Main.NatsStream = "OTHER" }},
		{name: "subject", mutate: func(cfg *config.Config) { cfg.Main.NatsSubject = "OTHER" }},
		{name: "encryption recipient", mutate: func(cfg *config.Config) { cfg.Main.EncryptionKey = otherPrivateKey }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := *base
			tt.mutate(&changed)
			namespace, err := scannerCacheNamespace(&changed, "laptop")
			require.NoError(t, err)
			assert.NotEqual(t, baseNamespace, namespace)
		})
	}

	otherHost, err := scannerCacheNamespace(base, "other-laptop")
	require.NoError(t, err)
	assert.NotEqual(t, baseNamespace, otherHost)

	passwordChanged := *base
	passwordChanged.Main.NatsServerURL = "tls://user:different@server.example.com:4222"
	passwordNamespace, err := scannerCacheNamespace(&passwordChanged, "laptop")
	require.NoError(t, err)
	assert.Equal(t, baseNamespace, passwordNamespace, "passwords must not be part of cache identity")

	for _, address := range []string{"127.0.0.1:4222", "[::1]:4222"} {
		withoutScheme := *base
		withoutScheme.Main.NatsServerURL = address
		_, err := scannerCacheNamespace(&withoutScheme, "laptop")
		assert.NoError(t, err)
	}
}

func TestRunEveryRejectsNonpositiveInterval(t *testing.T) {
	for _, interval := range []string{"0s", "-1s"} {
		t.Run(interval, func(t *testing.T) {
			ctx, cancel := periodicScanContext(t, interval)
			defer cancel()

			err := runEveryWith(ctx, func(*cli.Context) error {
				t.Fatal("scan should not run for an invalid interval")
				return nil
			})

			assert.ErrorContains(t, err, "interval must be greater than zero")
		})
	}
}

func TestRunEveryScansImmediately(t *testing.T) {
	ctx, cancel := periodicScanContext(t, "1h")
	calls := 0

	err := runEveryWith(ctx, func(*cli.Context) error {
		calls++
		cancel()
		return nil
	})

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls)
}

func TestRunEveryRetriesAfterScheduledScanFailure(t *testing.T) {
	ctx, cancel := periodicScanContext(t, "1ms")
	defer cancel()
	scanErr := errors.New("scan failed")
	calls := 0

	err := runEveryWith(ctx, func(*cli.Context) error {
		calls++
		if calls == 1 {
			return nil
		}
		if calls == 2 {
			return scanErr
		}
		cancel()
		return nil
	})

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 3, calls)
}

func periodicScanContext(t *testing.T, interval string) (*cli.Context, context.CancelFunc) {
	t.Helper()
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.String("every", "", "")
	require.NoError(t, set.Set("every", interval))
	ctx, cancel := context.WithCancel(context.Background())
	cliContext := cli.NewContext(&cli.App{}, set, nil)
	cliContext.Context = ctx
	return cliContext, cancel
}

func TestScannerConcurrencyPrecedence(t *testing.T) {
	config := &config.Config{Scanner: config.ScannerConfig{ScanningConcurrency: 3}}

	ctx := scannerCLIContext(t, "")
	concurrency, err := scannerConcurrency(ctx, config)
	require.NoError(t, err)
	assert.Equal(t, 3, concurrency)

	ctx = scannerCLIContext(t, "2")
	concurrency, err = scannerConcurrency(ctx, config)
	require.NoError(t, err)
	assert.Equal(t, 2, concurrency)

	ctx = scannerCLIContext(t, "0")
	_, err = scannerConcurrency(ctx, config)
	assert.ErrorContains(t, err, "concurrency must be greater than zero")
}

func scannerCLIContext(t *testing.T, concurrency string) *cli.Context {
	t.Helper()
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.Int("concurrency", 0, "")
	if concurrency != "" {
		require.NoError(t, set.Set("concurrency", concurrency))
	}
	return cli.NewContext(&cli.App{}, set, nil)
}
