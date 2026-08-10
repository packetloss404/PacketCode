// Package sugar implements the built-in Sugar provider. Sugar exposes a
// private OpenAI-compatible API whose live model catalog includes Conduit, the
// task-aware router, plus every directly selectable model in the current
// stable.
package sugar

import (
	"context"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/openaicompat"
)

const (
	Slug           = "sugar"
	DisplayName    = "Sugar"
	DefaultBaseURL = "http://localhost:3211/api/v1"
	DefaultModel   = "sugar/conduit"
)

var brandColor = lipgloss.Color("#A8D34F")

type Provider struct {
	client *openaicompat.Client
}

func New(apiKey string) *Provider {
	return NewWithBaseURL(DefaultBaseURL, apiKey)
}

func NewWithBaseURL(baseURL, apiKey string) *Provider {
	return &Provider{client: openaicompat.NewClient(NormalizeBaseURL(baseURL), apiKey)}
}

func NormalizeBaseURL(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base == "" {
		return DefaultBaseURL
	}
	if strings.HasSuffix(base, "/api/v1") {
		return base
	}
	if strings.HasSuffix(base, "/v1") {
		base = strings.TrimSuffix(base, "/v1")
	}
	return base + "/api/v1"
}

func (p *Provider) Name() string               { return DisplayName }
func (p *Provider) Slug() string               { return Slug }
func (p *Provider) BrandColor() lipgloss.Color { return brandColor }

func (p *Provider) ValidateKey(ctx context.Context, apiKey string) error {
	return p.client.ValidateKey(ctx, apiKey)
}

func (p *Provider) ListModels(ctx context.Context) ([]provider.Model, error) {
	models, err := p.client.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	for i := range models {
		models[i] = enrich(models[i])
	}
	return prioritizeConduit(models), nil
}

func (p *Provider) ChatCompletion(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	return p.client.ChatCompletion(ctx, req)
}

// Sugar meters upstream usage at the service. Packetcode reports zero local
// API cost so it never presents a second, misleading bill.
func (p *Provider) Pricing(string) (float64, float64) { return 0, 0 }

func (p *Provider) ContextWindow(modelID string) int {
	switch modelID {
	case "sugar/kimi":
		return 262_144
	case "sugar/conduit", "sugar/glm", "sugar/deepseek", "sugar/minimax":
		return 200_000
	default:
		return 128_000
	}
}

func (p *Provider) SupportsTools(string) bool { return true }

func enrich(model provider.Model) provider.Model {
	switch model.ID {
	case "sugar/conduit":
		model.DisplayName = "Conduit (automatic)"
	case "sugar/kimi":
		model.DisplayName = "Kimi"
	case "sugar/glm":
		model.DisplayName = "GLM"
	case "sugar/deepseek":
		model.DisplayName = "DeepSeek"
	case "sugar/minimax":
		model.DisplayName = "MiniMax"
	}
	model.ContextWindow = (&Provider{}).ContextWindow(model.ID)
	model.SupportsTools = true
	model.InputPer1M = 0
	model.OutputPer1M = 0
	return model
}

func prioritizeConduit(models []provider.Model) []provider.Model {
	for i, model := range models {
		if model.ID != DefaultModel || i == 0 {
			continue
		}
		out := make([]provider.Model, 0, len(models))
		out = append(out, model)
		out = append(out, models[:i]...)
		out = append(out, models[i+1:]...)
		return out
	}
	return models
}
