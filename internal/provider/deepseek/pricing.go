package deepseek

type pricingEntry struct {
	Input         float64
	Output        float64
	ContextWindow int
	SupportsTools bool
}

// Prices last verified against DeepSeek's public API pricing in July 2026.
// deepseek-chat and deepseek-reasoner are stable pointers that track the
// current V4 flagship; the -v4- entries name specific tiers. Context windows
// come from the DeepSeek model overview (V4 is 1M).
var pricingTable = map[string]pricingEntry{
	"deepseek-chat":     {Input: 0.28, Output: 0.42, ContextWindow: 1_000_000, SupportsTools: true},
	"deepseek-reasoner": {Input: 0.55, Output: 2.19, ContextWindow: 1_000_000, SupportsTools: true},
	"deepseek-v4-pro":   {Input: 0.44, Output: 0.87, ContextWindow: 1_000_000, SupportsTools: true},
	"deepseek-v4-flash": {Input: 0.14, Output: 0.28, ContextWindow: 1_000_000, SupportsTools: true},
}

// fallbackModels are surfaced when the API's model-list endpoint is
// unavailable, keeping the model selector functional with zero discovery.
var fallbackModels = []string{
	"deepseek-chat",
	"deepseek-reasoner",
	"deepseek-v4-pro",
	"deepseek-v4-flash",
}
