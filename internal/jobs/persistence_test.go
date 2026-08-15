package jobs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSaveSnapshot_AtomicWrite confirms the temp-file-then-rename
// dance: after Save, only the final file (no .tmp) is present.
func TestSaveSnapshot_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	j := &Job{
		ID:             "abcd1234",
		SessionID:      "main-job-abcd1234",
		Prompt:         "do",
		Provider:       "scripted",
		Model:          "model",
		State:          StateCompleted,
		CreatedAt:      time.Now().UTC(),
		FinishedAt:     time.Now().UTC(),
		WorktreePath:   filepath.Join(dir, "worktrees", "abcd1234"),
		WorktreeBranch: "packetcode-job-abcd1234",
		WorktreeBase:   "0123456789abcdef",
		WorktreeNote:   "ready",
		Artifacts: []Artifact{{
			ID:         "A1",
			Kind:       "file_change",
			Title:      "hello.txt",
			Summary:    "wrote hello.txt",
			Path:       "hello.txt",
			SourceTool: "write_file",
			Metadata:   map[string]any{"bytes": 2},
			CreatedAt:  time.Now().UTC(),
		}},
	}
	require.NoError(t, saveSnapshot(dir, j))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "abcd1234.json", entries[0].Name())

	// Round-trip: parse back via the persisted shape.
	data, err := os.ReadFile(filepath.Join(dir, "abcd1234.json"))
	require.NoError(t, err)
	var p persistedJob
	require.NoError(t, json.Unmarshal(data, &p))
	assert.Equal(t, "abcd1234", p.ID)
	assert.Equal(t, "completed", p.State)
	assert.Equal(t, "pending", p.ResultStatus)
	assert.Equal(t, j.WorktreePath, p.WorktreePath)
	assert.Equal(t, j.WorktreeBranch, p.WorktreeBranch)
	assert.Equal(t, j.WorktreeBase, p.WorktreeBase)
	assert.Equal(t, j.WorktreeNote, p.WorktreeNote)
	require.Len(t, p.Artifacts, 1)
	assert.Equal(t, "file_change", p.Artifacts[0].Kind)

	roundTripped := fromPersisted(p)
	assert.Equal(t, j.WorktreePath, roundTripped.WorktreePath)
	assert.Equal(t, j.WorktreeBranch, roundTripped.WorktreeBranch)
	assert.Equal(t, j.WorktreeBase, roundTripped.WorktreeBase)
	assert.Equal(t, j.WorktreeNote, roundTripped.WorktreeNote)
	require.Len(t, roundTripped.Artifacts, 1)
	assert.Equal(t, "hello.txt", roundTripped.Artifacts[0].Path)
}

func TestPersistedResultStatusDefaultsToPending(t *testing.T) {
	j := fromPersisted(persistedJob{
		ID:        "legacy01",
		SessionID: "main-job-legacy01",
		Provider:  "p",
		Model:     "m",
		State:     "completed",
		CreatedAt: time.Now().UTC(),
	})
	assert.Equal(t, ResultStatusPending, j.ResultStatus)
}

func TestPersistedResultStatusParsesConsumed(t *testing.T) {
	j := fromPersisted(persistedJob{
		ID:           "consume1",
		SessionID:    "main-job-consume1",
		Provider:     "p",
		Model:        "m",
		State:        "completed",
		ResultStatus: "consumed",
		CreatedAt:    time.Now().UTC(),
	})
	assert.Equal(t, ResultStatusConsumed, j.ResultStatus)
}

