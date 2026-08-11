package main

import (
	"os"
	"strings"
	"time"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/app"
	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/anthropic"
	"github.com/packetcode/packetcode/internal/provider/codex"
	"github.com/packetcode/packetcode/internal/provider/custom"
	"github.com/packetcode/packetcode/internal/provider/deepseek"
	"github.com/packetcode/packetcode/internal/provider/gemini"
	"github.com/packetcode/packetcode/internal/provider/grok"
	"github.com/packetcode/packetcode/internal/provider/minimax"
	"github.com/packetcode/packetcode/internal/provider/mistral"
	"github.com/packetcode/packetcode/internal/provider/ollama"
	"github.com/packetcode/packetcode/internal/provider/openai"
	"github.com/packetcode/packetcode/internal/provider/openrouter"
	"github.com/packetcode/packetcode/internal/provider/sugar"
)

func packetcodeSugarCacheConfig(cfg *config.Config) agent.SugarCacheConfig {
	if cfg == nil {
		return agent.SugarCacheConfig{}
	}
	return agent.SugarCacheConfig{
		Mode:      provider.SugarCacheMode(cfg.Sugar.CacheMode),
		Retention: provider.SugarCacheRetention(cfg.Sugar.CacheRetention),
		Privacy:   provider.SugarPrivacyMode(cfg.Sugar.Privacy),
	}
}

func packetcodeConduitShadowConfig(cfg *config.Config) agent.ConduitShadowConfig {
	if cfg == nil {
		return agent.ConduitShadowConfig{}
	}
	return agent.ConduitShadowConfig{
		Enabled:         cfg.Conduit.ShadowEnabled,
		Timeout:         time.Duration(cfg.Conduit.TimeoutMS) * time.Millisecond,
		CapsuleMaxBytes: cfg.Conduit.CapsuleMaxBytes,
	}
}

func providerFactoriesFromConfig(cfg *config.Config) app.FactoryMap {
	factories := app.FactoryMap{
		"openai":     func(key string) provider.Provider { return openai.New(key) },
		"anthropic":  func(key string) provider.Provider { return anthropic.New(key) },
		"gemini":     func(key string) provider.Provider { return gemini.New(key) },
		"minimax":    func(key string) provider.Provider { return minimax.New(key) },
		"deepseek":   func(key string) provider.Provider { return deepseek.New(key) },
		"grok":       func(key string) provider.Provider { return grok.New(key) },
		"mistral":    func(key string) provider.Provider { return mistral.New(key) },
		"openrouter": func(key string) provider.Provider { return openrouter.New(key) },
		"sugar": func(key string) provider.Provider {
			p := sugar.NewWithBaseURL(sugarBaseURL(cfg), key)
			if cfg != nil && cfg.Conduit.ShadowEnabled {
				p.SetRuntimeHooks(sugar.NewRuntimeClient(sugarBaseURL(cfg), key, nil, true))
			}
			return p
		},
		"ollama": func(_ string) provider.Provider { return ollama.NewWithOptions(ollamaHost(cfg), ollamaOptions(cfg)) },
		"codex": func(_ string) provider.Provider {
			p := codex.New(codexAuthPath(cfg))
			if cfg != nil {
				pc := cfg.Providers["codex"]
				model := pc.DefaultModel
				if model == "" && cfg.Default.Provider == "codex" {
					model = cfg.Default.Model
				}
				if model == "" {
					model = codex.DefaultModel
				}
				_ = p.SetReasoningEffort(model, pc.ReasoningEffort)
			}
			return p
		},
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
	return []string{"sugar", "openai", "codex", "anthropic", "gemini", "minimax", "deepseek", "grok", "mistral", "openrouter", "ollama"}
}

func sugarBaseURL(cfg *config.Config) string {
	if base := strings.TrimSpace(os.Getenv("PACKETCODE_SUGAR_BASE_URL")); base != "" {
		return sugar.NormalizeBaseURL(base)
	}
	if cfg != nil {
		if pc, ok := cfg.Providers["sugar"]; ok && strings.TrimSpace(pc.BaseURL) != "" {
			return sugar.NormalizeBaseURL(pc.BaseURL)
		}
	}
	return sugar.DefaultBaseURL
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
