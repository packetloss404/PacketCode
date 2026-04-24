package theme

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snapshot captures every theme var — colours, the providerColors
// map, and every pre-built Style* — so a test can mutate them and
// have them restored via t.Cleanup. Each test calls snapshotTheme(t)
// at the top so the package-level state is hermetic between tests.
type themeSnapshot struct {
	baseBg, baseSurface, baseSurfaceBright, baseBorder, baseBorderBright lipgloss.Color
	textPrimary, textSecondary, textDim, textInverse                     lipgloss.Color
	accentPrimary, accentPrimaryDim, accentSecondary                     lipgloss.Color
	success, warning, errorColor, info                                   lipgloss.Color
	providerColors                                                       map[string]lipgloss.Color

	stylePrimary, styleSecondary, styleDim, styleAccent, styleAccentDim  lipgloss.Style
	styleSuccess, styleWarning, styleError, styleInfo                    lipgloss.Style
	styleTopBar, styleUserMessage, styleAgentMessage, styleSystemMessage lipgloss.Style
	styleApprovalPrompt, styleToolCall                                   lipgloss.Style
	styleInputIdle, styleInputFocused                                    lipgloss.Style
	styleDiffAdded, styleDiffRemoved, styleDiffHunk                      lipgloss.Style
}

func snapshotTheme(t *testing.T) {
	t.Helper()
	snap := themeSnapshot{
		baseBg:            BaseBg,
		baseSurface:       BaseSurface,
		baseSurfaceBright: BaseSurfaceBright,
		baseBorder:        BaseBorder,
		baseBorderBright:  BaseBorderBright,

		textPrimary:   TextPrimary,
		textSecondary: TextSecondary,
		textDim:       TextDim,
		textInverse:   TextInverse,

		accentPrimary:    AccentPrimary,
		accentPrimaryDim: AccentPrimaryDim,
		accentSecondary:  AccentSecondary,

		success:    Success,
		warning:    Warning,
		errorColor: Error,
		info:       Info,

		providerColors: copyProviderColors(),

		stylePrimary:   StylePrimary,
		styleSecondary: StyleSecondary,
		styleDim:       StyleDim,
		styleAccent:    StyleAccent,
		styleAccentDim: StyleAccentDim,

		styleSuccess: StyleSuccess,
		styleWarning: StyleWarning,
		styleError:   StyleError,
		styleInfo:    StyleInfo,

		styleTopBar:         StyleTopBar,
		styleUserMessage:    StyleUserMessage,
		styleAgentMessage:   StyleAgentMessage,
		styleSystemMessage:  StyleSystemMessage,
		styleApprovalPrompt: StyleApprovalPrompt,
		styleToolCall:       StyleToolCall,

		styleInputIdle:    StyleInputIdle,
		styleInputFocused: StyleInputFocused,

		styleDiffAdded:   StyleDiffAdded,
		styleDiffRemoved: StyleDiffRemoved,
		styleDiffHunk:    StyleDiffHunk,
	}
	t.Cleanup(func() {
		BaseBg = snap.baseBg
		BaseSurface = snap.baseSurface
		BaseSurfaceBright = snap.baseSurfaceBright
		BaseBorder = snap.baseBorder
		BaseBorderBright = snap.baseBorderBright

		TextPrimary = snap.textPrimary
		TextSecondary = snap.textSecondary
		TextDim = snap.textDim
		TextInverse = snap.textInverse

		AccentPrimary = snap.accentPrimary
		AccentPrimaryDim = snap.accentPrimaryDim
		AccentSecondary = snap.accentSecondary

		Success = snap.success
		Warning = snap.warning
		Error = snap.errorColor
		Info = snap.info

		providerColors = snap.providerColors

		StylePrimary = snap.stylePrimary
		StyleSecondary = snap.styleSecondary
		StyleDim = snap.styleDim
		StyleAccent = snap.styleAccent
		StyleAccentDim = snap.styleAccentDim

		StyleSuccess = snap.styleSuccess
		StyleWarning = snap.styleWarning
		StyleError = snap.styleError
		StyleInfo = snap.styleInfo

		StyleTopBar = snap.styleTopBar
		StyleUserMessage = snap.styleUserMessage
		StyleAgentMessage = snap.styleAgentMessage
		StyleSystemMessage = snap.styleSystemMessage
		StyleApprovalPrompt = snap.styleApprovalPrompt
		StyleToolCall = snap.styleToolCall

		StyleInputIdle = snap.styleInputIdle
		StyleInputFocused = snap.styleInputFocused

		StyleDiffAdded = snap.styleDiffAdded
		StyleDiffRemoved = snap.styleDiffRemoved
		StyleDiffHunk = snap.styleDiffHunk
	})
}

