package mistral

type pricingEntry struct {
	Input         float64
	Output        float64
	ContextWindow int
	SupportsTools bool
}

// Prices last verified against Mistral AI's public API pricing in July 2026.
// The -latest suffixes are Mistral's stable pointers to the current release
// of each family. devstral and codestral are the coding-focused tiers.
var pricingTable = map[string]pricingEntry{
	"mistral-large-latest":   {Input: 0.50, Output: 1.50, ContextWindow: 128_000, SupportsTools: true},
	"devstral-medium-latest": {Input: 0.40, Output: 2.00, ContextWindow: 256_000, SupportsTools: true},
	"codestral-latest":       {Input: 0.30, Output: 0.90, ContextWindow: 256_000, SupportsTools: true},
	"mistral-small-latest":   {Input: 0.10, Output: 0.30, ContextWindow: 128_000, SupportsTools: true},
}

// fallbackModels are surfaced when the API's model-list endpoint is
// unavailable, keeping the model selector functional with zero discovery.
var fallbackModels = []string{
	"mistral-large-latest",
	"devstral-medium-latest",
	"codestral-latest",
	"mistral-small-latest",
}
