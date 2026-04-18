# User-Customisable Theme — Round 6 Design Spec

## Summary

Load an optional user theme at `~/.packetcode/theme.toml` at startup and override the Terminal Noir colour tokens in `internal/ui/theme/theme.go`. Every UI component already references tokens through the `theme` package, so the change is a parse-and-apply pass done once during `run()` before Bubble Tea starts. Ship a handful of example themes (baseline, light, high-contrast, solarized-dark) alongside the spec.

## User stories

1. **Palette override.** User drops `~/.packetcode/theme.toml` with `bg = "#FFFFFF"` / `text.primary = "#000000"` and every surface recolours without a rebuild.
2. **Partial override.** User only wants to change the accent; `[accent] primary = "#FF00FF"` and everything else keeps its default.
3. **Ship a preset.** Maintainer hands users `cp docs/themes/high-contrast.toml ~/.packetcode/theme.toml` and that's the whole install.

## Design

### Refactor approach — Option C (exported vars + `Apply`)

Keep exported `var` declarations in `theme.go` unchanged. Do NOT move to a getter — that would touch ~93 call sites across 11 component files for zero benefit. Add one function `theme.Apply(t *Theme)` that:

1. Mutates the package-level colour vars for every non-empty field.
2. Rebuilds every pre-built `Style*` value (20 in total) so they reflect the new colours.

Components continue reading `theme.AccentPrimary` / `theme.StyleAccent` at render time — no component code changes.

Parallel-test concern: theme loads happen once in `cmd/packetcode/main.go` before `tea.NewProgram`. Unit tests for `Apply` run in the `theme` package and snapshot+restore colour vars via `t.Cleanup`. No mutex needed.

### TOML schema

Flat tables mirroring design-system groups. Every field optional; absent fields keep defaults. Hex accepts `#RRGGBB` or `#RGB` (expanded).

```toml
[base]
bg              = "#0F0F0F"
surface         = "#1A1A2E"
surface_bright  = "#232340"
border          = "#2A2A3D"
border_bright   = "#3D3D5C"

[text]
primary   = "#E1E1E8"
secondary = "#8888A0"
dim       = "#4A4A60"
inverse   = "#0F0F0F"

[accent]
primary     = "#00D9FF"
primary_dim = "#0891B2"
secondary   = "#FF6B6B"

[semantic]
success = "#4ADE80"
warning = "#FBBF24"
error   = "#F87171"
info    = "#60A5FA"

[provider]
openai     = "#10A37F"
gemini     = "#4285F4"
minimax    = "#FF8C00"
openrouter = "#EC4899"
ollama     = "#E1E1E8"
```

Field-to-var mapping:

| TOML | Go var |
|---|---|
| `base.bg` | `BaseBg` |
| `base.surface` | `BaseSurface` |
| `base.surface_bright` | `BaseSurfaceBright` |
| `base.border` | `BaseBorder` |
| `base.border_bright` | `BaseBorderBright` |
| `text.primary` | `TextPrimary` |
| `text.secondary` | `TextSecondary` |
| `text.dim` | `TextDim` |
| `text.inverse` | `TextInverse` |
| `accent.primary` | `AccentPrimary` |
| `accent.primary_dim` | `AccentPrimaryDim` |
| `accent.secondary` | `AccentSecondary` |
| `semantic.success` | `Success` |
| `semantic.warning` | `Warning` |
| `semantic.error` | `Error` |
| `semantic.info` | `Info` |
| `provider.<slug>` | `providerColors[slug]` map entry |

### Loader API

`internal/ui/theme/loader.go` (new):

```go
type Theme struct {
    Base     BaseSection       `toml:"base"`
    Text     TextSection       `toml:"text"`
    Accent   AccentSection     `toml:"accent"`
    Semantic SemanticSection   `toml:"semantic"`
    Provider map[string]string `toml:"provider"`
}

type BaseSection struct {
    Bg, Surface, SurfaceBright, Border, BorderBright string
}
type TextSection struct {
    Primary, Secondary, Dim, Inverse string
}
type AccentSection struct {
    Primary, PrimaryDim, Secondary string
}
type SemanticSection struct {
    Success, Warning, Error, Info string
}

// Load returns (nil, nil) when the file is absent, (nil, err) on parse
// failure, or (*Theme, nil) on success. Lenient on unknown fields for
// forward-compat.
func Load(path string) (*Theme, error)

// Apply mutates the package-level colour vars and rebuilds all Style*.
// Apply(nil) is a no-op. Invalid hex per-field logs to stderr and keeps
// the default.
func Apply(t *Theme)
```

