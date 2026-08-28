// Package skills implements model-initiated retrieval of reference material.
//
// A skill is a markdown file with frontmatter, discovered from
// `<project>/.packetcode/skills/<name>/SKILL.md` and `~/.packetcode/skills/`,
// plus a set embedded at build time. Only each skill's name and description
// reach the system prompt (IndexBlock); a body enters context only when the
// model asks for it by name through the `skill` tool. That split is the whole
// point — an unused skill costs one short index line instead of its body.
//
// This is neither another slash command nor another workflow. A slash command
// is typed by a human and a workflow is started by one; neither can be selected
// by the model mid-turn, which is exactly what a skill exists to do.
package skills

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/packetcode/packetcode/internal/config"
)

const (
	// FileName is the fixed body filename inside a skill directory. Fixing it
	// keeps the directory name the single source of the skill's identity, so
	// two files in one directory can never disagree about what it is called.
	FileName = "SKILL.md"

	// MaxDescriptionBytes bounds one index entry. Every skill pays this cost on
	// every turn whether or not it is ever loaded, so the bound is the feature:
	// an over-long description turns the index back into the thing it replaced.
	MaxDescriptionBytes = 240

	// MaxBodyBytes bounds one loaded body. The cap is enforced at discovery
	// rather than at read time so an oversized skill is reported once, at load,
	// instead of surprising a turn that is already half spent.
	MaxBodyBytes = 16 * 1024
	// MaxSkillsPerScope bounds how many skills one scope contributes. The
	// per-skill caps bound a single index line; this bounds the block.
	MaxSkillsPerScope = 128

	// maxFileBytes bounds what discovery will read off disk at all, leaving the
	// body cap room for frontmatter above it.
	maxFileBytes = MaxBodyBytes + 4*1024

	// dirName is the per-scope skills directory under the packetcode home and
	// under a project's .packetcode directory.
	dirName = "skills"
)

// Source records where a skill body came from. It is a trust label as much as
// a provenance one: see Skill.Trusted.
const (
	SourceBuiltin = "builtin"
	SourceUser    = "user"
	SourceProject = "project"
)

// Skill is one loaded skill.
type Skill struct {
	Name        string
	Description string
	Body        string
	Source      string
	// Path is the file the body came from. Empty for embedded builtins, which
	// have no on-disk location to report.
	Path string
}

// Trusted reports whether the body is under the operator's control rather than
// the repository's.
//
// A project-directory skill body is attacker-controllable in a hostile repo.
// That is not a new exposure: project slash commands (.packetcode/commands) and
// project workflows (.packetcode/workflows) are the same trust class, and
// packetcode already accepts it. The tool labels project bodies so the model
// treats them as repository content, and every action a body suggests stays
// individually approval-gated, which is where the real boundary is.
func (s Skill) Trusted() bool { return s.Source != SourceProject }

// Registry is the resolved skill set for one working directory.
type Registry struct {
	ordered []Skill
	byName  map[string]int
	errors  []string
}

// Load discovers skills for workingDir.
//
// Precedence is project > user > builtin, matching workflows rather than slash
// commands: a project may replace a builtin skill outright, because a skill is
// reference material about this repo's conventions and the repo is the better
// authority on those. Discovery never fails: an unreadable scope contributes to
// Errors and the other scopes still resolve.
func Load(workingDir string) *Registry {
	r := &Registry{byName: make(map[string]int)}
	r.loadBuiltins()

	userDir, err := UserSkillsDir()
	if err != nil {
		r.errors = append(r.errors, fmt.Sprintf("user skills: %s", err))
	} else {
		r.loadDir(userDir, SourceUser)
	}
	if strings.TrimSpace(workingDir) != "" {
		r.loadDir(ProjectSkillsDir(workingDir), SourceProject)
	}
	// Sorted once, here, now that upsert no longer does it per insert. A
	// stable prompt prefix is what makes provider-side prompt caching work.
	sortSkills(r)
	return r
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

// Lookup returns the resolved skill for name.
func (r *Registry) Lookup(name string) (Skill, bool) {
	if r == nil {
		return Skill{}, false
	}
	idx, ok := r.byName[strings.TrimSpace(name)]
	if !ok {
		return Skill{}, false
	}
	return r.ordered[idx], true
}

// Skills returns every resolved skill in name order.
func (r *Registry) Skills() []Skill {
	if r == nil {
		return nil
	}
	return append([]Skill(nil), r.ordered...)
}

// Names returns the resolved skill names in the same order as Skills.
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.ordered))
	for _, s := range r.ordered {
		out = append(out, s.Name)
	}
	return out
}

// Errors returns every malformed skill found during discovery.
//
// Malformed skills are reported rather than silently dropped: a skill file the
// user wrote and packetcode ignored is indistinguishable, from the user's
// chair, from a skill that was consulted and did nothing. The caller decides
// where these surface; nothing here writes to a stream.
func (r *Registry) Errors() []string {
	if r == nil {
		return nil
	}
	return append([]string(nil), r.errors...)
}