func TestLoadPersistedJobs_HydratesTerminalJobsAndRebuildsResults(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	completed := &Job{
		ID:           "done1111",
		SessionID:    "main-job-done1111",
		Prompt:       "done",
		Provider:     "p",
		Model:        "m",
		State:        StateCompleted,
		CreatedAt:    now,
		ResultStatus: ResultStatusSeen,
		Artifacts: []Artifact{{
			ID:      "A1",
			Kind:    "test",
			Title:   "go test ./...",
			Summary: "go test ./... [exit 0]",
		}},
	}
	require.NoError(t, saveSnapshot(dir, completed))

	mgr, recovered, err := NewManager(Config{JobsDir: dir})
	require.NoError(t, err)
	assert.Equal(t, 0, recovered)

	snap, ok := mgr.Get("done1111")
	require.True(t, ok)
	assert.Equal(t, StateCompleted, snap.State)
	require.Len(t, snap.Artifacts, 1)
	assert.Equal(t, "test", snap.Artifacts[0].Kind)

	pending := mgr.PendingResults(0)
	require.Len(t, pending, 1)
	assert.Equal(t, ResultStatusSeen, pending[0].Status)
	require.Len(t, pending[0].Artifacts, 1)
}

func TestNewManager_HydratedResultsAreOldestFirst(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	newer := &Job{
		ID:           "aaaa1111",
		SessionID:    "main-job-aaaa1111",
		Prompt:       "new",
		Provider:     "p",
		Model:        "m",
		State:        StateCompleted,
		CreatedAt:    now,
		FinishedAt:   now.Add(2 * time.Minute),
		ResultStatus: ResultStatusPending,
	}
	older := &Job{
		ID:           "zzzz9999",
		SessionID:    "main-job-zzzz9999",
		Prompt:       "old",
		Provider:     "p",
		Model:        "m",
		State:        StateCompleted,
		CreatedAt:    now,
		FinishedAt:   now.Add(time.Minute),
		ResultStatus: ResultStatusPending,
	}
	require.NoError(t, saveSnapshot(dir, newer))
	require.NoError(t, saveSnapshot(dir, older))

	mgr, recovered, err := NewManager(Config{JobsDir: dir})
	require.NoError(t, err)
	assert.Equal(t, 0, recovered)

	pending := mgr.PendingResults(0)
	require.Len(t, pending, 2)
	assert.Equal(t, "zzzz9999", pending[0].JobID)
	assert.Equal(t, "aaaa1111", pending[1].JobID)
}

func TestManagerPersistenceDebouncesNonterminalUpdates(t *testing.T) {
	dir := t.TempDir()
	mgr, _, err := NewManager(Config{JobsDir: dir})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Shutdown(time.Second) })
	mgr.persistDelay = time.Hour

	for seq := int64(1); seq <= 3; seq++ {
		require.NoError(t, mgr.savePersistedSnapshot(persistedJob{
			ID: "debounce", State: "running", Seq: seq, LastMessage: fmt.Sprintf("update-%d", seq),
		}))
	}
	_, err = os.Stat(filepath.Join(dir, "debounce.json"))
	require.ErrorIs(t, err, os.ErrNotExist, "nonterminal updates must not write synchronously")

	mgr.flushPendingSnapshot("debounce")
	got, ok := readPersistedJob(filepath.Join(dir, "debounce.json"))
	require.True(t, ok)
	assert.Equal(t, int64(3), got.Seq)
	assert.Equal(t, "update-3", got.LastMessage)
}

func TestManagerPersistenceTerminalFlushIsSynchronous(t *testing.T) {
	dir := t.TempDir()
	mgr, _, err := NewManager(Config{JobsDir: dir})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Shutdown(time.Second) })
	mgr.persistDelay = time.Hour
	require.NoError(t, mgr.savePersistedSnapshot(persistedJob{ID: "terminal", State: "running", Seq: 1}))
	require.NoError(t, mgr.savePersistedSnapshot(persistedJob{ID: "terminal", State: "completed", Seq: 2, Summary: "done"}))

	got, ok := readPersistedJob(filepath.Join(dir, "terminal.json"))
	require.True(t, ok, "terminal snapshot must be durable before return")
	assert.Equal(t, "completed", got.State)
	assert.Equal(t, int64(2), got.Seq)
	mgr.persistMu.Lock()
	_, pending := mgr.persistPending["terminal"]
	mgr.persistMu.Unlock()
	assert.False(t, pending)
}

