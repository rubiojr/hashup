package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rubiojr/hashup/internal/crypto"
	hsdb "github.com/rubiojr/hashup/internal/db"
	"github.com/rubiojr/hashup/internal/log"
	"github.com/rubiojr/hashup/internal/types"
	"github.com/vmihailenco/msgpack/v5"
)

type Store interface {
	Listen(context.Context) error
}

type sqliteStore struct {
	db            *sql.DB
	natsServerURL string
	natsStream    string
	consumerName  string
	natsSubject   string
	dbPath        string
	encryptionKey string
	stats         *ProcessStats
}

type Option func(*sqliteStore)

func WithNatsURL(url string) Option {
	return func(s *sqliteStore) {
		s.natsServerURL = url
	}
}

func WithNatsStream(stream string) Option {
	return func(s *sqliteStore) {
		s.natsStream = stream
	}
}

func WithConsumerName(name string) Option {
	return func(s *sqliteStore) {
		s.consumerName = name
	}
}

func WithNatsSubject(subject string) Option {
	return func(s *sqliteStore) {
		s.natsSubject = subject
	}
}

func WithStats(stats *ProcessStats) Option {
	return func(s *sqliteStore) {
		s.stats = stats
	}
}

func NewSqliteStore(dbPath string, encryptionKey string, options ...Option) (Store, error) {
	// Connect to SQLite database
	//db, err := hsdb.OpenDatabase(cfg.Store.DBPath)
	db, err := hsdb.OpenDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	if encryptionKey == "" {
		return nil, fmt.Errorf("encryption key is required")
	}

	store := &sqliteStore{
		db:            db,
		natsServerURL: "localhost:4222",
		natsStream:    "HASHUP",
		natsSubject:   "FILES",
		consumerName:  "hsnats-store-consumer",
		dbPath:        dbPath,
		encryptionKey: encryptionKey,
	}
	for _, option := range options {
		option(store)
	}

	return store, nil
}

func (s *sqliteStore) Listen(ctx context.Context) error {
	// Connect to NATS with JetStream enabled
	nc, err := nats.Connect(s.natsServerURL)
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
	_, err = js.StreamInfo(s.natsStream)
	if err != nil {
		return fmt.Errorf("failed to subscribe to stream: %v", err)
	}

	// Create subscription with the consumer configuration
	sub, err := js.PullSubscribe(
		//cfg.Main.NatsSubject,
		s.natsSubject,
		s.consumerName,
		nats.AckExplicit(),
		nats.DeliverAll(),
	)

	if err != nil {
		return fmt.Errorf("failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	var ek []byte

	// Derive a 32-byte key for AES-256 from the encryption key
	hasher := sha256.New()
	hasher.Write([]byte(s.encryptionKey))
	ek = hasher.Sum(nil)
	cryptom, err := crypto.New(ek)
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
			if s.stats != nil {
				s.stats.IncrementReceived()
			}
			var plaintext []byte
			// Check if the message is encrypted
			isEncrypted := msg.Header.Get("Encrypted") == "true"

			if isEncrypted {
				// Decrypt the message
				decrypted, err := cryptom.Decrypt(msg.Data)
				if err != nil {
					log.Errorf("Failed to decrypt message: %v\n", err)
					if s.stats != nil {
						s.stats.IncrementSkipped()
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
				if s.stats != nil {
					s.stats.IncrementSkipped()
				}
				continue
			}

			log.Debugf("[%s] received file: %s (size: %d, hash: %s)\n",
				fileMsg.Hostname, fileMsg.Path, fileMsg.Size, fileMsg.Hash)

			// Update stats for the host and extension
			if s.stats != nil {
				s.stats.RecordHost(fileMsg.Hostname)
				s.stats.RecordExtension(fileMsg.Extension)
			}

			// Process the file (save to database)
			wasWritten, err := s.saveFileToDatabase(fileMsg)
			if err != nil {
				log.Errorf("Failed to save file to database: %v\n", err)
				if s.stats != nil {
					s.stats.IncrementSkipped()
				}
			} else if wasWritten {
				if s.stats != nil {
					s.stats.IncrementWritten()
				}
			} else {
				if s.stats != nil {
					s.stats.IncrementAlreadyPresent()
				}
			}

			msg.Ack()
		}
	}
}

// Modified to return whether a new record was written
func (s *sqliteStore) saveFileToDatabase(fileMsg *types.ScannedFile) (bool, error) {
	// Track if we've written a new record
	recordWritten := false

	// Begin transaction
	tx, err := s.db.Begin()
	if err != nil {
		return recordWritten, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Check if hash already exists in file_hashes
	var hashID int64
	row := tx.QueryRow("SELECT id FROM file_hashes WHERE file_hash = ?", fileMsg.Hash)
	err = row.Scan(&hashID)
	if err == sql.ErrNoRows {
		// Insert hash if it doesn't exist
		result, err := tx.Exec("INSERT INTO file_hashes (file_hash) VALUES (?)", fileMsg.Hash)
		if err != nil {
			return recordWritten, fmt.Errorf("failed to insert file hash: %v", err)
		}
		hashID, err = result.LastInsertId()
		if err != nil {
			return recordWritten, fmt.Errorf("failed to get last insert ID: %v", err)
		}
		recordWritten = true
	} else if err != nil {
		return recordWritten, fmt.Errorf("failed to query file hash: %v", err)
	}

	// Check if file_info already exists
	var fileID int64
	row = tx.QueryRow(
		"SELECT id FROM file_info WHERE file_path = ? AND host = ? AND file_hash = ?",
		fileMsg.Path, fileMsg.Hostname, fileMsg.Hash,
	)
	err = row.Scan(&fileID)

	// Format mod time for SQL
	modTimeStr := fileMsg.ModTime.Format("2006-01-02 15:04:05")

	if err == sql.ErrNoRows {
		// Insert file_info if it doesn't exist
		result, err := tx.Exec(
			`INSERT INTO file_info (
                file_path, file_size, modified_date, hash_id,
                host, extension, file_hash
            ) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			fileMsg.Path, fileMsg.Size, modTimeStr, hashID,
			fileMsg.Hostname, fileMsg.Extension, fileMsg.Hash,
		)
		if err != nil {
			return recordWritten, fmt.Errorf("failed to insert file info: %v", err)
		}
		fileID, err = result.LastInsertId()
		if err != nil {
			return recordWritten, fmt.Errorf("failed to get last insert ID: %v", err)
		}
		recordWritten = true
	} else if err != nil {
		return recordWritten, fmt.Errorf("failed to query file info: %v", err)
	} else {
		// Check if we need to update (file size or mod time changed)
		var currentSize int64
		var currentModTime string
		err = tx.QueryRow(
			"SELECT file_size, modified_date FROM file_info WHERE id = ?",
			fileID,
		).Scan(&currentSize, &currentModTime)

		if err != nil {
			return recordWritten, fmt.Errorf("failed to query current file details: %v", err)
		}

		// Only update if needed
		if currentSize != fileMsg.Size || currentModTime != modTimeStr {
			// Update existing file_info
			_, err = tx.Exec(
				`UPDATE file_info SET
					file_size = ?, modified_date = ?, updated_date = CURRENT_TIMESTAMP
				WHERE id = ?`,
				fileMsg.Size, modTimeStr, fileID,
			)
			if err != nil {
				return recordWritten, fmt.Errorf("failed to update file info: %v", err)
			}
			recordWritten = true
		}
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return recordWritten, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return recordWritten, nil
}
