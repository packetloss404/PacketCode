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
//go:embed builtin/*/SKILL.md
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
		skill, err := newSkill(name, string(data), SourceBuiltin, "")
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
