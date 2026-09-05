// Package codex implements the provider.Provider interface for OpenAI Codex
// accessed through a ChatGPT subscription ("Sign in with ChatGPT").
//
// Unlike the openai provider, codex does not use an API key. It reuses the
// OAuth credentials the official Codex CLI writes to ~/.codex/auth.json
// (see internal/provider/codexauth) and talks to the ChatGPT backend's
// Responses API (see internal/provider/responses). Because the subscription
// bills a flat rate rather than per token, pricing is reported as $0.
package codex

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"

	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/codexauth"
	"github.com/packetcode/packetcode/internal/provider/responses"
)

const (
	// defaultBaseURL is the ChatGPT backend's Codex Responses endpoint host.
	// The responses client appends "/responses".
	defaultBaseURL = "https://chatgpt.com/backend-api/codex"

	// DefaultModel is the model selected when none is configured. It matches
	// the Codex CLI's flagship default; if the account cannot use it, the live
	// model cache surfaces the alternatives in the picker.
	DefaultModel = "gpt-5.6-sol"

	// defaultContextWindow is the fallback gauge size for a model with no
	// cached context window.
	defaultContextWindow = 272_000

	// defaultEffort is the reasoning effort used when a model's default is
	// unknown. Every Codex model supports "medium".
	defaultEffort = "medium"

	slug        = "codex"
	displayName = "Codex (ChatGPT)"
)

// brandColor is the ChatGPT green, distinct enough from the openai provider
// dot to be recognizable in the selector.
var brandColor = lipgloss.Color("#19C37D")

// Provider implements provider.Provider for Codex subscription auth.
type Provider struct {
	store          *codexauth.Store
	client         *responses.Client
	authPath       string
	mu             sync.RWMutex
	catalog        []cachedModel
	effortOverride string
}

// New constructs a Codex provider. authPath is the path to the Codex auth.json
// file; when empty, the conventional location ($CODEX_HOME/auth.json or
// ~/.codex/auth.json) is used. A resolution error is deferred to ValidateKey so
// construction never fails and the provider can still register.
func New(authPath string) *Provider {
	if authPath == "" {
		if p, err := codexauth.DefaultPath(); err == nil {
			authPath = p
		}
	}
	store := codexauth.New(authPath)
	p := &Provider{
		store:    store,
		client:   responses.NewClient(defaultBaseURL, authAdapter{store: store}),
		authPath: authPath,
		catalog:  loadCatalog(authPath),
	}
	p.client.EffortFor = p.effortFor
	p.client.SummaryFor = p.summaryFor
	return p
}

// NewWithBaseURL is exposed for testing against an httptest server.
func NewWithBaseURL(authPath, baseURL string) *Provider {
	p := New(authPath)
	p.client = responses.NewClient(baseURL, authAdapter{store: p.store})
	p.client.EffortFor = p.effortFor
	p.client.SummaryFor = p.summaryFor
	return p
}

// effortFor returns the reasoning effort to send for a model, using the Codex
// CLI's per-model default from the cache when known.
func (p *Provider) effortFor(model string) string {
	return p.ReasoningEffort(model)
}

// ReasoningEffort returns the effective level for model: a supported user
// override when set, otherwise the model's cache-advertised default.
func (p *Provider) ReasoningEffort(model string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, m := range p.catalog {
		if m.Slug != model {
			continue
		}
		if p.effortOverride != "" && supportsReasoningEffort(m, p.effortOverride) {
			return p.effortOverride
		}
		if m.DefaultEffort != "" {
			return m.DefaultEffort
		}
	}
	if p.effortOverride != "" {
		return p.effortOverride
	}
	return defaultEffort
}

func (p *Provider) ReasoningEfforts(model string) []provider.ReasoningEffort {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, m := range p.catalog {
		if m.Slug == model {
			return append([]provider.ReasoningEffort(nil), m.SupportedEfforts...)
		}
	}
	return nil
}

// SetReasoningEffort sets the provider-wide override used for the active
// model. "default", "auto", or an empty value clears it.
func (p *Provider) SetReasoningEffort(model, effort string) error {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" || effort == "default" || effort == "auto" {
		p.mu.Lock()
		p.effortOverride = ""
		p.mu.Unlock()
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	for _, m := range p.catalog {
		if m.Slug != model {
			continue
		}
		if supportsReasoningEffort(m, effort) {
			p.effortOverride = effort
			return nil
		}
		return fmt.Errorf("reasoning effort %q is not supported by %s", effort, model)
	}
	return fmt.Errorf("cannot set reasoning effort for unknown model %q", model)
}

func supportsReasoningEffort(model cachedModel, effort string) bool {
	for _, option := range model.SupportedEfforts {
		if option.ID == effort {
			return true
		}
	}
	return false
}

// summaryFor returns the reasoning.summary value to request for a model:
// "auto" when the model accepts the parameter, "" to omit it when it doesn't
// (gpt-5.3-codex-spark rejects it with a 400). Requesting "auto" makes live
// "thinking" appear on models that stream summaries (gpt-5.4/5.5) and is
// harmlessly ignored by those that don't (the gpt-5.6 family). Unknown models
// default to "auto".
func (p *Provider) summaryFor(model string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, m := range p.catalog {
		if m.Slug == model {
			if m.SummarySupported {
				return "auto"
			}
			return ""
		}
	}
	return "auto"
}

func (p *Provider) Name() string               { return displayName }
func (p *Provider) Slug() string               { return slug }
func (p *Provider) BrandColor() lipgloss.Color { return brandColor }

// ValidateKey ignores the key argument (Codex is keyless) and instead confirms
// that a ChatGPT subscription login is present in auth.json. It does not force
// a token refresh: an expired access token is refreshed reactively on the first
// request, so startup does not fail just because the token has aged out.
func (p *Provider) ValidateKey(_ context.Context, _ string) error {
	if err := p.store.Available(); err != nil {
		return err
	}
	return nil
}

// ListModels returns the account's Codex models, read from the Codex CLI's
// live models_cache.json when available and a static fallback otherwise. It
// performs no network call — the ChatGPT backend exposes no model-listing
// endpoint for subscription auth. The cache is re-read each call so newly
// available models appear without restarting packetcode.
func (p *Provider) ListModels(_ context.Context) ([]provider.Model, error) {
	catalog := loadCatalog(p.authPath)
	p.mu.Lock()
	p.catalog = catalog
	p.mu.Unlock()
	return toProviderModels(catalog), nil
}

func (p *Provider) ChatCompletion(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	return p.client.ChatCompletion(ctx, req)
}

// Pricing reports $0 — a ChatGPT subscription bills a flat rate, not per token.
func (p *Provider) Pricing(string) (float64, float64) { return 0, 0 }

func (p *Provider) ContextWindow(modelID string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, m := range p.catalog {
		if m.Slug == modelID && m.Context > 0 {
			return m.Context
		}
	}
	return defaultContextWindow
}

func (p *Provider) SupportsTools(string) bool { return true }

// authAdapter bridges codexauth.Store to the responses.Auth interface,
// projecting the token struct down to the two strings the wire client needs.
type authAdapter struct {
	store *codexauth.Store
}

func (a authAdapter) Token(ctx context.Context) (string, string, error) {
	t, err := a.store.Token(ctx)
	if err != nil {
		return "", "", err
	}
	return t.AccessToken, t.AccountID, nil
}

func (a authAdapter) Refresh(ctx context.Context) (string, string, error) {
	t, err := a.store.Refresh(ctx)
	if err != nil {
		return "", "", fmt.Errorf("refresh codex token: %w", err)
	}
	return t.AccessToken, t.AccountID, nil
}
