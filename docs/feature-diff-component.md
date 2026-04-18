# Diff Component & Richer Tool-Call Rendering — Round 4 Design Spec

## Summary

Extract diff rendering into a new presentation-only `internal/ui/components/diff` package that parses a unified-diff string into structured hunks and renders them with colour, a right-aligned line-number gutter, and capped-height truncation. Wire it into the approval prompt so `write_file` and `patch_file` show a real preview diff (computed via new tool-package helpers `WriteFileTool.PreviewDiff` and `PatchFileTool.PreviewPatchDiff`) instead of raw JSON. Wire it into the conversation pane so the completed `patch_file` tool-call block renders its existing `difflib`-produced unified diff through the same component — visual parity between "approve" and "approved".

## User stories

1. **Approve a targeted refactor.** The agent calls `patch_file` with three search/replace ops. The approval now shows `+12 −7 across 3 hunks` and a coloured line-numbered diff; the user approves confident they understand the change.
2. **Approve a brand-new file.** `write_file` to a nonexistent path → approval shows `path/to/file (new file)` and the full proposed content as all-green `+` lines.
3. **Audit a completed patch.** After approving, the conversation block renders the same diff style — identical colours and gutter.

## Component design

`internal/ui/components/diff/diff.go`. **Immutable builder style** (value receivers returning new `Model`). Presentation-only — no `tea.Msg` handling, no `Update`.

### Types

```go
type LineKind int
const (
    LineContext LineKind = iota
    LineAdded
    LineRemoved
)

type Line struct {
    Kind    LineKind
    Text    string  // without +/-/space prefix, without trailing \n
    OldLine int     // 1-indexed in old file; 0 for LineAdded
    NewLine int     // 1-indexed in new file; 0 for LineRemoved
}

type Hunk struct {
    Header   string  // raw "@@ -A,B +C,D @@ optional-heading"
    OldStart int
    OldLines int
    NewStart int
    NewLines int
    Lines    []Line
}

type Model struct {
    fromFile string
    toFile   string
    hunks    []Hunk
    width    int
    maxRows  int  // 0 = no cap
}

func Parse(unified string) (Model, error)
func NewFile(path, content string) Model
func (m Model) SetWidth(w int) Model
func (m Model) SetMaxRows(n int) Model
func (m Model) Stats() (added, removed int)
func (m Model) Empty() bool
func (m Model) View() string
```

### Parse rules

- Split on `\n`, walk lines.
- `--- ` / `+++ ` capture into `fromFile`/`toFile` (portion after space).
- `@@ ` starts a hunk. Regex: `^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`. Missing counts default to 1. Trailing text → `Hunk.Header`.
- Inside hunk: first byte classifies (`+` added, `-` removed, space/empty context). Strip `\r`. Strip `+/-/space` prefix for `Line.Text`.
- `\ No newline at end of file` markers skipped.
- Line numbers: start at `OldStart`/`NewStart`; increment `OldLine` on context+removed; `NewLine` on context+added.
- Malformed `@@` → `fmt.Errorf("diff: malformed hunk header: %q", line)`.
- Lines before the first `@@` (other than file headers) are silently ignored (lets `patch_file` preambles pass through).

### NewFile

Returns Model with one synthetic hunk: `OldStart=0, OldLines=0, NewStart=1, NewLines=lineCount(content)`, all `LineAdded` with `NewLine` counters. `toFile = path + " (new file)"`, `fromFile = ""`. Empty content → `Empty() == true`, `View() == ""`.

### View

1. File header: `theme.StyleDim.Render(fromFile + " → " + toFile)` (omitted if both empty).
2. Per hunk:
   - `theme.StyleDiffHunk.Render("@@ -A,B +C,D @@" + trailingText)`.
   - Rows: `<gutter><sign><space><text>`.
   - Gutter: right-aligned line number, width = max(2, len(maxNumberAsString)), followed by `" | "`. All in `theme.StyleDim`.
   - Number shown: `NewLine` for Context/Added; `OldLine` for Removed.
   - Sign: `+` / `-` / ` ` styled `StyleDiffAdded` / `StyleDiffRemoved` / `StylePrimary`.
   - Text: styled to match sign for added/removed; `StylePrimary` for context.
