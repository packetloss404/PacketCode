// Package minimax implements provider.Provider against MiniMax's
// OpenAI-compatible chat-completions endpoint.
//
// MiniMax's protocol is identical to OpenAI's at the wire level, so this
// package is little more than a thin shell over openaicompat.Client with
// MiniMax-specific identity, base URL, fallback model list, and pricing.
package minimax

import (
	"context"

	"github.com/charmbracelet/lipgloss"

	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/openaicompat"
)

const (
	defaultBaseURL = "https://api.minimax.chat/v1"
	slug           = "minimax"
	displayName    = "MiniMax"
)

var brandColor = lipgloss.Color("#FF8C00")

type Provider struct {
	client *openaicompat.Client
}

func New(apiKey string) *Provider {
	return NewWithBaseURL(defaultBaseURL, apiKey)
}

func NewWithBaseURL(baseURL, apiKey string) *Provider {
	return &Provider{client: openaicompat.NewClient(baseURL, apiKey)}
}

func (p *Provider) Name() string                { return displayName }
func (p *Provider) Slug() string                { return slug }
func (p *Provider) BrandColor() lipgloss.Color  { return brandColor }

func (p *Provider) ValidateKey(ctx context.Context, apiKey string) error {
	return p.client.ValidateKey(ctx, apiKey)
}

// ListModels asks the upstream catalog first; if MiniMax responds with an
// error or an empty list (their /models endpoint is not always available),
// we fall back to the curated list in pricing.go so the selector still
// shows something usable.
func (p *Provider) ListModels(ctx context.Context) ([]provider.Model, error) {
	raw, err := p.client.ListModels(ctx)
	if err != nil || len(raw) == 0 {
		return p.fallback(), nil
	}
	out := make([]provider.Model, 0, len(raw))
	for _, m := range raw {
		entry := pricingTable[m.ID]
		out = append(out, provider.Model{
			ID:            m.ID,
			DisplayName:   m.ID,
			ContextWindow: entry.ContextWindow,
			SupportsTools: true,
			InputPer1M:    entry.Input,
			OutputPer1M:   entry.Output,
		})
	}
	return out, nil
}

func (p *Provider) fallback() []provider.Model {
	out := make([]provider.Model, 0, len(fallbackModels))
	for _, id := range fallbackModels {
		entry := pricingTable[id]
		out = append(out, provider.Model{
			ID:            id,
			DisplayName:   id,
			ContextWindow: entry.ContextWindow,
			SupportsTools: true,
			InputPer1M:    entry.Input,
			OutputPer1M:   entry.Output,
		})
	}
	return out
}

func (p *Provider) ChatCompletion(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	return p.client.ChatCompletion(ctx, req)
}

func (p *Provider) Pricing(modelID string) (float64, float64) {
	if entry, ok := pricingTable[modelID]; ok {
		return entry.Input, entry.Output
	}
	return 1.00, 1.00
}

func (p *Provider) ContextWindow(modelID string) int {
	if entry, ok := pricingTable[modelID]; ok {
		return entry.ContextWindow
	}
	return 245_000
}

// SupportsTools — MiniMax-Text-01 supports function calling natively.
// Older abab models did not; we conservatively report true and let the
// agent loop surface upstream errors if a model rejects tool use.
func (p *Provider) SupportsTools(modelID string) bool { return true }
