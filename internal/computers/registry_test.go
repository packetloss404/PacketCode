package computers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_MissingFileIsEmptyNotError(t *testing.T) {
	r, err := Load(t.TempDir())
	require.NoError(t, err, "first run must not be an error")
	assert.Empty(t, r.List())
}

func TestLoad_EmptyDirRejected(t *testing.T) {
	_, err := Load("")
	require.Error(t, err)
}

func TestUpsertAndReload(t *testing.T) {
	dir := t.TempDir()
	r, err := Load(dir)
	require.NoError(t, err)

	stored, err := r.Upsert(Computer{
		Name:         "workstation",
		Kind:         KindSSH,
		SSHHost:      "build.example.internal",
		SSHUser:      "ian",
		SSHPort:      22,
		ProjectRoots: []string{"/srv/projects"},
		Capabilities: Capabilities{Shell: true, Filesystem: true, Jobs: true},
	})
	require.NoError(t, err)
	assert.Equal(t, "pc_workstation", stored.ID)
	assert.False(t, stored.CreatedAt.IsZero())

	reloaded, err := Load(dir)
	require.NoError(t, err)
	list := reloaded.List()
	require.Len(t, list, 1)
	assert.Equal(t, "workstation", list[0].Name)
	assert.Equal(t, KindSSH, list[0].Kind)
	assert.Equal(t, "build.example.internal", list[0].SSHHost)
	assert.Equal(t, []string{"/srv/projects"}, list[0].ProjectRoots)
}

// A record with no policy on disk must come back conservative, not
// permissive — an unconfigured machine should never read as allow-all.
func TestUpsert_AppliesConservativePolicyDefaults(t *testing.T) {
	r, err := Load(t.TempDir())
	require.NoError(t, err)

	stored, err := r.Upsert(Computer{Name: "laptop", Kind: KindLocal})
	require.NoError(t, err)
	assert.Equal(t, PolicyAsk, stored.Policy.Network)
	assert.Equal(t, PolicyAsk, stored.Policy.Write)
	assert.Equal(t, PolicyAsk, stored.Policy.Shell)
	assert.Equal(t, PolicyDeny, stored.Policy.Secrets,
		"a remote computer must not inherit local secrets by default")
	assert.Equal(t, ApprovalExplicit, stored.Policy.Approval)
}

// The approval axis must default to explicit when absent from disk. A bool
// would decode an absent field as false and silently widen trust; this test
// pins the enum encoding that avoids it.
func TestLoad_AbsentApprovalDefaultsToExplicit(t *testing.T) {
	dir := t.TempDir()
	raw := `{"version":1,"computers":[{"id":"pc_y","name":"y","kind":"local",
	  "capabilities":{},"policy":{"network":"ask","write":"ask","shell":"ask","secrets":"deny"},
	  "created_at":"2026-01-01T00:00:00Z"}]}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, FileName), []byte(raw), 0o600))

	r, err := Load(dir)
	require.NoError(t, err)
	got, ok := r.Get("y")
	require.True(t, ok)
	assert.Equal(t, ApprovalExplicit, got.Policy.Approval)
}

func TestLoad_InvalidApprovalFallsBackToExplicit(t *testing.T) {
	dir := t.TempDir()
	raw := `{"version":1,"computers":[{"id":"pc_z","name":"z","kind":"local",
	  "capabilities":{},"policy":{"approval":"trust-everything"},
	  "created_at":"2026-01-01T00:00:00Z"}]}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, FileName), []byte(raw), 0o600))

	r, err := Load(dir)
	require.NoError(t, err)
	got, ok := r.Get("z")
	require.True(t, ok)
	assert.Equal(t, ApprovalExplicit, got.Policy.Approval,
		"an unknown approval mode must never widen trust")
}

// A legitimately-configured wide approval mode must be preserved.
func TestUpsert_PreservesValidWideApprovalMode(t *testing.T) {
	r, err := Load(t.TempDir())
	require.NoError(t, err)
	stored, err := r.Upsert(Computer{
		Name:   "trusted",
		Kind:   KindLocal,
		Policy: Policy{Approval: ApprovalTrustComputer},
	})
	require.NoError(t, err)
	assert.Equal(t, ApprovalTrustComputer, stored.Policy.Approval)
}

