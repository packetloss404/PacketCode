package picker

import "testing"

func TestMatches_EmptyFilter(t *testing.T) {
	it := Item{ID: "openai", Label: "OpenAI", Detail: "gpt-4.1"}
	if !matches(it, "") {
		t.Fatalf("empty filter must match every item")
	}
	if !matches(it, "   ") {
		t.Fatalf("whitespace-only filter must match every item")
	}
}

func TestMatches_CaseInsensitiveSubstring(t *testing.T) {
	it := Item{ID: "gpt-4.1", Label: "OpenAI gpt-4.1"}
	if !matches(it, "GPT") {
		t.Fatalf("uppercase filter should match lowercase id")
	}
	if !matches(it, "openai") {
		t.Fatalf("lowercase filter should match mixed-case label")
	}
}

func TestMatches_WhitespaceToDash(t *testing.T) {
	it := Item{ID: "gpt-4.1", Label: "OpenAI gpt-4.1"}
	if !matches(it, "gpt 4.1") {
		t.Fatalf("filter with space should match dashed id")
	}
}

func TestMatches_MultipleTokens(t *testing.T) {
	it := Item{ID: "anthropic/claude-sonnet-3.7", Label: "Claude Sonnet 3.7"}
	if !matches(it, "claude sonnet") {
		t.Fatalf("multi-word filter should match concatenated id")
	}
}

func TestMatches_NoMatch(t *testing.T) {
	it := Item{ID: "gpt-4.1", Label: "OpenAI gpt-4.1", Detail: "reasoning"}
	if matches(it, "foo") {
		t.Fatalf("filter %q should not match", "foo")
	}
}

func TestMatches_SearchesAllFields(t *testing.T) {
	it := Item{ID: "alpha", Label: "Bravo", Detail: "Charlie"}
	if !matches(it, "alpha") {
		t.Fatalf("should match on ID")
	}
	if !matches(it, "bravo") {
		t.Fatalf("should match on Label")
	}
	if !matches(it, "charlie") {
		t.Fatalf("should match on Detail")
	}
}
