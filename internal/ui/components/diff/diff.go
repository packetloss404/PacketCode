// Package diff parses a unified-diff string into structured hunks and
// renders it with colour, a right-aligned line-number gutter, and
// capped-height truncation.
//
// The component is presentation-only — no tea.Msg handling, no Update.
// Callers pass a unified-diff string (typically produced by
// difflib.GetUnifiedDiffString) to Parse, chain SetWidth / SetMaxRows,
// then call View. The builder style is immutable: every mutator takes
// a value receiver and returns a new Model.
package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/packetcode/packetcode/internal/ui/theme"
)

// LineKind classifies a diff line. Context lines appear in both old and
// new, Added exist only in the proposed file, Removed only in the
// current file.
type LineKind int

const (
	LineContext LineKind = iota
	LineAdded
	LineRemoved
)

// Line is a single row within a hunk. OldLine / NewLine are the
// 1-indexed line numbers in the old and new files respectively; one of
// them is 0 for pure additions / removals.
type Line struct {
	Kind    LineKind
	Text    string
	OldLine int
	NewLine int
}

// Hunk is a contiguous block of changes, bracketed by a `@@` header.
type Hunk struct {
	Header   string
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Lines    []Line
}

// Model is the parsed diff plus render configuration. Construct via
// Parse or NewFile; modify via SetWidth / SetMaxRows.
type Model struct {
	fromFile string
	toFile   string
	hunks    []Hunk
	width    int
	maxRows  int
}

var hunkHeaderRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)

// Parse reads a unified-diff string into a Model. Lines before the
// first @@ (other than --- / +++ file-header lines) are silently
// ignored so patch_file preambles pass through. A malformed hunk
// header returns a structured error.
func Parse(unified string) (Model, error) {
	m := Model{}
	if unified == "" {
		return m, nil
	}

	lines := strings.Split(unified, "\n")
	// A trailing "\n" on the input produces an empty last element from
	// strings.Split — drop it so we don't render a phantom blank context
	// row at the end of every diff.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var current *Hunk
	var oldCursor, newCursor int

	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")

		switch {
		case strings.HasPrefix(line, "--- "):
			m.fromFile = strings.TrimSpace(strings.TrimPrefix(line, "--- "))
			continue
		case strings.HasPrefix(line, "+++ "):
			m.toFile = strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
			continue
		case strings.HasPrefix(line, "@@ "):
			match := hunkHeaderRE.FindStringSubmatch(line)
			if match == nil {
				return Model{}, fmt.Errorf("diff: malformed hunk header: %q", line)
			}
			h := Hunk{Header: match[5]}
			h.OldStart, _ = strconv.Atoi(match[1])
			if match[2] == "" {
				h.OldLines = 1
			} else {
				h.OldLines, _ = strconv.Atoi(match[2])
			}
			h.NewStart, _ = strconv.Atoi(match[3])
			if match[4] == "" {
				h.NewLines = 1
			} else {
				h.NewLines, _ = strconv.Atoi(match[4])
			}
			m.hunks = append(m.hunks, h)
			current = &m.hunks[len(m.hunks)-1]
			oldCursor = current.OldStart
			newCursor = current.NewStart
			continue
		case strings.HasPrefix(line, "\\ No newline at end of file"):
			continue
		}

		if current == nil {
			// Lines before the first @@ that aren't file headers are ignored.
			continue
		}

		if line == "" {
			// Empty line inside a hunk is a context line ("" in unified diff
			// is a blank row, not a signal). Treat as context with empty text.
			current.Lines = append(current.Lines, Line{
				Kind:    LineContext,
				Text:    "",
				OldLine: oldCursor,
				NewLine: newCursor,
			})
			oldCursor++
			newCursor++
			continue
		}

		prefix := line[0]
		text := line[1:]
		switch prefix {
		case '+':
			current.Lines = append(current.Lines, Line{
				Kind:    LineAdded,
				Text:    text,
				OldLine: 0,
				NewLine: newCursor,
			})
			newCursor++
		case '-':
			current.Lines = append(current.Lines, Line{
				Kind:    LineRemoved,
				Text:    text,
				OldLine: oldCursor,
				NewLine: 0,
			})
			oldCursor++
		case ' ':
			current.Lines = append(current.Lines, Line{
				Kind:    LineContext,
				Text:    text,
				OldLine: oldCursor,
				NewLine: newCursor,
			})
			oldCursor++
			newCursor++
		default:
			// Unknown prefix: treat whole line as context so we don't lose
			// data. Keeps Parse tolerant of slightly off-spec producers.
			current.Lines = append(current.Lines, Line{
				Kind:    LineContext,
				Text:    line,
				OldLine: oldCursor,
				NewLine: newCursor,
			})
			oldCursor++
			newCursor++
		}
	}

	return m, nil
}

