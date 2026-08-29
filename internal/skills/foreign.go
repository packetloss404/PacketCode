package skills

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/packetcode/packetcode/internal/config"
)

// Origin records which directory layout a skill was found under, independently
// of Source, which records its trust scope.
//
// The two are genuinely different questions. Source answers "who wrote this",
// which is what decides whether the body carries an untrusted label. Origin
// answers "which convention was it filed under", which is what a person needs
// when a skill they installed for another agent does not behave here. Folding
// them together would make `/skills` unable to tell someone that the deploy
// skill it is describing is the one in `.claude/`, not the one they just
// edited in `.packetcode/`.
const (
	// OriginNative is packetcode's own layout, `.packetcode/skills/`.
	OriginNative = "packetcode"
	// OriginClaude is `.claude/skills/`, Claude Code's layout and the one
	// almost every published skill actually ships under.
	OriginClaude = "claude"
	// OriginAgents is `.agents/skills/`, the vendor-neutral Agent Skills
	// location.
	OriginAgents = "agents"
)

// layout is one directory convention, and the order of skillLayouts is the
// precedence within a scope: later entries displace earlier ones, so the
// native layout is listed last and wins.
//
// That direction is deliberate. If both `.packetcode/skills/deploy` and
// `.claude/skills/deploy` exist in one project, the packetcode-specific one
// was written for packetcode by someone who knew this program would read it,
// and the `.claude` one may have arrived with a checkout aimed at a different
// agent entirely. The more deliberate file wins.
type layout struct {
	// base is the directory under the scope root, e.g. ".claude".
	base string
	// origin is the Origin constant for skills found under it.
	origin string
	// foreign is false only for packetcode's own layout. A foreign layout at
	// project scope is the one that needs approval before it does anything:
	// see NeedsApproval.
	foreign bool
}

var skillLayouts = []layout{
	{base: ".agents", origin: OriginAgents, foreign: true},
	{base: ".claude", origin: OriginClaude, foreign: true},
	{base: ".packetcode", origin: OriginNative, foreign: false},
}

// UserSkillsDir returns ~/.packetcode/skills/. It is not created: an absent
// user skills directory is the normal case, not a state to be materialised.
func UserSkillsDir() (string, error) {
	home, err := config.ResolveHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, dirName), nil
}

// ProjectSkillsDir returns <working-dir>/.packetcode/skills/.
func ProjectSkillsDir(workingDir string) string {
	if strings.TrimSpace(workingDir) == "" {
		workingDir = "."
	}
	return filepath.Join(workingDir, ".packetcode", dirName)
}

// skillScope is one directory to scan, with everything the loader needs to
// label what it finds there.
type skillScope struct {
	path   string
	source string
	origin string
	// foreignProject marks a foreign layout at project scope -- repository
	// content filed under another agent's convention, which is the one
	// combination that does not load until a human says so.
	foreignProject bool
}

// userScopeDirs returns the three user-scope directories, weakest first.
//
// Only the packetcode one lives under the packetcode data home, which
// PACKETCODE_HOME relocates. `~/.claude` and `~/.agents` are other programs'
// directories in the operating system's home and are not relocated by it --
// pointing PACKETCODE_HOME at a scratch directory must not silently change
// which other-agent skills are visible, in either direction.
func userScopeDirs() ([]skillScope, error) {
	var out []skillScope
	osHome, osErr := os.UserHomeDir()
	for _, l := range skillLayouts {
		if !l.foreign {
			dir, err := UserSkillsDir()
			if err != nil {
				return out, err
			}
			out = append(out, skillScope{path: dir, source: SourceUser, origin: l.origin})
			continue
		}
		if osErr != nil {
			continue
		}
		out = append(out, skillScope{
			path:   filepath.Join(osHome, l.base, dirName),
			source: SourceUser,
			origin: l.origin,
		})
	}
	return out, osErr
}

// projectScopeDirs returns the three project-scope directories, weakest first.
func projectScopeDirs(workingDir string) []skillScope {
	if strings.TrimSpace(workingDir) == "" {
		return nil
	}
	out := make([]skillScope, 0, len(skillLayouts))
	for _, l := range skillLayouts {
		out = append(out, skillScope{
			path:           filepath.Join(workingDir, l.base, dirName),
			source:         SourceProject,
			origin:         l.origin,
			foreignProject: l.foreign,
		})
	}
	return out
}
