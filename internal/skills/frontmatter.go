package skills

import (
	"fmt"
	"strings"
)

// Frontmatter is everything the loader reads out of a SKILL.md header.
//
// The two invocation flags are stored as their negative ("disabled") so the
// zero value is permissive: a skill that says nothing is invocable by both a
// human and the model, which is what every skill written before these existed
// meant.
type Frontmatter struct {
	Description string
	// DisableModelInvocation mirrors `disable-model-invocation: true`. The
	// model may not select the skill, and it is kept out of the index so the
	// model is not told about something it cannot use. This is what makes a
	// destructive skill safe to keep beside the others: /deploy stays a thing
	// a person does.
	DisableModelInvocation bool
	// DisableUserInvocation mirrors `user-invocable: false` -- background
	// knowledge the model may consult but a person does not trigger directly.
	DisableUserInvocation bool
	// Warnings names header values that were not understood.
	//
	// A flag whose value does not parse keeps its default, and for both of
	// these the default is permissive -- so a typo on the line that says "the
	// model must not run this" otherwise means "the model may run this", with
	// nothing said anywhere. The caller surfaces these. Nothing here refuses a
	// skill over one: a bad flag is not a reason to make reference material
	// vanish, it is a reason to say so.
	Warnings []string
}

// ParseFrontmatter keeps the two-value form the rest of the loader uses.
func ParseFrontmatter(raw string) (body, description string) {
	body, fm := ParseFrontmatterFields(raw)
	return body, fm.Description
}

// ParseFrontmatterFields is ParseFrontmatter plus the invocation flags.
//
// Names and defaults follow Claude Code: `user-invocable` defaults to true and
// `disable-model-invocation` to false, so a skill that sets neither is
// reachable both ways. crush spells the two keys identically but takes them as
// plain unmarshalled bools, which makes an absent `user-invocable` mean false
// -- opt-in rather than opt-out. Claude Code is the ecosystem a published
// skill is written against, so its defaults are the ones followed here; a
// skill authored for crush gains a typed verb it did not ask for, which is the
// safe direction of that disagreement.
func ParseFrontmatterFields(raw string) (body string, fm Frontmatter) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.TrimPrefix(raw, "\ufeff")
	if !strings.HasPrefix(raw, "---\n") {
		return raw, Frontmatter{}
	}
	rest := strings.TrimPrefix(raw, "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return raw, Frontmatter{}
	}
	header := rest[:end]
	seen := map[string]bool{}
	body = strings.TrimPrefix(rest[end:], "\n---")
	body = strings.TrimPrefix(body, "\n")
	for _, line := range strings.Split(header, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		name := strings.TrimSpace(strings.ToLower(key))
		if seen[name] {
			// First occurrence wins, for every key. Last-wins would let a
			// second, unparseable `user-invocable:` line lift the restriction
			// the first one asked for -- a duplicate key is a mistake, and the
			// resolution of a mistake must not be "the safer of the two loses".
			if name == "description" || name == "disable-model-invocation" || name == "user-invocable" {
				fm.Warnings = append(fm.Warnings,
					fmt.Sprintf("%s appears more than once; the first value was used", name))
			}
			continue
		}
		switch name {
		case "description":
			seen[name] = true
			if isBlockScalar(val) {
				// `description: >-` with the text on following indented lines.
				// This parser is line-based, so the value here is ">-", which
				// is non-empty and single-line and therefore passes every
				// validation downstream -- the skill loads with a meaningless
				// index entry and nothing reports it.
				fm.Warnings = append(fm.Warnings,
					"description uses a YAML block scalar, which is not supported; put it on one line")
				fm.Description = ""
				continue
			}
			fm.Description = val
		case "disable-model-invocation":
			seen[name] = true
			fm.DisableModelInvocation = fm.parseFlag(name, val, false)
		case "user-invocable":
			seen[name] = true
			// Stored inverted: the field says who MAY, the struct says who may
			// not, so an absent field stays permissive.
			fm.DisableUserInvocation = !fm.parseFlag(name, val, true)
		}
	}
	return body, fm
}

// parseFlag reads one boolean header value, recording anything it could not
// understand rather than quietly falling back to the permissive default.
func (fm *Frontmatter) parseFlag(name, val string, def bool) bool {
	got, ok := parseFrontmatterBool(stripInlineComment(val))
	if !ok {
		fm.Warnings = append(fm.Warnings, fmt.Sprintf(
			"%s: %q is not a true/false value, so the default (%v) was used", name, val, def))
		return def
	}
	return got
}

// stripInlineComment drops a YAML trailing comment from a scalar value.
//
// `disable-model-invocation: true  # destructive, humans only` is close to the
// most likely thing an author writes on exactly that line, and without this
// the value is `true  # destructive, humans only`, which matches no spelling
// and leaves the flag off -- the restriction silently not applied. YAML wants
// whitespace before an inline `#`, which is what keeps a description reading
// "#1 priority" intact.
func stripInlineComment(val string) string {
	for i := 0; i < len(val); i++ {
		if val[i] != '#' {
			continue
		}
		if i == 0 || val[i-1] == ' ' || val[i-1] == '\t' {
			return strings.TrimSpace(val[:i])
		}
	}
	return val
}

// isBlockScalar reports a `>`/`|` folding indicator standing alone as a value.
func isBlockScalar(val string) bool {
	switch strings.TrimSpace(val) {
	case ">", ">-", ">+", "|", "|-", "|+":
		return true
	}
	return false
}

// parseFrontmatterBool reports the value and whether it was understood at all.
// The spellings match Claude Code, which accepts yes/no/on/off/1/0 in any case
// alongside true/false. An unrecognised value is reported, not absorbed.
func parseFrontmatterBool(val string) (value, ok bool) {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "true", "yes", "on", "1":
		return true, true
	case "false", "no", "off", "0":
		return false, true
	default:
		return false, false
	}
}

// ValidName reports whether name is usable as a skill name. It is an
// allowlist, which is what makes it safe for callers to rely on: a
// qualified lookup key of the form "source:name" is unambiguous only
// because ':' is not in the set, and the same holds for '/', '\' and '.'.
// Widening this set is not a local change -- check every caller that builds
// a composite key or a path from a skill name first.
//
// The rule matches
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