func TestManagerShutdownFlushesPendingSnapshots(t *testing.T) {
	dir := t.TempDir()
	mgr, _, err := NewManager(Config{JobsDir: dir})
	require.NoError(t, err)
	mgr.persistDelay = time.Hour
	require.NoError(t, mgr.savePersistedSnapshot(persistedJob{ID: "shutdown", State: "running", Seq: 7, LastMessage: "latest"}))

	require.NoError(t, mgr.Shutdown(time.Second))
	got, ok := readPersistedJob(filepath.Join(dir, "shutdown.json"))
	require.True(t, ok)
	assert.Equal(t, int64(7), got.Seq)
	assert.Equal(t, "latest", got.LastMessage)
}

func TestSavePersistedSnapshotSkipsStaleSeq(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	newer := persistedJob{
		ID:        "seqjob01",
		SessionID: "main-job-seqjob01",
		Provider:  "p",
		Model:     "m",
		State:     "running",
		Seq:       2,
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, savePersistedSnapshot(dir, newer))

	older := newer
	older.Seq = 1
	older.State = "queued"
	require.NoError(t, savePersistedSnapshot(dir, older))

	data, err := os.ReadFile(filepath.Join(dir, "seqjob01.json"))
	require.NoError(t, err)
	var got persistedJob
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, int64(2), got.Seq)
	assert.Equal(t, "running", got.State)
}

// TestLoadOrphaned_RewritesRunningAndQueued asserts that any persisted
// job in StateRunning is rewritten as Abandoned and one in StateQueued as
// Cancelled, both with reason "previous app exit". The split is the honest
// one: a queued job provably never ran, so its outcome is known, while a
// running job's outcome is not. Returns the resurrected jobs so callers can
// hydrate their map.
func TestLoadOrphaned_RewritesRunningAndQueued(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()

	// Pre-write 3 jobs: Running, Queued, Completed.
	cases := []struct {
		id    string
		state State
	}{
		{"r1111111", StateRunning},
		{"q2222222", StateQueued},
		{"c3333333", StateCompleted},
	}
	for _, c := range cases {
		j := &Job{ID: c.id, SessionID: "main-job-" + c.id, Provider: "p", Model: "m", State: c.state, CreatedAt: now}
		require.NoError(t, saveSnapshot(dir, j))
	}

	resurrected, err := loadOrphaned(dir)
	require.NoError(t, err)
	require.Len(t, resurrected, 2, "Running + Queued, not Completed")

	ids := map[string]bool{}
	byID := map[string]*Job{}
	for _, j := range resurrected {
		ids[j.ID] = true
		byID[j.ID] = j
		assert.Equal(t, "previous app exit", j.Reason)
		assert.False(t, j.FinishedAt.IsZero())
	}
	// A running job's outcome is unknown; a queued one's is not.
	assert.Equal(t, StateAbandoned, byID["r1111111"].State,
		"a job that was running is abandoned, never a confirmed cancellation")
	assert.Equal(t, AbandonCauseAppExit, byID["r1111111"].AbandonCause)
	assert.Equal(t, StateCancelled, byID["q2222222"].State,
		"a queued job provably never started, so cancelled is honest")
	assert.Empty(t, byID["q2222222"].AbandonCause,
		"only abandoned jobs carry a cause")
	assert.True(t, ids["r1111111"])
	assert.True(t, ids["q2222222"])
	assert.False(t, ids["c3333333"])

	// A second call should find nothing — they're now terminal.
	again, err := loadOrphaned(dir)
	require.NoError(t, err)
	assert.Empty(t, again)
}

