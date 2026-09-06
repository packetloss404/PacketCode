package skills

import (
	"fmt"
	"strings"
)

// ShellTool is the packetcode tool that runs a command, and the only tool an
// argument-scoped grant can name. It is the only registered tool whose
// parameters contain a shell program, so it is the only one the permission
// policy has a rule shape for -- see permissions.Policy.WithCommandPrefixRule.
const ShellTool = "execute_command"

// ToolGrant is one entry of a skill's `allowed-tools` list.
//
// The zero scope -- no Command, no CommandPrefix -- is a grant of the whole
// tool, which is what a bare `read_file` in the header means. At most one of
// the two scope fields is ever set.
type ToolGrant struct {
	// Tool is a packetcode tool name, already translated from the ecosystem
	// spelling the author wrote. See shellToolName.
	Tool string
	// Command is a complete shell program the grant named exactly, from
	// `execute_command(git status)`. Matched byte-for-byte.
	Command string
	// CommandPrefix is the leading words a command must start with, from the
	// `:*` form: `Bash(npm run test:*)` is {"npm", "run", "test"}.
	CommandPrefix []string
}

// Scoped reports whether the grant is narrowed to particular commands rather
// than covering the whole tool.
func (g ToolGrant) Scoped() bool { return g.Command != "" || len(g.CommandPrefix) > 0 }

// Label renders the grant the way it is reported to the user.
func (g ToolGrant) Label() string {
	switch {
	case len(g.CommandPrefix) > 0:
		return fmt.Sprintf("%s `%s …`", g.Tool, strings.Join(g.CommandPrefix, " "))
	case g.Command != "":
		return fmt.Sprintf("%s `%s`", g.Tool, g.Command)
	default:
		return g.Tool
	}
}

func (g ToolGrant) clone() ToolGrant {
	g.CommandPrefix = append([]string(nil), g.CommandPrefix...)
	return g
}

// shellToolName translates the spellings a published skill uses for "run a
// shell command" onto packetcode's tool name.
//
// This is the one name packetcode translates, and only ever for a grant that
// carries a scope. A bare `Bash` still matches nothing and is reported,
// because turning it into `execute_command` would hand a foreign skill the
// whole shell on a guess. `Bash(gh:*)` is not that guess: the author wrote
// both the tool family and the narrowing, and the most the translation can
// produce is permission to run `gh …` -- strictly less than the bare-name
// grant that is refused. The specifier is what makes the translation safe.
func shellToolName(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case ShellTool, "bash":
		return ShellTool, true
	}
	return "", false
}

// parseAllowedTools reads the comma-separated `allowed-tools` list.
//
// A name carrying a parenthesised specifier -- `Bash(gh:*)`, the ecosystem's
// way of narrowing a grant to particular commands -- is honoured for the shell
// tool and refused for every other, never reduced to its bare name. Dropping
// the parentheses would turn permission to run one command into permission to
// run any, which is the opposite of what the author wrote.
//
// Refused, for anything but the shell tool, because the specifier then means
// something packetcode's policy has no rule shaped like: `Read(src/**)` is a
// path glob, and there is no narrowing available for the file tools. Granting
// nothing and saying so is the only honest answer to a scope that cannot be
// expressed.
func (fm *Frontmatter) parseAllowedTools(val string) []ToolGrant {
	var out []ToolGrant
	var refused []string
	for _, raw := range splitGrants(val) {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		open := strings.Index(entry, "(")
		if open < 0 {
			if strings.Contains(entry, ")") {
				refused = append(refused, entry)
				continue
			}
			out = append(out, ToolGrant{Tool: entry})
			continue
		}
		// Everything to the last `)`, so a specifier containing parentheses
		// of its own survives intact rather than being cut in half.
		if !strings.HasSuffix(entry, ")") {
			refused = append(refused, entry)
			continue
		}
		grant, ok := scopedGrant(entry[:open], entry[open+1:len(entry)-1])
		if !ok {
			refused = append(refused, entry)
			continue
		}
		out = append(out, grant)
	}
	if len(refused) > 0 {
		fm.Warnings = append(fm.Warnings, refusalWarning(refused))
	}
	return out
}

// scopedGrant builds the grant for `tool(spec)`, or reports that the scope
// cannot be expressed as a permission rule.
func scopedGrant(tool, spec string) (ToolGrant, bool) {
	name, ok := shellToolName(tool)
	if !ok {
		return ToolGrant{}, false
	}
	spec = strings.TrimSpace(spec)
	// `Bash(*)` is the whole shell with punctuation on it, which is the
	// bare-name grant this declines to guess at rather than a narrowing.
	if spec == "" || spec == "*" || spec == ":*" {
		return ToolGrant{}, false
	}
	if rest, cut := strings.CutSuffix(spec, ":*"); cut {
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return ToolGrant{}, false
		}
		return ToolGrant{Tool: name, CommandPrefix: fields}, true
	}
	return ToolGrant{Tool: name, Command: spec}, true
}

// splitGrants splits the list on commas that are not inside a specifier, so
// `Bash(git add, git commit)` stays one entry rather than becoming two
// malformed ones.
func splitGrants(val string) []string {
	var (
		out   []string
		cur   strings.Builder
		depth int
	)
	for _, r := range val {
		switch {
		case r == '(':
			depth++
		case r == ')' && depth > 0:
			depth--
		case r == ',' && depth == 0:
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteRune(r)
	}
	out = append(out, cur.String())
	return out
}

// refusalWarning is one line per skill, not one per grant. A skill listing
// eight scoped entries -- which is what a real ported skill looks like --
// otherwise produced eight near-identical warnings on every startup, and a
// warning repeated eight times is one nobody reads the ninth time.
func refusalWarning(refused []string) string {
	shown := refused
	const cap = 3
	suffix := ""
	if len(shown) > cap {
		shown = shown[:cap]
		suffix = fmt.Sprintf(" and %d more", len(refused)-cap)
	}
	one := len(refused) == 1
	return fmt.Sprintf(
		"allowed-tools: %s%s %s a grant to particular arguments, which packetcode can express "+
			"only for %s; nothing was granted for %s",
		strings.Join(quoteAll(shown), ", "), suffix,
		map[bool]string{true: "narrows", false: "narrow"}[one],
		ShellTool,
		map[bool]string{true: "it", false: "them"}[one])
}

func quoteAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, fmt.Sprintf("%q", s))
	}
	return out
}
