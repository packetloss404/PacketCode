package jobs

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedAbandonedJob writes a Running job into jobsDir so that the next
// Manager constructed over that dir reconciles it exactly as a restart
// after an unclean exit would.
func seedAbandonedJob(t *testing.T, jobsDir, id, prompt string) {
	t.Helper()
	j := &Job{
		ID:        id,
		SessionID: "main-job-" + id,
		Prompt:    prompt,
		Provider:  "scripted",
		Model:     "scripted-model",
		State:     StateRunning,
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, saveSnapshot(jobsDir, j))
}

// managerOverAbandoned builds a Manager whose jobs dir already contains an
// abandoned job, mirroring the real restart path through NewManager.
func managerOverAbandoned(t *testing.T, id, prompt string) (*Manager, string) {
	t.Helper()
	jobsDir := t.TempDir()
	seedAbandonedJob(t, jobsDir, id, prompt)
	mgr, _ := newTestManager(t, &scriptedProvider{turns: scriptedHello()}, func(c *Config) {
		c.JobsDir = jobsDir
	})
	return mgr, jobsDir
}

// Reconciliation must mark the job so callers can offer a resubmit without
// re-deriving "was this abandoned?" from the free-form reason string.
func TestLoadOrphaned_MarksRecovered(t *testing.T) {
	dir := t.TempDir()
	seedAbandonedJob(t, dir, "ab111111", "do the thing")

	recovered, err := loadOrphaned(dir)
	require.NoError(t, err)
	require.Len(t, recovered, 1)

	assert.True(t, recovered[0].Recovered, "reconciled job must be marked Recovered")
	assert.Equal(t, StateAbandoned, recovered[0].State)
	assert.Equal(t, AbandonCauseAppExit, recovered[0].AbandonCause)
	assert.Equal(t, "previous app exit", recovered[0].Reason)
	assert.Empty(t, recovered[0].ResubmittedAs, "nothing has been resubmitted yet")

	// The marker must survive the round-trip to disk.
	reloaded, _, _, err := loadPersistedJobs(dir, "")
	require.NoError(t, err)
	require.Len(t, reloaded, 1)
	assert.True(t, reloaded[0].Recovered)
}

func TestResubmit_UnknownJob(t *testing.T) {
	mgr, _ := newTestManager(t, &scriptedProvider{turns: scriptedHello()})
	_, err := mgr.Resubmit("nope1234")
	require.NotNil(t, err)
	assert.Equal(t, "unknown_job", err.Code)
}

// A job that finished normally must not be resubmittable — only work a
// previous process abandoned is.
func TestResubmit_RejectsJobThatWasNotAbandoned(t *testing.T) {
	mgr, _ := newTestManager(t, &scriptedProvider{turns: scriptedHello()})
	snap, perr := mgr.Spawn(SpawnRequest{Prompt: "hello"})
	require.Nil(t, perr)
	waitFor(t, 5*time.Second, "job terminal", func() bool {
		s, ok := mgr.Get(snap.ID)
		return ok && s.State.IsTerminal()
	})

	_, err := mgr.Resubmit(snap.ID)
	require.NotNil(t, err)
	assert.Equal(t, "not_recovered", err.Code)
}

