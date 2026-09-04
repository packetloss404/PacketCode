package jobs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/computers"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
)

// verifyRootProvider routes on the job's own prompt so a work job and the
// verifier that judges it can run against one fake. The verifier deliberately
// tries to write before it reads: a "read-only" verifier rooted in the tree it
// is judging must not be able to edit the work into passing.
type verifyRootProvider struct {
	scriptedProvider
}

func (p *verifyRootProvider) ChatCompletion(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	prompt := firstUserContent(req)
	turn := toolTurnCount(req)
	ch := make(chan provider.StreamEvent, 4)
	defer close(ch)

	emitCall := func(name, args string) {
		ch <- provider.StreamEvent{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: name}}
		ch <- provider.StreamEvent{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: args}}
		ch <- provider.StreamEvent{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}}
	}
	done := func(text string) {
		ch <- provider.StreamEvent{Type: provider.EventTextDelta, TextDelta: text}
		ch <- provider.StreamEvent{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 1, OutputTokens: 1}}
	}

	switch {
	case strings.Contains(prompt, "WORK") && turn == 0:
		emitCall("write_file", `{"path":"candidate.txt","content":"candidate content"}`)
	case strings.Contains(prompt, "WORK"):
		done("wrote the candidate change")
	case turn == 0:
		emitCall("write_file", `{"path":"pwned.txt","content":"verifier edit"}`)
	case turn == 1:
		emitCall("read_file", `{"path":"candidate.txt"}`)
	default:
		done("verifier saw: " + lastToolContent(req))
	}
	return ch, nil
}

func firstUserContent(req provider.ChatRequest) string {
	for _, m := range req.Messages {
		if m.Role == provider.RoleUser {
			return m.Content
		}
	}
	return ""
}

func lastToolContent(req provider.ChatRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == provider.RoleTool {
			return req.Messages[i].Content
		}
	}
	return ""
}

func toolTurnCount(req provider.ChatRequest) int {
	n := 0
	for _, m := range req.Messages {
		if m.Role == provider.RoleTool {
			n++
		}
	}
	return n
}

func waitTerminal(t *testing.T, mgr *Manager, id string, want State) Snapshot {
	t.Helper()
	waitFor(t, 5*time.Second, "job "+id+" reaches "+want.String(), func() bool {
		got, ok := mgr.Get(id)
		return ok && got.State.IsTerminal()
	})
	got, _ := mgr.Get(id)
	require.Equal(t, want, got.State, "job %s: %s", id, got.Error)
	return got
}

// A verifier rooted at the project root can only re-read code the work agent
// never touched, so its verdict attests to nothing but the work agent's own
// summary. It must read the candidate change itself — and must not be able to
// edit it.
func TestManager_VerifierReadsWorkWorktreeAndCannotWriteIt(t *testing.T) {
	root := initTestGitRepo(t)
	worktreesDir := t.TempDir()
	mgr, _ := newTestManager(t, &verifyRootProvider{}, func(c *Config) {
		c.Root = root
		c.WorktreesDir = worktreesDir
		c.Tools = makeMainRegistry(t, root)
	})

	work, perr := mgr.Spawn(SpawnRequest{Prompt: "WORK: write the candidate", AllowWrite: true})
	require.Nil(t, perr)
	workDone := waitTerminal(t, mgr, work.ID, StateCompleted)
	require.NotEmpty(t, workDone.WorktreePath)
	require.FileExists(t, filepath.Join(workDone.WorktreePath, "candidate.txt"))
	require.NoFileExists(t, filepath.Join(root, "candidate.txt"))

	verifier, perr := mgr.Spawn(SpawnRequest{
		Prompt:           "VERIFY: check the candidate",
		VerifyWorktreeOf: work.ID,
	})
	require.Nil(t, perr)
	verifierDone := waitTerminal(t, mgr, verifier.ID, StateCompleted)

	assert.Contains(t, verifierDone.Summary, "candidate content",
		"the verifier must be able to read the work agent's change")
	assert.NoFileExists(t, filepath.Join(workDone.WorktreePath, "pwned.txt"),
		"a read-only verifier must not be able to write into the worktree it judges")
	assert.NoFileExists(t, filepath.Join(root, "pwned.txt"))
}

