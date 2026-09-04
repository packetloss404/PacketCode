package agent

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/toolout"
)

var spillHandle = regexp.MustCompile(`out_[0-9a-f]{32}`)

// toolResultRig runs one turn whose single tool call returns content, with a
// real spill store attached, and returns the persisted tool message.
func toolResultRig(t *testing.T, content string, opts toolout.Options) (provider.Message, []AgentEvent, *toolout.Store) {
	t.Helper()
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: "big", ArgumentsDelta: `{}`}},
			{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}},
			{Type: provider.EventDone},
		},
		{
			{Type: provider.EventTextDelta, TextDelta: "done"},
			{Type: provider.EventDone},
		},
	}}
	tool := &recordingTool{name: "big"}
	tool.result.Content = content
	a, sm, _ := newAgentRig(t, prov, tool)
	store, err := toolout.Open(t.TempDir(), opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	a.toolOutput = store

	events := collect(a.Run(context.Background(), "go"))

	var toolMsg provider.Message
	for _, m := range sm.Current().Messages {
		if m.Role == provider.RoleTool {
			toolMsg = m
		}
	}
	require.Equal(t, provider.RoleTool, toolMsg.Role, "the turn must have produced a tool message")
	return toolMsg, events, store
}

// The chokepoint is where every native, MCP, and skill result becomes a
// message, so the cap must hold there — while the UI and the session file keep
// the full bytes they already render.
func TestAgent_OversizedToolResultIsCappedForTheModelAndSpilled(t *testing.T) {
	content := strings.Repeat("compiler diagnostic line\n", 20000)
	msg, events, store := toolResultRig(t, content, toolout.Options{ExcerptLimit: 4096})

	assert.Equal(t, content, msg.Content, "the session and TUI keep the full result")
	require.NotEmpty(t, msg.ModelContent, "the model must see a bounded projection")
	assert.LessOrEqual(t, len(msg.ModelContent), 4096)
	assert.Contains(t, msg.ModelContent, "read_tool_output")

	var executed *AgentEvent
	for i := range events {
		if events[i].Type == EventToolCallExecuted {
			executed = &events[i]
		}
	}
	require.NotNil(t, executed)
	assert.Equal(t, content, executed.ToolResult.Content, "live scrollback still receives the whole result")

	projected := session.ModelMessages([]provider.Message{msg})
	assert.Equal(t, msg.ModelContent, projected[0].Content, "the excerpt is what reaches the provider")

	handle := spillHandle.FindString(msg.ModelContent)
	require.NotEmpty(t, handle)
	page, ok := store.Read(handle, int64(len(content)/2), 128)
	require.True(t, ok, "the handle reads the omitted region back")
	assert.Contains(t, content, page.Text)
}

func TestAgent_SmallToolResultIsUnchangedWithNoHandle(t *testing.T) {
	msg, _, store := toolResultRig(t, "3 files changed\n", toolout.Options{ExcerptLimit: 4096})

	assert.Equal(t, "3 files changed\n", msg.Content)
	assert.Empty(t, msg.ModelContent, "an ordinary result must reach the model byte-identical")
	entries, err := os.ReadDir(store.Dir())
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing is written to disk for a small result")
}

func TestAgent_NoStoreLeavesResultsUntouched(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: "big", ArgumentsDelta: `{}`}},
			{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}},
			{Type: provider.EventDone},
		},
		{{Type: provider.EventDone}},
	}}
	tool := &recordingTool{name: "big"}
	tool.result.Content = strings.Repeat("x", 8192)
	a, sm, _ := newAgentRig(t, prov, tool)

	collect(a.Run(context.Background(), "go"))

	for _, m := range sm.Current().Messages {
		if m.Role != provider.RoleTool {
			continue
		}
		assert.Equal(t, tool.result.Content, m.Content)
		assert.Empty(t, m.ModelContent, "with no store the agent adds no projection of its own")
	}
}

// A spilled result is sent as its excerpt, so counting the retained full copy
// would report context the request never carries and compact prematurely.
func TestMessageChars_CountsTheModelFacingProjection(t *testing.T) {
	full := strings.Repeat("a", 100_000)
	excerpt := strings.Repeat("a", 1_000)
	messages := []provider.Message{{Role: provider.RoleTool, Content: full, ModelContent: excerpt}}

	chars := messageChars(messages)

	assert.Less(t, chars, 2_000)
	assert.GreaterOrEqual(t, chars, 1_000)
}
