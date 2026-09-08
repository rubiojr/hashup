package scanner

import (
	"fmt"
	"os"

	"github.com/rubiojr/hashup/internal/util"
)

type fileSnapshot struct {
	hash string
	info os.FileInfo
}

func captureFileSnapshot(path string) (fileSnapshot, error) {
	file, openedInfo, err := openRegularFile(path)
	if err != nil {
		return fileSnapshot{}, err
	}
	defer file.Close()

	fileHash, err := util.ComputeReaderHash(file)
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("compute xxhash for %q: %w", path, err)
	}
	afterInfo, err := file.Stat()
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("stat hashed file %q: %w", path, err)
	}
	if openedInfo.Size() != afterInfo.Size() || !openedInfo.ModTime().Equal(afterInfo.ModTime()) {
		return fileSnapshot{}, fmt.Errorf("file %q changed while being hashed", path)
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("inspect hashed file %q: %w", path, err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 {
		return fileSnapshot{}, fmt.Errorf("file %q was replaced by a symlink", path)
	}
	if !os.SameFile(openedInfo, pathInfo) {
		return fileSnapshot{}, fmt.Errorf("file %q was replaced while being hashed", path)
	}
	return fileSnapshot{hash: fileHash, info: openedInfo}, nil
}
