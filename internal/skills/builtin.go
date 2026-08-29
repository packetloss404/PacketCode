package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// builtinFS holds the skills packetcode ships about its own configuration.
// They are embedded rather than installed into ~/.packetcode so that a fresh
// checkout answers "add a hook that blocks force-pushes" correctly with no
// setup step, and so upgrading the binary upgrades the guidance.
//
// The whole tree is embedded, not just each SKILL.md, so a builtin can carry
// resource files beside its body on the same terms as an installed skill.
//
//go:embed builtin
var builtinFS embed.FS

const builtinRoot = "builtin"

func (r *Registry) loadBuiltins() {
	entries, err := fs.ReadDir(builtinFS, builtinRoot)
	if err != nil {
		r.errors = append(r.errors, fmt.Sprintf("builtin skills: %s", err))
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		data, err := builtinFS.ReadFile(path.Join(builtinRoot, name, FileName))
		if err != nil {
			r.errors = append(r.errors, fmt.Sprintf("builtin skill %q: %s", name, err))
			continue
		}
		// Path stays empty: there is no on-disk file to report. Dir is an
		// embedded-FS path, which only ReadResource knows how to open.
		skill, err := newSkill(name, string(data), SourceBuiltin, "", path.Join(builtinRoot, name))
		if err != nil {
			// A malformed builtin is a build-time mistake, not user state; the
			// package test asserts this list stays empty.
			r.errors = append(r.errors, fmt.Sprintf("builtin skill %q: %s", name, err))
			continue
		}
		r.upsert(skill)
	}
}

// BuiltinNames lists the embedded skills. Useful to callers that want to show
// what ships in the box without resolving user and project scopes first.
func BuiltinNames() []string {
	entries, err := fs.ReadDir(builtinFS, builtinRoot)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			out = append(out, entry.Name())
		}
	}
	return out
}
