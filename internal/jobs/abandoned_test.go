package jobs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/testwait"
	"github.com/packetcode/packetcode/internal/tools"
)

// writeRawJobFile seeds a record as literal JSON. Round-tripping through
// toPersisted would only prove this build agrees with itself; these tests need
// bytes that a different build could plausibly have written.
func writeRawJobFile(t *testing.T, dir, id, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".json"), []byte(body), 0o600))
}

// PCMP10. A context carries no cause: a user cancel, an app shutdown, and a
// dead transport all reach the worker as an identical context.Canceled. These
// tests pin the discrimination that makes the difference reportable, because
// the failure mode is silent — every one of these cases used to be written as
// a confirmed cancellation with no error text at all.

// classifyCancelled is the whole discriminator, so it gets a direct table
// rather than only end-to-end coverage. Each row is a claim packetcode makes
// to the user about what happened to their work.
func TestClassifyCancelled_DiscriminatesRequestFromLoss(t *testing.T) {
	mgr, _ := newTestManager(t, &scriptedProvider{turns: scriptedHello()})

	cases := []struct {
		name      string
		request   CancelRequest
		fallback  AbandonCause
		wantState State
		wantCause AbandonCause
	}{
		{
			name:      "a human asked, so the outcome is known",
			request:   CancelRequestUser,
			fallback:  AbandonCauseUnknown,
			wantState: StateCancelled,
			wantCause: "",
		},
		{
			name:      "the app exited underneath a running job",
			request:   CancelRequestShutdown,
			fallback:  AbandonCauseUnknown,
			wantState: StateAbandoned,
			wantCause: AbandonCauseAppExit,
		},
		{
			name:      "nothing asked and nothing else is known",
			request:   CancelRequestNone,
			fallback:  AbandonCauseUnknown,
			wantState: StateAbandoned,
			wantCause: AbandonCauseUnknown,
		},
		{
			name:      "nothing asked, but a transport error was observed",
			request:   CancelRequestNone,
			fallback:  AbandonCauseTransportLost,
			wantState: StateAbandoned,
			wantCause: AbandonCauseTransportLost,
		},
		{
			// A recorded request is the stronger fact. Evidence of a broken
			// transport must not override someone deliberately stopping the
			// job, or every cancel of a remote job reads as a loss.
			name:      "a recorded request beats transport evidence",
			request:   CancelRequestUser,
			fallback:  AbandonCauseTransportLost,
			wantState: StateCancelled,
			wantCause: "",
		},
		{
			// An unreadable request is not evidence that anyone asked.
			name:      "an unrecognised request is treated as no request",
			request:   CancelRequest("garbage-from-a-newer-build"),
			fallback:  AbandonCauseUnknown,
			wantState: StateAbandoned,
			wantCause: AbandonCauseUnknown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j := &Job{ID: "cls00001", CancelRequest: tc.request}
			state, cause := mgr.classifyCancelledWithCause(j, tc.fallback)
			assert.Equal(t, tc.wantState, state)
			assert.Equal(t, tc.wantCause, cause)
		})
	}
}

// An explicit /cancel is still a confirmed cancellation. This is the control
// case: the new state must not swallow the one outcome packetcode genuinely
// does know.
func TestCancel_RecordsRequestAndStaysCancelled(t *testing.T) {
	jobsDir := t.TempDir()
	mgr, _ := newTestManager(t, &scriptedProvider{holdOpen: true}, func(c *Config) {
		c.JobsDir = jobsDir
	})

	snap, err := mgr.Spawn(SpawnRequest{Prompt: "hold"})
	require.Nil(t, err)
	waitFor(t, 2*time.Second, "job running", func() bool {
		s, ok := mgr.Get(snap.ID)
		return ok && s.State == StateRunning
	})

	require.True(t, mgr.Cancel(snap.ID))
	waitFor(t, 2*time.Second, "job terminal", func() bool {
		s, ok := mgr.Get(snap.ID)
		return ok && s.State.IsTerminal()
	})

	got, ok := mgr.Get(snap.ID)
	require.True(t, ok)
	assert.Equal(t, StateCancelled, got.State, "a user cancel is a confirmed cancellation")
	assert.Empty(t, got.AbandonCause, "only abandoned jobs carry a cause")
	assert.False(t, got.State.IsSuccess())

	// The request must be on disk, not just in memory. It is written before
	// the context is cancelled precisely so a crash between the two cannot
	// turn a deliberate cancel into a phantom loss.
	//
	// Shutdown first: markTerminal flips memory before it flushes, and
	// loadPersistedJobs reconciles anything still marked running — so reading
	// the directory under a live manager would race the flush and rewrite the
	// very record under test.
	require.NoError(t, mgr.Shutdown(testwait.Timeout(2*time.Second)))

	reloaded, _, unread, lerr := loadPersistedJobs(jobsDir, "")
	require.NoError(t, lerr)
	assert.Empty(t, unread)
	require.Len(t, reloaded, 1)
	assert.Equal(t, CancelRequestUser, reloaded[0].CancelRequest)
	assert.Equal(t, StateCancelled, reloaded[0].State,
		"a cancel the user asked for must survive as a cancellation, not a loss")
	assert.Empty(t, reloaded[0].AbandonCause)
}

