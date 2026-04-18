package diff

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Small helper to strip ANSI escape sequences so tests can assert on
// visible characters without caring about lipgloss styling.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func TestParse_Empty(t *testing.T) {
	m, err := Parse("")
	require.NoError(t, err)
	assert.True(t, m.Empty())
	assert.Equal(t, "", m.View())
}

func TestParse_FileHeadersOnly(t *testing.T) {
	in := "--- old.go\n+++ new.go\n"
	m, err := Parse(in)
	require.NoError(t, err)
	assert.True(t, m.Empty())
	assert.Equal(t, "old.go", m.fromFile)
	assert.Equal(t, "new.go", m.toFile)
}

func TestParse_SingleHunk(t *testing.T) {
	in := "--- a.go\n+++ a.go\n@@ -1,3 +1,3 @@\n alpha\n-beta\n+BETA\n gamma\n"
	m, err := Parse(in)
	require.NoError(t, err)
	require.Len(t, m.hunks, 1)
	h := m.hunks[0]
	assert.Equal(t, 1, h.OldStart)
	assert.Equal(t, 3, h.OldLines)
	assert.Equal(t, 1, h.NewStart)
	assert.Equal(t, 3, h.NewLines)
	require.Len(t, h.Lines, 4)
	assert.Equal(t, LineContext, h.Lines[0].Kind)
	assert.Equal(t, LineRemoved, h.Lines[1].Kind)
	assert.Equal(t, LineAdded, h.Lines[2].Kind)
	assert.Equal(t, LineContext, h.Lines[3].Kind)
}

func TestParse_MissingCountsDefaultToOne(t *testing.T) {
	in := "@@ -5 +5 @@\n-old\n+new\n"
	m, err := Parse(in)
	require.NoError(t, err)
	require.Len(t, m.hunks, 1)
	h := m.hunks[0]
	assert.Equal(t, 5, h.OldStart)
	assert.Equal(t, 1, h.OldLines)
	assert.Equal(t, 5, h.NewStart)
	assert.Equal(t, 1, h.NewLines)
}

func TestParse_HunkHeading(t *testing.T) {
	in := "@@ -1,2 +1,2 @@ func Foo()\n-old\n+new\n"
	m, err := Parse(in)
	require.NoError(t, err)
	require.Len(t, m.hunks, 1)
	assert.Equal(t, " func Foo()", m.hunks[0].Header)
}

func TestParse_MalformedHunk(t *testing.T) {
	_, err := Parse("@@ bogus @@\n-x\n+y\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed hunk header")
}

func TestParse_IgnoresNoNewlineMarker(t *testing.T) {
	in := "@@ -1,1 +1,1 @@\n-old\n+new\n\\ No newline at end of file\n"
	m, err := Parse(in)
	require.NoError(t, err)
	require.Len(t, m.hunks, 1)
	assert.Len(t, m.hunks[0].Lines, 2)
}

func TestParse_IgnoresPreamble(t *testing.T) {
	in := "Applied 1 patch(es) to foo.go.\n\n--- foo.go\n+++ foo.go\n@@ -1,1 +1,1 @@\n-a\n+b\n"
	m, err := Parse(in)
	require.NoError(t, err)
	require.Len(t, m.hunks, 1)
	assert.Equal(t, "foo.go", m.fromFile)
}

func TestParse_StripsCR(t *testing.T) {
	in := "@@ -1,1 +1,1 @@\r\n-old\r\n+new\r\n"
	m, err := Parse(in)
	require.NoError(t, err)
	require.Len(t, m.hunks, 1)
	require.Len(t, m.hunks[0].Lines, 2)
	assert.Equal(t, "old", m.hunks[0].Lines[0].Text)
	assert.Equal(t, "new", m.hunks[0].Lines[1].Text)
}

func TestParse_LineNumberTracking(t *testing.T) {
	in := "@@ -10,3 +20,3 @@\n ctx1\n-rem\n+add\n ctx2\n"
	m, err := Parse(in)
	require.NoError(t, err)
	lines := m.hunks[0].Lines
	assert.Equal(t, 10, lines[0].OldLine)
	assert.Equal(t, 20, lines[0].NewLine)
	assert.Equal(t, 11, lines[1].OldLine) // removed increments old
	assert.Equal(t, 0, lines[1].NewLine)
	assert.Equal(t, 0, lines[2].OldLine) // added increments new
	assert.Equal(t, 21, lines[2].NewLine)
	assert.Equal(t, 12, lines[3].OldLine)
	assert.Equal(t, 22, lines[3].NewLine)
}

func TestStats(t *testing.T) {
	in := "@@ -1,4 +1,4 @@\n ctx\n-a\n-b\n+x\n+y\n+z\n ctx\n"
	m, err := Parse(in)
	require.NoError(t, err)
	added, removed := m.Stats()
	assert.Equal(t, 3, added)
	assert.Equal(t, 2, removed)
}

func TestNewFile_Empty(t *testing.T) {
	m := NewFile("foo.go", "")
	assert.True(t, m.Empty())
	assert.Equal(t, "", m.View())
}

func TestNewFile_WithContent(t *testing.T) {
	m := NewFile("foo.go", "line1\nline2\nline3\n")
	added, removed := m.Stats()
	assert.Equal(t, 3, added)
	assert.Equal(t, 0, removed)
	require.Len(t, m.hunks, 1)
	assert.Equal(t, 1, m.hunks[0].NewStart)
	assert.Equal(t, 3, m.hunks[0].NewLines)
	assert.Contains(t, m.toFile, "(new file)")
}

