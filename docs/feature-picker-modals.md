# Provider and Model Pickers

Provider and model selection use the shared `picker` component.

## Open and Use

- `Ctrl+P`, `/provider`, or `/providers`: provider picker.
- `/model` or `/models`: active-provider model picker. `Ctrl+M` also works when the terminal reports it distinctly instead of carriage return.
- Type to filter; matching is case-insensitive and normalized.
- Up/Down, Ctrl+N/P, and Ctrl+J/K move.
- PageUp/PageDown move by half a page.
- Enter selects; Esc closes; Ctrl+U clears the filter.
- In the provider picker, Ctrl+A opens API-key entry for the focused provider.

Selecting a provider persists its default model. Model discovery runs asynchronously and shows loading, empty, and error/retry states without blocking the Bubble Tea update loop. Providers with incomplete/unavailable model-list endpoints may return curated fallback models.

Keyless providers (`codex` and `ollama`) never open an API-key prompt. Custom OpenAI-compatible providers participate through the same registry.

## Focus Rules

The picker owns keyboard input while visible and sits above the transcript/input. Approval has higher urgency and cannot be stacked with a new picker; provider/model switching is rejected while a turn is active.

Implementation: `internal/ui/components/picker`, `internal/app/provider_switch.go`, and App picker handlers/tests.