// The abandoned state and its cause must survive the round-trip to disk, or
// the honest verdict is lost at exactly the moment it matters: after a
// restart, when the original process is gone.
func TestPersistence_AbandonedRoundTripsWithCause(t *testing.T) {
	dir := t.TempDir()
	for _, cause := range []AbandonCause{AbandonCauseAppExit, AbandonCauseTransportLost, AbandonCauseUnknown} {
		j := &Job{
			ID:           "ab" + string(cause)[:6],
			SessionID:    "main-job",
			Provider:     "p",
			Model:        "m",
			State:        StateAbandoned,
			AbandonCause: cause,
			Error:        "ssh: connection lost",
			CreatedAt:    time.Now().UTC(),
			FinishedAt:   time.Now().UTC(),
		}
		require.NoError(t, saveSnapshot(dir, j))
	}

	loaded, _, unreadable, err := loadPersistedJobs(dir, "")
	require.NoError(t, err)
	assert.Empty(t, unreadable, "this build must read the state it writes")
	require.Len(t, loaded, 3)

	byCause := map[AbandonCause]*Job{}
	for _, j := range loaded {
		assert.Equal(t, StateAbandoned, j.State)
		assert.True(t, j.State.IsTerminal())
		assert.False(t, j.State.IsSuccess())
		assert.Equal(t, "ssh: connection lost", j.Error,
			"the transport error is evidence and must not be dropped")
		byCause[j.AbandonCause] = j
	}
	assert.Contains(t, byCause, AbandonCauseAppExit)
	assert.Contains(t, byCause, AbandonCauseTransportLost)
	assert.Contains(t, byCause, AbandonCauseUnknown)
}

// Records written before this state existed carry Cancelled + Recovered. They
// must keep loading unchanged: reconciliation history is evidence, and
// rewriting it would retroactively restate what an older build reported.
func TestPersistence_LegacyRecoveredCancelledStillLoads(t *testing.T) {
	dir := t.TempDir()
	writeRawJobFile(t, dir, "lg111111", `{
		"format_version": 1,
		"id": "lg111111",
		"session_id": "main-job-lg111111",
		"prompt": "legacy",
		"provider": "p",
		"model": "m",
		"state": "cancelled",
		"reason": "previous app exit",
		"recovered": true,
		"created_at": "2026-08-01T00:00:00Z"
	}`)

	loaded, _, unreadable, err := loadPersistedJobs(dir, "")
	require.NoError(t, err)
	assert.Empty(t, unreadable)
	require.Len(t, loaded, 1)

	assert.Equal(t, StateCancelled, loaded[0].State, "legacy records are not rewritten")
	assert.True(t, loaded[0].Recovered, "resubmit eligibility still keys off Recovered")
	assert.Empty(t, loaded[0].AbandonCause, "a non-abandoned record carries no cause")
}

// An abandoned record with a cause this build does not recognise reads back as
// unknown rather than being rejected. The cause is descriptive detail hung off
// the state; "unknown" is precisely what an unreadable cause means, so coercing
// it loses nothing that was ever claimed. The State itself is deliberately NOT
// treated this way — see the unrecognised-state case in persistence_test.go.
func TestPersistence_UnknownCauseDegradesToUnknownNotUnreadable(t *testing.T) {
	dir := t.TempDir()
	writeRawJobFile(t, dir, "uc111111", `{
		"format_version": 1,
		"id": "uc111111",
		"session_id": "main-job-uc111111",
		"provider": "p",
		"model": "m",
		"state": "abandoned",
		"abandon_cause": "eaten-by-a-grue",
		"created_at": "2026-08-01T00:00:00Z"
	}`)

	loaded, _, unreadable, err := loadPersistedJobs(dir, "")
	require.NoError(t, err)
	assert.Empty(t, unreadable, "an unreadable cause must not cost us the whole record")
	require.Len(t, loaded, 1)
	assert.Equal(t, StateAbandoned, loaded[0].State)
	assert.Equal(t, AbandonCauseUnknown, loaded[0].AbandonCause)
}

// IsSuccess exists so callers stop enumerating failure states. An allowlist of
// failures silently reports every state added later as a success, which is how
// an abandoned sub-agent would have been handed to the parent model as a win.
func TestState_IsSuccessIsAnAllowlistOfOne(t *testing.T) {
	assert.True(t, StateCompleted.IsSuccess())
	for _, s := range []State{StateQueued, StateRunning, StateFailed, StateCancelled, StateAbandoned} {
		assert.False(t, s.IsSuccess(), "%s must never read as success", s)
	}
	assert.Equal(t, "abandoned", StateAbandoned.String())
	assert.True(t, StateAbandoned.IsTerminal(), "abandoned is terminal; a non-terminal one hangs Shutdown")
}

