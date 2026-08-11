package provider

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"
)

const cachePrefixFingerprintVersion = "packetcode-cache-prefix-v1\x00"

// CachePrefixFingerprint returns a deterministic identifier for the immutable
// model prefix Packetcode owns: its system prompt and tool schemas. Tool order
// and JSON object key order are canonicalized so registration/map iteration
// order cannot split an otherwise identical cache lineage.
func CachePrefixFingerprint(systemPrompt string, tools []ToolDefinition) string {
	canonicalDefinitions := CanonicalToolDefinitions(tools)
	type canonicalTool struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	}

	canonical := make([]canonicalTool, 0, len(canonicalDefinitions))
	for _, tool := range canonicalDefinitions {
		canonical = append(canonical, canonicalTool{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		})
	}

	payload, err := json.Marshal(struct {
		System string          `json:"system"`
		Tools  []canonicalTool `json:"tools"`
	}{System: systemPrompt, Tools: canonical})
	if err != nil {
		// Invalid schemas will also fail request serialization, but distinct
		// invalid byte sequences must never collapse to one fingerprint.
		payload, _ = json.Marshal(struct {
			System string `json:"system"`
			Tools  []struct {
				Name, Description, Parameters string
			} `json:"tools"`
		}{
			System: systemPrompt,
			Tools:  fallbackTools(canonicalDefinitions),
		})
	}
	sum := sha256.Sum256(append([]byte(cachePrefixFingerprintVersion), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// CanonicalToolDefinitions returns the exact deterministic order and JSON
// schema representation Packetcode sends on OpenAI-compatible wires.
func CanonicalToolDefinitions(tools []ToolDefinition) []ToolDefinition {
	canonical := make([]ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		tool.Parameters = canonicalJSON(tool.Parameters)
		canonical = append(canonical, tool)
	}
	sort.Slice(canonical, func(i, j int) bool {
		if canonical[i].Name != canonical[j].Name {
			return canonical[i].Name < canonical[j].Name
		}
		if canonical[i].Description != canonical[j].Description {
			return canonical[i].Description < canonical[j].Description
		}
		return string(canonical[i].Parameters) < string(canonical[j].Parameters)
	})
	return canonical
}

func canonicalJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("null")
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		// Tool schemas are validated elsewhere. Retaining invalid bytes here is
		// still deterministic and avoids making fingerprinting a failure path.
		return append(json.RawMessage(nil), raw...)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		// Tool schemas are validated elsewhere. Retaining invalid bytes here is
		// still deterministic and avoids making fingerprinting a failure path.
		return append(json.RawMessage(nil), raw...)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return append(json.RawMessage(nil), raw...)
	}
	return canonical
}

func fallbackTools(tools []ToolDefinition) []struct{ Name, Description, Parameters string } {
	out := make([]struct{ Name, Description, Parameters string }, len(tools))
	for i, tool := range tools {
		out[i].Name = tool.Name
		out[i].Description = tool.Description
		out[i].Parameters = string(tool.Parameters)
	}
	return out
}
