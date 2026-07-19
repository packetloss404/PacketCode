# Theming

packetcode uses semantic Lip Gloss tokens with optional TOML overrides from `~/.packetcode/theme.toml`. Missing files are ignored; malformed values warn and fall back without preventing startup.

## Example

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

Three- and six-digit hex values are accepted. Unknown fields are ignored for forward compatibility. Partial themes merge over built-in Terminal Noir values; applying an empty theme does not reset earlier overrides.

Styles are rebuilt after loading so components see the new semantic tokens. Provider colors may include custom provider slugs; unknown providers fall back to primary text.

The current Claude Code-inspired layout intentionally uses understated horizontal input rules, flat status lines, semantic tool/error colors, and provider accents rather than rounded colored boxes.

Presets live under `docs/themes/`:

```bash
cp docs/themes/high-contrast.toml ~/.packetcode/theme.toml
```

Hot reload, inheritance, and an interactive `/theme` command are not implemented.