// NewFile returns a synthetic Model representing a brand-new file —
// every line is an addition and there is no "old" side. Used by
// write_file previews when the target path does not yet exist.
func NewFile(path, content string) Model {
	m := Model{
		toFile: path + " (new file)",
	}
	if content == "" {
		return m
	}
	// Split on newlines; a trailing newline produces a trailing empty
	// element that we drop so the line count matches `wc -l`.
	split := strings.Split(content, "\n")
	if len(split) > 0 && split[len(split)-1] == "" {
		split = split[:len(split)-1]
	}
	if len(split) == 0 {
		return m
	}
	h := Hunk{
		OldStart: 0,
		OldLines: 0,
		NewStart: 1,
		NewLines: len(split),
	}
	for i, t := range split {
		h.Lines = append(h.Lines, Line{
			Kind:    LineAdded,
			Text:    t,
			OldLine: 0,
			NewLine: i + 1,
		})
	}
	m.hunks = append(m.hunks, h)
	return m
}

// SetWidth returns a copy with the render width clamped. Zero or
// negative disables width clamping.
func (m Model) SetWidth(w int) Model {
	m.width = w
	return m
}

// SetMaxRows returns a copy with the row cap set. Zero disables
// truncation.
func (m Model) SetMaxRows(n int) Model {
	m.maxRows = n
	return m
}

// Stats returns (added, removed) counts across all hunks.
func (m Model) Stats() (added, removed int) {
	for _, h := range m.hunks {
		for _, l := range h.Lines {
			switch l.Kind {
			case LineAdded:
				added++
			case LineRemoved:
				removed++
			}
		}
	}
	return added, removed
}

// Empty reports whether the diff has zero renderable rows.
func (m Model) Empty() bool {
	for _, h := range m.hunks {
		if len(h.Lines) > 0 {
			return false
		}
	}
	return true
}

// View renders the diff as a multi-line string. The first line (if any)
// is a dim "from → to" header; each hunk is then a dim @@ header
// followed by coloured content rows with a right-aligned line-number
// gutter. Rows are truncated — never wrapped — when they overflow the
// configured width.
func (m Model) View() string {
	if m.Empty() {
		return ""
	}

	added, removed := m.Stats()

	// Gutter width: largest line number we'll ever show.
	maxNum := 0
	for _, h := range m.hunks {
		for _, l := range h.Lines {
			n := l.NewLine
			if l.Kind == LineRemoved {
				n = l.OldLine
			}
			if n > maxNum {
				maxNum = n
			}
		}
	}
	gutterW := len(strconv.Itoa(maxNum))
	if gutterW < 2 {
		gutterW = 2
	}

	var rows []string
	hunkCount := len(m.hunks)
	for _, h := range m.hunks {
		rows = append(rows, renderHunkHeader(h, m.width))
		for _, l := range h.Lines {
			rows = append(rows, renderContentRow(l, gutterW, m.width))
		}
	}

	var body string
	if m.maxRows > 0 && len(rows) > m.maxRows {
		body = truncateRows(rows, m.maxRows, added, removed, hunkCount)
	} else {
		body = strings.Join(rows, "\n")
	}

	if m.fromFile == "" && m.toFile == "" {
		return body
	}
	header := theme.StyleDim.Render(m.fromFile + " → " + m.toFile)
	return header + "\n" + body
}