// InspectRecords must never write. doctor runs it against the shared jobs
// directory while another packetcode may be live in the same project root,
// and the loader it shares code with actively rewrites queued/running records.
// If inspection ever gained that behaviour, running a diagnostic would mark a
// colleague's in-flight work abandoned.
func TestInspectRecords_ReportsWithoutMutating(t *testing.T) {
	dir := t.TempDir()
	running := &Job{
		ID: "in111111", SessionID: "main-job-in111111", Provider: "p", Model: "m",
		State: StateRunning, CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, saveSnapshot(dir, running))
	writeRawJobFile(t, dir, "in222222", `{"format_version":99,"id":"in222222","state":"running"}`)
	writeRawJobFile(t, dir, "in333333", `{"format_version":1,"id":"in333333","state":"teleported"}`)

	before, rerr := os.ReadFile(filepath.Join(dir, "in111111.json"))
	require.NoError(t, rerr)

	readable, unreadable, err := InspectRecords(dir)
	require.NoError(t, err)
	assert.Equal(t, 1, readable)
	require.Len(t, unreadable, 2, "a future version and an unknown state are both unreadable")

	reasons := unreadable[0].Reason + " " + unreadable[1].Reason
	assert.Contains(t, reasons, "newer than this build supports")
	assert.Contains(t, reasons, "unrecognised job state")
	for _, u := range unreadable {
		assert.NotEmpty(t, u.Path, "the path is the actionable part of the report")
	}

	after, rerr := os.ReadFile(filepath.Join(dir, "in111111.json"))
	require.NoError(t, rerr)
	assert.Equal(t, string(before), string(after),
		"inspection must leave a running record byte-identical")

	// And the running record must still be reconcilable afterwards, i.e.
	// inspection did not consume it.
	recovered, lerr := loadOrphaned(dir)
	require.NoError(t, lerr)
	require.Len(t, recovered, 1)
	assert.Equal(t, StateAbandoned, recovered[0].State)
}

// A job's plan must reach a snapshot while the job is still running: that is
// the whole point of surfacing it in Agent View, where "what is it doing now"
// is the question being asked.
func TestSnapshot_CarriesTheJobsTodoList(t *testing.T) {
	mgr, _ := newTestManager(t, &scriptedProvider{holdOpen: true})
	snap, err := mgr.Spawn(SpawnRequest{Prompt: "hold"})
	require.Nil(t, err)

	mgr.mu.RLock()
	job := mgr.jobs[snap.ID]
	mgr.mu.RUnlock()
	require.NotNil(t, job, "the manager should still own the job")

	// Write through the store the worker's tool shares.
	job.todos.Replace([]TodoItem{
		{Content: "read the code", Status: TodoCompleted},
		{Content: "make the change", Status: TodoInProgress},
		{Content: "run the tests", Status: TodoPending},
	})

	got, ok := mgr.Get(snap.ID)
	require.True(t, ok)
	require.Len(t, got.Todos, 3)
	assert.Equal(t, "make the change", got.Todos[1].Content)
	assert.Equal(t, TodoInProgress, got.Todos[1].Status)
}

// The plan is evidence of what an interrupted job was part-way through, so it
// has to survive the round-trip to disk like the rest of the record.
func TestPersistence_TodoListRoundTrips(t *testing.T) {
	dir := t.TempDir()
	j := &Job{
		ID: "td111111", SessionID: "main-job-td111111", Provider: "p", Model: "m",
		State: StateAbandoned, AbandonCause: AbandonCauseAppExit,
		CreatedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(),
		todos: newTodoStoreForTest([]TodoItem{
			{Content: "done bit", Status: TodoCompleted},
			{Content: "the bit it was on", Status: TodoInProgress},
		}),
	}
	require.NoError(t, saveSnapshot(dir, j))

	loaded, _, unreadable, err := loadPersistedJobs(dir, "")
	require.NoError(t, err)
	assert.Empty(t, unreadable)
	require.Len(t, loaded, 1)

	got := loaded[0].Todos()
	require.Len(t, got, 2, "an abandoned job should still show what it was part-way through")
	assert.Equal(t, "the bit it was on", got[1].Content)
	assert.Equal(t, TodoInProgress, got[1].Status)
}

// A malformed entry loses that line, not the record. The job's State says what
// happened to the work; a bad todo says nothing, so it must not cost us the
// evidence around it.
func TestPersistence_MalformedTodoDropsTheLineNotTheJob(t *testing.T) {
	dir := t.TempDir()
	writeRawJobFile(t, dir, "td222222", `{
		"format_version": 1,
		"id": "td222222",
		"session_id": "main-job-td222222",
		"provider": "p",
		"model": "m",
		"state": "completed",
		"created_at": "2026-08-01T00:00:00Z",
		"todos": [
			{"content": "good", "status": "completed"},
			{"content": "bad status", "status": "teleported"},
			{"content": "", "status": "pending"}
		]
	}`)

	loaded, _, unreadable, err := loadPersistedJobs(dir, "")
	require.NoError(t, err)
	assert.Empty(t, unreadable, "a bad todo must not make the whole job unreadable")
	require.Len(t, loaded, 1)

	got := loaded[0].Todos()
	require.Len(t, got, 1)
	assert.Equal(t, "good", got[0].Content)
}

func newTodoStoreForTest(items []TodoItem) *tools.TodoStore {
	store := tools.NewTodoStore()
	store.Replace(items)
	return store
}
