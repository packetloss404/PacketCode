package tools

import (
	"io"
	"os"
)

// readFileBounded reads up to 1MB of a file as a string. Used by the
// search fallback to avoid loading huge binaries into memory.
func readFileBounded(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	const maxBytes = 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(f, maxBytes))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
