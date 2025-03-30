package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rubiojr/hashup/internal/config"
	"github.com/rubiojr/hashup/internal/crypto"
	"github.com/rubiojr/hashup/internal/log"
	"github.com/rubiojr/hashup/internal/store"
	"github.com/urfave/cli/v2"

	_ "github.com/mattn/go-sqlite3"
	"github.com/nats-io/nats.go"
	hsdb "github.com/rubiojr/hashup/internal/db"
	natsp "github.com/rubiojr/hashup/internal/processors/nats"
	"github.com/vmihailenco/msgpack/v5"
)

func runStore(clictx *cli.Context) error {
	cfg, err := config.LoadConfigFromCLI(clictx)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize statistics
	stats := store.NewProcessStats()
	stats.StartStatsPrinters(ctx, cfg.Store.StatsInterval)

	// Connect to SQLite database
	db, err := hsdb.OpenDatabase(cfg.Store.DBPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}
	defer db.Close()

	// Connect to NATS with JetStream enabled
	nc, err := nats.Connect(cfg.Main.NatsServerURL)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	// Access JetStream context
	js, err := nc.JetStream()
	if err != nil {
		return fmt.Errorf("failed to get JetStream context: %v", err)
	}

	_, err = js.StreamInfo(cfg.Main.NatsStream)
	if err != nil {
		return fmt.Errorf("failed to subscribe to stream: %v", err)
	}

	// Create a durable consumer
	consumerName := "hsnats-store-consumer"

	// Create subscription with the consumer configuration
	sub, err := js.PullSubscribe(
		cfg.Main.NatsSubject,
		consumerName,
		nats.AckExplicit(),
		nats.DeliverAll(),
	)

	if err != nil {
		return fmt.Errorf("failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	log.Printf("Listening for files on %s...\n", cfg.Main.NatsSubject)
	log.Printf("Saving data to %s\n", cfg.Store.DBPath)
	if cfg.Store.StatsInterval > 0 {
		log.Printf("Statistics will be printed every %d seconds\n", cfg.Store.StatsInterval)
	}

	var encryptionKey []byte

	keyString := cfg.Main.EncryptionKey
	if keyString == "" {
		return fmt.Errorf("encryption key is required")
	}

	// Derive a 32-byte key for AES-256 from the encryption key
	hasher := sha256.New()
	hasher.Write([]byte(keyString))
	encryptionKey = hasher.Sum(nil)
	cryptom, err := crypto.New(encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to create crypto instance: %v", err)
	}

	filterHost := ""
	// Process messages
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Debug("Message processing stopped due to cancellation")
				return
			default:
				// Continue processing
			}

			// Fetch a batch of messages
			messages, err := sub.Fetch(10, nats.MaxWait(1*time.Second))
			if err != nil {
				if err == context.Canceled {
					return
				}

				if err == nats.ErrTimeout {
					continue
				}

				log.Errorf("error fetching messages: %v\n", err)
				continue
			}

			for _, msg := range messages {
				stats.IncrementReceived()

				var plaintext []byte
				// Check if the message is encrypted
				isEncrypted := msg.Header.Get("Encrypted") == "true"

				if isEncrypted {
					// Decrypt the message
					decrypted, err := cryptom.Decrypt(msg.Data)
					if err != nil {
						log.Errorf("Failed to decrypt message: %v\n", err)
						stats.IncrementSkipped()
						continue
					}
					plaintext = decrypted
				} else {
					// Message is not encrypted
					plaintext = msg.Data
				}

				var fileMsg natsp.FileMessage

				// Unmarshal using MessagePack
				if err := msgpack.Unmarshal(plaintext, &fileMsg); err != nil {
					log.Errorf("Failed to unmarshal message: %v\n", err)
					stats.IncrementSkipped()
					continue
				}

				// Only store for this hostname
				if filterHost != "" && fileMsg.Hostname != filterHost {
					stats.IncrementSkipped()
					continue
				}

				log.Debugf("[%s] received file: %s (size: %d, hash: %s)\n",
					fileMsg.Hostname, fileMsg.Path, fileMsg.Size, fileMsg.Hash)

				// Update stats for the host and extension
				stats.RecordHost(fileMsg.Hostname)
				stats.RecordExtension(fileMsg.Extension)

				// Process the file (save to database)
				wasWritten, err := saveFileToDatabase(db, fileMsg)
				if err != nil {
					log.Errorf("Failed to save file to database: %v\n", err)
					stats.IncrementSkipped()
				} else if wasWritten {
					stats.IncrementWritten()
				} else {
					stats.IncrementAlreadyPresent() // File already exists and wasn't updated
				}

				// Create a response
				response := natsp.ProcessResponse{
					RequestID: fileMsg.RequestID,
					Success:   err == nil, // Success is true if there was no error
				}

				if err != nil {
					response.Error = err.Error()
				}

				msg.Ack()
			}
		}
	}()

	// Wait for interrupt signal
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	<-signalCh

	// Print final stats before exiting
	fmt.Println("\n\nFinal statistics:")
	stats.PrintStats()

	cancel()
	log.Debug("Shutting down...")

	return nil
}

// Modified to return whether a new record was written
func saveFileToDatabase(db *sql.DB, fileMsg natsp.FileMessage) (bool, error) {
	// Track if we've written a new record
	recordWritten := false

	// Begin transaction
	tx, err := db.Begin()
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