3. Width clamping (truncation, not wrap): if row exceeds `m.width`, truncate text portion and append `…`. Uses `utf8.RuneCountInString` (no grapheme-cluster handling). `m.width <= 0` → no clamping.
4. Row cap: if `totalRows > maxRows > 0`:
   - Show first `headN = maxRows*2/3` rows (min 1).
   - Separator: `… N lines omitted (+X added, −Y removed across Z hunks) …` (U+2026 `…`, U+2212 `−`).
   - Show last `maxRows - headN - 1` rows.
   - If `maxRows <= 2`: single line `… diff too large to preview (+X, −Y) …`.
5. Rows joined with `\n`.

## Tool helpers

### `(*WriteFileTool).PreviewDiff(path, content string) (unified string, newFile bool, err error)`

1. `abs, err := resolveInRoot(t.Root, path)` → path-outside-project-root error verbatim.
2. `data, err := os.ReadFile(abs)`. `os.IsNotExist(err)` → return `("", true, nil)`. Other IO errors → `fmt.Errorf("preview_diff: %w", err)`.
3. Binary guard: `if !utf8.Valid(data)` → `fmt.Errorf("preview_diff: %s appears to be binary", path)`.
4. If `string(data) == content` → return `("", false, nil)`.
5. `difflib.GetUnifiedDiffString` with `FromFile: path + " (current)"`, `ToFile: path + " (proposed)"`, `Context: 3`.

### `(*PatchFileTool).PreviewPatchDiff(path string, patches []PatchOp) (string, error)`

Rename unexported `patchOp` → exported `PatchOp` (JSON tags unchanged; JSON wire format stable).

Extract private `applyPatches(original string, patches []PatchOp, path string) (updated string, unified string, err error)` that both `Execute` and `PreviewPatchDiff` call.

Validation errors (exact strings, no `patch_file:` prefix — callers prepend):
- `"at least one patch is required"` (empty slice)
- `"patch #%d has empty search string"`
- `"patch #%d search text not found in %s"`
- `"patch #%d search text matches %d times in %s; must be unique"`

`FromFile`/`ToFile` in the diff: `path + " (current)"` / `path + " (proposed)"`.

## Approval integration

### Renderer registry

`internal/ui/components/approval/renderers.go` (NEW):

```go
type RenderContext struct {
    Tool      tools.Tool
    Arguments string
    Width     int
}

type BodyRenderer func(RenderContext) string

var renderers = map[string]BodyRenderer{
    "write_file": renderWriteFile,
    "patch_file": renderPatchFile,
}

func Register(toolName string, r BodyRenderer) { renderers[toolName] = r }
```

`approval.Model.View` consults `renderers[m.tool.Name()]` before falling back to `summariseParams`. Width passed in: `width - 4` (border + padding).

### renderWriteFile

1. Unmarshal args into `{Path, Content string}`. On error → error fallback + raw args.
2. Type-assert `*tools.WriteFileTool`. Non-match → fall through to `summariseParams` (defensive).
3. `unified, newFile, err := wt.PreviewDiff(Path, Content)`.
4. Error → error fallback body (see below) with raw content preview (first 40 lines).
5. `newFile == true` → `diff.NewFile(Path, Content).SetWidth(ctx.Width).SetMaxRows(40)`. Header badge + stats `+X −0` + blank + `View()`.
6. `unified == "" && !newFile` → single dim line `<path> — no changes (proposed content matches current file)`.
7. Else `diff.Parse` + `SetWidth/SetMaxRows(40)` + render with header badge + stats line + blank + `View()`.

### renderPatchFile

1. Unmarshal into `{Path string; Patches []tools.PatchOp}`. On error → error fallback with `summariseParams` fallback preview.
2. Type-assert `*tools.PatchFileTool`.
3. `unified, err := pt.PreviewPatchDiff(Path, Patches)`.
4. Error → error fallback with `summariseParams` as preview body (shows raw search/replace pairs).
5. Parse + render same shape as `renderWriteFile` with `SetMaxRows(40)`.

### Error fallback template

```
! could not compute diff: <error.Error()>

<first 40 lines of preview>
… N more lines …       ← only if overflow
```

`!` line styled `theme.StyleError`. Approval buttons still shown — user may approve or reject.

### Unregistered tools

`execute_command`, `spawn_agent`, etc. use existing `summariseParams` path. No regression.

