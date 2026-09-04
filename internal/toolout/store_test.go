package toolout

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var handlePattern = regexp.MustCompile(`out_[0-9a-f]{32}`)

func openStore(t *testing.T, opts Options) *Store {
	t.Helper()
	store, err := Open(t.TempDir(), opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// readAll pages the whole spilled result back the way the model is told to:
// start at offset 0 and follow next_offset until the store reports the end.
func readAll(t *testing.T, store *Store, handle string) string {
	t.Helper()
	var b strings.Builder
	offset := int64(0)
	for i := 0; i < 10_000; i++ {
		page, ok := store.Read(handle, offset, MaxPageBytes)
		require.True(t, ok, "handle must stay readable while paging")
		b.WriteString(page.Text)
		if page.EOF {
			return b.String()
		}
		require.Greater(t, page.Next, offset, "paging must advance")
		offset = page.Next
	}
	t.Fatal("paging did not terminate")
	return ""
}

func TestCapture_SmallResultIsUnchangedAndNotSpilled(t *testing.T) {
	store := openStore(t, Options{ExcerptLimit: 1024})
	content := "ok: 3 files changed\n"

	got, capped := store.Capture("write_file", content)

	assert.Equal(t, content, got, "a small result must be byte-identical")
	assert.False(t, capped)
	entries, err := os.ReadDir(store.Dir())
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing to spill means nothing written")
}

func TestCapture_SpillsOversizedResultAndHandleReadsTheRest(t *testing.T) {
	store := openStore(t, Options{ExcerptLimit: 4096})
	var raw strings.Builder
	for i := 0; raw.Len() < 200_000; i++ {
		raw.WriteString("line ")
		raw.WriteString(strings.Repeat("x", 40))
		raw.WriteString("\n")
	}
	content := raw.String()

	excerpt, capped := store.Capture("execute_command", content)

	require.True(t, capped)
	assert.LessOrEqual(t, len(excerpt), 4096, "excerpt must respect the limit")
	assert.True(t, strings.HasPrefix(content, excerpt[:200]), "head of the output is kept")
	assert.True(t, strings.HasSuffix(content, excerpt[len(excerpt)-200:]), "tail carries the verdict and is kept")
	assert.Contains(t, excerpt, "read_tool_output", "the model learns how to retrieve the rest from the result itself")
	assert.Contains(t, excerpt, fmt.Sprintf("was %d bytes", len(content)), "the excerpt states how much output there was")

	handle := handlePattern.FindString(excerpt)
	require.NotEmpty(t, handle, "the excerpt must name a handle")
	require.True(t, ValidHandle(handle))
	assert.Equal(t, content, readAll(t, store, handle), "paging the handle recovers the full output")
}

func TestRead_UnknownHandleDegradesInsteadOfProbingTheFilesystem(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(root, "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("password=hunter2"), 0o600))
	store, err := Open(filepath.Join(root, "spill"), Options{ExcerptLimit: 64})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// A real handle exists, so a miss cannot be explained by an empty store.
	live, capped := store.Capture("read_file", strings.Repeat("a", 4096))
	require.True(t, capped)
	require.NotEmpty(t, handlePattern.FindString(live))

	for _, handle := range []string{
		"",
		"   ",
		"../../../etc/passwd",
		"out_../../../../etc/passwd",
		secret,
		filepath.Join("..", "secret.txt"),
		"out_" + strings.Repeat("z", 32),
		"out_" + strings.Repeat("a", 32), // valid shape, never minted
		strings.Repeat("a", 32),
	} {
		page, ok := store.Read(handle, 0, MaxPageBytes)
		assert.False(t, ok, "handle %q must not resolve", handle)
		assert.Empty(t, page.Text, "handle %q must read nothing", handle)
	}
}

func TestRead_HandleFromAnotherSessionIsNotReadable(t *testing.T) {
	root := t.TempDir()
	foreground, err := Open(root, Options{ExcerptLimit: 64})
	require.NoError(t, err)
	t.Cleanup(func() { _ = foreground.Close() })
	background, err := Open(root, Options{ExcerptLimit: 64})
	require.NoError(t, err)
	t.Cleanup(func() { _ = background.Close() })

	excerpt, capped := foreground.Capture("execute_command", strings.Repeat("secret ", 1000))
	require.True(t, capped)
	handle := handlePattern.FindString(excerpt)
	require.NotEmpty(t, handle)

	_, ok := foreground.Read(handle, 0, MaxPageBytes)
	assert.True(t, ok, "the minting session can read its own output")
	page, ok := background.Read(handle, 0, MaxPageBytes)
	assert.False(t, ok, "a background job must not read the foreground session's output")
	assert.Empty(t, page.Text)
}

func TestCapture_KeepsUTF8Intact(t *testing.T) {
	store := openStore(t, Options{ExcerptLimit: 2048})
	// No ASCII and no newlines: every cut lands mid-rune unless the code
	// realigns, and no line boundary can rescue it.
	content := strings.Repeat("日本語テキスト🌍", 4000)

	excerpt, capped := store.Capture("search_codebase", content)
	require.True(t, capped)
	assert.True(t, utf8.ValidString(excerpt), "excerpt must not split a rune")

	handle := handlePattern.FindString(excerpt)
	require.NotEmpty(t, handle)
	assert.Equal(t, content, readAll(t, store, handle))

	// A model-chosen offset that lands mid-rune must still return valid text.
	page, ok := store.Read(handle, 1, 64)
	require.True(t, ok)
	assert.True(t, utf8.ValidString(page.Text))
	assert.Greater(t, page.Offset, int64(0), "offset advances to a rune boundary")
	page, ok = store.Read(handle, 0, 5)
	require.True(t, ok)
	assert.True(t, utf8.ValidString(page.Text), "a window that ends mid-rune drops the partial rune")
}

func TestRead_OffsetPastEndReportsEndOfOutput(t *testing.T) {
	store := openStore(t, Options{ExcerptLimit: 512})
	excerpt, capped := store.Capture("execute_command", strings.Repeat("z", 4096))
	require.True(t, capped)
	handle := handlePattern.FindString(excerpt)
	require.NotEmpty(t, handle)

	page, ok := store.Read(handle, 1_000_000, MaxPageBytes)
	require.True(t, ok)
	assert.Empty(t, page.Text)
	assert.True(t, page.EOF)
	assert.Equal(t, int64(4096), page.Total)
}

func TestRead_ClampsPageSize(t *testing.T) {
	store := openStore(t, Options{ExcerptLimit: 512})
	excerpt, _ := store.Capture("execute_command", strings.Repeat("z", 4*MaxPageBytes))
	handle := handlePattern.FindString(excerpt)
	require.NotEmpty(t, handle)

	page, ok := store.Read(handle, 0, 10*MaxPageBytes)
	require.True(t, ok)
	assert.Equal(t, MaxPageBytes, len(page.Text), "one call can never return an unbounded result")
	assert.False(t, page.EOF)
}

func TestCapture_BudgetEvictsOldestAndOldHandleDegrades(t *testing.T) {
	store := openStore(t, Options{ExcerptLimit: 128, Budget: 3000})

	first, capped := store.Capture("execute_command", strings.Repeat("1", 2000))
	require.True(t, capped)
	firstHandle := handlePattern.FindString(first)
	require.NotEmpty(t, firstHandle)
	_, ok := store.Read(firstHandle, 0, 16)
	require.True(t, ok)

	second, capped := store.Capture("execute_command", strings.Repeat("2", 2000))
	require.True(t, capped)
	secondHandle := handlePattern.FindString(second)
	require.NotEmpty(t, secondHandle)

	_, ok = store.Read(firstHandle, 0, 16)
	assert.False(t, ok, "the evicted handle degrades rather than reading recycled bytes")
	_, ok = store.Read(secondHandle, 0, 16)
	assert.True(t, ok)

	entries, err := os.ReadDir(store.Dir())
	require.NoError(t, err)
	assert.Len(t, entries, 1, "eviction removes the spill file, not just the registry entry")
}

func TestCapture_OutputLargerThanBudgetIsExcerptedWithoutAHandle(t *testing.T) {
	store := openStore(t, Options{ExcerptLimit: 256, Budget: 1024})

	excerpt, capped := store.Capture("execute_command", strings.Repeat("q", 8192))

	require.True(t, capped)
	assert.Empty(t, handlePattern.FindString(excerpt), "no handle is offered when nothing was retained")
	assert.Contains(t, excerpt, "not retained")
	entries, err := os.ReadDir(store.Dir())
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestClose_RemovesTheSessionSpillDirectory(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root, Options{ExcerptLimit: 128})
	require.NoError(t, err)
	excerpt, _ := store.Capture("execute_command", strings.Repeat("k", 4096))
	handle := handlePattern.FindString(excerpt)
	require.NotEmpty(t, handle)

	require.NoError(t, store.Close())

	_, err = os.Stat(store.Dir())
	assert.True(t, os.IsNotExist(err), "session end must not leave spill files behind")
	_, ok := store.Read(handle, 0, 16)
	assert.False(t, ok, "a handle after close degrades rather than erroring")
	assert.NoError(t, store.Close(), "close is idempotent")
}

func TestOpen_PrunesStaleSpillDirectoriesButNothingElse(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, spillDirPrefix+"deadbeef")
	require.NoError(t, os.MkdirAll(stale, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(stale, "out_1"+spillFileExt), []byte("old"), 0o600))
	fresh := filepath.Join(root, spillDirPrefix+"livecafe")
	require.NoError(t, os.MkdirAll(fresh, 0o700))
	unrelated := filepath.Join(root, "not-a-spill-dir")
	require.NoError(t, os.MkdirAll(unrelated, 0o700))
	old := time.Now().Add(-72 * time.Hour)
	require.NoError(t, os.Chtimes(stale, old, old))
	require.NoError(t, os.Chtimes(unrelated, old, old))

	store, err := Open(root, Options{MaxAge: time.Hour})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	_, err = os.Stat(stale)
	assert.True(t, os.IsNotExist(err), "a crashed session's spill must not survive forever")
	_, err = os.Stat(fresh)
	assert.NoError(t, err, "a recently active session is left alone")
	_, err = os.Stat(unrelated)
	assert.NoError(t, err, "prune only touches directories it created")
}

func TestNilStoreIsInert(t *testing.T) {
	var store *Store
	content, capped := store.Capture("execute_command", strings.Repeat("v", 1<<20))
	assert.Equal(t, 1<<20, len(content), "a disabled store leaves output untouched")
	assert.False(t, capped)
	_, ok := store.Read("out_"+strings.Repeat("0", 32), 0, 16)
	assert.False(t, ok)
	assert.NoError(t, store.Close())
}

func TestDefaultRoot_FollowsTheConfiguredDataHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PACKETCODE_HOME", home)

	root, err := DefaultRoot()

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "tool-output"), root)
	_, statErr := os.Stat(root)
	assert.True(t, os.IsNotExist(statErr), "resolving the root must not create it")
}
