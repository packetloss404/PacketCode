package theme

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/lipgloss"
)

// Theme mirrors the five design-system groups the loader understands.
// Every field is optional; absent fields keep their built-in default.
type Theme struct {
	Base     BaseSection       `toml:"base"`
	Text     TextSection       `toml:"text"`
	Accent   AccentSection     `toml:"accent"`
	Semantic SemanticSection   `toml:"semantic"`
	Provider map[string]string `toml:"provider"`
}

// BaseSection holds overrides for the five base surface tokens.
type BaseSection struct {
	Bg            string `toml:"bg"`
	Surface       string `toml:"surface"`
	SurfaceBright string `toml:"surface_bright"`
	Border        string `toml:"border"`
	BorderBright  string `toml:"border_bright"`
}

// TextSection holds overrides for the four text tokens.
type TextSection struct {
	Primary   string `toml:"primary"`
	Secondary string `toml:"secondary"`
	Dim       string `toml:"dim"`
	Inverse   string `toml:"inverse"`
}

// AccentSection holds overrides for the three accent tokens.
type AccentSection struct {
	Primary    string `toml:"primary"`
	PrimaryDim string `toml:"primary_dim"`
	Secondary  string `toml:"secondary"`
}

// SemanticSection holds overrides for the four semantic tokens.
type SemanticSection struct {
	Success string `toml:"success"`
	Warning string `toml:"warning"`
	Error   string `toml:"error"`
	Info    string `toml:"info"`
}

// validHex matches `#RRGGBB` and short-form `#RGB`.
var validHex = regexp.MustCompile(`^#([0-9A-Fa-f]{3}|[0-9A-Fa-f]{6})$`)

// Load reads an optional theme file. Returns (nil, nil) when the file
// is absent (the common case — users opt into themes), (nil, err) on
// parse failure with the path in the error for context, else
// (*Theme, nil). Unknown fields are silently ignored for forward-compat.
func Load(path string) (*Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read theme %s: %w", path, err)
	}
	var t Theme
	if err := toml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse theme %s: %w", path, err)
	}
	return &t, nil
}

// Apply mutates the package-level colour vars for every non-empty
// field, merges the provider map, and rebuilds every pre-built Style*
// value so they pick up the new palette. Apply(nil) is a no-op.
//
// Invalid hex per field logs `packetcode: theme: invalid hex for
// <field>: <value> (keeping default)` to stderr and leaves the
// corresponding var untouched — other fields still apply.
//
// Apply is additive, not replacing: calling Apply(&Theme{}) after a
// previous Apply does NOT reset the palette to the built-in defaults.
// Callers that want the defaults back should restart the process.
func Apply(t *Theme) {
	if t == nil {
		return
	}

	// Base.
	if v, ok := parseHex("base.bg", t.Base.Bg); ok {
		BaseBg = lipgloss.Color(v)
	}
	if v, ok := parseHex("base.surface", t.Base.Surface); ok {
		BaseSurface = lipgloss.Color(v)
	}
	if v, ok := parseHex("base.surface_bright", t.Base.SurfaceBright); ok {
		BaseSurfaceBright = lipgloss.Color(v)
	}
	if v, ok := parseHex("base.border", t.Base.Border); ok {
		BaseBorder = lipgloss.Color(v)
	}
	if v, ok := parseHex("base.border_bright", t.Base.BorderBright); ok {
		BaseBorderBright = lipgloss.Color(v)
	}

	// Text.
	if v, ok := parseHex("text.primary", t.Text.Primary); ok {
		TextPrimary = lipgloss.Color(v)
	}
	if v, ok := parseHex("text.secondary", t.Text.Secondary); ok {
		TextSecondary = lipgloss.Color(v)
	}
	if v, ok := parseHex("text.dim", t.Text.Dim); ok {
		TextDim = lipgloss.Color(v)
	}
	if v, ok := parseHex("text.inverse", t.Text.Inverse); ok {
		TextInverse = lipgloss.Color(v)
	}

	// Accent.
	if v, ok := parseHex("accent.primary", t.Accent.Primary); ok {
		AccentPrimary = lipgloss.Color(v)
	}
	if v, ok := parseHex("accent.primary_dim", t.Accent.PrimaryDim); ok {
		AccentPrimaryDim = lipgloss.Color(v)
	}
	if v, ok := parseHex("accent.secondary", t.Accent.Secondary); ok {
		AccentSecondary = lipgloss.Color(v)
	}

	// Semantic.
	if v, ok := parseHex("semantic.success", t.Semantic.Success); ok {
		Success = lipgloss.Color(v)
	}
	if v, ok := parseHex("semantic.warning", t.Semantic.Warning); ok {
		Warning = lipgloss.Color(v)
	}
	if v, ok := parseHex("semantic.error", t.Semantic.Error); ok {
		Error = lipgloss.Color(v)
	}
	if v, ok := parseHex("semantic.info", t.Semantic.Info); ok {
		Info = lipgloss.Color(v)
	}

	// Provider. Unknown slugs are welcome; unmentioned slugs preserved.
	for slug, raw := range t.Provider {
		if v, ok := parseHex("provider."+slug, raw); ok {
			providerColors[slug] = lipgloss.Color(v)
		}
	}

	rebuildStyles()
}

