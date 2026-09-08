package nats

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rubiojr/hashup/internal/crypto"
	"github.com/rubiojr/hashup/internal/errmsg"
	"github.com/rubiojr/hashup/internal/log"
	"github.com/rubiojr/hashup/internal/natsstream"
	"github.com/rubiojr/hashup/internal/types"
	"github.com/vmihailenco/msgpack/v5"
)

type Stats struct {
	AttemptedFiles uint64
	FailedFiles    uint64
	QueuedFiles    uint64
}

type natsProcessor struct {
	ctx         context.Context
	nc          *nats.Conn
	js          nats.JetStreamContext
	subjectName string
	timeout     time.Duration
	encryptKey  []byte // AES encryption key (only used if encrypt is true)
	encrypt     bool   // field to control encryption behavior
	statsChan   chan Stats
	crypto      crypto.Machine
	clientCert  string
	clientKey   string
	caCert      string
	closeOnce   sync.Once
	closeErr    error
	attempted   atomic.Uint64
	failed      atomic.Uint64
	queued      atomic.Uint64
}

type contextDialer struct {
	ctx     context.Context
	timeout time.Duration
}

func (d *contextDialer) Dial(network, address string) (net.Conn, error) {
	return (&net.Dialer{Timeout: d.timeout}).DialContext(d.ctx, network, address)
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
		np.encryptKey = []byte(key)
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Create processor with default settings
	processor := &natsProcessor{
		ctx:         ctx,
		subjectName: subject,
		timeout:     timeout,
		encrypt:     true,
	}
	// Apply options
	for _, opt := range opts {
		opt(processor)
	}

	nopts := []nats.Option{
		nats.SetCustomDialer(&contextDialer{ctx: ctx, timeout: timeout}),
		nats.SkipHostLookup(),
	}
	if timeout > 0 {
		nopts = append(nopts, nats.Timeout(timeout))
	}
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
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}
	processor.nc = nc

	// Get JetStream context
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to get JetStream context: %w", err)
	}
	processor.js = js

	if err := natsstream.Ensure(js, streamName, subject, nats.Context(ctx)); err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to ensure stream: %w", err)
	}

	// If encryption is enabled but no key was provided, generate a random one
	if processor.encryptKey == nil {
		nc.Close()
		return nil, fmt.Errorf("encryption enabled but no key provided")
	}

	processor.crypto, err = crypto.NewAge(string(processor.encryptKey))
	if err != nil {
		nc.Close()
		return nil, err
	}

	return processor, nil
}

// Process method with optional encryption
func (np *natsProcessor) Process(path string, msg types.ScannedFile) (err error) {
	np.attempted.Add(1)
	stats := Stats{AttemptedFiles: 1}
	defer func() {
		if err != nil {
			np.failed.Add(1)
			stats.FailedFiles = 1
		}
		if np.statsChan != nil {
			select {
			case np.statsChan <- stats:
			case <-np.ctx.Done():
				select {
				case np.statsChan <- stats:
				default:
				}
			}
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
	} else {
		publishData = plainData
	}

	// Add a header to indicate if the message is encrypted
	headers := nats.Header{}
	if np.encrypt {
		headers.Set("Encrypted", "true")
	}

	// Publish the data with headers
	publishCtx := np.ctx
	if np.timeout > 0 {
		var cancel context.CancelFunc
		publishCtx, cancel = context.WithTimeout(np.ctx, np.timeout)
		defer cancel()
	}
	_, err = np.js.PublishMsg(&nats.Msg{
		Subject: np.subjectName,
		Data:    publishData,
		Header:  headers,
	}, nats.Context(publishCtx))
	if err != nil {
		return contextualPublishError(err, publishCtx)
	}

	stats.QueuedFiles++
	np.queued.Add(1)

	return nil
}

func contextualPublishError(err error, ctx context.Context) error {
	publishErr := fmt.Errorf("failed to publish message: %w: %w", errmsg.ErrPublishFailed, err)
	contextErr := ctx.Err()
	if contextErr == nil {
		return publishErr
	}
	if errors.Is(err, contextErr) {
		return contextErr
	}
	return errors.Join(publishErr, contextErr)
}

// Close closes the NATS connection
func (np *natsProcessor) Stats() Stats {
	return Stats{
		AttemptedFiles: np.attempted.Load(),
		FailedFiles:    np.failed.Load(),
		QueuedFiles:    np.queued.Load(),
	}
}

func (np *natsProcessor) Close() error {
	np.closeOnce.Do(func() {
		if np.nc != nil && !np.nc.IsClosed() {
			closed := np.nc.StatusChanged(nats.CLOSED)
			if err := np.nc.Drain(); err != nil {
				np.nc.Close()
				np.closeErr = fmt.Errorf("drain NATS connection: %w", err)
				return
			}
			wait := np.timeout
			if wait <= 0 {
				wait = 5 * time.Second
			}
			select {
			case <-closed:
				if err := np.nc.LastError(); err != nil {
					np.closeErr = fmt.Errorf("drain NATS connection: %w", err)
				}
			case <-np.ctx.Done():
				np.nc.Close()
			case <-time.After(wait):
				np.nc.Close()
				np.closeErr = fmt.Errorf("drain NATS connection: timed out after %s", wait)
			}
		}
	})
	return np.closeErr
}
