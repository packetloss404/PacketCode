package provider

// Cost estimation, and the one thing it must not do: charge for work the
// provider gave away.
//
// Every provider here bills a token served from its prompt cache at a fraction
// of a fresh one, and reports how many it served. packetcode recorded that
// number faithfully and then multiplied the whole input count -- cached tokens
// included -- by the standard rate. In a six-task benchmark 93% of input came
// from cache, so the figure shown to the user was roughly six times the real
// bill. It reported a tool that was cheap as though it were expensive, which is
// the wrong direction for a number nobody can check.
//
// The counts were always there. Only the arithmetic was wrong.

const (
	// CacheReadMultiplier scales the input rate for tokens served from the
	// provider's prompt cache.
	//
	// A tenth is what OpenAI and Anthropic both charge for a cache read.
	// Providers that differ implement CacheRated and say so; this is the
	// default for the rest, and it is an estimate. It is a far better one
	// than 1.0, which is not an estimate but an error -- no provider charges
	// full price for a cache hit.
	CacheReadMultiplier = 0.10

	// CacheWriteMultiplier scales the input rate for tokens written into the
	// cache. One, because most providers charge writes at the ordinary input
	// rate; Anthropic charges a premium and says so through CacheRated.
	CacheWriteMultiplier = 1.00
)

// CacheRated is implemented by providers whose cache pricing differs from the
// defaults above. Optional: a provider that does not implement it is costed
// with the defaults.
type CacheRated interface {
	// CacheMultipliers returns the read and write multipliers on the model's
	// input rate.
	CacheMultipliers(modelID string) (read, write float64)
}

// CacheMultipliersFor asks a provider for its cache pricing, falling back to
// the defaults.
//
// A provider that returns a negative multiplier is ignored rather than
// honoured: a negative rate would subtract from the bill, and a cost display
// that can be driven downward by a bad value is worse than one that is merely
// approximate.
func CacheMultipliersFor(p Provider, modelID string) (read, write float64) {
	read, write = CacheReadMultiplier, CacheWriteMultiplier
	if r, ok := p.(CacheRated); ok {
		gotRead, gotWrite := r.CacheMultipliers(modelID)
		if gotRead >= 0 {
			read = gotRead
		}
		if gotWrite >= 0 {
			write = gotWrite
		}
	}
	return read, write
}

// EstimateCost returns the USD cost of one usage total.
//
// cacheRead and cacheCreation are SUBSETS of input, never addends. Every
// provider adapter normalises to that shape -- anthropic.go sums its three
// separate wire counts into InputTokens for exactly this reason, and OpenAI's
// prompt_tokens already includes its cached tokens -- so the uncached remainder
// is what is left after both are taken out.
//
// Defensive about the arithmetic rather than trusting the invariant: a provider
// that ever reports more cached tokens than input tokens would otherwise
// produce a negative remainder and a cost below zero, and a wrong number that
// looks impossible is easier to notice than a wrong number that looks fine.
func EstimateCost(input, output, cacheRead, cacheCreation int, inputPer1M, outputPer1M, readMultiplier, writeMultiplier float64) float64 {
	if cacheRead < 0 {
		cacheRead = 0
	}
	if cacheCreation < 0 {
		cacheCreation = 0
	}
	cached := cacheRead + cacheCreation
	if cached > input {
		// Trust the smaller, provider-reported split over the total, but do
		// not let it drive the uncached count negative.
		cached = input
		if cacheRead > input {
			cacheRead = input
		}
		cacheCreation = cached - cacheRead
	}
	uncached := input - cached

	perToken := inputPer1M / 1_000_000
	return float64(uncached)*perToken +
		float64(cacheRead)*perToken*readMultiplier +
		float64(cacheCreation)*perToken*writeMultiplier +
		float64(output)*outputPer1M/1_000_000
}
