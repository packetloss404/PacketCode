package grok

type pricingEntry struct {
	Input         float64
	Output        float64
	ContextWindow int
	SupportsTools bool
}

// Prices last verified against xAI's public API pricing in July 2026.
// grok-4.5 is the current flagship (launched 2026-07-08); the fast tiers
// carry much larger context windows at lower cost.
var pricingTable = map[string]pricingEntry{
	"grok-4.5":      {Input: 2.00, Output: 6.00, ContextWindow: 500_000, SupportsTools: true},
	"grok-4.3":      {Input: 1.25, Output: 2.50, ContextWindow: 1_000_000, SupportsTools: true},
	"grok-4.1-fast": {Input: 0.20, Output: 0.50, ContextWindow: 2_000_000, SupportsTools: true},
	"grok-4-fast":   {Input: 0.20, Output: 0.50, ContextWindow: 2_000_000, SupportsTools: true},
}

// fallbackModels are surfaced when the API's model-list endpoint is
// unavailable, keeping the model selector functional with zero discovery.
var fallbackModels = []string{
	"grok-4.5",
	"grok-4.3",
	"grok-4.1-fast",
	"grok-4-fast",
}
