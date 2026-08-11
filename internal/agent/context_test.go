package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
)

func representativeRequest() (string, []provider.Message, []provider.ToolDefinition) {
	return "You are a coding agent.", []provider.Message{
		{Role: provider.RoleUser, Content: "Inspect the failing test."},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "read_file", Arguments: `{"path":"main.go"}`}}},
		{Role: provider.RoleTool, ToolCallID: "call-1", Name: "read_file", Content: "package main\n\nfunc main() {}"},
	}, []provider.ToolDefinition{{Name: "read_file", Description: "Read a file", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)}}
}

func TestEstimateRequestContributions(t *testing.T) {
	system, messages, tools := representativeRequest()
	got := NewContextManager(80).EstimateRequest(system, messages, tools)
	if got.SystemPrompt <= 0 || got.Transcript <= 0 || got.ToolSchemas <= 0 {
		t.Fatalf("missing contribution: %+v", got)
	}
	if got.Total != got.SystemPrompt+got.Transcript+got.ToolSchemas {
		t.Fatalf("total does not sum: %+v", got)
	}
	if again := NewContextManager(80).EstimateRequest(system, messages, tools); again != got {
		t.Fatalf("estimate is not deterministic: %+v != %+v", got, again)
	}
}

func TestCanCompactRequiresAnOlderBodyMessage(t *testing.T) {
	cm := NewContextManager(80)
	messages := make([]provider.Message, 10)
	for i := range messages {
		messages[i] = provider.Message{Role: provider.RoleUser, Content: "message"}
	}
	if cm.CanCompact(messages, 10) {
		t.Fatal("ten body messages should all be preserved")
	}
	messages = append(messages, provider.Message{Role: provider.RoleAssistant, Content: "eleventh"})
	if !cm.CanCompact(messages, 10) {
		t.Fatal("eleven body messages should leave one message to summarize")
	}
	withSystem := append([]provider.Message{{Role: provider.RoleSystem, Content: "system"}}, messages[:10]...)
	if cm.CanCompact(withSystem, 10) {
		t.Fatal("system message must not count as compactable body history")
	}
}

func TestCanCompactRejectsCutInsideOnlyToolGroup(t *testing.T) {
	cm := NewContextManager(80)
	calls := make([]provider.ToolCall, 11)
	messages := make([]provider.Message, 1, 12)
	for i := range calls {
		id := fmt.Sprintf("call-%d", i)
		calls[i] = provider.ToolCall{ID: id, Name: "read_file", Arguments: `{}`}
		messages = append(messages, provider.Message{Role: provider.RoleTool, ToolCallID: id, Name: "read_file", Content: "result"})
	}
	messages[0] = provider.Message{Role: provider.RoleAssistant, ToolCalls: calls}
	if cm.CanCompact(messages, 10) {
		t.Fatal("cutting inside the only complete tool group would produce a no-op compaction")
	}
}

func TestOlderToolResultCompactionReducesEstimatedRequest(t *testing.T) {
	system, messages, tools := representativeRequest()
	messages[2].Content = strings.Repeat("old tool output line\n", 4000)
	messages = append(messages,
		provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "call-2", Name: "read_file", Arguments: `{"path":"recent.go"}`}}},
		provider.Message{Role: provider.RoleTool, ToolCallID: "call-2", Name: "read_file", Content: "recent result remains verbatim"},
	)
	cm := NewContextManager(80)
	before := cm.EstimateRequest(system, messages, tools)
	afterMessages := persistedModelMessages(t, messages)
	after := cm.EstimateRequest(system, afterMessages, tools)
	if after.Total >= before.Total {
		t.Fatalf("request estimate did not shrink: before=%+v after=%+v", before, after)
	}
	if got := afterMessages[len(afterMessages)-1].Content; got != "recent result remains verbatim" {
		t.Fatalf("newest tool result was not preserved: %q", got)
	}
}

var benchmarkRequestTokens RequestTokens

func BenchmarkRequestTokenAccounting(b *testing.B) {
	system, messages, tools := representativeRequest()
	cm := NewContextManager(80)
	messages[2].Content = strings.Repeat("old tool output line\n", 4000)
	messages = append(messages,
		provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "call-2", Name: "read_file", Arguments: `{"path":"recent.go"}`}}},
		provider.Message{Role: provider.RoleTool, ToolCallID: "call-2", Name: "read_file", Content: "recent result remains verbatim"},
	)
	before := cm.EstimateRequest(system, messages, tools)
	afterMessages := persistedModelMessages(b, messages)
	after := cm.EstimateRequest(system, afterMessages, tools)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkRequestTokens = cm.EstimateRequest(system, messages, tools)
	}
	b.StopTimer()
	b.ReportMetric(float64(before.SystemPrompt), "before/system_tokens")
	b.ReportMetric(float64(before.Transcript), "before/transcript_tokens")
	b.ReportMetric(float64(before.ToolSchemas), "before/tool_schema_tokens")
	b.ReportMetric(float64(before.Total), "before/total_tokens")
	b.ReportMetric(float64(after.Total), "after/total_tokens")
	b.ReportMetric(float64(before.Total-after.Total), "saved_tokens")
	_ = fmt.Sprintf("%d", after.Total)
}

func persistedModelMessages(tb testing.TB, messages []provider.Message) []provider.Message {
	tb.Helper()
	manager := session.NewManager(tb.TempDir())
	if _, err := manager.New("test", "test"); err != nil {
		tb.Fatal(err)
	}
	for _, message := range messages {
		if err := manager.AddMessage(message); err != nil {
			tb.Fatal(err)
		}
	}
	return session.ModelMessages(manager.Current().Messages)
}
