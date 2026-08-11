package handoff

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeIsBoundedDeterministicAndSecretSafe(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	in := SpecialistCapsule{
		Intent:      "Implement this without changing routing.\nAPI_KEY=do-not-store",
		Constraints: []string{"never change the live model", "password: hunter2"},
		Changes: []Change{
			{Path: `internal\agent\agent.go`, Summary: "wire lifecycle", PatchExcerpt: strings.Repeat("+ safe change\n", 300)},
			{Path: `C:\Users\ian\secret.go`, Summary: "must be dropped"},
		},
		FailedGates: []FailedGate{{Name: "test", Summary: "test failed", Fingerprint: digest, Excerpt: "Authorization: Bearer abcdef\nexpected 1 got 2"}},
		Evidence:    []Evidence{{Kind: "test", Command: "go test ./...", Result: "failed", Fingerprint: digest}},
	}
	one := Normalize(in, 2048)
	two := Normalize(in, 2048)
	oneJSON, err := json.Marshal(one)
	require.NoError(t, err)
	twoJSON, err := json.Marshal(two)
	require.NoError(t, err)
	assert.Equal(t, oneJSON, twoJSON)
	assert.LessOrEqual(t, len(oneJSON), 2048)
	assert.Contains(t, string(oneJSON), "internal/agent/agent.go")
	assert.NotContains(t, string(oneJSON), `C:\\Users`)
	assert.NotContains(t, string(oneJSON), "do-not-store")
	assert.NotContains(t, string(oneJSON), "hunter2")
	assert.NotContains(t, string(oneJSON), "Bearer abcdef")
	assert.NotContains(t, string(oneJSON), "transcript")
}
