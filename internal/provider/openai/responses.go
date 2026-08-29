package openai

import "strings"

// Several OpenAI models refuse function tools on /v1/chat/completions and are
// only usable through /v1/responses. packetcode sends tools on every turn, so
// for those models the chat-completions endpoint fails every request:
//
//	400: Function tools with reasoning_effort are not supported for
//	gpt-5.6-sol in /v1/chat/completions. To use function tools, use
//	/v1/responses or set reasoning_effort to 'none'.
//
// The `reasoning_effort` in that message is the API's own default for the
// model, not something packetcode sends -- there is no such field in
// openaicompat's request body. Setting it to 'none' is not an option worth
// taking either: it would buy chat-completions compatibility by turning off
// the reasoning that is the entire reason to choose one of these models.
//
// So they are routed to the Responses API instead, which packetcode already
// speaks for the codex provider.

// responsesOnlyPrefixes are model-ID prefixes that must use /v1/responses.
//
// Matched by prefix so dated snapshots come along: gpt-5.6-sol-2026-04-01 is
// the same model with the same constraint, and a table of exact IDs would go
// stale the first time OpenAI pins one.
var responsesOnlyPrefixes = []string{
	"gpt-5.6-sol",
	"gpt-5.6-terra",
	"gpt-5.6-luna",
}

// responsesOnlySuffixes are model-ID suffixes that must use /v1/responses.
//
// The "-pro" family (o1-pro, o3-pro, gpt-5.5-pro) has always been
// Responses-only. It used to be filtered out of the catalog entirely for that
// reason; now that there is somewhere for it to go, it is offered instead.
var responsesOnlySuffixes = []string{
	"-pro",
}

// requiresResponsesAPI reports whether modelID must be sent to /v1/responses.
//
// Conservative by design: a model wrongly routed here would lose nothing but
// would be exercising a less-travelled code path, while a model wrongly left
// on chat-completions fails every single turn with a 400. The list therefore
// holds only families known to require it, and everything else -- including
// model families released after this build -- keeps the endpoint that has
// always worked for them.
func requiresResponsesAPI(modelID string) bool {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return false
	}
	for _, prefix := range responsesOnlyPrefixes {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	for _, suffix := range responsesOnlySuffixes {
		if strings.HasSuffix(id, suffix) {
			return true
		}
	}
	return false
}
