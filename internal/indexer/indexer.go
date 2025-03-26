package indexer

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/rubiojr/hashub/internal/log"
	"github.com/rubiojr/hashub/internal/pool"
	"github.com/rubiojr/hashub/internal/processors"
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

func ScanDirectory(ctx context.Context, processor processors.Processor, rootDir string, ignoreList []string, ignoreHidden bool, pool *pool.Pool, pCount chan int64) (int64, error) {
	defer pool.Stop()
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

		if ignoreHidden && info.IsDir() && len(info.Name()) > 1 && info.Name()[0] == '.' {
			log.Printf("ignoring hidden directory: %s", path)
			return filepath.SkipDir
		}

		if ignoreHidden && info.Name()[0] == '.' {
			log.Printf("ignoring hidden file: %s", path)
			return nil
		}

		// Skip files that cannot be accessed.
		absPath, _ := filepath.Abs(path)
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
			log.Printf("Warn: not a regular file %q", path)
			return nil
		}

		// Skip ignored files
		if slices.Contains(ignoredFiles, info.Name()) {
			log.Printf("ignoring file %s", path)
			return nil
		}

		// ignoreList is a list of regular expressions to ignore
		for _, pattern := range ignoreList {
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
			pCount <- 1
			err = processor.Process(absPath, info, hostname)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				log.Errorf("failed processing %q: %v", path, err)
			}
			return nil
		}

		count++
		pool.Submit(f)
		return nil
	})

	return count, err
}
