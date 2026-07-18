package main

import (
	"strings"

	"github.com/packetcode/packetcode/internal/app"
	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/anthropic"
	"github.com/packetcode/packetcode/internal/provider/codex"
	"github.com/packetcode/packetcode/internal/provider/custom"
	"github.com/packetcode/packetcode/internal/provider/gemini"
	"github.com/packetcode/packetcode/internal/provider/minimax"
	"github.com/packetcode/packetcode/internal/provider/ollama"
	"github.com/packetcode/packetcode/internal/provider/openai"
	"github.com/packetcode/packetcode/internal/provider/openrouter"
)

func providerFactoriesFromConfig(cfg *config.Config) app.FactoryMap {
	factories := app.FactoryMap{
		"openai":     func(key string) provider.Provider { return openai.New(key) },
		"anthropic":  func(key string) provider.Provider { return anthropic.New(key) },
		"gemini":     func(key string) provider.Provider { return gemini.New(key) },
		"minimax":    func(key string) provider.Provider { return minimax.New(key) },
		"openrouter": func(key string) provider.Provider { return openrouter.New(key) },
		"ollama":     func(_ string) provider.Provider { return ollama.NewWithOptions(ollamaHost(cfg), ollamaOptions(cfg)) },
		"codex":      func(_ string) provider.Provider { return codex.New(codexAuthPath(cfg)) },
	}
	if cfg == nil {
		return factories
	}
	for slug, pc := range cfg.Providers {
		if !pc.IsOpenAICompatible() {
			continue
		}
		slug := strings.TrimSpace(slug)
		pc := pc
		if slug == "" || isBuiltInProvider(slug) {
			continue
		}
		factories[slug] = func(key string) provider.Provider {
			return custom.NewOpenAICompatible(custom.Config{
				Slug:           slug,
				DisplayName:    pc.DisplayName,
				BaseURL:        pc.BaseURL,
				APIKey:         key,
				APIKeyRequired: pc.RequiresAPIKey(slug),
				BrandColor:     pc.BrandColor,
				Headers:        pc.Headers,
				DefaultModel:   pc.DefaultModel,
				Models:         customModelConfigs(pc.Models),
			})
		}
	}
	return factories
}

// ollamaOptions maps the [providers.ollama] tuning knobs into the provider's
// Options. All are optional — an absent block yields the zero value, i.e.
// packetcode's smart defaults.
func ollamaOptions(cfg *config.Config) ollama.Options {
	if cfg == nil {
		return ollama.Options{}
	}
	pc, ok := cfg.Providers["ollama"]
	if !ok {
		return ollama.Options{}
	}
	return ollama.Options{
		NumCtx:      pc.NumCtx,
		KeepAlive:   pc.KeepAlive,
		Temperature: pc.Temperature,
	}
}

func customModelConfigs(in []config.ProviderModelConfig) []custom.ModelConfig {
	if len(in) == 0 {
		return nil
	}
	out := make([]custom.ModelConfig, 0, len(in))
	for _, m := range in {
		out = append(out, custom.ModelConfig{
			ID:            m.ID,
			DisplayName:   m.DisplayName,
			ContextWindow: m.ContextWindow,
			SupportsTools: m.SupportsTools,
			InputPer1M:    m.InputPer1M,
			OutputPer1M:   m.OutputPer1M,
		})
	}
	return out
}

func providerRequiresAPIKey(cfg *config.Config, slug string) bool {
	if cfg == nil {
		return !config.IsKeylessProvider(slug)
	}
	if pc, ok := cfg.Providers[slug]; ok {
		return pc.RequiresAPIKey(slug)
	}
	return !config.IsKeylessProvider(slug)
}

func builtInProviderSlugs() []string {
	return []string{"openai", "codex", "anthropic", "gemini", "minimax", "openrouter", "ollama"}
}

// codexAuthPath resolves the Codex auth.json location. An explicit
// [providers.codex] host override wins (useful for tests and non-standard
// CODEX_HOME layouts); otherwise the codex provider falls back to the
// conventional path.
func codexAuthPath(cfg *config.Config) string {
	if cfg != nil {
		if pc, ok := cfg.Providers["codex"]; ok && strings.TrimSpace(pc.Host) != "" {
			return strings.TrimSpace(pc.Host)
		}
	}
	return ""
}

func isBuiltInProvider(slug string) bool {
	for _, known := range builtInProviderSlugs() {
		if slug == known {
			return true
		}
	}
	return false
}
