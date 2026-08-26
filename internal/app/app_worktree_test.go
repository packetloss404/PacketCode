package app

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/packetcode/packetcode/internal/jobs"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/ui/components/conversation"
)

func TestRenderJobsTable_WorktreeStates(t *testing.T) {
	now := time.Now().Add(-10 * time.Second)
	snaps := []jobs.Snapshot{
		{
			ID:             "ok123456",
			State:          jobs.StateCompleted,
			Provider:       "openai",
			Model:          "gpt-5",
			Prompt:         "write docs",
			AllowWrite:     true,
			CreatedAt:      now,
			WorktreePath:   "wt/ok123456",
			WorktreeBranch: "packetcode-job-ok123456",
			WorktreeBase:   "0123456789abcdef",
		},
		{
			ID:           "bad12345",
			State:        jobs.StateFailed,
			Provider:     "openai",
			Model:        "gpt-5",
			Prompt:       "write code",
			AllowWrite:   true,
			CreatedAt:    now,
			WorktreeNote: "git rejected repository ownership",
		},
	}

	out := renderJobsTable(snaps)
	assert.Contains(t, out, "ROOT")
	assert.Contains(t, out, "worktree")
	assert.Contains(t, out, "failed")
	assert.Contains(t, out, "worktree: wt/ok123456")
	assert.Contains(t, out, "branch packetcode-job-ok123456")
	assert.Contains(t, out, "worktree unavailable: git rejected repository ownership")
	assert.NotContains(t, failedRow(out, "bad12"), "pending")
}

func TestAgentResultBodyIncludesWorktreeBranchAndBase(t *testing.T) {
	body := agentResultBody(jobs.Result{
		JobID:   "abc12345",
		State:   jobs.StateCompleted,
		Summary: "updated files",
		Artifacts: []jobs.Artifact{{
			ID:      "A1",
			Kind:    "file_change",
			Summary: "wrote main.go",
			Path:    "main.go",
		}},
		WorktreePath:   "wt/abc12345",
		WorktreeBranch: "packetcode-job-abc12345",
		WorktreeBase:   "deadbeef",
	})

	assert.Contains(t, body, "[Background job abc12345 handoff]")
	assert.Contains(t, body, "Outcome: completed")
	assert.Contains(t, body, "updated files")
	assert.Contains(t, body, "worktree: wt/abc12345")
	assert.Contains(t, body, "branch packetcode-job-abc12345")
	assert.Contains(t, body, "base deadbeef")
	assert.Contains(t, body, "Artifacts:")
	assert.Contains(t, body, "A1 file_change: wrote main.go")
}

func TestWorktreeNotificationsOnlyEmitOnce(t *testing.T) {
	a := &App{
		deps: Deps{
			Registry:   provider.NewRegistry(),
			Sessions:   session.NewManager(""),
			WorkingDir: t.TempDir(),
		},
		conversation:    conversation.New(),
		jobSeqSeen:      map[string]int64{},
		jobWorktreeSeen: map[string]bool{},
		jobTerminalSeen: map[string]bool{},
	}
	snap := jobs.Snapshot{
		ID:             "abc12345",
		State:          jobs.StateRunning,
		Seq:            1,
		AllowWrite:     true,
		WorktreePath:   "wt/abc12345",
		WorktreeBranch: "packetcode-job-abc12345",
		WorktreeBase:   "deadbeef",
	}

	_, _ = a.handleJobUpdate(snap)
	snap.Seq = 2
	_, _ = a.handleJobUpdate(snap)

	a.conversation.Resize(120, 20)
	out := a.conversation.View()
	assert.Equal(t, 1, strings.Count(out, "[job:abc12345 worktree]"))
	assert.Contains(t, out, "branch packetcode-job-")
	assert.Contains(t, out, "abc12345")
}

func failedRow(table, idPrefix string) string {
	for _, line := range strings.Split(table, "\n") {
		if strings.HasPrefix(line, idPrefix) {
			return line
		}
	}
	return ""
}

