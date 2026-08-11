package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/handoff"
)

func TestSpecialistCapsulePersistsOutsideTranscriptAndCacheLineage(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(dir)
	created, err := manager.New("sugar", "sugar/conduit")
	require.NoError(t, err)
	require.NoError(t, manager.SetSpecialistCapsule(handoff.SpecialistCapsule{
		Intent:      "repair tests",
		Changes:     []handoff.Change{{Path: "internal/agent/agent.go", Summary: "changed"}},
		FailedGates: []handoff.FailedGate{{Name: "test", Summary: "failed", Fingerprint: "sha256:" + strings.Repeat("b", 64)}},
	}, 4096))

	current := manager.Current()
	require.NotNil(t, current.SpecialistCapsule)
	assert.Empty(t, current.Messages)
	assert.Equal(t, 0, current.Cache.CompactionGeneration)

	reloaded := NewManager(dir)
	loaded, err := reloaded.Load(created.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded.SpecialistCapsule)
	assert.Equal(t, handoff.SchemaVersion, loaded.SpecialistCapsule.SchemaVersion)
	assert.Equal(t, "repair tests", loaded.SpecialistCapsule.Intent)
}
