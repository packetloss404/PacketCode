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

	if err := t.Backend.WriteFile(ctx, p.Path, []byte(p.Content)); err != nil {
		rollbackBackup(t.Backups, resolved)
		return ToolResult{Content: fmt.Sprintf("write_file: %s", err), IsError: true}, nil
	}

	lineCount := strings.Count(p.Content, "\n")
	if !strings.HasSuffix(p.Content, "\n") && len(p.Content) > 0 {
		lineCount++
	}
	return ToolResult{
		Content: fmt.Sprintf("Wrote %s (%d bytes, %d lines).", p.Path, len(p.Content), lineCount),
		Metadata: map[string]any{
			"path":  p.Path,
			"bytes": len(p.Content),
			"lines": lineCount,
		},
	}, nil
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