## Conversation integration

`renderToolCall` refactored. New helpers:

```go
func renderToolResultBody(msg Message, width int) string {
    if msg.IsError { return theme.StyleError.Render(msg.ToolResult) }
    if msg.Collapsed {
        lines := strings.Count(msg.ToolResult, "\n") + 1
        return theme.StyleDim.Render(fmt.Sprintf("▶ Output collapsed (%d lines) — press Tab to expand", lines))
    }
    if msg.ToolName == "patch_file" {
        if rendered, ok := tryRenderDiffResult(msg.ToolResult, width); ok {
            return rendered
        }
    }
    return msg.ToolResult
}

func tryRenderDiffResult(content string, width int) (string, bool) {
    idx := strings.Index(content, "--- ")
    if idx < 0 { idx = strings.Index(content, "@@ ") }
    if idx < 0 { return "", false }
    prefix := strings.TrimRight(content[:idx], "\n")
    m, err := diff.Parse(content[idx:])
    if err != nil || m.Empty() { return "", false }
    m = m.SetWidth(width).SetMaxRows(200)
    out := m.View()
    if prefix != "" {
        return theme.StyleDim.Render(prefix) + "\n" + out, true
    }
    return out, true
}
```

Conversation uses **200** row cap (has scrollable viewport); approval uses **40** (fixed modal height).

`write_file` conversation-side rendering unchanged (tool returns a text summary, not a diff).

## File-by-file change list

### Bucket A — Implementation

| Path | Changes |
|---|---|
| `internal/ui/components/diff/diff.go` | **NEW.** Full component. ~260 LOC. |
| `internal/tools/write_file.go` | Add `PreviewDiff(path, content string) (string, bool, error)`. Import `difflib`, `unicode/utf8`. |
| `internal/tools/patch_file.go` | Rename `patchOp` → `PatchOp`. Update `patchFileParams.Patches` type. Add `PreviewPatchDiff`. Extract private `applyPatches` helper shared by `Execute` and `PreviewPatchDiff`. |
| `internal/ui/components/approval/renderers.go` | **NEW.** Registry + `RenderContext` + `BodyRenderer` + `renderWriteFile` + `renderPatchFile` + `renderDiffErrorFallback` helper. |
| `internal/ui/components/approval/approval.go` | `View` consults `renderers[m.tool.Name()]` before `summariseParams`. No signature change. |
| `internal/ui/components/conversation/conversation.go` | Add `renderToolResultBody` + `tryRenderDiffResult`. `renderToolCall` uses them. Import `diff`. |

### Bucket B — Tests + docs + commit

| Path | Changes |
|---|---|
| `internal/ui/components/diff/diff_test.go` | **NEW.** ~25 component tests. |
| `internal/tools/write_file_test.go` | Add 6 `TestWriteFile_PreviewDiff_*` cases. |
| `internal/tools/patch_file_test.go` | Add 9 `TestPatchFile_PreviewPatchDiff_*` + `PatchOp` JSON compat test. |
| `internal/ui/components/approval/approval_test.go` | **NEW.** ~13 integration tests. |
| `internal/ui/components/conversation/conversation_test.go` | **NEW.** 6 tool-result rendering tests. |
| `README.md` | Brief note in Approvals / Tool-call rendering subsection. |
| `CHANGELOG.md` | Added bullet; remove from Deferred. |
| `docs/roadmap-deferred.md` | Mark Round 4 landed. |

## Tests

See full lists in the spec body (enumerated above). Key counts:
- diff component: 25 tests
- write_file: 6 new
- patch_file: 9 new + 1 JSON-compat
- approval: 13 new (package previously had no tests)
- conversation: 6 new (package previously had no tests)
- Total new tests: ~60

## Out of scope (Round 5+)

- Side-by-side diff layout (component API allows future extension).
- Syntax highlighting inside diff text.
- Word-level intra-line highlighting.
- Expanding context on demand.
- Scrollable diff viewport in the approval modal.
- `write_file` conversation-side diff (tool doesn't emit one).
- Per-line accept/reject within a patch.
- Binary diff preview.
- ANSI-aware width truncation.
- Theming of diff colours via user theme.toml (Round 6).
- Persistent "always approve diffs under N lines" trust setting.
