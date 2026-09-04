package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pmezard/go-difflib/difflib"

	"github.com/packetcode/packetcode/internal/computers"
)

const approvalPreviewTimeout = 10 * time.Second

const writeFileSchema = `{
  "type": "object",
  "properties": {
    "path":    { "type": "string", "description": "File path relative to the project root. Parent directories are created as needed." },
    "content": { "type": "string", "description": "Complete file contents. Overwrites any existing file." }
  },
  "required": ["path", "content"]
}`

type WriteFileTool struct {
	Root       string
	Backups    BackupManager
	Backend    computers.RuntimeBackend
	backendErr error

	// DiagnosticsDisabled suppresses the post-edit diagnostics block. The zero
	// value keeps them on so a tool built by struct literal behaves like one
	// built by the constructors.
	DiagnosticsDisabled bool
}

func NewWriteFileTool(root string, backups BackupManager) *WriteFileTool {
	if backups == nil {
		backups = NoopBackupManager()
	}
	backend, err := computers.NewLocalBackend(root)
	return &WriteFileTool{Root: root, Backups: backups, Backend: backend, backendErr: err}
}

func NewWriteFileToolWithBackend(backend computers.RuntimeBackend) *WriteFileTool {
	if backend == nil {
		return &WriteFileTool{Backups: NoopBackupManager(), backendErr: fmt.Errorf("runtime backend is nil")}
	}
	return &WriteFileTool{Root: backend.Root(), Backups: NoopBackupManager(), Backend: backend}
}

func (*WriteFileTool) Name() string            { return "write_file" }
func (*WriteFileTool) RequiresApproval() bool  { return true }
func (*WriteFileTool) Schema() json.RawMessage { return json.RawMessage(writeFileSchema) }
func (t *WriteFileTool) Description() string {
	if t.Backend != nil && t.Backend.Kind() == computers.KindSSH {
		return "Write a complete remote file using SFTP temp-file plus rename. Requires user approval. Remote /undo is unavailable."
	}
	return "Write a complete file (creating it or overwriting an existing file). Requires user approval. Backs up the previous contents so /undo can revert."
}

type writeFileParams struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Execute writes atomically: it streams content to a temp file in the
// destination directory, closes it, then renames it into place. This
// guards against half-written files if the process is killed mid-write.
func (t *WriteFileTool) Execute(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
	var p writeFileParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return ToolResult{}, fmt.Errorf("write_file: parse params: %w", err)
	}
	if t.backendErr != nil {
		return ToolResult{Content: fmt.Sprintf("write_file: %s", t.backendErr), IsError: true}, nil
	}
	resolved, err := t.Backend.Resolve(ctx, p.Path, true)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("write_file: %s", err), IsError: true}, nil
	}

	if err := t.Backups.Backup(resolved); err != nil {
		return ToolResult{Content: fmt.Sprintf("write_file: backup failed: %s", err), IsError: true}, nil
	}

	// Captured before the overwrite, and only when diagnostics can use it: the
	// pre-edit contents are the only way to tell an error this edit introduced
	// from one the file already had. fileTooLarge also covers the missing-file
	// case, and nil is the right answer there — every diagnostic in a brand new
	// file was introduced by the write that created it.
	analyze := t.postEditDiagnosticsEnabled(p.Path) && len(p.Content) <= maxPostEditSourceBytes
	var prior []byte
	if analyze && !fileTooLarge(resolved, maxPostEditSourceBytes) {
		readCtx, cancel := context.WithTimeout(ctx, postEditPriorReadTimeout)
		prior, _ = t.Backend.ReadFile(readCtx, p.Path)
		cancel()
	}

	if err := t.Backend.WriteFile(ctx, p.Path, []byte(p.Content)); err != nil {
		rollbackBackup(t.Backups, resolved)
		return ToolResult{Content: fmt.Sprintf("write_file: %s", err), IsError: true}, nil
	}

	lineCount := strings.Count(p.Content, "\n")
	if !strings.HasSuffix(p.Content, "\n") && len(p.Content) > 0 {
		lineCount++
	}
	content := fmt.Sprintf("Wrote %s (%d bytes, %d lines).", p.Path, len(p.Content), lineCount)
	metadata := map[string]any{
		"path":  p.Path,
		"bytes": len(p.Content),
		"lines": lineCount,
	}
	if analyze {
		if report := postEditDiagnostics(p.Path, []byte(p.Content), prior); report.Block != "" {
			// Appended, never substituted, and IsError stays false: the write
			// did succeed, and a model that reads this as a failure would undo
			// or repeat a good edit.
			content += "\n\n" + report.Block
			metadata["diagnostics"] = report.Introduced
			metadata["pre_existing_diagnostics"] = report.PreExisting
		}
	}
	return ToolResult{Content: content, Metadata: metadata}, nil
}

// postEditDiagnosticsEnabled reports whether this write should be analysed.
// Remote backends are skipped: the analysis is local and in-process, and a file
// on a Packet Computer belongs to that machine's toolchain and build context,
// not this one's.
func (t *WriteFileTool) postEditDiagnosticsEnabled(path string) bool {
	if t.DiagnosticsDisabled || t.Backend == nil || t.Backend.Kind() != computers.KindLocal {
		return false
	}
	return postEditDiagnosticsSupported(path)
}

// PreviewDiff computes the unified diff between the on-disk contents
// of path and the proposed content, without writing anything. Used by
// the approval renderer to show a real diff in the confirmation
// modal.
//
// Return shape:
//   - unified == "" && newFile == true  → path does not yet exist
//     (caller should render with diff.NewFile)
//   - unified == "" && newFile == false → proposed == current, no-op
//   - unified != ""                     → normal overwrite diff
//
// Mirrors read_file's binary guard so approval never attempts to
// render an invalid-UTF-8 diff.
func (t *WriteFileTool) PreviewDiff(path, content string) (string, bool, error) {
	if t.backendErr != nil {
		return "", false, t.backendErr
	}
	ctx, cancel := context.WithTimeout(context.Background(), approvalPreviewTimeout)
	defer cancel()
	data, err := t.Backend.ReadFile(ctx, path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", true, nil
		}
		return "", false, fmt.Errorf("preview_diff: %w", err)
	}
	if !utf8.Valid(data) {
		return "", false, fmt.Errorf("preview_diff: %s appears to be binary", path)
	}
	if string(data) == content {
		return "", false, nil
	}
	unified, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(data)),
		B:        difflib.SplitLines(content),
		FromFile: path + " (current)",
		ToFile:   path + " (proposed)",
		Context:  3,
	})
	return unified, false, nil
}
