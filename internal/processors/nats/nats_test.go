package nats

import (
	"context"
	"errors"
	"testing"

	"github.com/rubiojr/hashup/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNATSProcessorCloseIsNilSafeAndIdempotent(t *testing.T) {
	processor := &natsProcessor{}

	assert.NotPanics(t, processor.Close)
	assert.NotPanics(t, processor.Close)
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
	assert.Equal(t, uint8(1), stats.AttemptedFiles)
	assert.Equal(t, uint8(1), stats.FailedFiles)
	assert.Zero(t, stats.QueuedFiles)
}