// IndexBlock renders the <available_skills> section for the system prompt.
//
// It carries names and descriptions only. Returns "" when no skills resolved,
// so a caller can append it unconditionally without emitting an empty block.
func (r *Registry) IndexBlock() string {
	if r == nil || len(r.ordered) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<available_skills>\n")
	b.WriteString("Reference material you can load during a turn. Each line is a name and what it covers; the instructions themselves are not in context. When one matches the task, call the skill tool with that name before acting.\n")
	for _, s := range r.ordered {
		// The source is named on every line. A project skill's description is
		// repository content, and an unlabelled line in the system prompt
		// reads with the same authority as the sentence above it.
		b.WriteString("- ")
		b.WriteString(s.Name)
		b.WriteString(" (")
		b.WriteString(s.Source)
		b.WriteString("): ")
		// Escaped, not rejected. A description is copied into the system
		// prompt -- the highest-authority position in the conversation -- and
		// a project skill's is attacker-controllable in a hostile repo, so it
		// must not be able to close <available_skills> and continue as if it
		// were operator text. Refusing angle brackets outright would also
		// refuse legitimate <name> placeholders, which several builtins use.
		b.WriteString(escapeIndexText(s.Description))
		b.WriteString("\n")
	}
	b.WriteString("</available_skills>")
	return b.String()
}

// escapeIndexText neutralises markup in text destined for the system prompt.
func escapeIndexText(s string) string {
	s = strings.ReplaceAll(s, "<", "&lt;")
	return strings.ReplaceAll(s, ">", "&gt;")
}

func (r *Registry) upsert(s Skill) {
	if idx, ok := r.byName[s.Name]; ok {
		r.ordered[idx] = s
		return
	}
	r.byName[s.Name] = len(r.ordered)
	r.ordered = append(r.ordered, s)
	// Sorted once by Load, not here. Sorting per insert made loading N skills
	// O(N^2 log N) for an ordering nothing reads until the index is built.
}

// sortSkills keeps the index deterministic across runs. A stable prompt prefix
// is what makes provider-side prompt caching worth anything.
func sortSkills(r *Registry) {
	sort.Slice(r.ordered, func(i, j int) bool { return r.ordered[i].Name < r.ordered[j].Name })
	for i, s := range r.ordered {
		r.byName[s.Name] = i
	}
}

func (r *Registry) loadDir(dir, source string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// An absent scope is the normal case. Any other failure is unreadable
		// state and gets reported.
		if !os.IsNotExist(err) {
			r.errors = append(r.errors, fmt.Sprintf("%s skills: %s", source, err))
		}
		return
	}
	loaded := 0
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			// Dotfiles are editor and OS droppings, not attempted skills.
			continue
		}
		// The per-skill description cap bounds one index line; without a cap
		// on the count, a directory of 5000 skills still puts a megabyte of
		// attacker-supplied text in front of every request. Reported once
		// rather than per entry, so the overflow cannot flood the terminal
		// either.
		if loaded >= MaxSkillsPerScope {
			r.errors = append(r.errors, fmt.Sprintf(
				"%s skills: stopped after %d; the rest were not loaded", source, MaxSkillsPerScope))
			return
		}
		if !entry.IsDir() {
			r.errors = append(r.errors, fmt.Sprintf("%s skill %q: expected a directory containing %s", source, name, FileName))
			continue
		}
		skill, err := loadSkillDir(filepath.Join(dir, name), name, source)
		if err != nil {
			r.errors = append(r.errors, fmt.Sprintf("%s skill %q: %s", source, name, err))
			continue
		}
		r.upsert(skill)
		loaded++
	}
}

func loadSkillDir(dir, name, source string) (Skill, error) {
	if !ValidName(name) {
		return Skill{}, fmt.Errorf("invalid skill name")
	}
	path := filepath.Join(dir, FileName)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Skill{}, fmt.Errorf("missing %s", FileName)
		}
		return Skill{}, err
	}
	// A cheap pre-read guard so a huge file is never pulled into memory just to
	// be rejected. It is deliberately looser than MaxBodyBytes because the file
	// also carries frontmatter; the exact body cap is applied after parsing.
	// Stat follows symlinks, so its size is only a hint about a regular file
	// and says nothing at all about a device or FIFO. A SKILL.md symlinked to
	// /dev/zero stats as 0 bytes, passes any cap, and then reads forever.
	if !info.Mode().IsRegular() {
		return Skill{}, fmt.Errorf("%s is not a regular file", FileName)
	}
	if info.Size() > maxFileBytes {
		return Skill{}, fmt.Errorf("file is %d bytes, over the %d byte cap", info.Size(), maxFileBytes)
	}
	data, err := readCapped(path)
	if err != nil {
		return Skill{}, err
	}
	return newSkill(name, string(data), source, path)
}

// readCapped reads at most maxFileBytes+1 bytes and refuses anything longer.
// os.ReadFile treats the stat size as a capacity hint and reads to EOF
// regardless, so it cannot be trusted to honour a cap that Stat reported --
// including when the file grows between the two calls.
func readCapped(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileBytes {
		return nil, fmt.Errorf("file is over the %d byte cap", maxFileBytes)
	}
	return data, nil
}

func newSkill(name, raw, source, path string) (Skill, error) {
	body, description := ParseFrontmatter(raw)
	body = strings.TrimSpace(body)
	description = strings.TrimSpace(description)
	if description == "" {
		// Without a description the skill cannot be indexed, and a skill the
		// model cannot see is not a skill.
		return Skill{}, fmt.Errorf("missing frontmatter description")
	}
	if strings.ContainsAny(description, "\r\n") {
		return Skill{}, fmt.Errorf("description must be a single line")
	}
	if len(description) > MaxDescriptionBytes {
		return Skill{}, fmt.Errorf("description is %d bytes, over the %d byte index cap", len(description), MaxDescriptionBytes)
	}
	if body == "" {
		return Skill{}, fmt.Errorf("empty body")
	}
	if len(body) > MaxBodyBytes {
		return Skill{}, fmt.Errorf("body is %d bytes, over the %d byte cap", len(body), MaxBodyBytes)
	}
	return Skill{
		Name:        name,
		Description: description,
		Body:        body,
		Source:      source,
		Path:        path,
	}, nil
}
