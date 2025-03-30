package nats

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cespare/xxhash/v2"
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
	IndexedFiles uint8
}

// ProcessResponse represents the response from the NATS consumer
type ProcessResponse struct {
	RequestID string `msgpack:"request_id"`
	Success   bool   `msgpack:"success"`
	Error     string `msgpack:"error,omitempty"`
}

type natsProcessor struct {
	nc           *nats.Conn
	js           nats.JetStreamContext
	subjectName  string
	responseSub  string
	responsesSub *nats.Subscription
	timeout      time.Duration
	waitForAck   bool   // field to control acknowledgment behavior
	encryptKey   []byte // AES encryption key (only used if encrypt is true)
	encrypt      bool   // field to control encryption behavior
	cache        *cache.FileCache
	statsChan    chan Stats
	crypto       *crypto.Machine
}

// Options for configuring the NATS processor
type Option func(*natsProcessor)

// WithWaitForAck configures whether the processor should wait for acknowledgment
func WithWaitForAck(wait bool) Option {
	return func(np *natsProcessor) {
		np.waitForAck = wait
	}
}

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

// Update NewNATSProcessor to use JetStream and support optional encryption
func NewNATSProcessor(ctx context.Context, url, streamName, subject string, timeout time.Duration, opts ...Option) (*natsProcessor, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %v", err)
	}

	// Get JetStream context
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to get JetStream context: %v", err)
	}

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

	// Create a unique subject for responses
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	pid := os.Getpid()
	responseSub := fmt.Sprintf("%s.responses.%s.%d", subject, hostname, pid)

	// Create processor with default settings
	processor := &natsProcessor{
		nc:          nc,
		js:          js,
		subjectName: subject,
		responseSub: responseSub,
		timeout:     timeout,
		waitForAck:  true, // default to waiting for acknowledgment
		encrypt:     true, // default to no encryption
	}

	// Apply options
	for _, opt := range opts {
		opt(processor)
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

	stats.IndexedFiles++
	stats.SkippedFiles = 0

	return nil
}

// Close closes the NATS connection
func (np *natsProcessor) Close() {
	if np.nc != nil && !np.nc.IsClosed() {
		np.nc.Close()
	}
	close(np.statsChan)
}

// computeFileHash opens a file, streams its contents through an xxhash hasher,
// and returns the computed 64-bit hash in hexadecimal string format.
func computeFileHash(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := xxhash.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	// Convert the 64-bit hash to hexadecimal.
	return fmt.Sprintf("%016x", hasher.Sum64()), nil
}
