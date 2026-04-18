// Package openai implements the provider.Provider interface for OpenAI's
// chat-completions API (GPT-4.1, o3, o4-mini).
//
// All wire-protocol logic lives in internal/provider/openaicompat — this
// package contributes only OpenAI-specific identity, base URL, model
// filtering, and the pricing table.
package openai

import (
	"context"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/openaicompat"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	slug           = "openai"
	displayName    = "OpenAI"
)

// brandColor matches the green from the design system's provider brand
// palette. Kept package-private; exposed via BrandColor().
var brandColor = lipgloss.Color("#10A37F")

// Provider implements provider.Provider for OpenAI.
type Provider struct {
	client *openaicompat.Client
}

// New constructs an OpenAI provider with the given API key. An empty key
// is allowed at construction — ValidateKey will reject it later.
func New(apiKey string) *Provider {
	return &Provider{
		client: openaicompat.NewClient(defaultBaseURL, apiKey),
	}
}

// NewWithBaseURL is exposed for testing against an httptest server.
func NewWithBaseURL(baseURL, apiKey string) *Provider {
	return &Provider{
		client: openaicompat.NewClient(baseURL, apiKey),
	}
}

func (p *Provider) Name() string                { return displayName }
func (p *Provider) Slug() string                { return slug }
func (p *Provider) BrandColor() lipgloss.Color  { return brandColor }

func (p *Provider) ValidateKey(ctx context.Context, apiKey string) error {
	return p.client.ValidateKey(ctx, apiKey)
}

// ListModels filters the upstream catalog to chat-capable models we know
// about. Anything outside supportedPrefixes is hidden so the selector
// modal doesn't drown in embeddings, TTS, etc.
func (p *Provider) ListModels(ctx context.Context) ([]provider.Model, error) {
	raw, err := p.client.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Model, 0, len(raw))
	for _, m := range raw {
		if !isSupported(m.ID) {
			continue
		}
		entry, hasPricing := pricingTable[m.ID]
		out = append(out, provider.Model{
			ID:            m.ID,
			DisplayName:   m.ID,
			ContextWindow: entry.ContextWindow,
			SupportsTools: !hasPricing || entry.SupportsTools, // unknown but supported-prefix → assume yes
			InputPer1M:    entry.Input,
			OutputPer1M:   entry.Output,
		})
	}
	return out, nil
}

func (p *Provider) ChatCompletion(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	return p.client.ChatCompletion(ctx, req)
}

func (p *Provider) Pricing(modelID string) (float64, float64) {
	if entry, ok := pricingTable[modelID]; ok {
		return entry.Input, entry.Output
	}
	// Conservative fallback so unknown models don't report $0 cost.
	return 3.00, 15.00
}

func (p *Provider) ContextWindow(modelID string) int {
	if entry, ok := pricingTable[modelID]; ok {
		return entry.ContextWindow
	}
	return 128_000
}

func (p *Provider) SupportsTools(modelID string) bool {
	if entry, ok := pricingTable[modelID]; ok {
		return entry.SupportsTools
	}
	return isSupported(modelID)
}

func isSupported(id string) bool {
	for _, prefix := range supportedPrefixes {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}
