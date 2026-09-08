package natsstream

import (
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeManager struct {
	info      *nats.StreamInfo
	infoErr   error
	addErr    error
	addConfig *nats.StreamConfig
}

func (m *fakeManager) StreamInfo(string, ...nats.JSOpt) (*nats.StreamInfo, error) {
	return m.info, m.infoErr
}

func (m *fakeManager) AddStream(config *nats.StreamConfig, _ ...nats.JSOpt) (*nats.StreamInfo, error) {
	m.addConfig = config
	if m.addErr != nil {
		return nil, m.addErr
	}
	return &nats.StreamInfo{Config: *config}, nil
}

func TestEnsureCreatesMissingStream(t *testing.T) {
	manager := &fakeManager{infoErr: nats.ErrStreamNotFound}

	require.NoError(t, Ensure(manager, "HASHUP", "FILES"))
	require.NotNil(t, manager.addConfig)
	assert.Equal(t, "HASHUP", manager.addConfig.Name)
	assert.Equal(t, []string{"FILES"}, manager.addConfig.Subjects)
	assert.Equal(t, nats.WorkQueuePolicy, manager.addConfig.Retention)
	assert.Equal(t, nats.FileStorage, manager.addConfig.Storage)
}

func TestEnsureUsesExistingCompatibleStream(t *testing.T) {
	manager := &fakeManager{info: &nats.StreamInfo{Config: *Config("HASHUP", "FILES")}}

	require.NoError(t, Ensure(manager, "HASHUP", "FILES"))
	assert.Nil(t, manager.addConfig)
}

func TestEnsureDoesNotCreateAfterLookupFailure(t *testing.T) {
	lookupErr := errors.New("permission denied")
	manager := &fakeManager{infoErr: lookupErr}

	err := Ensure(manager, "HASHUP", "FILES")

	assert.ErrorIs(t, err, lookupErr)
	assert.Nil(t, manager.addConfig)
}

func TestEnsureRejectsIncompatibleStream(t *testing.T) {
	tests := []struct {
		name   string
		config nats.StreamConfig
		error  string
	}{
		{
			name:   "missing subject",
			config: nats.StreamConfig{Name: "HASHUP", Subjects: []string{"OTHER"}, Retention: nats.WorkQueuePolicy},
			error:  `stream "HASHUP" does not include subject "FILES"`,
		},
		{
			name:   "wrong retention",
			config: nats.StreamConfig{Name: "HASHUP", Subjects: []string{"FILES"}, Retention: nats.LimitsPolicy},
			error:  `stream "HASHUP" must use work queue retention`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &fakeManager{info: &nats.StreamInfo{Config: tt.config}}

			err := Ensure(manager, "HASHUP", "FILES")

			assert.EqualError(t, err, tt.error)
			assert.Nil(t, manager.addConfig)
		})
	}
}