func TestNewFile_NoTrailingNewline(t *testing.T) {
	m := NewFile("foo.go", "onlyline")
	added, _ := m.Stats()
	assert.Equal(t, 1, added)
}

func TestView_RendersHeaderAndRows(t *testing.T) {
	in := "--- a.go\n+++ a.go\n@@ -1,2 +1,2 @@\n-old\n+new\n"
	m, err := Parse(in)
	require.NoError(t, err)
	out := stripANSI(m.View())
	assert.Contains(t, out, "a.go → a.go")
	assert.Contains(t, out, "@@ -1,2 +1,2 @@")
	assert.Contains(t, out, "-")
	assert.Contains(t, out, "+")
}

func TestView_OmitsHeaderWhenNoFiles(t *testing.T) {
	in := "@@ -1,1 +1,1 @@\n-old\n+new\n"
	m, err := Parse(in)
	require.NoError(t, err)
	out := stripANSI(m.View())
	assert.NotContains(t, out, " → ")
}

func TestView_GutterWidthMinTwo(t *testing.T) {
	in := "@@ -1,1 +1,1 @@\n-a\n+b\n"
	m, err := Parse(in)
	require.NoError(t, err)
	out := stripANSI(m.View())
	// Expect at least 2 chars of gutter before " | "
	assert.Contains(t, out, " 1 | ")
}

func TestView_GutterGrowsWithLineNumbers(t *testing.T) {
	in := "@@ -100,1 +100,1 @@\n-a\n+b\n"
	m, err := Parse(in)
	require.NoError(t, err)
	out := stripANSI(m.View())
	assert.Contains(t, out, "100 | ")
}

func TestView_WidthClampTruncatesLongLines(t *testing.T) {
	long := strings.Repeat("x", 200)
	in := "@@ -1,1 +1,1 @@\n+" + long + "\n"
	m, err := Parse(in)
	require.NoError(t, err)
	m = m.SetWidth(40)
	out := stripANSI(m.View())
	assert.Contains(t, out, "\u2026")
	// No single rendered line should exceed width by much (ANSI stripped).
	for _, line := range strings.Split(out, "\n") {
		assert.LessOrEqual(t, len([]rune(line)), 50, "line exceeds expected width: %q", line)
	}
}

func TestView_WidthZeroNoClamp(t *testing.T) {
	long := strings.Repeat("x", 200)
	in := "@@ -1,1 +1,1 @@\n+" + long + "\n"
	m, err := Parse(in)
	require.NoError(t, err)
	out := stripANSI(m.View())
	assert.Contains(t, out, long)
}

func TestView_MaxRowsTruncates(t *testing.T) {
	var b strings.Builder
	b.WriteString("@@ -1,50 +1,50 @@\n")
	for i := 0; i < 50; i++ {
		b.WriteString("+added\n")
	}
	m, err := Parse(b.String())
	require.NoError(t, err)
	m = m.SetMaxRows(10)
	out := stripANSI(m.View())
	assert.Contains(t, out, "omitted")
	assert.Contains(t, out, "+50 added")
	assert.Contains(t, out, "\u2212")
	lines := strings.Split(out, "\n")
	assert.LessOrEqual(t, len(lines), 11, "should have at most maxRows rows (+ possibly header)")
}

func TestView_MaxRowsSingleLineFallback(t *testing.T) {
	var b strings.Builder
	b.WriteString("@@ -1,5 +1,5 @@\n")
	for i := 0; i < 5; i++ {
		b.WriteString("+added\n")
	}
	m, err := Parse(b.String())
	require.NoError(t, err)
	m = m.SetMaxRows(2)
	out := stripANSI(m.View())
	assert.Contains(t, out, "diff too large to preview")
	assert.Contains(t, out, "+5")
}

func TestView_NoTruncationWhenWithinCap(t *testing.T) {
	in := "@@ -1,2 +1,2 @@\n-a\n+b\n"
	m, err := Parse(in)
	require.NoError(t, err)
	m = m.SetMaxRows(100)
	out := stripANSI(m.View())
	assert.NotContains(t, out, "omitted")
}

func TestView_EmptyModelRendersBlank(t *testing.T) {
	m, err := Parse("")
	require.NoError(t, err)
	assert.Equal(t, "", m.View())
}

func TestView_NewFileRenders(t *testing.T) {
	m := NewFile("x.go", "alpha\nbeta\n")
	out := stripANSI(m.View())
	assert.Contains(t, out, "(new file)")
	assert.Contains(t, out, "alpha")
	assert.Contains(t, out, "beta")
}

func TestSetWidthReturnsCopy(t *testing.T) {
	m, _ := Parse("@@ -1,1 +1,1 @@\n-a\n+b\n")
	m2 := m.SetWidth(50)
	assert.Equal(t, 0, m.width)
	assert.Equal(t, 50, m2.width)
}

func TestSetMaxRowsReturnsCopy(t *testing.T) {
	m, _ := Parse("@@ -1,1 +1,1 @@\n-a\n+b\n")
	m2 := m.SetMaxRows(40)
	assert.Equal(t, 0, m.maxRows)
	assert.Equal(t, 40, m2.maxRows)
}

func TestRoundTrip_ParsePreservesText(t *testing.T) {
	in := "@@ -1,3 +1,3 @@\n alpha\n-beta\n+BETA\n gamma\n"
	m, err := Parse(in)
	require.NoError(t, err)
	out := stripANSI(m.View())
	assert.Contains(t, out, "alpha")
	assert.Contains(t, out, "beta")
	assert.Contains(t, out, "BETA")
	assert.Contains(t, out, "gamma")
}