// Without a verifier root the same run cannot see the change at all. This
// pins the failure the root exists to remove, so the test above cannot pass
// by accident.
func TestManager_ReadOnlyJobWithoutVerifyRootCannotSeeWorktree(t *testing.T) {
	root := initTestGitRepo(t)
	mgr, _ := newTestManager(t, &verifyRootProvider{}, func(c *Config) {
		c.Root = root
		c.WorktreesDir = t.TempDir()
		c.Tools = makeMainRegistry(t, root)
	})

	work, perr := mgr.Spawn(SpawnRequest{Prompt: "WORK: write the candidate", AllowWrite: true})
	require.Nil(t, perr)
	waitTerminal(t, mgr, work.ID, StateCompleted)

	verifier, perr := mgr.Spawn(SpawnRequest{Prompt: "VERIFY: check the candidate"})
	require.Nil(t, perr)
	verifierDone := waitTerminal(t, mgr, verifier.ID, StateCompleted)
	assert.NotContains(t, verifierDone.Summary, "candidate content")
}

// The tool registry, not the prompt, is what keeps a verifier read-only.
func TestRegistry_VerifierRootHasNoWriteTools(t *testing.T) {
	root := t.TempDir()
	worktree := t.TempDir()
	mgr := &Manager{cfg: Config{Tools: makeMainRegistry(t, root), Root: root}, pathLocks: pathLockMap{}}
	bm := session.NewBackupManager(t.TempDir(), "verifier-session")

	reg := mgr.buildJobToolRegistry(0, false /* allowWrite */, "ver00001", bm, nil, worktree)

	for _, name := range []string{"write_file", "patch_file", "execute_command"} {
		_, ok := reg.Get(name)
		assert.False(t, ok, "verifier registry must not expose %s", name)
	}
	readFile, ok := reg.Get("read_file")
	require.True(t, ok)
	require.NoError(t, os.WriteFile(filepath.Join(worktree, "candidate.txt"), []byte("candidate content"), 0o600))
	res, err := readFile.Execute(context.Background(), []byte(`{"path":"candidate.txt"}`))
	require.NoError(t, err)
	assert.False(t, res.IsError, res.Content)
	assert.Contains(t, res.Content, "candidate content")
}

// A job that may write cannot be rooted in the tree it verifies, whatever the
// caller asks for.
func TestSpawn_VerifyRootRefusedForWriteJob(t *testing.T) {
	root := initTestGitRepo(t)
	mgr, _ := newTestManager(t, &scriptedProvider{turns: scriptedHello()}, func(c *Config) {
		c.Root = root
		c.WorktreesDir = t.TempDir()
		c.Tools = makeMainRegistry(t, root)
	})
	mgr.recordWorktreeRoot(&Job{ID: "work0001"}, filepath.Join(t.TempDir(), "tree"))

	_, perr := mgr.Spawn(SpawnRequest{
		Prompt:           "verify",
		AllowWrite:       true,
		VerifyWorktreeOf: "work0001",
	})
	require.NotNil(t, perr)
	assert.Equal(t, "verify_root_denied", perr.Code)
}

