---
description: Change packetcode's TUI colors via ~/.packetcode/theme.toml - the semantic token tables, partial-override merge behavior, hex formats, and the shipped presets. Use for "change the theme" or "these colors are unreadable".
---

# Theming packetcode

packetcode styles the TUI with semantic Lip Gloss tokens. Optional overrides
come from `~/.packetcode/theme.toml`. There is no project-scoped theme.

## The tables

```toml
[base]
background = "#101014"
surface = "#18181F"
border = "#3A3A46"
border_bright = "#666678"

[text]
primary = "#F2F2F4"
secondary = "#B7B7C2"
dim = "#747482"

[accent]
primary = "#65D1FF"
secondary = "#D98BFF"

[semantic]
success = "#64D98B"
warning = "#F1C75B"
error = "#FF6B7A"
info = "#65D1FF"

[providers]
codex = "#10A37F"
anthropic = "#D97757"
ollama = "#FFFFFF"
```

Three- and six-digit hex are both accepted. Unknown fields are ignored for
forward compatibility, which also means a typo'd key silently does nothing -
check the token name against this list before concluding the theme is broken.

`[providers]` may name custom provider slugs; an unlisted provider falls back to
primary text.

## Merge behaviour, and the trap in it

A partial theme merges **over** the built-in Terminal Noir values, and styles
are rebuilt after loading so components pick up the new tokens.

The consequence people get wrong: **applying an empty theme does not reset
earlier overrides.** To go back to the defaults, remove the file (or the
specific keys), do not blank them out.

A missing theme file is ignored. A malformed one logs a single warning and
packetcode keeps the built-in theme rather than refusing to start.

## Presets

```bash
cp docs/themes/high-contrast.toml ~/.packetcode/theme.toml
```

Presets live under `docs/themes/`. Start from one rather than from an empty
file - a hand-written partial theme usually ends up with a readable foreground
on an unreadable background.

## What does not exist

Hot reload, theme inheritance, and an interactive `/theme` command are not
implemented. Editing the file takes effect on the next start. Do not promise a
live preview.

The current layout is deliberate: understated horizontal input rules, flat
status lines, semantic tool and error colors, provider accents - not rounded
colored boxes. Keep contrast changes inside that idiom.