### Validation policy

| Situation | Behaviour |
|---|---|
| File absent | `Load` returns `(nil, nil)`; no log. |
| Syntax error | `Load` returns `(nil, err)`; main.go prints `packetcode: failed to load theme: <err>; falling back to defaults` and continues. |
| Unknown field | Silently ignored (lenient unmarshal). |
| Invalid hex per field | Stderr warning `packetcode: theme: invalid hex for <field>: <value> (keeping default)`; that field keeps default; other fields apply. |
| `#RGB` short form | Normalised to `#RRGGBB`. |

Regex: `^#([0-9A-Fa-f]{3}|[0-9A-Fa-f]{6})$`.

### Style rebuild

`Apply` ends with `rebuildStyles()` that reassigns every exported `Style*` (20 total):

```
StylePrimary, StyleSecondary, StyleDim, StyleAccent, StyleAccentDim,
StyleSuccess, StyleWarning, StyleError, StyleInfo,
StyleTopBar, StyleUserMessage, StyleAgentMessage, StyleSystemMessage,
StyleApprovalPrompt, StyleToolCall,
StyleInputIdle, StyleInputFocused,
StyleDiffAdded, StyleDiffRemoved, StyleDiffHunk.
```

Must match `theme.go` one-to-one. Test `TestApply_RebuildsAllTwentyStyles` guards against drift.

### `ProviderColor` refactor

Replace the `switch` with a package-level map:

```go
var providerColors = map[string]lipgloss.Color{
    "openai":     lipgloss.Color("#10A37F"),
    "gemini":     lipgloss.Color("#4285F4"),
    "minimax":    lipgloss.Color("#FF8C00"),
    "openrouter": lipgloss.Color("#EC4899"),
    "ollama":     lipgloss.Color("#E1E1E8"),
}

func ProviderColor(slug string) lipgloss.Color {
    if c, ok := providerColors[slug]; ok { return c }
    return TextPrimary
}
```

`Apply` merges `t.Provider` into `providerColors`. Unknown slug → `TextPrimary` fallback preserved.

### Startup wiring

`cmd/packetcode/main.go` `run()` after `config.Load` success, before any provider/factory/app wiring:

```go
themePath, err := config.ThemePath()
if err == nil {
    if t, err := theme.Load(themePath); err != nil {
        fmt.Fprintf(os.Stderr, "packetcode: failed to load theme: %v; falling back to defaults\n", err)
    } else {
        theme.Apply(t) // nil-safe
    }
}
```

### `config.ThemePath()`

Add to `internal/config/paths.go` after `CostTallyPath`:

```go
func ThemePath() (string, error) {
    dir, err := HomeDir()
    if err != nil { return "", err }
    return filepath.Join(dir, "theme.toml"), nil
}
```

## File-by-file change list

### Bucket A — Implementation

| Path | Change |
|---|---|
| `internal/ui/theme/theme.go` | Replace `ProviderColor` switch with `providerColors` map + lookup. No other changes. |
| `internal/ui/theme/loader.go` | **NEW.** `Theme` + sections + `Load` + `Apply` + `parseHex` + `rebuildStyles`. |
| `internal/config/paths.go` | Add `ThemePath() (string, error)` after `CostTallyPath`. |
| `cmd/packetcode/main.go` | Import `theme`. After `config.Load`, resolve `config.ThemePath()`, call `theme.Load` and `theme.Apply` (nil-safe). |

### Bucket B — Tests

| Path | Change |
|---|---|
| `internal/ui/theme/loader_test.go` | **NEW.** ~13 tests (listed below). |
| `internal/config/paths_test.go` | Add `TestThemePath_UnderHomeDir` (create file if absent). |

### Bucket C — Docs + examples + commit

| Path | Change |
|---|---|
| `docs/feature-theming.md` | This spec. |
| `docs/themes/dark-terminal-noir.toml` | **NEW.** Baseline values with header comment (doubles as schema doc). |
| `docs/themes/light.toml` | **NEW.** Light-terminal variant. |
| `docs/themes/high-contrast.toml` | **NEW.** Accessibility variant. |
| `docs/themes/solarized-dark.toml` | **NEW.** Solarized-dark mapped onto the tokens. |
| `README.md` | Append "Custom themes" section with `cp docs/themes/...` example. |
| `CHANGELOG.md` | Added bullet for user theme; remove from Deferred. |
| `docs/roadmap-deferred.md` | Mark Round 6 **Landed**. |

