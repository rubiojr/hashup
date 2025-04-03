package store

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rubiojr/hashup/internal/crypto"
	"github.com/rubiojr/hashup/internal/log"
	"github.com/rubiojr/hashup/internal/types"
	"github.com/vmihailenco/msgpack/v5"
)

type Listener interface {
	Listen(context.Context) error
}

type NATSListenerOption func(*natsListener)

func WithNatsURL(url string) NATSListenerOption {
	return func(s *natsListener) {
		s.natsServerURL = url
	}
}

func WithNatsStream(stream string) NATSListenerOption {
	return func(s *natsListener) {
		s.natsStream = stream
	}
}

func WithConsumerName(name string) NATSListenerOption {
	return func(s *natsListener) {
		s.natsConsumer = name
	}
}

func WithNatsSubject(subject string) NATSListenerOption {
	return func(s *natsListener) {
		s.natsSubject = subject
	}
}

func WithStats(stats *ProcessStats) NATSListenerOption {
	return func(s *natsListener) {
		s.stats = stats
	}
}

func WithClientCert(cert string) NATSListenerOption {
	return func(s *natsListener) {
		s.clientCert = cert
	}
}

func WithClientKey(key string) NATSListenerOption {
	return func(s *natsListener) {
		s.clientKey = key
	}
}

func WithCACert(cert string) NATSListenerOption {
	return func(s *natsListener) {
		s.caCert = cert
	}
}

type natsListener struct {
	natsServerURL     string
	natsStream        string
	natsSubject       string
	natsConsumer      string
	natsEncryptionKey string
	stats             *ProcessStats
	storage           Storage
	clientCert        string
	clientKey         string
	caCert            string
}

func NewNatsListener(encryptionKey string, storage Storage, options ...NATSListenerOption) (Listener, error) {
	l := &natsListener{
		natsServerURL:     "localhost:4222",
		natsStream:        "HASHUP",
		natsSubject:       "FILES",
		natsConsumer:      "hsnats-store-consumer",
		natsEncryptionKey: encryptionKey,
		storage:           storage,
	}
	if encryptionKey == "" {
		return nil, fmt.Errorf("encryption key is required")
	}

	for _, option := range options {
		option(l)
	}

	return l, nil
}

func (l *natsListener) Listen(ctx context.Context) error {
	opts := []nats.Option{}

	if l.clientCert != "" {
		log.Debug("enabling Mutual TLS")
		opts = append(
			opts,
			nats.ClientCert(l.clientCert, l.clientKey),
			nats.RootCAs(l.caCert),
		)
	}

	nc, err := nats.Connect(
		l.natsServerURL,
		opts...,
	)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	// Access JetStream context
	js, err := nc.JetStream()
	if err != nil {
		return fmt.Errorf("failed to get JetStream context: %v", err)
	}

	//_, err = js.StreamInfo(cfg.Main.NatsStream)
	_, err = js.StreamInfo(l.natsStream)
	if err != nil {
		return fmt.Errorf("failed to subscribe to stream: %v", err)
	}

	// Create subscription with the consumer configuration
	sub, err := js.PullSubscribe(
		l.natsSubject,
		l.natsConsumer,
		nats.AckExplicit(),
		nats.DeliverAll(),
	)

	if err != nil {
		return fmt.Errorf("failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	cryptom, err := crypto.NewAge(l.natsEncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to create crypto instance: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			log.Debug("Message processing stopped due to cancellation")
			return nil
		default:
			// Continue processing
		}

		// Fetch a batch of messages
		messages, err := sub.Fetch(10, nats.MaxWait(1*time.Second))
		if err != nil {
			if err == context.Canceled {
				return nil
			}

			if err == nats.ErrTimeout {
				continue
			}

			log.Errorf("error fetching messages: %v\n", err)
			continue
		}

		for _, msg := range messages {
			if l.stats != nil {
				l.stats.IncrementReceived()
			}
			var plaintext []byte
			// Check if the message is encrypted
			isEncrypted := msg.Header.Get("Encrypted") == "true"

			if isEncrypted {
				// Decrypt the message
				decrypted, err := cryptom.Decrypt(msg.Data)
				if err != nil {
					log.Errorf("Failed to decrypt message: %v\n", err)
					if l.stats != nil {
						l.stats.IncrementSkipped()
					}
					continue
				}
				plaintext = decrypted
			} else {
				// Message is not encrypted
				plaintext = msg.Data
			}

			var fileMsg *types.ScannedFile

			// Unmarshal using MessagePack
			if err := msgpack.Unmarshal(plaintext, &fileMsg); err != nil {
				log.Errorf("Failed to unmarshal message: %v\n", err)
				if l.stats != nil {
					l.stats.IncrementSkipped()
				}
				continue
			}

			log.Debugf("[%s] received file: %s (size: %d, hash: %s)\n",
				fileMsg.Hostname, fileMsg.Path, fileMsg.Size, fileMsg.Hash)

			// Update stats for the host and extension
			if l.stats != nil {
				l.stats.RecordHost(fileMsg.Hostname)
				l.stats.RecordExtension(fileMsg.Extension)
			}

			// Process the file (save to database)
			wasWritten, err := l.storage.Store(ctx, fileMsg)
			if err != nil {
				log.Errorf("Failed to save file to database: %v\n", err)
				if l.stats != nil {
					l.stats.IncrementSkipped()
				}
			} else if wasWritten.Dirty() {
				if l.stats != nil {
					l.stats.IncrementWritten()
				}
			} else {
				if l.stats != nil {
					l.stats.IncrementAlreadyPresent()
				}
			}

			msg.Ack()
		}
	}
}
