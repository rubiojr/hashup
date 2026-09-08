package nats

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rubiojr/hashup/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNATSProcessorCloseIsNilSafeAndIdempotent(t *testing.T) {
	processor := &natsProcessor{}

	assert.NotPanics(t, func() { assert.NoError(t, processor.Close()) })
	assert.NotPanics(t, func() { assert.NoError(t, processor.Close()) })
}

type failingCrypto struct{}

func (failingCrypto) Encrypt([]byte) ([]byte, error) { return nil, errors.New("encrypt failed") }
func (failingCrypto) Decrypt([]byte) ([]byte, error) { return nil, nil }

func TestNATSProcessorReportsFailedAttempt(t *testing.T) {
	statsChan := make(chan Stats, 1)
	processor := &natsProcessor{
		ctx:       context.Background(),
		encrypt:   true,
		crypto:    failingCrypto{},
		statsChan: statsChan,
	}

	err := processor.Process("file", types.ScannedFile{})
	stats := <-statsChan

	require.Error(t, err)
	assert.Equal(t, uint64(1), stats.AttemptedFiles)
	assert.Equal(t, uint64(1), stats.FailedFiles)
	assert.Zero(t, stats.QueuedFiles)
	assert.Equal(t, stats, processor.Stats())
}

func TestNATSProcessorReportsStatsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	statsChan := make(chan Stats, 1)
	processor := &natsProcessor{
		ctx:       ctx,
		encrypt:   true,
		crypto:    failingCrypto{},
		statsChan: statsChan,
	}

	require.Error(t, processor.Process("file", types.ScannedFile{}))

	assert.Equal(t, uint64(1), (<-statsChan).FailedFiles)
}

func TestNewNATSProcessorRejectsCanceledContextBeforeConnecting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewNATSProcessor(ctx, "nats://192.0.2.1:4222", "HASHUP", "FILES", time.Minute)

	assert.ErrorIs(t, err, context.Canceled)
}
