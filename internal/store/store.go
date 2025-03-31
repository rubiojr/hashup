package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	hsdb "github.com/rubiojr/hashup/internal/db"
	"github.com/rubiojr/hashup/internal/types"
)

var ErrFileInfoExists = errors.New("file info already exists")
var ErrFileHashExists = errors.New("file hash already exists")

type Storage interface {
	Store(context.Context, *types.ScannedFile) (FileStored, error)
}

type StorageOption func(*sqliteStorage)

type sqliteStorage struct {
	db             *sql.DB
	dbPath         string
	pInsertHash    *sql.Stmt
	pInsertInfo    *sql.Stmt
	pQueryFileInfo *sql.Stmt
	pQueryFileHash *sql.Stmt
}

func NewSqliteStorage(dbPath string) (*sqliteStorage, error) {
	db, err := hsdb.OpenDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	storage := &sqliteStorage{
		db:     db,
		dbPath: dbPath,
	}

	storage.pInsertHash, err = db.Prepare("INSERT INTO file_hashes (file_hash) VALUES (?)")
	if err != nil {
		return nil, fmt.Errorf("failed to prepare insert hash statement: %v", err)
	}

	storage.pInsertInfo, err = db.Prepare(`
		INSERT INTO file_info (
            file_path, file_size, modified_date, hash_id,
            host, extension, file_hash
        ) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare insert info statement: %v", err)
	}

	storage.pQueryFileInfo, err = db.Prepare("SELECT id FROM file_info WHERE file_path = ? AND host = ? AND file_hash = ?")
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query file info statement: %v", err)
	}

	storage.pQueryFileHash, err = db.Prepare("SELECT id FROM file_hashes WHERE file_hash = ?")
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query file hash statement: %v", err)
	}

	return storage, nil
}

func (s *sqliteStorage) Store(ctx context.Context, fileMsg *types.ScannedFile) (FileStored, error) {
	recordStored := FileStored{
		FileHash: false,
		FileInfo: false,
	}

	hashID, err := s.saveFileHash(fileMsg.Hash)
	recordStored.FileHash = err == nil

	if err != nil && hashID == -1 {
		return recordStored, fmt.Errorf("failed to save hash to database: %w", err)
	}

	err = s.saveFileInfo(hashID, fileMsg)
	recordStored.FileInfo = err == nil
	if err != nil && err != ErrFileInfoExists {
		return recordStored, fmt.Errorf("failed to save file info to database: %w", err)
	}

	return recordStored, nil
}

func (s *sqliteStorage) saveFileHash(hash string) (int64, error) {
	// Check if hash already exists in file_hashes
	hashID := int64(-1)
	row := s.pQueryFileHash.QueryRow(hash)
	err := row.Scan(&hashID)
	if err == nil {
		return hashID, ErrFileHashExists
	}

	if err == sql.ErrNoRows {
		result, err := s.pInsertHash.Exec(hash)
		if err != nil {
			return hashID, fmt.Errorf("failed to insert file hash: %w", err)
		}
		hashID, err = result.LastInsertId()
		if err != nil {
			return hashID, fmt.Errorf("failed to get last insert ID: %w", err)
		}
	} else {
		return hashID, fmt.Errorf("failed to query file hash: %w", err)
	}

	return hashID, nil
}

type FileStored struct {
	FileHash bool
	FileInfo bool
}

func (r FileStored) Dirty() bool {
	return r.FileHash || r.FileInfo
}

func (r FileStored) Both() bool {
	return r.FileHash && r.FileInfo
}

func (r FileStored) Clean() bool {
	return !r.FileHash && !r.FileInfo
}

func (s *sqliteStorage) saveFileInfo(hashID int64, fileMsg *types.ScannedFile) error {
	// Check if file_info already exists
	var fileID int64
	row := s.pQueryFileInfo.QueryRow(
		fileMsg.Path, fileMsg.Hostname, fileMsg.Hash,
	)
	err := row.Scan(&fileID)

	if err == nil {
		return ErrFileInfoExists
	}

	// Format mod time for SQL
	modTimeStr := fileMsg.ModTime.Format("2006-01-02 15:04:05")

	if err == sql.ErrNoRows {
		// Insert file_info if it doesn't exist
		result, err := s.pInsertInfo.Exec(
			fileMsg.Path, fileMsg.Size, modTimeStr, hashID,
			fileMsg.Hostname, fileMsg.Extension, fileMsg.Hash,
		)
		if err != nil {
			return fmt.Errorf("failed to insert file info: %w", err)
		}
		fileID, err = result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to get last insert ID: %w", err)
		}
		return nil
	}

	return fmt.Errorf("failed to query file info: %w", err)
}
