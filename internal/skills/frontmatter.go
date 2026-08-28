package skills

import "strings"

// ParseFrontmatter splits a markdown file into its body and its frontmatter
// `description`, accepting exactly the dialect packetcode's markdown slash
// commands already use: an optional leading `---` block whose only recognised
// key is `description`, everything after it being the body.
//
// This is deliberately the same algorithm as internal/app's
// parseMarkdownCommandFile, which is unexported and therefore unreachable from
// here. This copy is meant to be the canonical one: internal/app should
// delegate to it rather than keeping two parsers that can drift apart on what
// counts as a valid skill or command file.
func ParseFrontmatter(raw string) (body, description string) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.TrimPrefix(raw, "\ufeff")
	if !strings.HasPrefix(raw, "---\n") {
		return raw, ""
	}
	rest := strings.TrimPrefix(raw, "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return raw, ""
	}
	fm := rest[:end]
	body = strings.TrimPrefix(rest[end:], "\n---")
	body = strings.TrimPrefix(body, "\n")
	for _, line := range strings.Split(fm, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(strings.ToLower(key)) != "description" {
			continue
		}
		description = strings.TrimSpace(val)
		description = strings.Trim(description, `"'`)
		break
	}
	return body, description
}

// ValidName reports whether name is usable as a skill name. The rule matches
// validSlashCommandName in internal/app, for the same reason: the name is a
// directory name and a model-supplied tool argument, so it must not be able to
// carry a path separator or traversal.
func ValidName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}
