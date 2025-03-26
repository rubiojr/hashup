package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rubiojr/hashub/internal/log"
)

type FileCount struct {
	Chan   chan int64
	Errors []error
}

func FileCounter(root string) *FileCount {
	count := &FileCount{
		Chan: make(chan int64),
	}

	go func() {
		defer close(count.Chan)
		counter := int64(0)
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Printf("error walking path %s: %v", path, err)
				return nil
			}

			if info.IsDir() {
				return nil
			}

			if !info.Mode().IsRegular() {
				return nil
			}

			counter++
			return nil
		})

		if err != nil {
			count.Errors = append(count.Errors, fmt.Errorf("failed walking directory: %w", err))
		}

		count.Chan <- counter
	}()

	return count
}