### Example theme bodies

Each example file includes a header comment then the section bodies.

**`docs/themes/light.toml`**:

```toml
# Light theme for terminals with a light background.
[base]
bg              = "#FFFFFF"
surface         = "#F4F4F7"
surface_bright  = "#E8E8EE"
border          = "#D0D0DA"
border_bright   = "#A8A8B8"

[text]
primary   = "#1A1A2E"
secondary = "#5A5A70"
dim       = "#9A9AB0"
inverse   = "#FFFFFF"

[accent]
primary     = "#0891B2"
primary_dim = "#0E7490"
secondary   = "#DC2626"

[semantic]
success = "#15803D"
warning = "#B45309"
error   = "#B91C1C"
info    = "#1D4ED8"
```

**`docs/themes/high-contrast.toml`**:

```toml
# High-contrast theme. Pure black/white surfaces and saturated accents.
[base]
bg              = "#000000"
surface         = "#000000"
surface_bright  = "#1A1A1A"
border          = "#FFFFFF"
border_bright   = "#FFFFFF"

[text]
primary   = "#FFFFFF"
secondary = "#FFFF00"
dim       = "#C0C0C0"
inverse   = "#000000"

[accent]
primary     = "#FFFF00"
primary_dim = "#CCCC00"
secondary   = "#FF00FF"

[semantic]
success = "#00FF00"
warning = "#FFFF00"
error   = "#FF0000"
info    = "#00FFFF"
```

**`docs/themes/solarized-dark.toml`**:

```toml
# Solarized Dark (Ethan Schoonover) mapped onto packetcode tokens.
[base]
bg              = "#002B36"
surface         = "#073642"
surface_bright  = "#0A4554"
border          = "#586E75"
border_bright   = "#657B83"

[text]
primary   = "#EEE8D5"
secondary = "#93A1A1"
dim       = "#586E75"
inverse   = "#002B36"

[accent]
primary     = "#268BD2"
primary_dim = "#2AA198"
secondary   = "#DC322F"

[semantic]
success = "#859900"
warning = "#B58900"
error   = "#DC322F"
info    = "#268BD2"
```

## Tests

All in `internal/ui/theme/loader_test.go` unless noted. Use `snapshotTheme(t)` helper with `t.Cleanup` to restore package state between tests.

- `TestLoad_MissingFile_ReturnsNilNil` — `(nil, nil)` for absent path.
- `TestLoad_ValidTOML_ParsesAllSections` — full round-trip.
- `TestLoad_SyntaxError_ReturnsError` — malformed TOML → error naming path.
- `TestLoad_UnknownField_Ignored` — `[future]` section accepted; known fields still apply.
- `TestApply_Nil_IsNoOp` — `Apply(nil)` leaves vars unchanged.
- `TestApply_MutatesVars` — `AccentPrimary` reassigned after `Apply`.
- `TestApply_RebuildsStyleAccent` — `StyleAccent.Render("x")` reflects new colour.
- `TestApply_RebuildsAllTwentyStyles` — guards against rebuild-list drift.
- `TestApply_ShortHexExpanded` — `#ABC` → `#AABBCC`.
- `TestApply_InvalidHex_KeepsDefaultAndWarns` — stderr pipe captures the warning line; var unchanged.
- `TestApply_ProviderMapMerged` — overrides known slugs + adds custom slugs; unmentioned slugs preserved.
- `TestApply_PartialOverride_LeavesOthersIntact` — only `Semantic.Error` set.
- `TestApply_IsAdditiveNotReplacing` — `Apply` with empty theme after custom does NOT reset (documented contract).
- `TestProviderColor_UnknownSlug_ReturnsTextPrimary` — guards the map refactor.
- `internal/config/paths_test.go :: TestThemePath_UnderHomeDir` — env isolation + path shape.

## Out of scope (Round 7+)

- Hot-reload on file change.
- `/theme <name>` slash command.
- Contrast / accessibility linter.
- Theme inheritance (`extends = "dark-terminal-noir"`).
- JSON/YAML theme formats.
- Env-var overrides per token.
- Runtime terminal-depth detection (lipgloss handles it).
