package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const writeFileSchema = `{
  "type": "object",
  "properties": {
    "path":    { "type": "string", "description": "File path relative to the project root. Parent directories are created as needed." },
    "content": { "type": "string", "description": "Complete file contents. Overwrites any existing file." }
  },
  "required": ["path", "content"]
}`

type WriteFileTool struct {
	Root    string
	Backups BackupManager
}

func NewWriteFileTool(root string, backups BackupManager) *WriteFileTool {
	if backups == nil {
		backups = NoopBackupManager()
	}
	return &WriteFileTool{Root: root, Backups: backups}
}

func (*WriteFileTool) Name() string             { return "write_file" }
func (*WriteFileTool) RequiresApproval() bool   { return true }
func (*WriteFileTool) Schema() json.RawMessage  { return json.RawMessage(writeFileSchema) }
func (*WriteFileTool) Description() string {
	return "Write a complete file (creating it or overwriting an existing file). Requires user approval. Backs up the previous contents so /undo can revert."
}

type writeFileParams struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Execute writes atomically: it streams content to a temp file in the
// destination directory, fsync-equivalent on close, then renames into
// place. This guards against half-written files if the process is killed
// mid-write.
func (t *WriteFileTool) Execute(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
	var p writeFileParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return ToolResult{}, fmt.Errorf("write_file: parse params: %w", err)
	}
	abs, err := resolveInRoot(t.Root, p.Path)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	if err := t.Backups.Backup(abs); err != nil {
		return ToolResult{Content: fmt.Sprintf("write_file: backup failed: %s", err), IsError: true}, nil
	}

	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolResult{Content: fmt.Sprintf("write_file: create parent dir: %s", err), IsError: true}, nil
	}

	tmp, err := os.CreateTemp(dir, ".write.*.tmp")
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("write_file: create temp: %s", err), IsError: true}, nil
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(p.Content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return ToolResult{Content: fmt.Sprintf("write_file: write temp: %s", err), IsError: true}, nil
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return ToolResult{Content: fmt.Sprintf("write_file: close temp: %s", err), IsError: true}, nil
	}
	if err := os.Rename(tmpPath, abs); err != nil {
		_ = os.Remove(tmpPath)
		return ToolResult{Content: fmt.Sprintf("write_file: rename: %s", err), IsError: true}, nil
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
