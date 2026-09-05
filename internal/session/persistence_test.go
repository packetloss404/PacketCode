package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/provider"
)

// New and Load used to hand out the manager's own *Session, so a caller could
// mutate state the manager guards with a mutex, from outside it — and Save
// writes whatever m.current points at, so the mutation would silently persist.
// Current was already careful; these two were the doors left open.
func TestManager_NewReturnsACopyNotTheLiveSession(t *testing.T) {
	m := NewManager(t.TempDir())
	s, err := m.New("openai", "gpt-4.1")
	require.NoError(t, err)

	s.Name = "mutated behind the manager's back"
	s.Messages = append(s.Messages, provider.Message{Role: provider.RoleUser, Content: "injected"})

	cur := m.Current()
	require.NotNil(t, cur)
	assert.NotEqual(t, "mutated behind the manager's back", cur.Name,
		"a caller mutated manager-owned state through New's return value")
	assert.Empty(t, cur.Messages, "a caller appended to the manager's own message slice")
}

func TestManager_LoadReturnsACopyNotTheLiveSession(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	created, err := m.New("openai", "gpt-4.1")
	require.NoError(t, err)

	loaded, err := m.Load(created.ID)
	require.NoError(t, err)
	loaded.Name = "mutated"

	cur := m.Current()
	require.NotNil(t, cur)
	assert.NotEqual(t, "mutated", cur.Name,
		"a caller mutated manager-owned state through Load's return value")
}

// A corrupt session file used to be skipped in silence, so it vanished from
// /resume — indistinguishable, from the user's chair, from a session that
// never existed. That is the wrong failure for the one command whose job is to
// find a conversation the user knows they had.
func TestManager_ListReportsUnreadableSessions(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	good, err := m.New("openai", "gpt-4.1")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "0193b0de-dead-7000-8000-000000000001.json"),
		[]byte("{ not json"), 0o600))

	summaries, problems, err := m.ListWithProblems()
	require.NoError(t, err)

	// The readable one still lists: problems are reported alongside, never in
	// place of, the sessions that loaded.
	require.Len(t, summaries, 1)
	assert.Equal(t, good.ID, summaries[0].ID)

	require.Len(t, problems, 1, "the corrupt file was skipped silently")
	assert.Contains(t, problems[0], "0193b0de-dead-7000-8000-000000000001.json")

	// List keeps its old shape for callers that do not want them.
	plain, err := m.List()
	require.NoError(t, err)
	assert.Len(t, plain, 1)
}

// A file whose name disagrees with the id inside it would load differently
// depending on which spelling you used, so neither is offered without saying
// why.
func TestManager_ListReportsIDFilenameMismatch(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	created, err := m.New("openai", "gpt-4.1")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, created.ID+".json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "0193b0de-dead-7000-8000-000000000002.json"), data, 0o600))

	_, problems, err := m.ListWithProblems()
	require.NoError(t, err)
	require.NotEmpty(t, problems)
	joined := strings.Join(problems, "\n")
	assert.Contains(t, joined, "0193b0de-dead-7000-8000-000000000002.json")
	assert.Contains(t, joined, created.ID, "the message does not say which id the file actually holds")
}

// The write must land as valid JSON, not merely under the right name. This is
// the part fsync protects across a crash; here it asserts the ordinary path
// still round-trips through the shared writer.
func TestManager_SaveRoundTrips(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	s, err := m.New("openai", "gpt-4.1")
	require.NoError(t, err)

	require.NoError(t, m.AddMessage(provider.Message{Role: provider.RoleUser, Content: "hello"}))

	reloaded, err := NewManager(dir).Load(s.ID)
	require.NoError(t, err)
	require.Len(t, reloaded.Messages, 1)
	assert.Equal(t, "hello", reloaded.Messages[0].Content)

	// No temp files left behind for the next List to trip over.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasSuffix(e.Name(), ".tmp"), "left a temp file: %s", e.Name())
	}
}
