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

// PatchOp is a single search/replace operation. Exported so callers
// outside this package (the approval renderer) can type-check their
// decoded Patches slice. JSON tags are unchanged — the wire format is
// identical to the pre-rename unexported struct.
type PatchOp struct {
	Search  string `json:"search"`
	Replace string `json:"replace"`
}

type patchFileParams struct {
	Path    string    `json:"path"`
	Patches []PatchOp `json:"patches"`
}

// applyPatches is the shared core used by both Execute (which then
// writes the file) and PreviewPatchDiff (which does not). Validation
// errors are returned verbatim — callers prepend their own tool-name
// prefix when building a ToolResult.Content. Diff labels use the
// "(current)" / "(proposed)" wording so the approval modal and the
// post-apply conversation block read the same way.
func applyPatches(original string, patches []PatchOp, path string) (updated, unified string, err error) {
	if len(patches) == 0 {
		return "", "", fmt.Errorf("at least one patch is required")
	}
	updated = original
	for i, op := range patches {
		if op.Search == "" {
			return "", "", fmt.Errorf("patch #%d has empty search string", i+1)
		}
		count := strings.Count(updated, op.Search)
		if count == 0 {
			return "", "", fmt.Errorf("patch #%d search text not found in %s", i+1, path)
		}
		if count > 1 {
			return "", "", fmt.Errorf("patch #%d search text matches %d times in %s; must be unique", i+1, count, path)
		}
		updated = strings.Replace(updated, op.Search, op.Replace, 1)
	}
	unified, _ = difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(original),
		B:        difflib.SplitLines(updated),
		FromFile: path + " (current)",
		ToFile:   path + " (proposed)",
		Context:  3,
	})
	return updated, unified, nil
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

	updated, diff, err := applyPatches(string(original), p.Patches, p.Path)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("patch_file: %s", err), IsError: true}, nil
	}

	if err := t.Backups.Backup(abs); err != nil {
		return ToolResult{Content: fmt.Sprintf("patch_file: backup failed: %s", err), IsError: true}, nil
	}
	if err := atomicWrite(abs, []byte(updated)); err != nil {
		return ToolResult{Content: fmt.Sprintf("patch_file: %s", err), IsError: true}, nil
	}

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

// PreviewPatchDiff computes the unified diff a Execute call would
// produce, without writing the file. Used by the approval renderer so
// the user sees a real diff in the confirmation modal. Validation
// errors bubble out as-is so the caller can distinguish "bad patches"
// from "diff ready".
func (t *PatchFileTool) PreviewPatchDiff(path string, patches []PatchOp) (string, error) {
	abs, err := resolveInRoot(t.Root, path)
	if err != nil {
		return "", err
	}
	original, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	_, unified, err := applyPatches(string(original), patches, path)
	if err != nil {
		return "", err
	}
	return unified, nil
}
