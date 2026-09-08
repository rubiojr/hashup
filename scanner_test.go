package main

import (
	"testing"

	"github.com/rubiojr/hashup/internal/crypto"
	"github.com/rubiojr/hashup/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
}
