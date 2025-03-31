package nats

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rubiojr/hashup/internal/cache"
	"github.com/rubiojr/hashup/internal/crypto"
	"github.com/rubiojr/hashup/internal/errmsg"
	"github.com/rubiojr/hashup/internal/log"
	"github.com/rubiojr/hashup/internal/types"
	"github.com/vmihailenco/msgpack/v5"
)

type Stats struct {
	SkippedFiles uint8
	QueuedFiles  uint8
}

type natsProcessor struct {
	nc          *nats.Conn
	js          nats.JetStreamContext
	subjectName string
	timeout     time.Duration
	encryptKey  []byte // AES encryption key (only used if encrypt is true)
	encrypt     bool   // field to control encryption behavior
	cache       *cache.FileCache
	statsChan   chan Stats
	crypto      *crypto.Machine
	clientCert  string
	clientKey   string
	caCert      string
}

// Options for configuring the NATS processor
type Option func(*natsProcessor)

func WithStatsChannel(ch chan Stats) Option {
	return func(np *natsProcessor) {
		np.statsChan = ch
	}
}

// WithEncryptionKey sets a specific encryption key
func WithEncryptionKey(key string) Option {
	return func(np *natsProcessor) {
		// Convert the key to a fixed length by hashing it
		hasher := sha256.New()
		hasher.Write([]byte(key))
		np.encryptKey = hasher.Sum(nil)
	}
}
func WithClientCert(cert string) Option {
	return func(np *natsProcessor) {
		np.clientCert = cert
	}
}

func WithClientKey(key string) Option {
	return func(np *natsProcessor) {
		np.clientKey = key
	}
}

func WithCACert(cert string) Option {
	return func(np *natsProcessor) {
		np.caCert = cert
	}
}

// Update NewNATSProcessor to use JetStream and support optional encryption
func NewNATSProcessor(ctx context.Context, url, streamName, subject string, timeout time.Duration, opts ...Option) (*natsProcessor, error) {
	// Create processor with default settings
	processor := &natsProcessor{
		subjectName: subject,
		timeout:     timeout,
		encrypt:     true,
	}
	// Apply options
	for _, opt := range opts {
		opt(processor)
	}

	nopts := []nats.Option{}
	if processor.clientCert != "" {
		log.Debug("enabling Mutual TLS")
		log.Debugf("Client certificate: %s", processor.clientCert)
		log.Debugf("Client key: %s", processor.clientKey)
		log.Debugf("CA Cert: %s", processor.caCert)
		nopts = append(
			nopts,
			nats.ClientCert(processor.clientCert, processor.clientKey),
			nats.RootCAs(processor.caCert),
		)
	}
	log.Debugf("NATS URL: %s", url)
	nc, err := nats.Connect(url, nopts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %v", err)
	}
	processor.nc = nc

	// Get JetStream context
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to get JetStream context: %v", err)
	}
	processor.js = js

	_, err = js.StreamInfo(streamName)
	if err != nil {
		// Create the stream if it doesn't exist
		_, err = js.AddStream(&nats.StreamConfig{
			Name:              streamName,
			Subjects:          []string{subject},
			Storage:           nats.FileStorage,
			Discard:           nats.DiscardOld,
			Retention:         nats.WorkQueuePolicy,
			MaxMsgs:           -1,
			MaxBytes:          -1,
			MaxAge:            30 * 24 * time.Hour, // Messages expire after 30 days
			Replicas:          1,
			MaxMsgsPerSubject: -1,
		})
		if err != nil {
			nc.Close()
			return nil, fmt.Errorf("failed to create stream: %v", err)
		}
	}

	// If encryption is enabled but no key was provided, generate a random one
	if processor.encryptKey == nil {
		return nil, fmt.Errorf("encryption enabled but no key provided")
	}

	processor.crypto, err = crypto.New(processor.encryptKey)
	if err != nil {
		return nil, err
	}

	return processor, nil
}

// Process method with optional encryption
func (np *natsProcessor) Process(path string, msg types.ScannedFile) error {
	stats := Stats{SkippedFiles: 1}
	defer func() {
		if np.statsChan != nil {
			np.statsChan <- stats
		}
	}()

	// Marshal the message using MessagePack
	plainData, err := msgpack.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal file message: %v", err)
	}

	var publishData []byte
	// Encrypt the data if encryption is enabled
	if np.encrypt {
		encryptedData, err := np.crypto.Encrypt(plainData)
		if err != nil {
			return fmt.Errorf("failed to encrypt message: %v", err)
		}
		publishData = encryptedData
		log.Debugf("Message encrypted: %d bytes -> %d bytes", len(plainData), len(publishData))
	} else {
		publishData = plainData
	}

	// Add a header to indicate if the message is encrypted
	headers := nats.Header{}
	if np.encrypt {
		headers.Set("Encrypted", "true")
	}

	// Publish the data with headers
	_, err = np.js.PublishMsg(&nats.Msg{
		Subject: np.subjectName,
		Data:    publishData,
		Header:  headers,
	})
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", errmsg.ErrPublishFailed)
	}

	stats.QueuedFiles++
	stats.SkippedFiles = 0

	return nil
}

// Close closes the NATS connection
func (np *natsProcessor) Close() {
	defer np.nc.Drain()
	if np.nc != nil && !np.nc.IsClosed() {
		np.nc.Close()
	}
	close(np.statsChan)
}
