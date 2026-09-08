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
	"github.com/rubiojr/hashup/pkg/config"
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
	rootDir        string
	ignorePatterns []*regexp.Regexp
	ignoreHidden   bool
	force          bool
	cacheNamespace string
	configErr      error
	concurrency    int
	pool           *pool.Pool
	pCount         chan int64
	cache          cache.Cache
}

// Options for configuring the NATS processor
type Option func(*DirectoryScanner)

// WithIgnoreList configures whether the processor should wait for acknowledgment
func WithIgnoreList(ignoreList []string) Option {
	return func(s *DirectoryScanner) {
		s.ignorePatterns = nil
		for _, pattern := range ignoreList {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				s.configErr = fmt.Errorf("invalid ignore pattern %q: %w", pattern, err)
				return
			}
			s.ignorePatterns = append(s.ignorePatterns, compiled)
		}
	}
}

func WithScanningConcurrency(concurrency int) Option {
	return func(s *DirectoryScanner) {
		s.concurrency = concurrency
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

func WithCacheNamespace(namespace string) Option {
	return func(s *DirectoryScanner) {
		s.cacheNamespace = namespace
	}
}

func WithForce(force bool) Option {
	return func(s *DirectoryScanner) {
		s.force = force
	}
}

func NewDirectoryScanner(rootDir string, options ...Option) *DirectoryScanner {
	scanner := &DirectoryScanner{
		rootDir:      rootDir,
		ignoreHidden: true,
		concurrency:  5,
		// TODO: context propagagion
		cache: cache.NewFileCache(context.Background(), 100, config.DefaultCachePath()),
	}

	// apply options
	for _, option := range options {
		option(scanner)
	}

	if scanner.concurrency <= 0 {
		if scanner.configErr == nil {
			scanner.configErr = fmt.Errorf("scanning concurrency must be greater than zero")
		}
		scanner.concurrency = 1
	}
	scanner.pool = pool.NewPool(scanner.concurrency)
	scanner.pool.Start()

	return scanner
}

func (s *DirectoryScanner) CounterChan() chan int64 {
	if s.pCount == nil {
		s.pCount = make(chan int64, 1)
	}
	return s.pCount
}

func (s *DirectoryScanner) incCounter() {
	if s.pCount == nil {
		return
	}
	select {
	case s.pCount <- 1:
	default:
	}
}

func (s *DirectoryScanner) ScanDirectory(ctx context.Context, processor processors.Processor) (int64, error) {
	if s.configErr != nil {
		return 0, s.configErr
	}
	rootDir, err := filepath.Abs(s.rootDir)
	if err != nil {
		return 0, fmt.Errorf("resolve scan root: %w", err)
	}

	defer func() {
		s.pool.Stop()
		err := s.cache.Save()
		if err != nil {
			log.Errorf("Error saving cache: %v", err)
		}
	}()

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	var count int64

	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		// Check if the context has been cancelled
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err != nil {
			return handleWalkError(rootDir, path, info, err)
		}

		count++
		s.incCounter()

		if s.ignoreHidden && info.IsDir() && len(info.Name()) > 1 && info.Name()[0] == '.' {
			log.Debugf("ignoring hidden directory: %s", path)
			return filepath.SkipDir
		}

		if s.ignoreHidden && info.Name()[0] == '.' {
			log.Debugf("ignoring hidden file: %s", path)
			return nil
		}

		// Skip files that cannot be accessed.
		absPath, err := filepath.Abs(path)
		if err != nil {
			log.Debugf("Error accessing %q: %v", path, err)
			return nil
		}

		if ignored, err := s.ignorePath(absPath, info); ignored {
			return err
		}

		// Skip ignored directories
		if info.IsDir() && slices.Contains(ignoredDirectories, info.Name()) {
			log.Debugf("ignoring directory %s", path)
			return filepath.SkipDir
		}

		// Skip directories.
		if info.IsDir() {
			return nil
		}

		// return if the file is not a regular file
		if !info.Mode().IsRegular() {
			return nil
		}

		// Skip ignored files
		if slices.Contains(ignoredFiles, info.Name()) {
			log.Debugf("ignoring file %s", path)
			return nil
		}

		f := func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			// Calculate file hash
			fileHash, err := util.ComputeFileHash(absPath)
			if err != nil {
				return fmt.Errorf("error computing xxhash for %q: %v", path, err)
			}

			cachePath := absPath
			if s.cacheNamespace != "" {
				cachePath = s.cacheNamespace + "\x00" + absPath
			}
			if !s.force && s.cache.IsFileProcessed(cachePath, fileHash) {
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
				Path:      absPath,
				Size:      info.Size(),
				ModTime:   info.ModTime(),
				Hash:      fileHash,
				Extension: ext,
				Hostname:  hostname,
			}

			log.Debugf("Processing file %s\n", absPath)
			if err := processor.Process(absPath, msg); err != nil {
				return fmt.Errorf("failed processing %q: %w", absPath, err)
			}
			log.Debugf("Marking file %s processed\n", absPath)
			s.cache.MarkFileProcessed(cachePath, fileHash)
			return nil
		}
		s.pool.Submit(f)
		return nil
	})

	return count, err
}

func handleWalkError(rootDir, path string, info os.FileInfo, err error) error {
	log.Errorf("Error accessing %q: %v", path, err)
	if path == rootDir {
		return err
	}
	if info != nil && info.IsDir() {
		return filepath.SkipDir
	}
	return nil
}

func (s *DirectoryScanner) ignorePath(path string, info os.FileInfo) (bool, error) {
	for _, pattern := range s.ignorePatterns {
		if !pattern.MatchString(path) {
			continue
		}
		log.Debugf("ignoring path match %s", path)
		if info.IsDir() {
			return true, filepath.SkipDir
		}
		return true, nil
	}
	return false, nil
}
