package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/packetcode/packetcode/internal/cost"
	"github.com/packetcode/packetcode/internal/jobs"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/tools"
)

// Test 23: a completed background job's Result, once drained, is
// injected into the main session as a RoleUser message so the LLM
// sees it on the next turn. We exercise the injection path directly
// (per spec, option (a)) by seeding a synthetic Result into the
// manager's drain queue via a test-only helper, then calling
// App.injectPendingJobResults and inspecting the session.
func TestApp_SpawnInjectsResultIntoNextTurn(t *testing.T) {
	tmp := t.TempDir()

	// Keep stray writes contained by redirecting both HOME (Unix) and
	// USERPROFILE (Windows) in case any code path reaches os.UserHomeDir.
	_ = os.Setenv("HOME", tmp)
	_ = os.Setenv("USERPROFILE", tmp)

	sessionsDir := filepath.Join(tmp, "sessions")
	backupsDir := filepath.Join(tmp, "backups")
	jobsDir := filepath.Join(tmp, "jobs")
	for _, d := range []string{sessionsDir, backupsDir, jobsDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	sessions := session.NewManager(sessionsDir)
	if _, err := sessions.New("fake", "f1"); err != nil {
		t.Fatalf("session.New: %v", err)
	}

	tallyPath := filepath.Join(tmp, "tally.json")
	tracker, err := cost.NewTracker(tallyPath, func(string, string) (float64, float64) { return 0, 0 })
	if err != nil {
		t.Fatalf("cost.NewTracker: %v", err)
	}

	toolReg := tools.NewRegistry()

	mgr, _, err := jobs.NewManager(jobs.Config{
		Tools:         toolReg,
		MainSessions:  sessions,
		SessionsDir:   sessionsDir,
		BackupsDir:    backupsDir,
		JobsDir:       jobsDir,
		CostTracker:   tracker,
		MaxConcurrent: 1,
		MaxDepth:      2,
		MaxTotal:      8,
		Root:          tmp,
	})
	if err != nil {
		t.Fatalf("jobs.NewManager: %v", err)
	}
	defer mgr.Shutdown(500 * time.Millisecond)

	app := &App{
		deps: Deps{Sessions: sessions},
		jobs: mgr,
	}

	// Case 1: nothing to drain → no message appended.
	startLen := messageCount(sessions)
	app.injectPendingJobResults()
	if messageCount(sessions) != startLen {
		t.Fatalf("empty drain should not add messages")
	}

	// Case 2: seed a synthetic result and confirm the App appends a
	// RoleUser message matching the spec format.
	jobs.InjectResultForTests(mgr, jobs.Result{
		JobID:   "7f3a",
		State:   jobs.StateCompleted,
		Summary: "14 call sites in 8 files",
	})

	app.injectPendingJobResults()
	cur := sessions.Current()
	if cur == nil {
		t.Fatalf("no current session")
	}
	if len(cur.Messages) == 0 {
		t.Fatalf("expected a RoleUser message appended")
	}
	last := cur.Messages[len(cur.Messages)-1]
	if last.Role != provider.RoleUser {
		t.Fatalf("role = %s, want user", last.Role)
	}
	if !strings.Contains(last.Content, "[Background job 7f3a result]") {
		t.Fatalf("missing header in content: %q", last.Content)
	}
	if !strings.Contains(last.Content, "14 call sites") {
		t.Fatalf("missing summary in content: %q", last.Content)
	}

	// Case 3: a subsequent call with nothing new shouldn't double-inject.
	lenAfter := messageCount(sessions)
	app.injectPendingJobResults()
	if messageCount(sessions) != lenAfter {
		t.Fatalf("drain queue should have been emptied; re-injection occurred")
	}
}

// messageCount returns the number of messages on the manager's current
// session (0 if none). Used as a pre/post baseline.
func messageCount(sm *session.Manager) int {
	cur := sm.Current()
	if cur == nil {
		return 0
	}
	return len(cur.Messages)
}
