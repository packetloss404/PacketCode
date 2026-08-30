package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeNoSync is the old shape: temp file, close, rename, no fsync. Kept in the
// test file only, to measure what durability costs.
func writeNoSync(path string, data []byte, tmpPattern string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, tmpPattern)
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

func payload() []byte {
	// Roughly a small session: a few turns of conversation.
	var b []byte
	for i := 0; i < 200; i++ {
		b = append(b, []byte(fmt.Sprintf("{\"role\":\"user\",\"content\":\"line %d\"},", i))...)
	}
	return b
}

func BenchmarkWrite_WithSync(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "x.json")
	data := payload()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Write(path, data, 0o600, ".x.*.tmp"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWrite_WithoutSync(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "x.json")
	data := payload()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := writeNoSync(path, data, ".x.*.tmp"); err != nil {
			b.Fatal(err)
		}
	}
}
