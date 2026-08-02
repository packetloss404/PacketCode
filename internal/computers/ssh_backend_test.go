package computers

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestPinnedHostKeyCallback(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	key, err := ssh.NewPublicKey(public)
	require.NoError(t, err)

	callback, err := pinnedHostKeyCallback(ssh.FingerprintSHA256(key))
	require.NoError(t, err)
	require.NoError(t, callback("server.example:22", &net.TCPAddr{}, key))

	otherPublic, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	otherKey, err := ssh.NewPublicKey(otherPublic)
	require.NoError(t, err)
	err = callback("server.example:22", &net.TCPAddr{}, otherKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "host key mismatch")
}

func TestPinnedHostKeyCallback_RefusesMissingOrLegacyFingerprint(t *testing.T) {
	for _, fingerprint := range []string{"", "MD5:aa:bb"} {
		_, err := pinnedHostKeyCallback(fingerprint)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refusing unpinned host")
	}
}

func TestRemoteCandidateConfinesWorkspace(t *testing.T) {
	got, err := remoteCandidate("/srv/apps/widget", "src/main.go")
	require.NoError(t, err)
	assert.Equal(t, "/srv/apps/widget/src/main.go", got)

	for _, supplied := range []string{"../secret", "src/../../../secret", "/etc/passwd"} {
		_, err := remoteCandidate("/srv/apps/widget", supplied)
		require.Error(t, err, supplied)
	}
}

func TestShellQuote(t *testing.T) {
	assert.Equal(t, `'one'"'"'two'`, shellQuote("one'two"))
}

func TestSSHBackendCloseDoesNotWaitForSFTPLock(t *testing.T) {
	b := &SSHBackend{closed: make(chan struct{})}

	// Simulate an SFTP operation stuck in network I/O. Close must not wait
	// for the SFTP serialization lock because closing the transport is what
	// unblocks that operation in a real backend.
	b.sftpMu.Lock()
	defer b.sftpMu.Unlock()

	done := make(chan error, 1)
	go func() { done <- b.Close() }()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("SSHBackend.Close blocked behind sftpMu")
	}
	select {
	case <-b.closed:
	default:
		t.Fatal("SSHBackend.Close did not publish closure")
	}
}

func TestSSHBackendCloseIsConcurrentAndIdempotent(t *testing.T) {
	b := &SSHBackend{closed: make(chan struct{})}
	const callers = 32

	var wg sync.WaitGroup
	wg.Add(callers)
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			errs <- b.Close()
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("concurrent SSHBackend.Close calls did not finish")
	}
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

func TestDrainSSHSessionIsBounded(t *testing.T) {
	done := make(chan error)
	started := time.Now()
	drainSSHSession(done, 15*time.Millisecond)
	elapsed := time.Since(started)
	assert.GreaterOrEqual(t, elapsed, 10*time.Millisecond)
	assert.Less(t, elapsed, 250*time.Millisecond)
}

func TestDrainSSHSessionReturnsWhenWaitFinishes(t *testing.T) {
	done := make(chan error, 1)
	done <- nil
	started := time.Now()
	drainSSHSession(done, time.Second)
	assert.Less(t, time.Since(started), 250*time.Millisecond)
}
