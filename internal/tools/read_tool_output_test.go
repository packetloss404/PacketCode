package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/toolout"
)

var toolOutputHandle = regexp.MustCompile(`out_[0-9a-f]{32}`)

func spilledFixture(t *testing.T, content string) (*ReadToolOutputTool, string) {
	t.Helper()
	store, err := toolout.Open(t.TempDir(), toolout.Options{ExcerptLimit: 2048})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	excerpt, capped := store.Capture("execute_command", content)
	require.True(t, capped)
	handle := toolOutputHandle.FindString(excerpt)
	require.NotEmpty(t, handle)
	return NewReadToolOutputTool(store), handle
}

func runReadToolOutput(t *testing.T, tool *ReadToolOutputTool, body string) ToolResult {
	t.Helper()
	res, err := tool.Execute(context.Background(), json.RawMessage(body))
	require.NoError(t, err, "the tool must never return a transport error")
	return res
}

func TestReadToolOutput_RetrievesTheOmittedRegion(t *testing.T) {
	content := strings.Repeat("alpha bravo charlie delta\n", 4000)
	tool, handle := spilledFixture(t, content)

	res := runReadToolOutput(t, tool, fmt.Sprintf(`{"handle":%q,"offset":10000,"limit":256}`, handle))

	assert.False(t, res.IsError)
	assert.Contains(t, res.Content, fmt.Sprintf("bytes 10000-10256 of %d", len(content)))
	assert.Contains(t, res.Content, "continue with offset 10256")
	assert.Contains(t, res.Content, content[10000:10256])
}

func TestReadToolOutput_ReportsEndOfOutput(t *testing.T) {
	content := strings.Repeat("z", 5000)
	tool, handle := spilledFixture(t, content)

	res := runReadToolOutput(t, tool, fmt.Sprintf(`{"handle":%q,"offset":4990}`, handle))

	assert.Contains(t, res.Content, "end of output")
	assert.False(t, res.IsError)
}

func TestReadToolOutput_ClampsPageSize(t *testing.T) {
	content := strings.Repeat("y", 4*toolout.MaxPageBytes)
	tool, handle := spilledFixture(t, content)

	res := runReadToolOutput(t, tool, fmt.Sprintf(`{"handle":%q,"limit":1000000}`, handle))

	assert.Less(t, len(res.Content), toolout.MaxPageBytes+512,
		"the recovery tool must not be able to blow the budget truncation protects")
}

// A pruned, evicted, or invented handle must read as an ordinary result. An
// error here would end the model's turn over output it no longer needs.
func TestReadToolOutput_UnknownHandleDegradesWithoutErroring(t *testing.T) {
	tool, _ := spilledFixture(t, strings.Repeat("q", 4096))

	res := runReadToolOutput(t, tool, `{"handle":"out_0123456789abcdef0123456789abcdef"}`)

	assert.False(t, res.IsError, "a missing handle is not a failure the turn should stop for")
	assert.Contains(t, res.Content, "no longer retained")
	assert.Contains(t, res.Content, "Re-run the original tool")
}

func TestReadToolOutput_PathArgumentsReadNothing(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("password=hunter2"), 0o600))
	tool, handle := spilledFixture(t, strings.Repeat("q", 4096))

	for _, arg := range []string{
		secret,
		"../../../../../../etc/passwd",
		filepath.Join("..", "..", "secret.txt"),
		handle + "/../secret.txt",
		strings.TrimPrefix(handle, "out_"),
	} {
		res := runReadToolOutput(t, tool, fmt.Sprintf(`{"handle":%q}`, arg))
		assert.NotContains(t, res.Content, "hunter2", "handle %q must not become a file read", arg)
		assert.Contains(t, res.Content, "no longer retained", "handle %q must degrade, not probe", arg)
		assert.False(t, res.IsError)
	}
}

func TestReadToolOutput_MissingAndMalformedArguments(t *testing.T) {
	tool, _ := spilledFixture(t, strings.Repeat("q", 4096))

	res := runReadToolOutput(t, tool, `{"handle":""}`)
	assert.Contains(t, res.Content, "no handle supplied")
	assert.False(t, res.IsError)

	res = runReadToolOutput(t, tool, `not json`)
	assert.True(t, res.IsError, "an unparseable call is the model's mistake to correct")
	assert.Contains(t, res.Content, "invalid arguments")
}

func TestReadToolOutput_IsCheapAndUngated(t *testing.T) {
	tool := NewReadToolOutputTool(nil)

	assert.False(t, tool.RequiresApproval(), "reading back bytes the user already saw needs no approval")
	assert.Equal(t, "read_tool_output", tool.Name())
	assert.Contains(t, tool.Description(), "truncation marker")
	var schema map[string]any
	require.NoError(t, json.Unmarshal(tool.Schema(), &schema))

	// A nil store is what a session with spilling disabled looks like; it must
	// answer, not panic.
	res := runReadToolOutput(t, tool, `{"handle":"out_0123456789abcdef0123456789abcdef"}`)
	assert.Contains(t, res.Content, "no longer retained")
}
