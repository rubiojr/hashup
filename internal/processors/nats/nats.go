package nats

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/nats-io/nats.go"
	"github.com/rubiojr/hashup/internal/cache"
	"github.com/rubiojr/hashup/internal/errmsg"
	"github.com/rubiojr/hashup/internal/log"
	"github.com/vmihailenco/msgpack/v5"
)

type Stats struct {
	SkippedFiles uint8
	IndexedFiles uint8
}

// FileMessage represents the structure of the message sent to NATS
type FileMessage struct {
	Path       string    `msgpack:"path"`
	Size       int64     `msgpack:"size"`
	ModTime    time.Time `msgpack:"mod_time"`
	Hash       string    `msgpack:"hash"`
	Extension  string    `msgpack:"extension"`
	Hostname   string    `msgpack:"hostname"`
	IsRegular  bool      `msgpack:"is_regular"`
	IsDir      bool      `msgpack:"is_dir"`
	RequestID  string    `msgpack:"request_id"`
	ResponseTo string    `msgpack:"response_to"`
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
	cache := cache.NewFileCache(ctx, 100, cache.DefaultCachePath())

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
		cache:       cache,
	}

	// Apply options
	for _, opt := range opts {
		opt(processor)
	}

	// If encryption is enabled but no key was provided, generate a random one
	if processor.encrypt && processor.encryptKey == nil {
		return nil, fmt.Errorf("encryption enabled but no key provided")
	}

	// Only set up subscription if we're waiting for acknowledgments
	if processor.waitForAck {
		responsesSub, err := nc.SubscribeSync(responseSub)
		if err != nil {
			nc.Close()
			return nil, fmt.Errorf("failed to subscribe to responses: %v", err)
		}
		processor.responsesSub = responsesSub
	}

	return processor, nil
}

// encrypt uses AES-GCM to encrypt data with the processor's key
func (np *natsProcessor) encryptData(data []byte) ([]byte, error) {
	// Create a new cipher block from the key
	block, err := aes.NewCipher(np.encryptKey)
	if err != nil {
		return nil, err
	}

	// Create a new GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Create a nonce (12 bytes for GCM)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Encrypt and seal the data
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// decrypt uses AES-GCM to decrypt data with the processor's key
func (np *natsProcessor) decryptData(data []byte) ([]byte, error) {
	// Create a new cipher block from the key
	block, err := aes.NewCipher(np.encryptKey)
	if err != nil {
		return nil, err
	}

	// Create a new GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Get the nonce size
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extract the nonce and ciphertext
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]

	// Decrypt the data
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// Process method with optional encryption
func (np *natsProcessor) Process(path string, info os.FileInfo, hostname string) error {
	stats := Stats{SkippedFiles: 1}
	defer func() {
		if np.statsChan != nil {
			np.statsChan <- stats
		}
	}()

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s: %w", path, errmsg.ErrNotRegularFile)
	}

	// Generate a unique request ID
	requestID := nats.NewInbox()

	// Calculate file hash
	fileHash, err := computeFileHash(path)
	if err != nil {
		return fmt.Errorf("error computing xxhash for %q: %v", path, err)
	}

	if np.cache.IsFileProcessed(path, fileHash) {
		log.Debugf("File %s already processed", path)
		return nil
	}

	// Extract file extension
	ext := filepath.Ext(path)
	if ext != "" {
		ext = ext[1:] // Remove the dot
	}

	// Create the message
	msg := FileMessage{
		Path:       path,
		Size:       info.Size(),
		ModTime:    info.ModTime(),
		Hash:       fileHash,
		Extension:  ext,
		Hostname:   hostname,
		IsRegular:  info.Mode().IsRegular(),
		IsDir:      info.IsDir(),
		RequestID:  requestID,
		ResponseTo: np.responseSub,
	}

	// Marshal the message using MessagePack
	plainData, err := msgpack.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal file message: %v", err)
	}

	var publishData []byte
	// Encrypt the data if encryption is enabled
	if np.encrypt {
		encryptedData, err := np.encryptData(plainData)
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

	np.cache.MarkFileProcessed(path, fileHash)
	stats.IndexedFiles++
	stats.SkippedFiles = 0

	return nil
}

// Close closes the NATS connection
func (np *natsProcessor) Close() {
	if np.nc != nil && !np.nc.IsClosed() {
		np.nc.Close()
	}
	if err := np.cache.Save(); err != nil {
		log.Errorf("failed to save cache: %v", err)
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

// GetEncryptionKeyHex returns the current encryption key as a hex string
// or an empty string if encryption is disabled
func (np *natsProcessor) GetEncryptionKeyHex() string {
	if !np.encrypt {
		return ""
	}
	return hex.EncodeToString(np.encryptKey)
}

// IsEncryptionEnabled returns whether encryption is enabled for this processor
func (np *natsProcessor) IsEncryptionEnabled() bool {
	return np.encrypt
}
