// Package openai implements the provider.Provider interface for OpenAI.
//
// Two endpoints, chosen per model. Most models go to /v1/chat/completions via
// internal/provider/openaicompat. The gpt-5.6 family and the -pro variants
// refuse function tools there and go to /v1/responses via
// internal/provider/responses instead -- see responses.go for why. This
// package contributes only OpenAI-specific identity, base URLs, model
// filtering, routing, and the pricing table; no wire format lives here.
package openai

import (
	"context"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/openaicompat"
	"github.com/packetcode/packetcode/internal/provider/responses"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	DefaultModel   = "gpt-5.6-sol"
	slug           = "openai"
	displayName    = "OpenAI"
)

// brandColor matches the green from the design system's provider brand
// palette. Kept package-private; exposed via BrandColor().
var brandColor = lipgloss.Color("#10A37F")

// Provider implements provider.Provider for OpenAI.
//
// Both clients are built up front and share the base URL. Which one a turn
// uses is decided per model by requiresResponsesAPI, so switching models with
// /model does not need the provider rebuilt.
type Provider struct {
	client    *openaicompat.Client
	responses *responses.Client
}

// New constructs an OpenAI provider with the given API key. An empty key
// is allowed at construction — ValidateKey will reject it later.
func New(apiKey string) *Provider {
	return NewWithBaseURL(defaultBaseURL, apiKey)
}

// NewWithBaseURL is exposed for testing against an httptest server.
func NewWithBaseURL(baseURL, apiKey string) *Provider {
	return &Provider{
		client:    openaicompat.NewClient(baseURL, apiKey),
		responses: responses.NewAPIKeyClient(baseURL, apiKey),
	}
}

func (p *Provider) Name() string               { return displayName }
func (p *Provider) Slug() string               { return slug }
func (p *Provider) BrandColor() lipgloss.Color { return brandColor }

func (p *Provider) ValidateKey(ctx context.Context, apiKey string) error {
	return p.client.ValidateKey(ctx, apiKey)
}

// ListModels filters the upstream catalog to chat-capable models by
// dropping known non-chat types (embeddings, TTS, whisper, dall-e, etc.).
// Everything else passes through — including model families we don't
// have pricing for yet — so newly-released chat models appear in the
// selector without requiring a code release here.
//
// The upstream catalog is the source of truth for what the current key
// can choose in setup and the model picker. Pricing metadata enriches
// models that the API actually returned; it does not seed speculative
// aliases that may be inaccessible to the account.
func (p *Provider) ListModels(ctx context.Context) ([]provider.Model, error) {
	raw, err := p.client.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Model, 0, len(raw))
	for _, m := range raw {
		if !isChatModel(m.ID) {
			continue
		}
		entry, hasPricing := pricingTable[m.ID]
		if !hasPricing {
			entry.Input, entry.Output = p.Pricing(m.ID)
			entry.ContextWindow = p.ContextWindow(m.ID)
			entry.SupportsTools = p.SupportsTools(m.ID)
		}
		out = append(out, provider.Model{
			ID:            m.ID,
			DisplayName:   m.ID,
			ContextWindow: entry.ContextWindow,
			SupportsTools: entry.SupportsTools,
			InputPer1M:    entry.Input,
			OutputPer1M:   entry.Output,
		})
	}
	return prioritizeDefaultModel(out), nil
}

// UsesResponsesAPI reports which endpoint a model will be sent to. Exported so
// /model and doctor can tell a user why one model behaves differently.
func (p *Provider) UsesResponsesAPI(modelID string) bool {
	return requiresResponsesAPI(modelID)
}

func (p *Provider) ChatCompletion(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	// Routed per request, not per provider: /model switches models mid-session
	// and the endpoint has to follow the model rather than whatever was
	// configured at startup.
	if requiresResponsesAPI(req.Model) {
		return p.responses.ChatCompletion(ctx, req)
	}
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
	// Unknown model: assume yes for chat models, no for non-chat.
	// A live chat completion will surface any mismatch as an API error.
	return isChatModel(modelID)
}

// isChatModel returns true unless the ID matches a known non-chat
// pattern. Intentionally permissive: drop the embeddings / audio /
// image / legacy-completion families, let everything else through.
func isChatModel(id string) bool {
	lower := strings.ToLower(id)
	for _, bad := range nonChatIndicators {
		if strings.Contains(lower, bad) {
			return false
		}
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
