package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrite_CreatesAndOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")

	require.NoError(t, Write(path, []byte("first"), 0o600, ".x.*.tmp"))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "first", string(got))

	require.NoError(t, Write(path, []byte("second"), 0o600, ".x.*.tmp"))
	got, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "second", string(got), "the overwrite did not replace the contents")
}

// A leftover temp file is not harmless: the directory is scanned by callers
// that list sessions and job records, so a stray one becomes an entry nobody
// can explain.
func TestWrite_LeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Write(filepath.Join(dir, "x.json"), []byte("data"), 0o600, ".x.*.tmp"))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "x.json", entries[0].Name())
}

func TestWrite_AppliesPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not honour POSIX file modes")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")
	require.NoError(t, Write(path, []byte("data"), 0o600, ".x.*.tmp"))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"a file holding a conversation must not be world-readable")
}

// A failure must not leave debris behind either.
func TestWrite_FailedRenameCleansUp(t *testing.T) {
	dir := t.TempDir()
	// A directory where the destination file should go: the rename cannot
	// succeed, and the temp file must not survive the failure.
	target := filepath.Join(dir, "x.json")
	require.NoError(t, os.MkdirAll(target, 0o700))

	err := Write(target, []byte("data"), 0o600, ".x.*.tmp")
	require.Error(t, err)

	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	for _, e := range entries {
		assert.False(t, strings.HasSuffix(e.Name(), ".tmp"),
			"a failed write left a temp file: %s", e.Name())
	}
}