func TestFormatTerminalJobLine_AbandonedIsNotDone(t *testing.T) {
	started := time.Now().Add(-30 * time.Second)
	line := formatTerminalJobLine(jobs.Snapshot{
		ID:           "lost1234",
		State:        jobs.StateAbandoned,
		AbandonCause: jobs.AbandonCauseTransportLost,
		Provider:     "openai",
		Model:        "gpt-5",
		Error:        "ssh: connection reset by peer",
		StartedAt:    started,
		FinishedAt:   started.Add(30 * time.Second),
	})

	assert.Contains(t, line, "[job:lost1234 — abandoned (transport-lost)")
	// "— done" rather than "done": "abandoned" contains the substring.
	assert.NotContains(t, line, "— done")
	assert.NotContains(t, line, "cancelled")
	// The transport error is the only evidence the user has about why the
	// outcome was never confirmed, so it must survive into the notification.
	assert.Contains(t, line, "error: ssh: connection reset by peer")
}

func TestFormatTerminalJobLine_AbandonedWithoutCauseOmitsIt(t *testing.T) {
	line := formatTerminalJobLine(jobs.Snapshot{
		ID:    "lost5678",
		State: jobs.StateAbandoned,
	})
	assert.Contains(t, line, "[job:lost5678 — abandoned ·")

	// Completed and cancelled labels are unchanged by the new default.
	assert.Contains(t, formatTerminalJobLine(jobs.Snapshot{ID: "ok123456", State: jobs.StateCompleted}), "— done ·")
	assert.Contains(t, formatTerminalJobLine(jobs.Snapshot{ID: "cn123456", State: jobs.StateCancelled}), "— cancelled ·")
	assert.Contains(t, formatTerminalJobLine(jobs.Snapshot{ID: "fl123456", State: jobs.StateFailed}), "— failed ·")
}

func TestRenderJobsTable_AbandonedWriteJobDoesNotClaimCleanRoot(t *testing.T) {
	now := time.Now().Add(-10 * time.Second)
	out := renderJobsTable([]jobs.Snapshot{{
		ID:           "lost1234",
		State:        jobs.StateAbandoned,
		AbandonCause: jobs.AbandonCauseAppExit,
		Provider:     "openai",
		Model:        "gpt-5",
		Prompt:       "refactor handlers",
		AllowWrite:   true,
		CreatedAt:    now,
	}})

	// "abandoned" is 9 chars and must survive the 10-wide STATE column intact.
	assert.Contains(t, out, "abandoned")
	assert.NotContains(t, out, "abandone…")
	// ROOT must not read "none": that asserts the worktree was released
	// cleanly, which is precisely what an unconfirmed outcome cannot assert.
	assert.NotContains(t, failedRow(out, "lost1"), "none")
	assert.Contains(t, failedRow(out, "lost1"), "failed")
}

// Prompts and session names are arbitrary user text. The table cells used to
// be clipped with a byte slice, which cuts a multi-byte rune in half (the
// terminal then renders U+FFFD) and leaves the cell's byte length disagreeing
// with the rune-counting padRight that aligns the next column.
func TestRenderJobsTable_PromptTruncationIsRuneSafe(t *testing.T) {
	out := renderJobsTable([]jobs.Snapshot{{
		ID:        "utf81234",
		State:     jobs.StateRunning,
		Provider:  "openai",
		Model:     "gpt-5",
		Prompt:    strings.Repeat("π", 80), // 80 runes, 160 bytes
		CreatedAt: time.Now(),
	}})

	assert.NotContains(t, out, "�")
	assert.True(t, strings.HasSuffix(out, strings.Repeat("π", 47)+"..."),
		"prompt cell should end in 47 whole runes plus an ellipsis, got %q", out)
}

func TestRenderJobsTable_MultilinePromptStaysOnItsOwnRow(t *testing.T) {
	out := renderJobsTable([]jobs.Snapshot{{
		ID:        "nl123456",
		State:     jobs.StateRunning,
		Prompt:    "first line\nsecond line",
		CreatedAt: time.Now(),
	}})

	// Header + exactly one row: an embedded newline must not break the table.
	assert.Len(t, strings.Split(out, "\n"), 2)
	assert.Contains(t, out, "first line second line")
}

func TestRenderSessionsTable_NameTruncationIsRuneSafe(t *testing.T) {
	out := renderSessionsTable([]session.Summary{{
		ID:           "abcdefgh1234",
		Name:         strings.Repeat("é", 40), // 40 runes, 80 bytes
		Provider:     "openai",
		Model:        "gpt-5",
		MessageCount: 3,
		UpdatedAt:    time.Now(),
	}}, "")

	assert.NotContains(t, out, "�")
	assert.Contains(t, out, strings.Repeat("é", 29)+"...")
}
