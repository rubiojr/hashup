package store

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"filippo.io/age"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	hscrypto "github.com/rubiojr/hashup/internal/crypto"
	"github.com/rubiojr/hashup/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
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

func TestNatsListenerRetriesStorageFailure(t *testing.T) {
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
	storage := &failOnceStorage{stored: make(chan struct{})}
	listener, err := NewNatsListener(identity.String(), storage, WithNatsURL(server.ClientURL()))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- listener.Listen(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	js, err := nc.JetStream()
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err := js.ConsumerInfo("HASHUP", "hsnats-store-consumer")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	plain, err := msgpack.Marshal(&types.ScannedFile{Path: "/tmp/retry.txt", Hash: "hash", Hostname: "laptop"})
	require.NoError(t, err)
	machine, err := hscrypto.NewAge(identity.String())
	require.NoError(t, err)
	encrypted, err := machine.Encrypt(plain)
	require.NoError(t, err)
	message := &nats.Msg{Subject: "FILES", Data: encrypted, Header: nats.Header{}}
	message.Header.Set("Encrypted", "true")
	_, err = js.PublishMsg(message)
	require.NoError(t, err)

	select {
	case <-storage.stored:
	case <-time.After(2 * time.Second):
		t.Fatal("message was not redelivered after storage failure")
	}
	assert.Equal(t, int32(2), storage.calls.Load())
}

type recordingStorage struct{}

func (*recordingStorage) Store(context.Context, *types.ScannedFile) (FileStored, error) {
	return FileStored{}, nil
}

type failOnceStorage struct {
	calls  atomic.Int32
	stored chan struct{}
	once   sync.Once
}

func (s *failOnceStorage) Store(context.Context, *types.ScannedFile) (FileStored, error) {
	if s.calls.Add(1) == 1 {
		return FileStored{}, errors.New("temporary storage failure")
	}
	s.once.Do(func() { close(s.stored) })
	return FileStored{}, nil
}