func copyProviderColors() map[string]lipgloss.Color {
	out := make(map[string]lipgloss.Color, len(providerColors))
	for k, v := range providerColors {
		out[k] = v
	}
	return out
}

// captureStderr swaps os.Stderr for a pipe, runs fn, restores via
// t.Cleanup, and returns whatever fn wrote to stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = orig })

	fn()

	require.NoError(t, w.Close())
	data, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(data)
}

// writeTheme writes body to a temp theme.toml and returns its path.
func writeTheme(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.toml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// ────────────────────────────────────────────────────────────────────
// Load
// ────────────────────────────────────────────────────────────────────

func TestLoad_MissingFile_ReturnsNilNil(t *testing.T) {
	snapshotTheme(t)

	path := filepath.Join(t.TempDir(), "does-not-exist.toml")
	got, err := Load(path)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestLoad_ValidTOML_ParsesAllSections(t *testing.T) {
	snapshotTheme(t)

	body := `
[base]
bg              = "#101010"
surface         = "#202020"
surface_bright  = "#303030"
border          = "#404040"
border_bright   = "#505050"

[text]
primary   = "#606060"
secondary = "#707070"
dim       = "#808080"
inverse   = "#909090"

[accent]
primary     = "#A0A0A0"
primary_dim = "#B0B0B0"
secondary   = "#C0C0C0"

[semantic]
success = "#D0D0D0"
warning = "#E0E0E0"
error   = "#F0F0F0"
info    = "#111111"

[provider]
openai = "#222222"
custom = "#333333"
`
	path := writeTheme(t, body)
	got, err := Load(path)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "#101010", got.Base.Bg)
	assert.Equal(t, "#202020", got.Base.Surface)
	assert.Equal(t, "#303030", got.Base.SurfaceBright)
	assert.Equal(t, "#404040", got.Base.Border)
	assert.Equal(t, "#505050", got.Base.BorderBright)

	assert.Equal(t, "#606060", got.Text.Primary)
	assert.Equal(t, "#707070", got.Text.Secondary)
	assert.Equal(t, "#808080", got.Text.Dim)
	assert.Equal(t, "#909090", got.Text.Inverse)

	assert.Equal(t, "#A0A0A0", got.Accent.Primary)
	assert.Equal(t, "#B0B0B0", got.Accent.PrimaryDim)
	assert.Equal(t, "#C0C0C0", got.Accent.Secondary)

	assert.Equal(t, "#D0D0D0", got.Semantic.Success)
	assert.Equal(t, "#E0E0E0", got.Semantic.Warning)
	assert.Equal(t, "#F0F0F0", got.Semantic.Error)
	assert.Equal(t, "#111111", got.Semantic.Info)

	assert.Equal(t, "#222222", got.Provider["openai"])
	assert.Equal(t, "#333333", got.Provider["custom"])
}

func TestLoad_SyntaxError_ReturnsError(t *testing.T) {
	snapshotTheme(t)

	body := `[base
bg = "oops"
`
	path := writeTheme(t, body)
	got, err := Load(path)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), path, "error should mention the path for context")
}

func TestLoad_UnknownField_Ignored(t *testing.T) {
	snapshotTheme(t)

	body := `
[future]
hyperspace = "#ABCDEF"

[base]
bg = "#111111"

[base.nested]
also_unknown = "#222222"
`
	path := writeTheme(t, body)
	got, err := Load(path)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "#111111", got.Base.Bg, "known fields still apply when unknown fields are present")
}

// ────────────────────────────────────────────────────────────────────
// Apply
// ────────────────────────────────────────────────────────────────────

