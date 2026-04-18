package conversation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

// TestToolResultBody_PatchFileRendersDiff verifies the patch_file
// result body is routed through the diff component rather than being
// dumped verbatim.
func TestToolResultBody_PatchFileRendersDiff(t *testing.T) {
	msg := Message{
		Kind:     KindToolCall,
		ToolName: "patch_file",
		ToolResult: "Applied 1 patch(es) to a.txt.\n\n" +
			"--- a.txt (current)\n" +
			"+++ a.txt (proposed)\n" +
			"@@ -1,2 +1,2 @@\n" +
			"-old\n" +
			"+new\n",
	}
	out := stripANSI(renderToolResultBody(msg, 80))
	assert.Contains(t, out, "Applied 1 patch(es) to a.txt.")
	assert.Contains(t, out, "@@ -1,2 +1,2 @@")
	assert.Contains(t, out, "- old")
	assert.Contains(t, out, "+ new")
	// Gutter present.
	assert.Contains(t, out, " | ")
}

// TestToolResultBody_PatchFileErrorFallsThrough verifies the red
// error branch wins before the diff attempt — a failed patch_file
// shouldn't try to diff an error message.
func TestToolResultBody_PatchFileErrorFallsThrough(t *testing.T) {
	msg := Message{
		Kind:       KindToolCall,
		ToolName:   "patch_file",
		ToolResult: "patch_file: patch #1 search text not found in a.txt",
		IsError:    true,
	}
	out := stripANSI(renderToolResultBody(msg, 80))
	assert.Contains(t, out, "patch #1 search text not found")
	assert.NotContains(t, out, "@@")
}

// TestToolResultBody_CollapsedPrecedesDiff verifies the collapsed
// placeholder wins before the diff render — a user who explicitly
// hid the output shouldn't see it re-rendered as a diff.
func TestToolResultBody_CollapsedPrecedesDiff(t *testing.T) {
	msg := Message{
		Kind:     KindToolCall,
		ToolName: "patch_file",
		ToolResult: "Applied 1 patch(es) to a.txt.\n\n" +
			"--- a.txt (current)\n" +
			"+++ a.txt (proposed)\n" +
			"@@ -1,1 +1,1 @@\n" +
			"-x\n" +
			"+y\n",
		Collapsed: true,
	}
	out := stripANSI(renderToolResultBody(msg, 80))
	assert.Contains(t, out, "Output collapsed")
	assert.NotContains(t, out, "@@")
}

// TestToolResultBody_NonPatchFileUnchanged verifies the body is
// dumped verbatim for tools that aren't patch_file, even if the text
// contains a `@@` substring (common in shell output).
func TestToolResultBody_NonPatchFileUnchanged(t *testing.T) {
	body := "free output text @@ nothing special"
	msg := Message{
		Kind:       KindToolCall,
		ToolName:   "execute_command",
		ToolResult: body,
	}
	out := renderToolResultBody(msg, 80)
	assert.Equal(t, body, out)
}

// TestTryRenderDiff_NoMarkerReturnsFalse covers the "result has no
// diff header" fast path.
func TestTryRenderDiff_NoMarkerReturnsFalse(t *testing.T) {
	_, ok := tryRenderDiffResult("plain summary, no diff", 80)
	assert.False(t, ok)
}

// TestTryRenderDiff_RowCapTruncates verifies the 200-row cap kicks in
// on a gigantic patch_file result so the conversation doesn't hang
// lipgloss.
func TestTryRenderDiff_RowCapTruncates(t *testing.T) {
	var b strings.Builder
	b.WriteString("--- a.txt (current)\n+++ a.txt (proposed)\n@@ -1,300 +1,300 @@\n")
	for i := 0; i < 300; i++ {
		b.WriteString("+added\n")
	}
	rendered, ok := tryRenderDiffResult(b.String(), 80)
	assert.True(t, ok)
	out := stripANSI(rendered)
	assert.Contains(t, out, "omitted")
	lines := strings.Split(out, "\n")
	assert.LessOrEqual(t, len(lines), 202, "renders at most ~200 rows + header")
}
