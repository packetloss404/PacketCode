// Package grok implements provider.Provider against xAI's OpenAI-compatible
// chat-completions endpoint (Grok).
//
// xAI's protocol is identical to OpenAI's at the wire level, so this package
// is a thin shell over openaicompat.Client with xAI-specific identity, base
// URL, fallback model list, and pricing.
package grok

import (
	"context"

	"github.com/charmbracelet/lipgloss"

	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/openaicompat"
)

const (
	defaultBaseURL = "https://api.x.ai/v1"
	DefaultModel   = "grok-4.5"
	slug           = "grok"
	displayName    = "Grok (xAI)"
)

var brandColor = lipgloss.Color("#9CA3AF")

type Provider struct {
	client *openaicompat.Client
}

func New(apiKey string) *Provider {
	return NewWithBaseURL(defaultBaseURL, apiKey)
}

func NewWithBaseURL(baseURL, apiKey string) *Provider {
	return &Provider{client: openaicompat.NewClient(baseURL, apiKey)}
}

func (p *Provider) Name() string               { return displayName }
func (p *Provider) Slug() string               { return slug }
func (p *Provider) BrandColor() lipgloss.Color { return brandColor }

func (p *Provider) ValidateKey(ctx context.Context, apiKey string) error {
	return p.client.ValidateKey(ctx, apiKey)
}

// ListModels asks the upstream catalog first; if xAI responds with an error
// or an empty list we fall back to the curated list in pricing.go so the
// selector still shows something usable.
func (p *Provider) ListModels(ctx context.Context) ([]provider.Model, error) {
	raw, err := p.client.ListModels(ctx)
	if err != nil || len(raw) == 0 {
		return p.fallback(), nil
	}
	out := make([]provider.Model, 0, len(raw))
	for _, m := range raw {
		in, outRate := p.Pricing(m.ID)
		out = append(out, provider.Model{
			ID:            m.ID,
			DisplayName:   m.ID,
			ContextWindow: p.ContextWindow(m.ID),
			SupportsTools: p.SupportsTools(m.ID),
			InputPer1M:    in,
			OutputPer1M:   outRate,
		})
	}
	return prioritizeDefaultModel(out), nil
}

func (p *Provider) fallback() []provider.Model {
	out := make([]provider.Model, 0, len(fallbackModels))
	for _, id := range fallbackModels {
		in, outRate := p.Pricing(id)
		out = append(out, provider.Model{
			ID:            id,
			DisplayName:   id,
			ContextWindow: p.ContextWindow(id),
			SupportsTools: p.SupportsTools(id),
			InputPer1M:    in,
			OutputPer1M:   outRate,
		})
	}
	return prioritizeDefaultModel(out)
}

func (p *Provider) ChatCompletion(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	return p.client.ChatCompletion(ctx, req)
}

func (p *Provider) Pricing(modelID string) (float64, float64) {
	if entry, ok := pricingTable[modelID]; ok {
		return entry.Input, entry.Output
	}
	return 2.00, 6.00
}

func (p *Provider) ContextWindow(modelID string) int {
	if entry, ok := pricingTable[modelID]; ok {
		return entry.ContextWindow
	}
	return 256_000
}

func (p *Provider) SupportsTools(modelID string) bool {
	if entry, ok := pricingTable[modelID]; ok {
		return entry.SupportsTools
	}
	return true
}

func prioritizeDefaultModel(models []provider.Model) []provider.Model {
	for i, m := range models {
		if m.ID != DefaultModel {
			continue
		}
		if i == 0 {
			return models
		}
		out := make([]provider.Model, 0, len(models))
		out = append(out, m)
		out = append(out, models[:i]...)
		out = append(out, models[i+1:]...)
		return out
	}
	return models
}