// A garbage policy value on disk must be replaced by the safe default
// rather than passed through.
func TestLoad_InvalidPolicyValueFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	raw := `{"version":1,"computers":[{"id":"pc_x","name":"x","kind":"local",
	  "capabilities":{},"policy":{"network":"yolo","write":"","shell":"allow","secrets":"nope"},
	  "created_at":"2026-01-01T00:00:00Z"}]}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, FileName), []byte(raw), 0o600))

	r, err := Load(dir)
	require.NoError(t, err)
	got, ok := r.Get("x")
	require.True(t, ok)
	assert.Equal(t, PolicyAsk, got.Policy.Network, "invalid mode must fall back to ask")
	assert.Equal(t, PolicyAsk, got.Policy.Write)
	assert.Equal(t, PolicyAllow, got.Policy.Shell, "a valid mode must be preserved")
	assert.Equal(t, PolicyDeny, got.Policy.Secrets)
}

// Milestone A has no heartbeat. A stored "online" with no daemon behind it
// must not be reported as live.
func TestNormalize_StatusIsUnknownWithoutDaemon(t *testing.T) {
	r, err := Load(t.TempDir())
	require.NoError(t, err)

	stored, err := r.Upsert(Computer{Name: "claims-online", Kind: KindLocal, Status: StatusOnline})
	require.NoError(t, err)
	assert.Equal(t, StatusUnknown, stored.Status,
		"no daemon has reported in, so status must not claim online")
}

func TestUpsert_RejectsInvalidRecords(t *testing.T) {
	r, err := Load(t.TempDir())
	require.NoError(t, err)

	cases := []struct {
		name string
		in   Computer
	}{
		{"empty name", Computer{Kind: KindLocal}},
		{"bad name chars", Computer{Name: "has space", Kind: KindLocal}},
		{"unknown kind", Computer{Name: "x", Kind: Kind("carrier-pigeon")}},
		{"ssh without host", Computer{Name: "x", Kind: KindSSH}},
		{"ssh port out of range", Computer{Name: "x", Kind: KindSSH, SSHHost: "h", SSHPort: 70000}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.Upsert(tc.in)
			require.Error(t, err)
		})
	}
	assert.Empty(t, r.List(), "no invalid record should have been stored")
}

func TestUpsert_RejectsDuplicateNameCaseInsensitively(t *testing.T) {
	r, err := Load(t.TempDir())
	require.NoError(t, err)

	_, err = r.Upsert(Computer{Name: "build", Kind: KindLocal})
	require.NoError(t, err)

	_, err = r.Upsert(Computer{ID: "pc_other", Name: "BUILD", Kind: KindLocal})
	require.Error(t, err, "/computers <name> must stay unambiguous")
}

func TestUpsert_PreservesCreatedAtOnUpdate(t *testing.T) {
	dir := t.TempDir()
	r, err := Load(dir)
	require.NoError(t, err)

	first, err := r.Upsert(Computer{Name: "keep", Kind: KindLocal})
	require.NoError(t, err)

	time.Sleep(2 * time.Millisecond)
	second, err := r.Upsert(Computer{Name: "keep", Kind: KindLocal, OS: "linux"})
	require.NoError(t, err)

	assert.Equal(t, first.CreatedAt, second.CreatedAt, "CreatedAt must survive an update")
	assert.Equal(t, "linux", second.OS)
	assert.Len(t, r.List(), 1, "update must not duplicate the record")
}

func TestGet_IsCaseInsensitive(t *testing.T) {
	r, err := Load(t.TempDir())
	require.NoError(t, err)
	_, err = r.Upsert(Computer{Name: "Alpha", Kind: KindLocal})
	require.NoError(t, err)

	got, ok := r.Get("alpha")
	require.True(t, ok)
	assert.Equal(t, "Alpha", got.Name)

	_, ok = r.Get("beta")
	assert.False(t, ok)
}

func TestGetByIDAndWorkspaceIdentity(t *testing.T) {
	r, err := Load(t.TempDir())
	require.NoError(t, err)
	stored, err := r.Upsert(Computer{
		ID: "pc_stable", Name: "Build", Kind: KindSSH,
		SSHUser: "deploy", SSHHost: "BUILD.EXAMPLE", SSHPort: 22,
		SSHHostFingerprint: "SHA256:key", ProjectRoots: []string{"/srv/app"},
	})
	require.NoError(t, err)

	got, ok := r.GetByID("pc_stable")
	require.True(t, ok)
	assert.Equal(t, stored.Name, got.Name)
	_, ok = r.GetByID("PC_STABLE")
	assert.False(t, ok, "stable ids are exact, unlike display names")

	identity := WorkspaceIdentity(stored, "/srv/app")
	assert.Equal(t, identity, WorkspaceIdentity(stored, "/srv/app/"))
	repointed := stored
	repointed.SSHHostFingerprint = "SHA256:replacement"
	assert.NotEqual(t, identity, WorkspaceIdentity(repointed, "/srv/app"))
	assert.NotEqual(t, identity, WorkspaceIdentity(stored, "/srv/other"))
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	r, err := Load(dir)
	require.NoError(t, err)
	_, err = r.Upsert(Computer{Name: "gone", Kind: KindLocal})
	require.NoError(t, err)

	removed, err := r.Remove("GONE")
	require.NoError(t, err)
	assert.True(t, removed)

	missing, err := r.Remove("never")
	require.NoError(t, err)
	assert.False(t, missing)

	reloaded, err := Load(dir)
	require.NoError(t, err)
	assert.Empty(t, reloaded.List())
}

// Losing the machine list silently is worse than failing loudly.
func TestLoad_MalformedFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, FileName), []byte("{not json"), 0o600))
	_, err := Load(dir)
	require.Error(t, err)
}

func TestLoad_RefusesNewerRegistryVersion(t *testing.T) {
	dir := t.TempDir()
	raw := `{"version":99,"computers":[]}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, FileName), []byte(raw), 0o600))
	_, err := Load(dir)
	require.Error(t, err, "a future registry must not be silently misread")
}