func TestApply_Nil_IsNoOp(t *testing.T) {
	snapshotTheme(t)

	beforeAccent := AccentPrimary
	beforeStyle := StyleAccent

	Apply(nil)

	assert.Equal(t, beforeAccent, AccentPrimary)
	// Style value identity is preserved — no rebuild ran.
	assert.Equal(t, beforeStyle, StyleAccent)
}

func TestApply_MutatesVars(t *testing.T) {
	snapshotTheme(t)

	Apply(&Theme{Accent: AccentSection{Primary: "#123456"}})
	assert.Equal(t, lipgloss.Color("#123456"), AccentPrimary)
}

func TestApply_RebuildsStyleAccent(t *testing.T) {
	snapshotTheme(t)

	Apply(&Theme{Accent: AccentSection{Primary: "#FF00AA"}})

	rendered := StyleAccent.Render("x")
	// Render output is terminal-specific; assert the pre-built style
	// now carries the new foreground colour.
	assert.Equal(t, lipgloss.Color("#FF00AA"), AccentPrimary)
	assert.NotEmpty(t, rendered)
	// The style's foreground should match AccentPrimary. lipgloss
	// returns the stored Color via GetForeground.
	fg := StyleAccent.GetForeground()
	assert.Equal(t, lipgloss.Color("#FF00AA"), fg)
}

func TestApply_RebuildsAllTwentyStyles(t *testing.T) {
	snapshotTheme(t)

	// Capture every pre-built Style by identity (value-equal to the
	// stored snapshot). After an Apply with a non-empty theme, every
	// one of the 20 must be reassigned — the exact values differ
	// because at least one colour changed, but more importantly the
	// rebuild function must cover each style slot. We guard by
	// swapping every colour and then asserting none of the 20 styles
	// still matches its pre-Apply value.
	type named struct {
		name   string
		before lipgloss.Style
		after  func() lipgloss.Style
	}
	styles := []named{
		{"StylePrimary", StylePrimary, func() lipgloss.Style { return StylePrimary }},
		{"StyleSecondary", StyleSecondary, func() lipgloss.Style { return StyleSecondary }},
		{"StyleDim", StyleDim, func() lipgloss.Style { return StyleDim }},
		{"StyleAccent", StyleAccent, func() lipgloss.Style { return StyleAccent }},
		{"StyleAccentDim", StyleAccentDim, func() lipgloss.Style { return StyleAccentDim }},
		{"StyleSuccess", StyleSuccess, func() lipgloss.Style { return StyleSuccess }},
		{"StyleWarning", StyleWarning, func() lipgloss.Style { return StyleWarning }},
		{"StyleError", StyleError, func() lipgloss.Style { return StyleError }},
		{"StyleInfo", StyleInfo, func() lipgloss.Style { return StyleInfo }},
		{"StyleTopBar", StyleTopBar, func() lipgloss.Style { return StyleTopBar }},
		{"StyleUserMessage", StyleUserMessage, func() lipgloss.Style { return StyleUserMessage }},
		{"StyleAgentMessage", StyleAgentMessage, func() lipgloss.Style { return StyleAgentMessage }},
		{"StyleSystemMessage", StyleSystemMessage, func() lipgloss.Style { return StyleSystemMessage }},
		{"StyleApprovalPrompt", StyleApprovalPrompt, func() lipgloss.Style { return StyleApprovalPrompt }},
		{"StyleToolCall", StyleToolCall, func() lipgloss.Style { return StyleToolCall }},
		{"StyleInputIdle", StyleInputIdle, func() lipgloss.Style { return StyleInputIdle }},
		{"StyleInputFocused", StyleInputFocused, func() lipgloss.Style { return StyleInputFocused }},
		{"StyleDiffAdded", StyleDiffAdded, func() lipgloss.Style { return StyleDiffAdded }},
		{"StyleDiffRemoved", StyleDiffRemoved, func() lipgloss.Style { return StyleDiffRemoved }},
		{"StyleDiffHunk", StyleDiffHunk, func() lipgloss.Style { return StyleDiffHunk }},
	}
	require.Len(t, styles, 20, "spec pins exactly 20 Style* to rebuild")

	// Swap every colour token to a distinct value so every style's
	// inputs change, guaranteeing rebuild is observable on each.
	Apply(&Theme{
		Base: BaseSection{
			Bg:            "#010101",
			Surface:       "#020202",
			SurfaceBright: "#030303",
			Border:        "#040404",
			BorderBright:  "#050505",
		},
		Text: TextSection{
			Primary:   "#060606",
			Secondary: "#070707",
			Dim:       "#080808",
			Inverse:   "#090909",
		},
		Accent: AccentSection{
			Primary:    "#0A0A0A",
			PrimaryDim: "#0B0B0B",
			Secondary:  "#0C0C0C",
		},
		Semantic: SemanticSection{
			Success: "#0D0D0D",
			Warning: "#0E0E0E",
			Error:   "#0F0F0F",
			Info:    "#101010",
		},
	})

	for _, s := range styles {
		assert.False(t, reflect.DeepEqual(s.before, s.after()),
			"%s was not rebuilt after Apply", s.name)
	}
}