// The field names a job, so a caller cannot nominate a directory. An id the
// Manager has no worktree for is not an error: a read-only work step never
// gets one, and its verifier correctly keeps the ordinary root.
func TestSpawn_VerifyRootOfUnknownJobKeepsProjectRoot(t *testing.T) {
	root := initTestGitRepo(t)
	mgr, _ := newTestManager(t, &verifyRootProvider{}, func(c *Config) {
		c.Root = root
		c.WorktreesDir = t.TempDir()
		c.Tools = makeMainRegistry(t, root)
	})

	snap, perr := mgr.Spawn(SpawnRequest{Prompt: "VERIFY: nothing", VerifyWorktreeOf: "nosuchjob"})
	require.Nil(t, perr)
	waitTerminal(t, mgr, snap.ID, StateCompleted)
	_, ok := mgr.verifyRootFor(snap.ID)
	assert.False(t, ok)
}

// A recorded root that no longer resolves inside the worktrees directory is
// refused rather than used. Without this the record — not the directory it
// points at — would be the whole boundary.
func TestSpawn_VerifyRootOutsideWorktreesDirIsRefused(t *testing.T) {
	root := initTestGitRepo(t)
	outside := t.TempDir()
	mgr, _ := newTestManager(t, &scriptedProvider{turns: scriptedHello()}, func(c *Config) {
		c.Root = root
		c.WorktreesDir = t.TempDir()
		c.Tools = makeMainRegistry(t, root)
	})
	mgr.recordWorktreeRoot(&Job{ID: "work0001"}, outside)

	_, perr := mgr.Spawn(SpawnRequest{Prompt: "verify", VerifyWorktreeOf: "work0001"})
	require.NotNil(t, perr)
	assert.Equal(t, "verify_root_denied", perr.Code)
	assert.Contains(t, perr.Reason, "not a packetcode worktree")
}

// A worktree on a Packet Computer is not a directory this machine has.
func TestSpawn_VerifyRootRefusesForeignWorkspace(t *testing.T) {
	root := initTestGitRepo(t)
	mgr, _ := newTestManager(t, &scriptedProvider{turns: scriptedHello()}, func(c *Config) {
		c.Root = root
		c.WorktreesDir = t.TempDir()
		c.Tools = makeMainRegistry(t, root)
	})
	mgr.recordWorktreeRoot(&Job{ID: "work0001", ComputerID: "pc_prod"}, "/home/dev/.packetcode/worktrees/k/work0001")

	_, perr := mgr.Spawn(SpawnRequest{Prompt: "verify", VerifyWorktreeOf: "work0001"})
	require.NotNil(t, perr)
	assert.Equal(t, "verify_root_denied", perr.Code)
	assert.Contains(t, perr.Reason, "pc_prod")
}

// A remote verifier inherits the work job's computer and opens its remote
// worktree, rather than the registered checkout that does not contain the
// change.
func TestSpawn_VerifierOpensRecordedRemoteWorktree(t *testing.T) {
	remoteWorktree := "/home/dev/.packetcode/worktrees/k/work0001"
	var mu sync.Mutex
	var opened []string

	mgr, _ := newTestManager(t, &scriptedProvider{turns: scriptedHello()}, func(c *Config) {
		c.ResolveWorkspace = func(string) (Workspace, error) {
			return Workspace{
				ComputerID:   "pc_prod",
				ComputerName: "prod",
				WorkingDir:   "/srv/app",
				Identity:     "endpoint-identity",
				Kind:         computers.KindSSH,
			}, nil
		}
		c.OpenBackend = func(_ context.Context, ws Workspace) (computers.RuntimeBackend, error) {
			mu.Lock()
			opened = append(opened, ws.WorkingDir)
			mu.Unlock()
			return &fakeRemoteBackend{id: ws.ComputerID, root: ws.WorkingDir}, nil
		}
	})
	mgr.recordWorktreeRoot(&Job{ID: "work0001", ComputerID: "pc_prod"}, remoteWorktree)

	snap, perr := mgr.Spawn(SpawnRequest{
		Prompt:           "verify",
		Computer:         "prod",
		VerifyWorktreeOf: "work0001",
	})
	require.Nil(t, perr)
	waitTerminal(t, mgr, snap.ID, StateCompleted)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{remoteWorktree}, opened)
}
