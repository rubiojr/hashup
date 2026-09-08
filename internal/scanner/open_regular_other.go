//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package scanner

import (
	"fmt"
	"os"
)

func openRegularFile(path string) (*os.File, os.FileInfo, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect file %q: %w", path, err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("refusing symlink %q", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || !os.SameFile(pathInfo, info) {
		file.Close()
		return nil, nil, fmt.Errorf("path %q changed before it was opened", path)
	}
	return file, info, nil
}
