package util

import (
	"fmt"
	"io"
	"os"

	"github.com/cespare/xxhash/v2"
)

// ComputeFileHash opens a file, streams its contents through an xxhash hasher,
// and returns the computed 64-bit hash in hexadecimal string format.
func ComputeFileHash(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := xxhash.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	// Convert the 64-bit hash to hexadecimal.
	return fmt.Sprintf("%016x", hasher.Sum64()), nil
}