func TestSave_WritesVersionedFileAndLeavesNoTemps(t *testing.T) {
	dir := t.TempDir()
	r, err := Load(dir)
	require.NoError(t, err)
	_, err = r.Upsert(Computer{Name: "one", Kind: KindLocal})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, FileName))
	require.NoError(t, err)
	var f struct {
		Version   int `json:"version"`
		Computers []struct {
			Name string `json:"name"`
		} `json:"computers"`
	}
	require.NoError(t, json.Unmarshal(data, &f))
	assert.Equal(t, 1, f.Version)
	require.Len(t, f.Computers, 1)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp", "atomic write must not leave temp files")
	}
}

func TestList_SortedByName(t *testing.T) {
	r, err := Load(t.TempDir())
	require.NoError(t, err)
	for _, n := range []string{"zeta", "alpha", "Mid"} {
		_, err := r.Upsert(Computer{Name: n, Kind: KindLocal})
		require.NoError(t, err)
	}
	list := r.List()
	require.Len(t, list, 3)
	assert.Equal(t, "alpha", list[0].Name)
	assert.Equal(t, "Mid", list[1].Name)
	assert.Equal(t, "zeta", list[2].Name)
}

func TestReachable(t *testing.T) {
	assert.True(t, Computer{Kind: KindLocal}.Reachable())
	assert.True(t, Computer{
		Kind: KindSSH, SSHHost: "h", SSHUser: "deploy",
		SSHHostFingerprint: "SHA256:key", ProjectRoots: []string{"/srv/app"},
	}.Reachable())
	assert.False(t, Computer{Kind: KindSSH, SSHHost: "h"}.Reachable(),
		"an incomplete legacy SSH record must remain stored but cannot connect")
	assert.False(t, Computer{Kind: KindSSH}.Reachable())
	assert.False(t, Computer{Kind: KindManaged}.Reachable(),
		"managed computers cannot be reached until provisioning exists")
}

// A row this build cannot read is a row a newer build wrote, or one a
// person edited by hand. Either way, editing a different computer must not
// delete it from disk -- that would be silent data loss in a file the user
// did not touch, the failure the compat contract exists to prevent.
func TestSave_PreservesRowsThisBuildCannotRead(t *testing.T) {
	dir := t.TempDir()
	newerRow := `{"id":"pc_future","name":"future","kind":"hologram","future_field":{"nested":true}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, FileName), []byte(`{"version":1,"computers":[`+newerRow+`]}`), 0o600))

	r, err := Load(dir)
	require.NoError(t, err)
	assert.Empty(t, r.List(), "an unreadable row is not listed")
	assert.Equal(t, 1, r.Unreadable())

	_, err = r.Upsert(Computer{
		Name: "laptop", Kind: KindSSH, SSHHost: "laptop.local", SSHUser: "ian", SSHPort: 22,
		ProjectRoots: []string{"/home/ian/src"},
	})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, FileName))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"pc_future"`)
	assert.Contains(t, string(data), `"future_field"`)
	assert.Contains(t, string(data), `"laptop"`)

	reloaded, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, reloaded.List(), 1)
	assert.Equal(t, 1, reloaded.Unreadable())
}