func TestApply_ShortHexExpanded(t *testing.T) {
	snapshotTheme(t)

	Apply(&Theme{Accent: AccentSection{Primary: "#ABC"}})
	assert.Equal(t, lipgloss.Color("#AABBCC"), AccentPrimary)
}

func TestApply_InvalidHex_KeepsDefaultAndWarns(t *testing.T) {
	snapshotTheme(t)

	before := AccentPrimary

	out := captureStderr(t, func() {
		Apply(&Theme{Accent: AccentSection{Primary: "not-a-hex"}})
	})

	assert.Equal(t, before, AccentPrimary, "invalid hex must leave the var untouched")
	assert.Contains(t, out, "packetcode: theme: invalid hex for accent.primary: not-a-hex (keeping default)")
}

func TestApply_ProviderMapMerged(t *testing.T) {
	snapshotTheme(t)

	// Preserve a pointer to the ollama default so we can assert it
	// survives the merge.
	ollamaBefore := providerColors["ollama"]

	Apply(&Theme{Provider: map[string]string{
		"openai": "#ABCDEF", // override built-in
		"custom": "#123123", // add new slug
	}})

	assert.Equal(t, lipgloss.Color("#ABCDEF"), providerColors["openai"], "override merged")
	assert.Equal(t, lipgloss.Color("#123123"), providerColors["custom"], "new slug merged")
	assert.Equal(t, ollamaBefore, providerColors["ollama"], "unmentioned slug preserved")

	// ProviderColor honours the merged map and falls back for unknowns.
	assert.Equal(t, lipgloss.Color("#ABCDEF"), ProviderColor("openai"))
	assert.Equal(t, lipgloss.Color("#123123"), ProviderColor("custom"))
}

func TestApply_PartialOverride_LeavesOthersIntact(t *testing.T) {
	snapshotTheme(t)

	beforeAccent := AccentPrimary
	beforeBg := BaseBg
	beforeSuccess := Success
	beforeInfo := Info

	Apply(&Theme{Semantic: SemanticSection{Error: "#DEADBE"}})

	assert.Equal(t, lipgloss.Color("#DEADBE"), Error, "target field changed")
	assert.Equal(t, beforeAccent, AccentPrimary, "unmentioned accent unchanged")
	assert.Equal(t, beforeBg, BaseBg, "unmentioned base unchanged")
	assert.Equal(t, beforeSuccess, Success, "sibling semantic unchanged")
	assert.Equal(t, beforeInfo, Info, "sibling semantic unchanged")
}

func TestApply_IsAdditiveNotReplacing(t *testing.T) {
	snapshotTheme(t)

	// First: apply a custom accent.
	Apply(&Theme{Accent: AccentSection{Primary: "#ABCDEF"}})
	assert.Equal(t, lipgloss.Color("#ABCDEF"), AccentPrimary)

	// Then: apply an empty theme. Contract: Apply is additive; empty
	// fields do NOT reset the palette back to built-in defaults.
	Apply(&Theme{})
	assert.Equal(t, lipgloss.Color("#ABCDEF"), AccentPrimary,
		"Apply(&Theme{}) must not reset previously-applied overrides")
}

// ────────────────────────────────────────────────────────────────────
// ProviderColor — map refactor guard
// ────────────────────────────────────────────────────────────────────

func TestProviderColor_UnknownSlug_ReturnsTextPrimary(t *testing.T) {
	snapshotTheme(t)

	got := ProviderColor("does-not-exist")
	assert.Equal(t, TextPrimary, got)
}
