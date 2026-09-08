package jobs

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func blockJobStorage(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.Remove(dir)) // The test directory is empty.
	require.NoError(t, os.WriteFile(dir, []byte("not a directory"), 0o600))
}

func restoreJobStorage(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.Remove(dir))
	require.NoError(t, os.Mkdir(dir, 0o700))
}

func TestSpawnRejectsUnwritableJobRecord(t *testing.T) {
	p := &scriptedProvider{turns: scriptedHello()}
	m, _ := newTestManager(t, p)
	blockJobStorage(t, m.cfg.JobsDir)
	snap, err := m.Spawn(SpawnRequest{Prompt: "hello"})
	require.NotNil(t, err)
	assert.Equal(t, "persistence_failed", err.Code)
	assert.Empty(t, snap.ID)
	assert.Empty(t, m.List())
	assert.Zero(t, m.totalSpawned)
	assert.Zero(t, atomic.LoadInt32(&p.turnIdx), "worker ran without a durable record")
	assert.NoError(t, m.Shutdown(time.Second), "a rejected job must not leave pending work")
}

func TestShutdownRetryStillWaitsForWorkers(t *testing.T) {
	m, _ := newTestManager(t, &scriptedProvider{})
	m.wg.Add(1)
	// Hold a worker beyond both deadlines without depending on scheduling.
	assert.Error(t, m.Shutdown(time.Millisecond))
	assert.Error(t, m.Shutdown(time.Millisecond), "closed must not mean workers have exited")
	m.wg.Done()
	require.NoError(t, m.Shutdown(time.Second))
}

func TestFailedJobSnapshotsRemainPendingUntilDurable(t *testing.T) {
	for _, immediate := range []bool{false, true} {
		name := "debounced"
		if immediate {
			name = "terminal"
		}
		t.Run(name, func(t *testing.T) {
			m, _ := newTestManager(t, &scriptedProvider{})
			m.persistDelay = time.Hour
			blockJobStorage(t, m.cfg.JobsDir)
			// Repair storage before helper cleanup even if an assertion fails.
			t.Cleanup(func() {
				if st, err := os.Stat(m.cfg.JobsDir); err == nil && !st.IsDir() {
					restoreJobStorage(t, m.cfg.JobsDir)
				}
			})
			p := persistedJob{ID: "unsaved", State: "running", Seq: 2, LastMessage: "latest"}
			if immediate {
				p.State = "completed"
				require.Error(t, m.savePersistedSnapshot(p))
			} else {
				require.NoError(t, m.savePersistedSnapshot(p))
				m.flushPendingSnapshot(p.ID)
			}
			assert.Zero(t, m.persistSeq[p.ID], "failed write advanced durable sequence")
			assert.Equal(t, p, m.persistPending[p.ID])
			require.ErrorContains(t, m.Shutdown(time.Second), "persist job unsaved")
			require.ErrorContains(t, m.Shutdown(time.Second), "persist job unsaved", "a failed flush was forgotten")
			// An older lifecycle callback must not replace the failed latest state.
			require.Error(t, m.savePersistedSnapshotImmediate(persistedJob{ID: p.ID, State: "running", Seq: 1}))
			assert.Equal(t, p, m.persistPending[p.ID])
			restoreJobStorage(t, m.cfg.JobsDir)
			require.NoError(t, m.Shutdown(time.Second))
			stored, ok := readPersistedJob(filepath.Join(m.cfg.JobsDir, p.ID+".json"))
			require.True(t, ok)
			assert.Equal(t, p.Seq, stored.Seq)
			assert.Equal(t, p.State, stored.State)
			assert.Equal(t, p.LastMessage, stored.LastMessage)
			assert.Empty(t, m.persistPending)
			assert.Equal(t, p.Seq, m.persistSeq[p.ID])
		})
	}
}
