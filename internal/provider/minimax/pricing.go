package minimax

type pricingEntry struct {
	Input         float64
	Output        float64
	ContextWindow int
	SupportsTools bool
}

// MiniMax-Text-01 is a 456B-param MoE with a 1M-token context. Public
// pricing as of Q1 2026 (USD per 1M tokens).
var pricingTable = map[string]pricingEntry{
	"MiniMax-Text-01": {Input: 0.20, Output: 1.10, ContextWindow: 1_000_000, SupportsTools: true},
	"abab6.5s-chat":   {Input: 1.00, Output: 1.00, ContextWindow: 245_000, SupportsTools: false},
	"abab6.5-chat":    {Input: 5.00, Output: 5.00, ContextWindow: 245_000, SupportsTools: false},
}

// fallbackModels are surfaced when the API doesn't expose a model-list
// endpoint we can use. Listing keeps the model selector functional even
// with zero discovery support upstream.
var fallbackModels = []string{
	"MiniMax-Text-01",
}
