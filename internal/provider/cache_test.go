package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCachePrefixFingerprintCanonicalizesToolAndSchemaOrder(t *testing.T) {
	left := []ToolDefinition{
		{Name: "zeta", Description: "last", Parameters: json.RawMessage(`{"type":"object","properties":{"b":{"type":"string"},"a":{"type":"integer"}}}`)},
		{Name: "alpha", Description: "first", Parameters: json.RawMessage(`{"required":["path"],"type":"object"}`)},
	}
	right := []ToolDefinition{
		{Name: "alpha", Description: "first", Parameters: json.RawMessage(`{ "type": "object", "required": ["path"] }`)},
		{Name: "zeta", Description: "last", Parameters: json.RawMessage(`{"properties":{"a":{"type":"integer"},"b":{"type":"string"}},"type":"object"}`)},
	}

	assert.Equal(t, CachePrefixFingerprint("stable system", left), CachePrefixFingerprint("stable system", right))
	assert.NotEqual(t, CachePrefixFingerprint("stable system", left), CachePrefixFingerprint("changed system", left))
}

func TestCachePrefixFingerprintDoesNotCollapseInvalidSchemas(t *testing.T) {
	left := []ToolDefinition{{Name: "broken", Parameters: json.RawMessage(`{"type":`)}}
	right := []ToolDefinition{{Name: "broken", Parameters: json.RawMessage(`{"other":`)}}
	assert.NotEqual(t, CachePrefixFingerprint("stable", left), CachePrefixFingerprint("stable", right))
}

func TestChatRequestJSONNeverContainsSugarCacheMetadata(t *testing.T) {
	payload, err := json.Marshal(ChatRequest{
		Model: "direct-model",
		SugarCache: &SugarCacheMetadata{
			ConversationID:    "session-1",
			PrefixFingerprint: CachePrefixFingerprint("", nil),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.NotContains(t, string(payload), "SugarCache")
	assert.NotContains(t, string(payload), "sugar_cache")
	assert.NotContains(t, string(payload), "session-1")
}
