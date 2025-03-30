package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/rubiojr/hashup/internal/cache"
	"github.com/rubiojr/hashup/internal/log"
	"github.com/rubiojr/hashup/internal/pool"
	"github.com/rubiojr/hashup/internal/processors"
	"github.com/rubiojr/hashup/internal/types"
	"github.com/rubiojr/hashup/internal/util"
)

var ignoredDirectories = []string{
	".@__thumb",
	".android",
	".arduino15",
	".arduinoIDE",
	".azure",
	".bun",
	".bundle",
	".cache",
	".cargo",
	".dartServer",
	".deno",
	".dotnet",
	".dart",
	".dartServer",
	".flutter",
	".flutter-devtools",
	".git",
	".gradle",
	".gradleServer",
	".java",
	".npm",
	".ollama",
	".pub-cache",
	".pyenv",
	".rbenv",
	".rustup",
	".rye",
	".streams",
	".vscode",
	"node_modules",
}

var ignoredFiles = []string{".DS_Store", "Thumbs.db", ".localized"}

type DirectoryScanner struct {
	rootDir      string
	ignoreList   []string
	ignoreHidden bool
	pool         *pool.Pool
	pCount       chan int64
	cache        cache.Cache
}

// Options for configuring the NATS processor
type Option func(*DirectoryScanner)

// WithIgnoreList configures whether the processor should wait for acknowledgment
func WithIgnoreList(ignoreList []string) Option {
	return func(s *DirectoryScanner) {
		s.ignoreList = ignoreList
	}
}

func WithScanningConcurrency(concurrency int) Option {
	return func(s *DirectoryScanner) {
		s.pool = pool.NewPool(concurrency)
		s.pool.Start()
	}
}

func WithIgnoreHidden(ignoreHidden bool) Option {
	return func(s *DirectoryScanner) {
		s.ignoreHidden = ignoreHidden
	}
}

func WithCache(cache cache.Cache) Option {
	return func(s *DirectoryScanner) {
		s.cache = cache
	}
}

func NewDirectoryScanner(rootDir string, options ...Option) *DirectoryScanner {
	scanner := &DirectoryScanner{
		rootDir:      rootDir,
		ignoreList:   []string{},
		ignoreHidden: true,
		pool:         pool.NewPool(5),
		// TODO: context propagagion
		cache: cache.NewFileCache(context.Background(), 100, cache.DefaultCachePath()),
	}

	// apply options
	for _, option := range options {
		option(scanner)
	}

	scanner.pool.Start()

	return scanner
}

func (s *DirectoryScanner) CounterChan() chan int64 {
	if s.pCount == nil {
		s.pCount = make(chan int64)
	}
	return s.pCount
}

func (s *DirectoryScanner) incCounter() {
	if s.pCount == nil {
		return
	}
	s.pCount <- 1
}

func (s *DirectoryScanner) ScanDirectory(ctx context.Context, processor processors.Processor) (int64, error) {
	defer s.pool.Stop()
	defer s.cache.Save()

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	var count int64

	err = filepath.Walk(s.rootDir, func(path string, info os.FileInfo, err error) error {
		// Check if the context has been cancelled
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.incCounter()

		if s.ignoreHidden && info.IsDir() && len(info.Name()) > 1 && info.Name()[0] == '.' {
			log.Printf("ignoring hidden directory: %s", path)
			return filepath.SkipDir
		}

		if s.ignoreHidden && info.Name()[0] == '.' {
			log.Printf("ignoring hidden file: %s", path)
			return nil
		}

		// Skip files that cannot be accessed.
		absPath, err := filepath.Abs(path)
		if err != nil {
			log.Printf("Error accessing %q: %v", path, err)
			return nil
		}

		// Skip ignored directories
		if info.IsDir() && slices.Contains(ignoredDirectories, info.Name()) {
			log.Printf("ignoring directory %s", path)
			return filepath.SkipDir
		}

		// Skip directories.
		if info.IsDir() {
			return nil
		}

		// return if the file is not a regular file
		if !info.Mode().IsRegular() {
			//log.Printf("Warn: not a regular file %q", path)
			return nil
		}

		// Skip ignored files
		if slices.Contains(ignoredFiles, info.Name()) {
			log.Printf("ignoring file %s", path)
			return nil
		}

		// ignoreList is a list of regular expressions to ignore
		for _, pattern := range s.ignoreList {
			matched, _ := regexp.MatchString(pattern, absPath)
			if matched {
				log.Printf("ignoring path match %s", path)
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		f := func() error {
			// Calculate file hash
			fileHash, err := util.ComputeFileHash(absPath)
			if err != nil {
				return fmt.Errorf("error computing xxhash for %q: %v", path, err)
			}

			if s.cache.IsFileProcessed(absPath, fileHash) {
				log.Debugf("File %s already processed", path)
				return nil
			}

			// Extract file extension
			ext := filepath.Ext(path)
			if ext != "" {
				ext = ext[1:] // Remove the dot
			}

			if filepath.Base(path) == filepath.Ext(path) {
				ext = ""
			}

			// Create the message
			msg := types.ScannedFile{
				Path:      path,
				Size:      info.Size(),
				ModTime:   info.ModTime(),
				Hash:      fileHash,
				Extension: ext,
				Hostname:  hostname,
			}

			err = processor.Process(absPath, msg)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				log.Errorf("failed processing %q: %v", path, err)
			}
			s.cache.MarkFileProcessed(absPath, fileHash)
			return nil
		}

		count++
		s.pool.Submit(f)
		return nil
	})

	return count, err
}
