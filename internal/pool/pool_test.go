package pool

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStopReturnsTaskErrors(t *testing.T) {
	firstErr := errors.New("first task failed")
	secondErr := errors.New("second task failed")
	workerPool := NewPool(2)
	workerPool.Start()
	workerPool.Submit(func() error { return firstErr })
	workerPool.Submit(func() error { return secondErr })

	err := workerPool.Stop()

	assert.ErrorIs(t, err, firstErr)
	assert.ErrorIs(t, err, secondErr)
}
