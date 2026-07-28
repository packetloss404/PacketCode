package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// @-file mentions: when a user writes @path/to/file anywhere in a prompt,
// packetcode reads that file and includes its contents in the message sent to
// the model (the displayed user message keeps the literal @mention). This is
// the "pull a file into context" half of Claude Code's @-mentions; a
// fuzzy-find popup on top of it is a separate enhancement.

// mentionMaxBytes caps how much of a single referenced file is inlined.
const mentionMaxBytes = 256 * 1024

// mentionCharClass is the character class for a path-like @mention token,
// shared between the submit-time expander (mentionPattern) and the
// interactive autocomplete popup (activeMentionToken). Keeping it in one
// place means the popup accepts exactly the tokens the expander will later
// resolve.
const mentionCharClass = `A-Za-z0-9_./~-`

// mentionPattern matches @ followed by a path-like token. Trailing sentence
// punctuation is trimmed afterward so "see @main.go." doesn't swallow the dot.
var mentionPattern = regexp.MustCompile(`@([` + mentionCharClass + `]+)`)

// expandFileMentions scans prompt for @<path> tokens, reads each that resolves
// to a readable text file within root, and returns the prompt augmented with
// those files' contents (prepended as context blocks) plus the sorted list of
// attached relative paths. Tokens that don't resolve to a readable in-project
// file are left untouched (they remain literal text in the prompt). When
// nothing resolves, the prompt is returned unchanged and attached is empty.
func expandFileMentions(prompt, root string) (expanded string, attached []string) {
	matches := mentionPattern.FindAllStringSubmatch(prompt, -1)
	if len(matches) == 0 {
		return prompt, nil
	}
	seen := map[string]string{} // rel path -> contents
	for _, m := range matches {
		raw := strings.TrimRight(m[1], ".,;:)!?") // strip trailing sentence punctuation
		if raw == "" {
			continue
		}
		rel, content, ok := readMention(raw, root)
		if !ok {
			continue
		}
		if _, dup := seen[rel]; !dup {
			seen[rel] = content
		}
	}
	if len(seen) == 0 {
		return prompt, nil
	}

	attached = make([]string, 0, len(seen))
	for rel := range seen {
		attached = append(attached, rel)
	}
	sort.Strings(attached)

	var b strings.Builder
	b.WriteString("The user referenced these files with @ mentions; their current contents:\n\n")
	for _, rel := range attached {
		fmt.Fprintf(&b, "<file path=%q>\n%s\n</file>\n\n", rel, seen[rel])
	}
	b.WriteString(prompt)
	return b.String(), attached
}

// readMention resolves a raw @token to an in-project text file and returns its
// project-relative path and contents. ok is false when the token doesn't point
// at a readable regular text file inside root.
func readMention(raw, root string) (rel, content string, ok bool) {
	// Resolve ~ and relative paths against root.
	p := raw
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	p = filepath.Clean(p)

	// Must stay within the project root (typed path, but don't read ~/.ssh).
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", false
	}
	relToRoot, err := filepath.Rel(absRoot, p)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", "", false
	}

	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		return "", "", false
	}

	data, err := os.ReadFile(p)
	if err != nil {
		return "", "", false
	}
	if isBinary(data) {
		return "", "", false
	}

	truncated := false
	if len(data) > mentionMaxBytes {
		data = data[:mentionMaxBytes]
		truncated = true
	}
	text := string(data)
	if truncated {
		text += "\n… [truncated]"
	}
	return filepath.ToSlash(relToRoot), text, true
}

// isBinary reports whether data looks non-text (contains a NUL in the first
// chunk), so we don't inline binaries.
func isBinary(data []byte) bool {
	n := len(data)
	if n > 8000 {
		n = 8000
	}
	return bytes.IndexByte(data[:n], 0) >= 0
}
