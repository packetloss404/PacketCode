package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

const patchFileSchema = `{
  "type": "object",
  "properties": {
    "path":    { "type": "string", "description": "File path relative to the project root. Must already exist." },
    "patches": {
      "type": "array",
      "description": "Ordered list of search/replace operations applied in sequence.",
      "items": {
        "type": "object",
        "properties": {
          "search":  { "type": "string", "description": "Exact text to find. Must match exactly once." },
          "replace": { "type": "string", "description": "Replacement text." }
        },
        "required": ["search", "replace"]
      }
    }
  },
  "required": ["path", "patches"]
}`

type PatchFileTool struct {
	Root    string
	Backups BackupManager
}

func NewPatchFileTool(root string, backups BackupManager) *PatchFileTool {
	if backups == nil {
		backups = NoopBackupManager()
	}
	return &PatchFileTool{Root: root, Backups: backups}
}

func (*PatchFileTool) Name() string             { return "patch_file" }
func (*PatchFileTool) RequiresApproval() bool   { return true }
func (*PatchFileTool) Schema() json.RawMessage  { return json.RawMessage(patchFileSchema) }
func (*PatchFileTool) Description() string {
	return "Apply one or more search/replace patches to an existing file. Each search must appear exactly once. Returns a unified diff. Requires user approval."
}

type patchOp struct {
	Search  string `json:"search"`
	Replace string `json:"replace"`
}

type patchFileParams struct {
	Path    string    `json:"path"`
	Patches []patchOp `json:"patches"`
}

func (t *PatchFileTool) Execute(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
	var p patchFileParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return ToolResult{}, fmt.Errorf("patch_file: parse params: %w", err)
	}
	if len(p.Patches) == 0 {
		return ToolResult{Content: "patch_file: at least one patch is required", IsError: true}, nil
	}

	abs, err := resolveInRoot(t.Root, p.Path)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}
	original, err := os.ReadFile(abs)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("patch_file: %s", err), IsError: true}, nil
	}

	updated := string(original)
	for i, op := range p.Patches {
		if op.Search == "" {
			return ToolResult{Content: fmt.Sprintf("patch_file: patch #%d has empty search string", i+1), IsError: true}, nil
		}
		count := strings.Count(updated, op.Search)
		if count == 0 {
			return ToolResult{
				Content: fmt.Sprintf("patch_file: patch #%d search text not found in %s", i+1, p.Path),
				IsError: true,
			}, nil
		}
		if count > 1 {
			return ToolResult{
				Content: fmt.Sprintf("patch_file: patch #%d search text matches %d times in %s; must be unique", i+1, count, p.Path),
				IsError: true,
			}, nil
		}
		updated = strings.Replace(updated, op.Search, op.Replace, 1)
	}

	if err := t.Backups.Backup(abs); err != nil {
		return ToolResult{Content: fmt.Sprintf("patch_file: backup failed: %s", err), IsError: true}, nil
	}
	if err := atomicWrite(abs, []byte(updated)); err != nil {
		return ToolResult{Content: fmt.Sprintf("patch_file: %s", err), IsError: true}, nil
	}

	diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(original)),
		B:        difflib.SplitLines(updated),
		FromFile: p.Path + " (original)",
		ToFile:   p.Path + " (patched)",
		Context:  3,
	})

	return ToolResult{
		Content: fmt.Sprintf("Applied %d patch(es) to %s.\n\n%s", len(p.Patches), p.Path, diff),
		Metadata: map[string]any{
			"path":          p.Path,
			"patch_count":   len(p.Patches),
			"original_size": len(original),
			"updated_size":  len(updated),
		},
	}, nil
}
