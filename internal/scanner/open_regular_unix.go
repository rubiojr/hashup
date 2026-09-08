//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package scanner

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openRegularFile(path string) (*os.File, os.FileInfo, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open non-symlink file %q: %w", path, err)
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, fmt.Errorf("stat opened file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, nil, fmt.Errorf("opened path %q is not a regular file", path)
	}
	return file, info, nil
}
