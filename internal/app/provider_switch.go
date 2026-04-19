package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/packetcode/packetcode/internal/provider"
)

// applyProviderSwitch mirrors the side-effect chain of `/provider <slug>`:
// resolve a model (config default → cached ListModels → fresh ListModels[0]
// → error), call Registry.SetActive, refresh the top bar, and append a
// "switched provider" system message. The returned error carries no
// prefix — callers (slash handler, picker) decide whether to annotate
// with "provider: " before surfacing to the user.
func (a *App) applyProviderSwitch(slug string) error {
	prov, ok := a.deps.Registry.Get(slug)
	if !ok {
		return fmt.Errorf("unknown provider %q", slug)
	}
	modelID := ""
	if a.deps.Config != nil {
		if pc, ok := a.deps.Config.Providers[slug]; ok {
			modelID = pc.DefaultModel
		}
	}
	if modelID == "" {
		if cached, ok := a.deps.Registry.CachedModels(slug); ok && len(cached) > 0 {
			modelID = cached[0].ID
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			models, err := prov.ListModels(ctx)
			cancel()
			if err == nil && len(models) > 0 {
				a.deps.Registry.SetCachedModels(slug, models)
				modelID = models[0].ID
			}
		}
	}
	if modelID == "" {
		return fmt.Errorf("%s has no default model; run /model <id> after switching", slug)
	}
	if err := a.deps.Registry.SetActive(slug, modelID); err != nil {
		return err
	}
	a.refreshTopBar()
	a.conversation.AppendSystem(fmt.Sprintf("switched provider: %s (%s)", slug, modelID))
	return nil
}

// applyModelSwitch validates modelID against the provider's catalog
// (so the user can't set a phantom ID that silently fails every turn)
// and, if valid, sets it active. Validation is skipped only when the
// catalog can't be fetched — we don't want a transient network blip
// to prevent the user from changing models.
func (a *App) applyModelSwitch(modelID string) error {
	prov, _ := a.deps.Registry.Active()
	if prov == nil {
		return fmt.Errorf("no active provider")
	}
	if err := a.validateModelID(prov, modelID); err != nil {
		return err
	}
	if err := a.deps.Registry.SetActive(prov.Slug(), modelID); err != nil {
		return err
	}
	a.refreshTopBar()
	a.conversation.AppendSystem(fmt.Sprintf("switched model: %s/%s", prov.Slug(), modelID))
	return nil
}

// validateModelID returns nil if modelID appears in the provider's
// model catalog, a descriptive error with near-match suggestions if
// not. If the catalog can't be reached (offline, rate-limited, etc.)
// we return nil so the user isn't stuck behind a transient failure —
// the runtime chat completion will surface a real API error then.
func (a *App) validateModelID(prov provider.Provider, modelID string) error {
	var models []provider.Model
	if cached, ok := a.deps.Registry.CachedModels(prov.Slug()); ok && len(cached) > 0 {
		models = cached
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		fetched, err := prov.ListModels(ctx)
		cancel()
		if err != nil {
			return nil // trust the user; runtime will surface API errors
		}
		a.deps.Registry.SetCachedModels(prov.Slug(), fetched)
		models = fetched
	}
	for _, m := range models {
		if m.ID == modelID {
			return nil
		}
	}
	suggestions := findNearMatches(modelID, models, 3)
	if len(suggestions) > 0 {
		return fmt.Errorf("unknown model %q for %s; did you mean: %s?",
			modelID, prov.Slug(), strings.Join(suggestions, ", "))
	}
	return fmt.Errorf("unknown model %q for %s; run /model to list available",
		modelID, prov.Slug())
}

// findNearMatches returns up to n catalog IDs whose string contains the
// query (or vice versa) — cheap typo hints for validateModelID.
func findNearMatches(query string, models []provider.Model, n int) []string {
	q := strings.ToLower(query)
	out := make([]string, 0, n)
	for _, m := range models {
		lower := strings.ToLower(m.ID)
		if strings.Contains(lower, q) || (len(q) > 4 && strings.Contains(q, lower)) {
			out = append(out, m.ID)
			if len(out) >= n {
				break
			}
		}
	}
	return out
}
