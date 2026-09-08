package nats

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNATSProcessorCloseIsNilSafeAndIdempotent(t *testing.T) {
	processor := &natsProcessor{}

	assert.NotPanics(t, processor.Close)
	assert.NotPanics(t, processor.Close)
}
