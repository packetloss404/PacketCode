package jobs

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/computers"
	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/tools"
)

type fakeRemoteBackend struct {
	id        string
	root      string
	closed    atomic.Int32
	executeFn func(context.Context, string, string, io.Writer) (computers.ExecResult, error)
}

func (f *fakeRemoteBackend) ComputerID() string   { return f.id }
func (f *fakeRemoteBackend) Kind() computers.Kind { return computers.KindSSH }
func (f *fakeRemoteBackend) Root() string         { return f.root }
func (f *fakeRemoteBackend) Close() error {
	f.closed.Add(1)
	return nil
}
func (f *fakeRemoteBackend) Resolve(_ context.Context, name string, _ bool) (string, error) {
	clean := path.Clean(strings.ReplaceAll(name, `\`, "/"))
	if clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", fmt.Errorf("outside root")
	}
	return path.Join(f.root, clean), nil
}
func (f *fakeRemoteBackend) ReadFile(context.Context, string) ([]byte, error) {
	return []byte("test\n"), nil
}
func (f *fakeRemoteBackend) WriteFile(context.Context, string, []byte) error { return nil }
func (f *fakeRemoteBackend) ReadDir(context.Context, string) ([]computers.FileEntry, error) {
	return []computers.FileEntry{{Name: "file.go", Mode: fs.FileMode(0o644)}}, nil
}
func (f *fakeRemoteBackend) Execute(ctx context.Context, command, cwd string, output io.Writer) (computers.ExecResult, error) {
	if f.executeFn != nil {
		return f.executeFn(ctx, command, cwd, output)
	}
	return computers.ExecResult{ExitCode: 0}, nil
}

func testRemoteWorkspace(name, id, root string) Workspace {
	return Workspace{
		ComputerID: id, ComputerName: name, WorkingDir: root,
		Identity: "identity-" + id + "-" + root, Kind: computers.KindSSH,
		Policy: computers.Policy{
			Write: computers.PolicyAsk, Shell: computers.PolicyAsk,
			Network: computers.PolicyAsk, Secrets: computers.PolicyDeny,
			Approval: computers.ApprovalExplicit,
		},
	}
}

func TestResolveSpawnWorkspacePrecedenceAndNestedConfinement(t *testing.T) {
	primary := testRemoteWorkspace("primary", "pc_primary", "/srv/primary")
	other := testRemoteWorkspace("other", "pc_other", "/srv/other")
	m := &Manager{
		cfg: Config{
			Root:             t.TempDir(),
			DefaultWorkspace: primary,
			ResolveWorkspace: func(selector string) (Workspace, error) {
				switch selector {
				case "primary", "pc_primary":
					return primary, nil
				case "other", "pc_other":
					return other, nil
				default:
					return Workspace{}, fmt.Errorf("unknown %s", selector)
				}
			},
		},
		jobs: map[string]*Job{},
	}

	got, perr := m.resolveSpawnWorkspace(SpawnRequest{})
	require.Nil(t, perr)
	assert.Equal(t, primary, got)

	got, perr = m.resolveSpawnWorkspace(SpawnRequest{Computer: "other"})
	require.Nil(t, perr)
	assert.Equal(t, other, got)

	m.jobs["parent"] = &Job{
		ID: "parent", ComputerID: primary.ComputerID, ComputerName: primary.ComputerName,
		WorkingDir: primary.WorkingDir, WorkspaceIdentity: primary.Identity, ComputerPolicy: primary.Policy,
	}
	got, perr = m.resolveSpawnWorkspace(SpawnRequest{ParentJobID: "parent"})
	require.Nil(t, perr)
	assert.True(t, sameWorkspace(primary, got))
	_, perr = m.resolveSpawnWorkspace(SpawnRequest{ParentJobID: "parent", Computer: "other"})
	require.NotNil(t, perr)
	assert.Equal(t, "cross_target_denied", perr.Code)
}

func TestRemoteJobsOwnAndCloseUniqueBackends(t *testing.T) {
	prov := &scriptedProvider{turns: append(scriptedHello(), scriptedHello()...)}
	ws := testRemoteWorkspace("build", "pc_build", "/srv/app")
	var mu sync.Mutex
	var opened []*fakeRemoteBackend
	mgr, _ := newTestManager(t, prov, func(c *Config) {
		c.Tools = makeMainRegistry(t, t.TempDir())
		c.DefaultWorkspace = ws
		c.OpenBackend = func(_ context.Context, target Workspace) (computers.RuntimeBackend, error) {
			backend := &fakeRemoteBackend{id: target.ComputerID, root: target.WorkingDir}
			mu.Lock()
			opened = append(opened, backend)
			mu.Unlock()
			return backend, nil
		}
	})

	a, perr := mgr.Spawn(SpawnRequest{Prompt: "one"})
	require.Nil(t, perr)
	b, perr := mgr.Spawn(SpawnRequest{Prompt: "two"})
	require.Nil(t, perr)
	for _, id := range []string{a.ID, b.ID} {
		waitFor(t, 3*time.Second, "remote job completes", func() bool {
			snap, ok := mgr.Get(id)
			return ok && snap.State.IsTerminal()
		})
		snap, _ := mgr.Get(id)
		assert.Equal(t, ws.ComputerID, snap.ComputerID)
		assert.Equal(t, ws.WorkingDir, snap.WorkingDir)
		assert.Equal(t, ws.Identity, snap.WorkspaceIdentity)
	}

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, opened, 2, "each remote job must open its own backend")
	assert.NotSame(t, opened[0], opened[1])
	assert.GreaterOrEqual(t, opened[0].closed.Load(), int32(1))
	assert.GreaterOrEqual(t, opened[1].closed.Load(), int32(1))

	requests := prov.snapshotRequests()
	require.Len(t, requests, 2)
	for _, req := range requests {
		require.NotEmpty(t, req.Messages)
		assert.Contains(t, req.Messages[0].Content, `Packet Computer "build"`)
		assert.Contains(t, req.Messages[0].Content, "process-lifetime only")
	}
}

func TestRemoteJobCancelClosesBackend(t *testing.T) {
	prov := &scriptedProvider{holdOpen: true}
	ws := testRemoteWorkspace("build", "pc_build", "/srv/app")
	opened := make(chan *fakeRemoteBackend, 1)
	mgr, _ := newTestManager(t, prov, func(c *Config) {
		c.Tools = makeMainRegistry(t, t.TempDir())
		c.DefaultWorkspace = ws
		c.OpenBackend = func(_ context.Context, target Workspace) (computers.RuntimeBackend, error) {
			backend := &fakeRemoteBackend{id: target.ComputerID, root: target.WorkingDir}
			opened <- backend
			return backend, nil
		}
	})
	snap, perr := mgr.Spawn(SpawnRequest{Prompt: "hold"})
	require.Nil(t, perr)
	backend := <-opened
	waitFor(t, time.Second, "job running", func() bool {
		got, _ := mgr.Get(snap.ID)
		return got.State == StateRunning
	})
	require.True(t, mgr.Cancel(snap.ID))
	waitFor(t, 2*time.Second, "job cancelled", func() bool {
		got, _ := mgr.Get(snap.ID)
		return got.State == StateCancelled
	})
	assert.GreaterOrEqual(t, backend.closed.Load(), int32(1))
}

func TestRemoteToolRegistryUsesBackendAndOmitsLocalCodeIntel(t *testing.T) {
	root := t.TempDir()
	backend := &fakeRemoteBackend{id: "pc_build", root: "/srv/app"}
	mgr := &Manager{cfg: Config{Tools: makeMainRegistry(t, root), Root: root}, pathLocks: pathLockMap{}}
	reg := mgr.buildJobToolRegistryForBackend(0, true, "job", nil, nil, backend)

	for _, name := range []string{"read_file", "search_codebase", "list_directory", "write_file", "patch_file", "execute_command"} {
		_, ok := reg.Get(name)
		assert.True(t, ok, "%s should be remote-aware", name)
	}
	for _, name := range []string{"list_symbols", "find_definition", "find_references", "get_diagnostics"} {
		_, ok := reg.Get(name)
		assert.False(t, ok, "%s is still local-only", name)
	}
	read, _ := reg.Get("read_file")
	assert.Same(t, backend, read.(*tools.ReadFileTool).Backend)
	execTool, _ := reg.Get("execute_command")
	assert.Same(t, backend, execTool.(*tools.ExecuteCommandTool).Backend)
}

func TestRemoteComputerPolicyOnlyReducesAuthority(t *testing.T) {
	base := permissions.DefaultPolicy().WithProfile(permissions.ProfileFull)
	ws := testRemoteWorkspace("locked", "pc_locked", "/srv/app")
	ws.Policy.Write = computers.PolicyDeny
	ws.Policy.Shell = computers.PolicyAllow
	ws.Policy.Approval = computers.ApprovalExplicit
	effective := policyForWorkspace(base, ws)

	assert.Equal(t, permissions.DecisionDeny, effective.Decide(permissions.Request{ToolName: "write_file", RequiresApproval: true}).Decision)
	assert.Equal(t, permissions.DecisionDeny, effective.Decide(permissions.Request{ToolName: "patch_file", RequiresApproval: true}).Decision)
	assert.Equal(t, permissions.DecisionAsk, effective.Decide(permissions.Request{ToolName: "execute_command", RequiresApproval: true}).Decision,
		"explicit remote approval must not inherit global auto-approval")
	assert.Equal(t, permissions.DecisionAllow, effective.Decide(permissions.Request{ToolName: "read_file"}).Decision)
}

func TestCreateRemoteWorktreePreservesWorkspaceSubdirectoryAndPermissions(t *testing.T) {
	var commands []string
	backend := &fakeRemoteBackend{id: "pc_build", root: "/srv/repo/services/api"}
	backend.executeFn = func(_ context.Context, command, _ string, output io.Writer) (computers.ExecResult, error) {
		commands = append(commands, command)
		switch {
		case command == "git rev-parse --show-toplevel":
			_, _ = io.WriteString(output, "/srv/repo\n")
		case strings.Contains(command, "rev-parse --verify"):
			_, _ = io.WriteString(output, "deadbeef\n")
		case strings.Contains(command, `printf '%s\n'`):
			_, _ = io.WriteString(output, "/home/deploy\n")
		}
		return computers.ExecResult{ExitCode: 0}, nil
	}

	info, err := createRemoteWorktree(context.Background(), backend, "abc12345")
	require.NoError(t, err)
	assert.Equal(t, "/home/deploy/.packetcode/worktrees/"+remoteRepoWorktreeKey("/srv/repo")+"/abc12345", info.Path)
	assert.Equal(t, info.Path+"/services/api", info.Root)
	assert.Equal(t, "packetcode-job-abc12345", info.Branch)
	assert.Equal(t, "deadbeef", info.Base)
	assert.Condition(t, func() bool {
		for _, command := range commands {
			if strings.Contains(command, "umask 077") && strings.Contains(command, "chmod 700") && strings.Contains(command, "mkdir -m 700") && strings.Contains(command, "test ! -L") {
				return true
			}
		}
		return false
	}, "remote state directories must be private")
}

func TestRemoteWorktreeTokenUsesFullSessionIdentity(t *testing.T) {
	one := remoteWorktreeToken(&Job{ID: "abc12345", SessionID: "main-job-session-one"})
	two := remoteWorktreeToken(&Job{ID: "abc12345", SessionID: "main-job-session-two"})
	assert.Regexp(t, `^[0-9a-f]{24}$`, one)
	assert.NotEqual(t, one, two)
}

func TestAppendRemoteWorktreeArtifacts(t *testing.T) {
	backend := &fakeRemoteBackend{id: "pc_build", root: "/worktrees/job"}
	backend.executeFn = func(_ context.Context, command, _ string, output io.Writer) (computers.ExecResult, error) {
		if command == "git status --porcelain" {
			_, _ = io.WriteString(output, " M app.go\n?? new.txt\n")
		}
		return computers.ExecResult{ExitCode: 0}, nil
	}
	artifacts := appendWorktreeArtifactsForBackend(context.Background(), nil, &Job{WorktreePath: "/worktrees/job"}, backend)
	require.Len(t, artifacts, 1)
	assert.Equal(t, "worktree_diff", artifacts[0].Kind)
	assert.Equal(t, 2, artifacts[0].Metadata["file_count"])
}

func TestRemoteWorkspacePersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ws := testRemoteWorkspace("build", "pc_build", "/srv/app")
	j := &Job{
		ID: "remote01", SessionID: "main-job-remote01", Prompt: "inspect",
		Provider: "scripted", Model: "scripted-model", State: StateCompleted,
		CreatedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(),
		ComputerID: ws.ComputerID, ComputerName: ws.ComputerName,
		WorkingDir: ws.WorkingDir, WorkspaceIdentity: ws.Identity, ComputerPolicy: ws.Policy,
	}
	require.NoError(t, saveSnapshot(dir, j))
	loaded, _, _, err := loadPersistedJobs(dir, "")
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.Equal(t, ws.ComputerID, loaded[0].ComputerID)
	assert.Equal(t, ws.ComputerName, loaded[0].ComputerName)
	assert.Equal(t, ws.WorkingDir, loaded[0].WorkingDir)
	assert.Equal(t, ws.Identity, loaded[0].WorkspaceIdentity)
	assert.Equal(t, ws.Policy, loaded[0].ComputerPolicy)
}

func TestRemoteResubmitRefusesEndpointIdentityChange(t *testing.T) {
	mgr, _ := managerOverAbandoned(t, "remote02", "inspect")
	old := testRemoteWorkspace("build", "pc_build", "/srv/app")
	changed := old
	changed.Identity = "replacement-endpoint"
	mgr.mu.Lock()
	j := mgr.jobs["remote02"]
	j.ComputerID = old.ComputerID
	j.ComputerName = old.ComputerName
	j.WorkingDir = old.WorkingDir
	j.WorkspaceIdentity = old.Identity
	j.ComputerPolicy = old.Policy
	mgr.cfg.ResolveWorkspace = func(selector string) (Workspace, error) {
		assert.Equal(t, old.ComputerID, selector, "resubmit must resolve by stable id")
		return changed, nil
	}
	mgr.mu.Unlock()

	_, perr := mgr.Resubmit("remote02")
	require.NotNil(t, perr)
	assert.Equal(t, "workspace_identity_mismatch", perr.Code)
	snap, _ := mgr.Get("remote02")
	assert.Empty(t, snap.ResubmittedAs)
	assert.Len(t, mgr.List(), 1, "identity mismatch must not enqueue a successor")
}

func TestLegacyLocalResubmitRefusesActiveRemoteDefault(t *testing.T) {
	mgr, _ := managerOverAbandoned(t, "legacy03", "inspect")
	mgr.mu.Lock()
	mgr.cfg.DefaultWorkspace = testRemoteWorkspace("build", "pc_build", "/srv/app")
	mgr.mu.Unlock()

	_, perr := mgr.Resubmit("legacy03")
	require.NotNil(t, perr)
	assert.Equal(t, "workspace_unbound", perr.Code)
	assert.Len(t, mgr.List(), 1)
}

func TestRemoteSubsessionPersistsWorkspaceBinding(t *testing.T) {
	dir := t.TempDir()
	j := &Job{
		ID: "remote04", SessionID: "main-job-remote04", Provider: "scripted", Model: "model",
		ComputerID: "pc_build", WorkingDir: "/srv/app", WorkspaceIdentity: "pcws_sha256:approved",
	}
	require.NoError(t, writeInitialSubSession(dir, j))
	sm := session.NewManager(dir)
	loaded, err := sm.Load(j.SessionID)
	require.NoError(t, err)
	assert.Equal(t, j.ComputerID, loaded.ComputerID)
	assert.Equal(t, j.WorkingDir, loaded.WorkingDir)
	assert.Equal(t, j.WorkspaceIdentity, loaded.WorkspaceIdentity)
	require.NoError(t, session.ValidateWorkspace(loaded, j.ComputerID, j.WorkingDir, j.WorkspaceIdentity))
	require.Error(t, session.ValidateWorkspace(loaded, "pc_other", j.WorkingDir))
	require.Error(t, session.ValidateWorkspace(loaded, j.ComputerID, j.WorkingDir, "pcws_sha256:changed"))
}