// parseHex validates and normalises a hex colour. Returns the
// normalised `#RRGGBB` form and true on success, empty string and
// false on either an empty input (skip) or malformed input (warn).
// Malformed inputs emit a stderr warning so the user sees which field
// fell through to default.
func parseHex(field, value string) (string, bool) {
	if value == "" {
		return "", false
	}
	if !validHex.MatchString(value) {
		fmt.Fprintf(os.Stderr, "packetcode: theme: invalid hex for %s: %s (keeping default)\n", field, value)
		return "", false
	}
	// Expand short form `#RGB` → `#RRGGBB` by doubling each nybble.
	if len(value) == 4 {
		var b strings.Builder
		b.WriteByte('#')
		for i := 1; i < 4; i++ {
			b.WriteByte(value[i])
			b.WriteByte(value[i])
		}
		return b.String(), true
	}
	return value, true
}

// rebuildStyles reassigns every pre-built Style* to reflect the
// current colour-token values. Called at the end of Apply so styles
// pick up the new palette. Kept byte-identical to the declarations in
// theme.go so default-palette tests remain stable.
func rebuildStyles() {
	StylePrimary = lipgloss.NewStyle().Foreground(TextPrimary)
	StyleSecondary = lipgloss.NewStyle().Foreground(TextSecondary)
	StyleDim = lipgloss.NewStyle().Foreground(TextDim)
	StyleAccent = lipgloss.NewStyle().Foreground(AccentPrimary).Bold(true)
	StyleAccentDim = lipgloss.NewStyle().Foreground(AccentPrimaryDim)

	StyleSuccess = lipgloss.NewStyle().Foreground(Success)
	StyleWarning = lipgloss.NewStyle().Foreground(Warning)
	StyleError = lipgloss.NewStyle().Foreground(Error)
	StyleInfo = lipgloss.NewStyle().Foreground(Info)

	StyleTopBar = lipgloss.NewStyle().
		Background(BaseSurface).
		Foreground(TextPrimary).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BaseBorder)

	StyleUserMessage = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(AccentPrimary).
		Padding(0, 1)

	StyleAgentMessage = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BaseBorder).
		Padding(0, 1)

	StyleSystemMessage = lipgloss.NewStyle().
		Foreground(TextSecondary).
		Italic(true).
		Padding(0, 2)

	StyleApprovalPrompt = lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(Warning).
		Padding(0, 1)

	StyleToolCall = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(BaseBorder).
		Background(BaseSurface).
		Padding(0, 1)

	StyleInputIdle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BaseBorder).
		Padding(0, 1)

	StyleInputFocused = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(AccentPrimary).
		Padding(0, 1)

	StyleDiffAdded = lipgloss.NewStyle().Foreground(Success)
	StyleDiffRemoved = lipgloss.NewStyle().Foreground(Error)
	StyleDiffHunk = lipgloss.NewStyle().Foreground(TextSecondary)
}
