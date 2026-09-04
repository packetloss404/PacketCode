package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const diagnosticsHeader = "Diagnostics for the new contents"

// brokenGo has one parse error the parser reports as several messages.
const brokenGo = "package p\n\nfunc a() {\n\tx := \n}\n"

func writeToolResult(t *testing.T, root, path, content string) ToolResult {
	t.Helper()
	tool := NewWriteFileTool(root, nil)
	args, err := json.Marshal(writeFileParams{Path: path, Content: content})
	require.NoError(t, err)
	res, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)
	return res
}

func TestWriteFile_IntroducedSyntaxErrorIsAppended(t *testing.T) {
	root := t.TempDir()
	res := writeToolResult(t, root, "broken.go", brokenGo)

	// The write succeeded; the diagnostics ride along with the success line and
	// must never turn the result into an error.
	assert.False(t, res.IsError)
	assert.Contains(t, res.Content, "Wrote broken.go")
	assert.Contains(t, res.Content, diagnosticsHeader)
	assert.Contains(t, res.Content, "broken.go:5:1: error: expected operand, found '}'")
	assert.Equal(t, 4, res.Metadata["diagnostics"])

	on, err := os.ReadFile(filepath.Join(root, "broken.go"))
	require.NoError(t, err)
	assert.Equal(t, brokenGo, string(on), "the file is written even when it does not parse")
}

func TestWriteFile_CleanGoAddsNothing(t *testing.T) {
	root := t.TempDir()
	res := writeToolResult(t, root, "ok.go", "package p\n\nfunc a() int { return 1 }\n")

	assert.Equal(t, "Wrote ok.go (37 bytes, 3 lines).", res.Content)
	assert.NotContains(t, res.Metadata, "diagnostics")
}

func TestWriteFile_NonGoFileAddsNothing(t *testing.T) {
	root := t.TempDir()
	// Content that is nonsense as Go: a language with no in-process checker
	// must produce no block at all, not a "no checker available" note.
	for _, path := range []string{"notes.md", "conf.yaml", "app.py", "app.ts"} {
		res := writeToolResult(t, root, path, "func a() {\n\tx := \n}\n")
		assert.NotContains(t, res.Content, diagnosticsHeader, path)
		assert.NotContains(t, res.Content, "error:", path)
	}
}

func TestWriteFile_PreExistingErrorsAreNotBlamedOnTheEdit(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "broken.go"), []byte(brokenGo), 0o644))

	// The edit adds a comment and leaves the existing breakage untouched, so
	// there is nothing to report: reporting the old errors would blame the
	// model for damage it did not do.
	res := writeToolResult(t, root, "broken.go", "// touched\n"+brokenGo)

	assert.False(t, res.IsError)
	assert.NotContains(t, res.Content, diagnosticsHeader)
	assert.NotContains(t, res.Metadata, "diagnostics")
}

func TestWriteFile_OnlyIntroducedErrorsAreListed(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "broken.go"), []byte(brokenGo), 0o644))

	res := writeToolResult(t, root, "broken.go", brokenGo+"\nfunc b() int {\n\treturn\n\n")

	assert.Contains(t, res.Content, diagnosticsHeader)
	assert.Contains(t, res.Content, "expected ';', found 'return'")
	assert.NotContains(t, res.Content, "expected operand, found '}'", "that error was already in the file")
	assert.Contains(t, res.Content, "already had before the edit")
	assert.Equal(t, 1, res.Metadata["diagnostics"])
	assert.Equal(t, 3, res.Metadata["pre_existing_diagnostics"])
}

func TestWriteFile_DiagnosticsAreBounded(t *testing.T) {
	root := t.TempDir()
	// 40 independent bad declarations: the model must not receive 40 lines of
	// parser output stapled to one write.
	res := writeToolResult(t, root, "flood.go", "package p\n"+strings.Repeat("type = int\n", 40))

	block := res.Content[strings.Index(res.Content, diagnosticsHeader):]
	assert.LessOrEqual(t, len(block), maxPostEditDiagnosticBytes)
	assert.Equal(t, maxPostEditDiagnostics, strings.Count(block, "error:"))
	assert.Contains(t, block, fmt.Sprintf("... %d more diagnostic(s) not shown.", 40-maxPostEditDiagnostics))
}