// The jobs directory is shared across every project on the machine. An
// instance rooted in one project must not rewrite a job created by an
// instance rooted in another: that instance may still be running it, and
// marking it Recovered makes it eligible for a duplicate resubmit.
func TestLoadPersistedJobs_LeavesAnotherRootsLiveJobsAlone(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()

	foreign := &Job{
		ID: "foreign1", SessionID: "main-job-foreign1", Provider: "p", Model: "m",
		State: StateRunning, CreatedAt: now, OwnerRoot: "/projects/other", AllowWrite: true,
	}
	mine := &Job{
		ID: "mine1111", SessionID: "main-job-mine1111", Provider: "p", Model: "m",
		State: StateRunning, CreatedAt: now, OwnerRoot: "/projects/here",
	}
	require.NoError(t, saveSnapshot(dir, foreign))
	require.NoError(t, saveSnapshot(dir, mine))

	loaded, recovered, unreadable, err := loadPersistedJobs(dir, "/projects/here")
	require.NoError(t, err)
	assert.Empty(t, unreadable)

	require.Len(t, recovered, 1, "only this root's job is abandoned")
	assert.Equal(t, "mine1111", recovered[0].ID)
	for _, j := range loaded {
		assert.NotEqual(t, "foreign1", j.ID, "another root's live job must not be loaded here")
	}

	// The foreign record is untouched on disk, so its owner still sees it running.
	stored, ok := readPersistedJob(filepath.Join(dir, "foreign1.json"))
	require.True(t, ok)
	assert.Equal(t, "running", stored.State)
	assert.False(t, stored.Recovered)
}

// A record written before ownership tracking has no root and must still be
// recovered, so upgrading does not strand existing jobs.
func TestLoadPersistedJobs_LegacyRecordsWithoutOwnerAreRecovered(t *testing.T) {
	dir := t.TempDir()
	legacy := &Job{
		ID: "legacy11", SessionID: "main-job-legacy11", Provider: "p", Model: "m",
		State: StateRunning, CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, saveSnapshot(dir, legacy))

	_, recovered, unreadable, err := loadPersistedJobs(dir, "/projects/here")
	require.NoError(t, err)
	assert.Empty(t, unreadable)
	require.Len(t, recovered, 1)
	assert.Equal(t, "legacy11", recovered[0].ID)
}

// A job that cannot be read must be reported, never silently dropped:
// vanishing without a word is indistinguishable from never having existed.
func TestLoadPersistedJobs_ReportsUnreadableRecords(t *testing.T) {
	dir := t.TempDir()
	good := &Job{
		ID: "good1111", SessionID: "main-job-good1111", Provider: "p", Model: "m",
		State: StateCompleted, CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, saveSnapshot(dir, good))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "future.json"),
		[]byte(`{"format_version":99,"id":"future","state":"running"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "weird.json"),
		[]byte(`{"format_version":1,"id":"weird","state":"teleported"}`), 0o600))

	loaded, _, unreadable, err := loadPersistedJobs(dir, "")
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.Len(t, unreadable, 3)

	reasons := map[string]string{}
	for _, u := range unreadable {
		reasons[filepath.Base(u.Path)] = u.Reason
	}
	assert.Contains(t, reasons["broken.json"], "malformed job record")
	assert.Contains(t, reasons["future.json"], "newer than this build supports")
	assert.Contains(t, reasons["weird.json"], `unrecognised job state "teleported"`)
}

// Refusing to overwrite a newer record keeps a future build's state intact
// rather than silently downgrading it.
func TestSavePersistedSnapshot_RefusesToClobberNewerFormat(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fut12345.json"),
		[]byte(`{"format_version":99,"id":"fut12345","state":"running","seq":4}`), 0o600))

	err := savePersistedSnapshot(dir, persistedJob{
		FormatVersion: jobFormatVersion, ID: "fut12345", State: "cancelled", Seq: 5,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "newer than this build supports")

	stored, ok := readPersistedJob(filepath.Join(dir, "fut12345.json"))
	require.True(t, ok)
	assert.Equal(t, "running", stored.State)
}

// TestLoadOrphaned_MissingDirReturnsEmpty ensures a non-existent jobs
// dir is not an error — first-run is normal.
func TestLoadOrphaned_MissingDirReturnsEmpty(t *testing.T) {
	resurrected, err := loadOrphaned(filepath.Join(t.TempDir(), "does-not-exist"))
	require.NoError(t, err)
	assert.Empty(t, resurrected)
}

// TestLoadOrphaned_SkipsNonJSONAndTempFiles ensures stray files don't
// crash the loader.
func TestLoadOrphaned_SkipsNonJSONAndTempFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".job.tmp.json.tmp"), []byte("garbage"), 0o600))
	resurrected, err := loadOrphaned(dir)
	require.NoError(t, err)
	assert.Empty(t, resurrected)
}
