package cache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/rubiojr/hashup/internal/log"
)

type Cache interface {
	IsFileProcessed(string, string) bool
	MarkFileProcessed(string, string)
	Save() error
}

// FileCache provides a fast in-memory cache for file metadata using fastcache
type FileCache struct {
	cache     *fastcache.Cache
	cachePath string
	stats     CacheStats
	ctx       context.Context
}

// CacheStats tracks cache hit/miss statistics
type CacheStats struct {
	Hits      int64
	Misses    int64
	Additions int64
}

func DefaultCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic("Failed to get user home directory")
	}

	dir := filepath.Join(home, ".cache", "hashup")

	err = os.MkdirAll(dir, 0755)
	if err != nil {
		panic("Failed to create cache directory")
	}

	return filepath.Join(dir, "cache")
}

// NewFileCache creates a new file cache with a specified size limit in MB
func NewFileCache(ctx context.Context, sizeMB int, cachePath string) *FileCache {
	log.Debugf("Creating or loading file cache with size %dMB at %s", sizeMB, cachePath)
	fc := &FileCache{
		cache:     fastcache.LoadFromFileOrNew(cachePath, sizeMB*1024*1024), // Convert MB to bytes
		cachePath: cachePath,
		ctx:       ctx,
	}

	go func() {
		// save the cache every 30 seconds
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				log.Debug("saving cache")
				fc.cache.SaveToFile(fc.cachePath)
			case <-fc.ctx.Done():
				log.Debug("saving cache")
				fc.cache.SaveToFile(fc.cachePath)
				return
			}
		}
	}()

	return fc
}

// createCacheKey creates a composite key from file path and hash
func createCacheKey(filePath, fileHash string) []byte {
	return []byte(fmt.Sprintf("%s:%s", filePath, fileHash))
}

// IsFileProcessed checks if a file has been processed before based on its path and hash
// This method checks if the composite key (path+hash) exists in the cache
func (fc *FileCache) IsFileProcessed(filePath, fileHash string) bool {
	// Create a composite key from the file path and hash
	key := createCacheKey(filePath, fileHash)

	// Check if this exact combination exists
	exists := fc.cache.Has(key)

	if exists {
		fc.stats.Hits++
	} else {
		fc.stats.Misses++
	}

	return exists
}

// MarkFileProcessed adds a file to the cache using the composite key
func (fc *FileCache) MarkFileProcessed(filePath, fileHash string) {
	key := createCacheKey(filePath, fileHash)
	fc.cache.Set(key, []byte{})
	fc.stats.Additions++
}

// Save persists the cache to disk
func (fc *FileCache) Save() error {
	if fc.cachePath == "" {
		return nil // No path specified, nothing to do
	}

	// Ensure the directory exists
	cacheDir := filepath.Dir(fc.cachePath)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %v", err)
	}

	log.Printf("Saving file cache to %s", fc.cachePath)
	return fc.cache.SaveToFile(fc.cachePath)
}

// GetStats returns the current cache statistics
func (fc *FileCache) GetStats() CacheStats {
	return fc.stats
}

// ResetStats resets the cache statistics
func (fc *FileCache) ResetStats() {
	fc.stats = CacheStats{}
}