func TestWriteFile_DiagnosticsDisabled(t *testing.T) {
	root := t.TempDir()
	tool := NewWriteFileTool(root, nil)
	tool.DiagnosticsDisabled = true

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"broken.go","content":"package p\nfunc a() {\n\tx := \n}\n"}`))
	require.NoError(t, err)
	assert.NotContains(t, res.Content, diagnosticsHeader)
}

func TestWriteFile_RemoteBackendSkipsDiagnostics(t *testing.T) {
	backend := &memoryRuntimeBackend{files: map[string][]byte{}}
	tool := NewWriteFileToolWithBackend(backend)

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"broken.go","content":"package p\nfunc a() {\n\tx := \n}\n"}`))
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.NotContains(t, res.Content, diagnosticsHeader, "local analysis says nothing about a remote machine's file")
}

func TestPatchFile_IntroducedErrorIsAppendedBeforeTheDiff(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "app.go"), []byte("package p\n\nfunc a() int {\n\treturn 1\n}\n"), 0o644))

	tool := NewPatchFileTool(root, nil)
	res, err := tool.Execute(context.Background(), json.RawMessage(`{
		"path":"app.go","patches":[{"search":"\treturn 1","replace":"\treturn 1 +"}]
	}`))
	require.NoError(t, err)

	assert.False(t, res.IsError)
	assert.Contains(t, res.Content, "Applied 1 patch(es) to app.go")
	assert.Contains(t, res.Content, diagnosticsHeader)
	assert.Contains(t, res.Content, "app.go:5:1: error:")
	// The TUI feeds everything from the first "--- " onward to the diff parser,
	// so diagnostics placed after the diff would be swallowed into a hunk.
	assert.Less(t, strings.Index(res.Content, diagnosticsHeader), strings.Index(res.Content, "--- app.go"))
}

func TestPatchFile_CleanEditAddsNothing(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "app.go"), []byte("package p\n\nfunc a() int {\n\treturn 1\n}\n"), 0o644))

	tool := NewPatchFileTool(root, nil)
	res, err := tool.Execute(context.Background(), json.RawMessage(`{
		"path":"app.go","patches":[{"search":"\treturn 1","replace":"\treturn 2"}]
	}`))
	require.NoError(t, err)
	assert.NotContains(t, res.Content, diagnosticsHeader)
	assert.NotContains(t, res.Metadata, "diagnostics")
}

func TestPatchFile_NonGoFileAddsNothing(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes.md"), []byte("alpha\n"), 0o644))

	tool := NewPatchFileTool(root, nil)
	res, err := tool.Execute(context.Background(), json.RawMessage(`{
		"path":"notes.md","patches":[{"search":"alpha","replace":"func a() {"}]
	}`))
	require.NoError(t, err)
	assert.NotContains(t, res.Content, diagnosticsHeader)
}

func TestPatchFile_RemoteBackendSkipsDiagnostics(t *testing.T) {
	backend := &memoryRuntimeBackend{files: map[string][]byte{"app.go": []byte("package p\n\nfunc a() int {\n\treturn 1\n}\n")}}
	tool := NewPatchFileToolWithBackend(backend)

	res, err := tool.Execute(context.Background(), json.RawMessage(`{
		"path":"app.go","patches":[{"search":"\treturn 1","replace":"\treturn 1 +"}]
	}`))
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.NotContains(t, res.Content, diagnosticsHeader)
}

func TestSplitPreExistingDiagnostics_CountsDuplicatesIndividually(t *testing.T) {
	before := []codeDiagnostic{{Message: "expected ';'"}}
	after := []codeDiagnostic{{Message: "expected ';'"}, {Message: "expected ';'"}, {Message: "expected '}'"}}

	introduced, preExisting := splitPreExistingDiagnostics(after, before)

	// One occurrence was already there; the second occurrence of the same
	// message is new and must still be reported.
	require.Len(t, introduced, 2)
	assert.Equal(t, "expected ';'", introduced[0].Message)
	assert.Equal(t, "expected '}'", introduced[1].Message)
	assert.Equal(t, 1, preExisting)
}

func TestPostEditDiagnostics_LargeFileIsSkipped(t *testing.T) {
	huge := []byte("package p\n\nfunc a() {\n\tx := \n}\n" + strings.Repeat("// filler\n", maxPostEditSourceBytes/10))
	require.Greater(t, len(huge), maxPostEditSourceBytes)

	assert.Empty(t, postEditDiagnostics("huge.go", huge, nil).Block)
}

func BenchmarkPostEditDiagnostics(b *testing.B) {
	src, err := os.ReadFile("code_intelligence.go")
	if err != nil {
		b.Fatal(err)
	}
	broken := append(append([]byte(nil), src...), []byte("\nfunc broken( {\n")...)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		postEditDiagnostics("code_intelligence.go", broken, src)
	}
}

func BenchmarkPostEditDiagnosticsClean(b *testing.B) {
	src, err := os.ReadFile("code_intelligence.go")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		postEditDiagnostics("code_intelligence.go", src, src)
	}
}