// The core honesty property of PCH4: resubmitting starts a separate job and
// never claims the abandoned run continued.
func TestResubmit_SpawnsNewJobAndLinksBothWays(t *testing.T) {
	mgr, jobsDir := managerOverAbandoned(t, "ab222222", "rebuild the index")

	old, ok := mgr.Get("ab222222")
	require.True(t, ok)
	require.True(t, old.Recovered)

	snap, err := mgr.Resubmit(old.ID)
	require.Nil(t, err)
	require.NotEqual(t, old.ID, snap.ID, "resubmit must create a new job, not reuse the id")
	assert.Equal(t, old.ID, snap.ResubmitOf)
	assert.Equal(t, "rebuild the index", snap.Prompt)
	assert.Equal(t, old.Provider, snap.Provider)
	assert.Equal(t, old.Model, snap.Model)

	// The abandoned job keeps its terminal state and gains a forward link.
	before, ok := mgr.Get(old.ID)
	require.True(t, ok)
	assert.Equal(t, StateAbandoned, before.State, "the original must stay abandoned")
	assert.Equal(t, AbandonCauseAppExit, before.AbandonCause)
	assert.True(t, before.Recovered)
	assert.Equal(t, snap.ID, before.ResubmittedAs)

	// Both links must be durable across a reload.
	//
	// Wait for the successor's *record*, not for mgr.Get to report a terminal
	// state. markTerminalCause flips the in-memory state under the manager
	// lock and only persists after releasing it, so "terminal in memory" does
	// not yet mean "terminal on disk". Reading during that window used to find
	// the record still Running, which sent the loader down its reconcile-and-
	// rewrite path against a file the manager was writing at the same moment --
	// and on Windows one of those two collides and the record is dropped.
	//
	// readPersistedJob is the right instrument for the wait because it only
	// reads. Polling loadPersistedJobs would rewrite what it is waiting on.
	successorRecord := filepath.Join(jobsDir, snap.ID+".json")
	waitFor(t, 5*time.Second, "successor terminal on disk", func() bool {
		p, ok := readPersistedJob(successorRecord)
		return ok && parseState(p.State).IsTerminal()
	})

	reloaded, _, unreadable, lerr := loadPersistedJobs(jobsDir, "")
	require.NoError(t, lerr)
	// Asserted rather than discarded: a record the loader rejects is dropped
	// from `reloaded`, so without this the failure is "map does not contain
	// <id>" and says nothing about why. It cost an afternoon once.
	require.Empty(t, unreadable, "every record written by this test must load back")
	byID := map[string]*Job{}
	for _, j := range reloaded {
		byID[j.ID] = j
	}
	require.Contains(t, byID, old.ID)
	require.Contains(t, byID, snap.ID)
	assert.Equal(t, snap.ID, byID[old.ID].ResubmittedAs)
	assert.Equal(t, old.ID, byID[snap.ID].ResubmitOf)
	assert.Equal(t, StateAbandoned, byID[old.ID].State,
		"the abandoned record must not be overwritten by its successor")
	assert.Equal(t, AbandonCauseAppExit, byID[old.ID].AbandonCause,
		"the cause must survive the round-trip to disk")
}

func TestResubmit_IsOnlyAllowedOnce(t *testing.T) {
	mgr, _ := managerOverAbandoned(t, "ab333333", "run it again")

	first, err := mgr.Resubmit("ab333333")
	require.Nil(t, err)

	_, second := mgr.Resubmit("ab333333")
	require.NotNil(t, second)
	assert.Equal(t, "already_resubmitted", second.Code)
	assert.Contains(t, second.Reason, first.ID,
		"the refusal should point at the existing successor")
}

// An oversize saved prompt is refused outright. Truncating it would start a
// materially different run than the one the user asked to re-run.
func TestResubmit_RefusesOversizePromptRatherThanTruncating(t *testing.T) {
	huge := strings.Repeat("x", MaxResubmitPromptBytes+1)
	mgr, _ := managerOverAbandoned(t, "ab444444", huge)

	_, err := mgr.Resubmit("ab444444")
	require.NotNil(t, err)
	assert.Equal(t, "prompt_too_large", err.Code)

	got, ok := mgr.Get("ab444444")
	require.True(t, ok)
	assert.Empty(t, got.ResubmittedAs, "nothing should have been started")
}

func TestResubmit_RefusesEmptyPrompt(t *testing.T) {
	mgr, _ := managerOverAbandoned(t, "ab555555", "")

	_, err := mgr.Resubmit("ab555555")
	require.NotNil(t, err)
	assert.Equal(t, "empty_prompt", err.Code)
}

func TestRecoveredResubmittable_ExcludesAlreadyResubmitted(t *testing.T) {
	jobsDir := t.TempDir()
	seedAbandonedJob(t, jobsDir, "ab666666", "first")
	seedAbandonedJob(t, jobsDir, "ab777777", "second")
	mgr, _ := newTestManager(t, &scriptedProvider{turns: scriptedHello()}, func(c *Config) {
		c.JobsDir = jobsDir
	})

	require.Len(t, mgr.RecoveredResubmittable(), 2)

	_, err := mgr.Resubmit("ab666666")
	require.Nil(t, err)

	pending := mgr.RecoveredResubmittable()
	require.Len(t, pending, 1)
	assert.Equal(t, "ab777777", pending[0].ID,
		"a resubmitted job must drop off the pending list")
}

// An empty-prompt abandoned job is never offered for resubmit, since
// Resubmit would reject it.
func TestRecoveredResubmittable_SkipsEmptyPrompt(t *testing.T) {
	jobsDir := t.TempDir()
	seedAbandonedJob(t, jobsDir, "ab888888", "")
	mgr, _ := newTestManager(t, &scriptedProvider{turns: scriptedHello()}, func(c *Config) {
		c.JobsDir = jobsDir
	})
	assert.Empty(t, mgr.RecoveredResubmittable())
}
