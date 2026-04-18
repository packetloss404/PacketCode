package app

import (
	"context"
	"fmt"
	"time"
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
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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

// applyModelSwitch sets the active model on the current provider and
// announces the change. Returns an error (no "model: " prefix) when no
// provider is active or Registry.SetActive refuses.
func (a *App) applyModelSwitch(modelID string) error {
	prov, _ := a.deps.Registry.Active()
	if prov == nil {
		return fmt.Errorf("no active provider")
	}
	if err := a.deps.Registry.SetActive(prov.Slug(), modelID); err != nil {
		return err
	}
	a.refreshTopBar()
	a.conversation.AppendSystem(fmt.Sprintf("switched model: %s/%s", prov.Slug(), modelID))
	return nil
}