// renderHunkHeader renders the `@@ -A,B +C,D @@ trailing` line.
func renderHunkHeader(h Hunk, width int) string {
	raw := fmt.Sprintf("@@ -%d,%d +%d,%d @@%s", h.OldStart, h.OldLines, h.NewStart, h.NewLines, h.Header)
	raw = clampWidth(raw, width)
	return theme.StyleDiffHunk.Render(raw)
}

// renderContentRow renders one line of a hunk with its gutter and sign.
func renderContentRow(l Line, gutterW, width int) string {
	num := l.NewLine
	if l.Kind == LineRemoved {
		num = l.OldLine
	}
	numStr := ""
	if num > 0 {
		numStr = strconv.Itoa(num)
	}
	gutter := theme.StyleDim.Render(padLeft(numStr, gutterW) + " | ")

	var sign string
	var textStyle, signStyle = theme.StylePrimary, theme.StylePrimary
	switch l.Kind {
	case LineAdded:
		sign = "+"
		signStyle = theme.StyleDiffAdded
		textStyle = theme.StyleDiffAdded
	case LineRemoved:
		sign = "-"
		signStyle = theme.StyleDiffRemoved
		textStyle = theme.StyleDiffRemoved
	default:
		sign = " "
	}

	// Width budget: total - gutter (gutterW + 3 for " | ") - 2 for sign+space.
	text := l.Text
	if width > 0 {
		budget := width - (gutterW + 3) - 2
		if budget < 1 {
			budget = 1
		}
		if utf8.RuneCountInString(text) > budget {
			text = truncateRunes(text, budget-1) + "\u2026"
		}
	}

	return gutter + signStyle.Render(sign) + textStyle.Render(" "+text)
}

// clampWidth truncates a plain string (not yet styled) to width with a
// trailing ellipsis. Width<=0 is a no-op.
func clampWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	if utf8.RuneCountInString(s) <= width {
		return s
	}
	if width <= 1 {
		return "\u2026"
	}
	return truncateRunes(s, width-1) + "\u2026"
}

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	var b strings.Builder
	count := 0
	for _, r := range s {
		if count >= n {
			break
		}
		b.WriteRune(r)
		count++
	}
	return b.String()
}

func padLeft(s string, width int) string {
	if utf8.RuneCountInString(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-utf8.RuneCountInString(s)) + s
}

// truncateRows builds the head + separator + tail layout used when the
// diff exceeds maxRows. The separator legend uses U+2026 and U+2212.
func truncateRows(rows []string, maxRows, added, removed, hunkCount int) string {
	if maxRows <= 2 {
		return theme.StyleDim.Render(fmt.Sprintf("\u2026 diff too large to preview (+%d, \u2212%d) \u2026", added, removed))
	}
	headN := maxRows * 2 / 3
	if headN < 1 {
		headN = 1
	}
	tailN := maxRows - headN - 1
	if tailN < 0 {
		tailN = 0
	}
	if headN+tailN+1 > len(rows) {
		// Shouldn't happen given the outer guard, but clamp defensively.
		headN = len(rows) - tailN - 1
		if headN < 0 {
			headN = 0
		}
	}
	omitted := len(rows) - headN - tailN
	separator := theme.StyleDim.Render(fmt.Sprintf("\u2026 %d lines omitted (+%d added, \u2212%d removed across %d hunks) \u2026", omitted, added, removed, hunkCount))

	parts := make([]string, 0, headN+1+tailN)
	parts = append(parts, rows[:headN]...)
	parts = append(parts, separator)
	parts = append(parts, rows[len(rows)-tailN:]...)
	return strings.Join(parts, "\n")
}
