package openai

// pricingEntry is USD per 1M tokens for input and output respectively.
type pricingEntry struct {
	Input  float64
	Output float64
	// ContextWindow is the model's max input tokens; 0 means unknown.
	ContextWindow int
	// SupportsTools is best-effort — every modern OpenAI chat model does.
	SupportsTools bool
}

// pricingTable is keyed by exact OpenAI model ID. Lookups for unknown IDs
// fall through to a conservative default in Pricing/ContextWindow.
//
// Prices last verified against OpenAI's public price list as of Q1 2026.
var pricingTable = map[string]pricingEntry{
	"gpt-4.1":      {Input: 2.00, Output: 8.00, ContextWindow: 1_000_000, SupportsTools: true},
	"gpt-4.1-mini": {Input: 0.40, Output: 1.60, ContextWindow: 1_000_000, SupportsTools: true},
	"gpt-4.1-nano": {Input: 0.10, Output: 0.40, ContextWindow: 1_000_000, SupportsTools: true},
	"o3":           {Input: 10.00, Output: 40.00, ContextWindow: 200_000, SupportsTools: true},
	"o4-mini":      {Input: 1.10, Output: 4.40, ContextWindow: 200_000, SupportsTools: true},
}

// supportedPrefixes is the prefix allow-list for the chat model filter in
// ListModels. We keep it tight to avoid surfacing fine-tunes, embeddings,
// audio models, etc., in the model selector.
var supportedPrefixes = []string{
	"gpt-4.1", "gpt-4o", "o3", "o4",
}
