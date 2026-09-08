package store

import (
	"context"
	"testing"
	"time"

	"filippo.io/age"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/rubiojr/hashup/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNatsListenerCreatesMissingStream(t *testing.T) {
	server, err := natsserver.NewServer(&natsserver.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	})
	require.NoError(t, err)
	server.Start()
	t.Cleanup(server.Shutdown)
	require.True(t, server.ReadyForConnections(5*time.Second))

	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	listener, err := NewNatsListener(
		identity.String(),
		&recordingStorage{},
		WithNatsURL(server.ClientURL()),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- listener.Listen(ctx)
	}()

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	js, err := nc.JetStream()
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err := js.StreamInfo("HASHUP")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	info, err := js.StreamInfo("HASHUP")
	require.NoError(t, err)
	assert.Equal(t, []string{"FILES"}, info.Config.Subjects)
	assert.Equal(t, nats.WorkQueuePolicy, info.Config.Retention)

	cancel()
	require.NoError(t, <-done)
}

type recordingStorage struct{}

func (*recordingStorage) Store(context.Context, *types.ScannedFile) (FileStored, error) {
	return FileStored{}, nil
}
