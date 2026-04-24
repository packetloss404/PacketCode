package mcp

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFraming_RoundTrip writes 3 Request envelopes through an io.Pipe
// and reads them back via the standard scanner. The third request
// includes embedded escape characters to confirm JSON encoding handles
// them transparently.
func TestFraming_RoundTrip(t *testing.T) {
	pr, pw := io.Pipe()
	defer pr.Close()

	reqs := []Request{
		newRequest(1, "alpha", map[string]any{"x": 1}),
		newRequest(2, "beta", map[string]any{"name": "two"}),
		newRequest(3, "gamma", map[string]any{"q": "embedded \"quotes\" and \\ slashes"}),
	}

	go func() {
		defer pw.Close()
		for _, r := range reqs {
			require.NoError(t, writeLine(pw, r))
		}
	}()

	scanner := newScanner(pr)
	got := make([]Request, 0, len(reqs))
	for scanner.Scan() {
		var r Request
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &r))
		got = append(got, r)
	}
	require.NoError(t, scanner.Err())
	require.Len(t, got, len(reqs))
	for i := range reqs {
		assert.Equal(t, reqs[i].ID, got[i].ID)
		assert.Equal(t, reqs[i].Method, got[i].Method)
		assert.JSONEq(t, string(reqs[i].Params), string(got[i].Params))
	}
}

// TestFraming_LargeMessage round-trips a single 2 MB payload to confirm
// the scanner buffer accommodates messages well above the default
// bufio.MaxScanTokenSize.
func TestFraming_LargeMessage(t *testing.T) {
	pr, pw := io.Pipe()
	defer pr.Close()

	big := strings.Repeat("x", 2*1024*1024)
	req := newRequest(42, "huge", map[string]any{"blob": big})

	go func() {
		defer pw.Close()
		require.NoError(t, writeLine(pw, req))
	}()

	scanner := newScanner(pr)
	require.True(t, scanner.Scan(), "scanner should produce one line")
	require.NoError(t, scanner.Err())

	var got Request
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &got))
	assert.JSONEq(t, `42`, string(got.ID))
	assert.Equal(t, "huge", got.Method)
	var params struct {
		Blob string `json:"blob"`
	}
	require.NoError(t, json.Unmarshal(got.Params, &params))
	assert.Equal(t, len(big), len(params.Blob))
}
