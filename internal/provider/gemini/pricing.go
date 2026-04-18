package gemini

type pricingEntry struct {
	Input         float64
	Output        float64
	ContextWindow int
}

// pricingTable is keyed by canonical model ID (without the "models/" prefix
// the API returns). Lookups normalise that prefix away first.
var pricingTable = map[string]pricingEntry{
	"gemini-2.5-pro":   {Input: 1.25, Output: 10.00, ContextWindow: 2_000_000},
	"gemini-2.5-flash": {Input: 0.15, Output: 0.60, ContextWindow: 1_000_000},
	"gemini-2.0-flash": {Input: 0.10, Output: 0.40, ContextWindow: 1_000_000},
}
