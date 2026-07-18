package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/packetcode/packetcode/internal/provider"
)

// modelsCacheName is the file the Codex CLI maintains alongside auth.json
// listing the models the signed-in ChatGPT account may use. packetcode reads
// it so its model picker stays in sync with the CLI instead of hard-coding a
// catalog that goes stale as OpenAI ships new models.
const modelsCacheName = "models_cache.json"

// staticFallbackCatalog is used when models_cache.json is missing or
// unreadable. It reflects the ChatGPT-account Codex line-up known at build
// time; the live cache supersedes it whenever present.
var staticFallbackCatalog = []cachedModel{
	{Slug: "gpt-5.6-sol", Display: "GPT-5.6-Sol", Context: 272_000, DefaultEffort: "low", Priority: 1, SummarySupported: true},
	{Slug: "gpt-5.6-terra", Display: "GPT-5.6-Terra", Context: 272_000, DefaultEffort: "medium", Priority: 2, SummarySupported: true},
	{Slug: "gpt-5.6-luna", Display: "GPT-5.6-Luna", Context: 272_000, DefaultEffort: "medium", Priority: 3, SummarySupported: true},
	{Slug: "gpt-5.5", Display: "GPT-5.5", Context: 272_000, DefaultEffort: "medium", Priority: 7, SummarySupported: true},
	{Slug: "gpt-5.4", Display: "GPT-5.4", Context: 272_000, DefaultEffort: "medium", Priority: 16, SummarySupported: true},
	{Slug: "gpt-5.4-mini", Display: "GPT-5.4-mini", Context: 272_000, DefaultEffort: "medium", Priority: 23, SummarySupported: true},
}

// cachedModel is the distilled view of one models_cache.json entry.
type cachedModel struct {
	Slug          string
	Display       string
	Context       int
	DefaultEffort string
	Priority      int
	// SummarySupported is whether the model accepts reasoning.summary. When
	// false (only gpt-5.3-codex-spark today) sending it 400s, so we omit it.
	// The gpt-5.6 family accepts it but ignores it (encrypted-only reasoning);
	// gpt-5.4/5.5 actually stream summaries when asked.
	SummarySupported bool
}

// rawModelsCache mirrors the on-disk models_cache.json shape (only the fields
// packetcode needs).
type rawModelsCache struct {
	Models []struct {
		Slug          string `json:"slug"`
		DisplayName   string `json:"display_name"`
		ContextWindow int    `json:"context_window"`
		Visibility    string `json:"visibility"`
		Priority      int    `json:"priority"`
		DefaultEffort string `json:"default_reasoning_level"`
		// SupportsReasoningSummaryParameter is a pointer so we can tell an
		// absent field (which means "supported" — the Codex CLI default) from
		// an explicit false (unsupported, e.g. gpt-5.3-codex-spark).
		SupportsReasoningSummaryParameter *bool `json:"supports_reasoning_summary_parameter"`
	} `json:"models"`
}

// loadCatalog returns the selectable models for the account. It prefers the
// live models_cache.json next to auth.json and falls back to the static list.
// Entries are ordered by the cache's priority so the flagship model sorts
// first. Models hidden by the backend (visibility != "list") are dropped.
func loadCatalog(authPath string) []cachedModel {
	models := readModelsCache(authPath)
	if len(models) == 0 {
		models = append([]cachedModel(nil), staticFallbackCatalog...)
	}
	sort.SliceStable(models, func(i, j int) bool {
		return models[i].Priority < models[j].Priority
	})
	return models
}

func readModelsCache(authPath string) []cachedModel {
	if authPath == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(authPath), modelsCacheName))
	if err != nil {
		return nil
	}
	var parsed rawModelsCache
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	out := make([]cachedModel, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		if m.Slug == "" || (m.Visibility != "" && m.Visibility != "list") {
			continue
		}
		out = append(out, cachedModel{
			Slug:             m.Slug,
			Display:          firstNonEmpty(m.DisplayName, m.Slug),
			Context:          m.ContextWindow,
			DefaultEffort:    m.DefaultEffort,
			Priority:         m.Priority,
			SummarySupported: m.SupportsReasoningSummaryParameter == nil || *m.SupportsReasoningSummaryParameter,
		})
	}
	return out
}

// toProviderModels converts the catalog into provider.Model values for the UI.
func toProviderModels(cat []cachedModel) []provider.Model {
	out := make([]provider.Model, 0, len(cat))
	for _, m := range cat {
		cw := m.Context
		if cw == 0 {
			cw = defaultContextWindow
		}
		out = append(out, provider.Model{
			ID:            m.Slug,
			DisplayName:   m.Display,
			ContextWindow: cw,
			SupportsTools: true,
		})
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
