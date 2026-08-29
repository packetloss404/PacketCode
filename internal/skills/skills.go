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
)

const (
	// FileName is the fixed body filename inside a skill directory. Fixing it
	// keeps the directory name the single source of the skill's identity, so
	// two files in one directory can never disagree about what it is called.
	FileName = "SKILL.md"

	// MaxDescriptionBytes bounds one index entry. Every skill pays this cost on
	// every turn whether or not it is ever loaded, so the bound is still the
	// feature; the number is a concession to the wider Agent Skills ecosystem.
	// A description there is not a summary but a router: it enumerates the
	// phrasings that should select the skill and names the siblings that
	// should win a near miss. Published skills land between 450 and 1350
	// bytes, so the old 240 rejected every one of them and the index they
	// could not enter was worth nothing.
	MaxDescriptionBytes = 1536

	// MaxBodyBytes bounds one loaded body. The cap is enforced at discovery
	// rather than at read time so an oversized skill is reported once, at load,
	// instead of surprising a turn that is already half spent.
	//
	// A body is paid for only when the model asks for it, which is what buys
	// the headroom the index cannot afford. Ecosystem skills that dispatch to
	// resource files (see Registry.ReadResource) run to 50KB.
	MaxBodyBytes = 64 * 1024
	// MaxSkillsPerScope bounds how many skills one scope contributes. The
	// per-skill caps bound a single index line; this bounds the block.
	MaxSkillsPerScope = 128

	// MaxIndexBytes bounds the whole rendered index.
	//
	// MaxDescriptionBytes bounds one line, which was sufficient when a line
	// could not exceed 240 bytes; at 1536 it is not. Seventeen ecosystem
	// skills at their published lengths come to roughly 14KB in front of
	// every request, and nothing stopped that from being 128. The block is
	// trimmed from the alphabetical tail rather than by usage, deliberately:
	// sortSkills exists to keep the prompt prefix stable across runs, and an
	// index that reorders with history would defeat provider-side caching on
	// every turn -- paying far more than the bytes it saved.
	//
	// Keep this above the size of a single maximal entry: the header plus
	// MaxDescriptionBytes plus a name, source and punctuation, so roughly
	// 1900 bytes today. IndexBlock returns "" when nothing was listed, which
	// is right when every skill is model-disabled but would be wrong if the
	// first entry simply did not fit -- the block and the "N omitted" notice
	// would both vanish with nothing said. That is unreachable at 16KB and
	// becomes reachable the moment someone lowers this.
	MaxIndexBytes = 16 * 1024

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
	// Dir is the directory holding SKILL.md and any resource files beside it.
	// For an on-disk skill this is an absolute filesystem path; for a builtin
	// it is a path within the embedded filesystem, which is why Source has to
	// be consulted before Dir is handed to anything that opens files.
	Dir string
	// Origin is the directory layout this was filed under: OriginNative,
	// OriginClaude or OriginAgents. Source says who wrote it; Origin says
	// which convention it was found by, which is what someone needs when a
	// skill they installed for another agent does not behave here.
	Origin string
	// NeedsApproval marks a foreign project-scope skill nobody has agreed to
	// yet: repository content, filed under another agent's convention, found
	// by opening the directory rather than by anyone installing it.
	//
	// Such a skill is discovered and listed and does nothing else. It is not
	// in the index, so the model is never told it exists; it registers no
	// slash verb; and the skill tool refuses it. The gate has to sit at
	// discovery rather than at invocation because two of the three exposures
	// -- a description in the system prompt, a name in /help -- have already
	// happened by the time anything is invoked.
	NeedsApproval bool
	// Invocation carries the two flags from the header. Stored as the parsed
	// frontmatter rather than as two bools so the permissive zero value is
	// the one thing a caller can rely on without reading the parser.
	Invocation Frontmatter
	// Shadows names the scopes this skill displaced, weakest first.
	//
	// The registry keeps only the winner, so without this the fact that one
	// scope's `deploy` replaced another's is not recoverable by anything
	// downstream. Since the precedence inversion the displaced one is the
	// repository's, which is a surprise rather than an attack -- but it is
	// still someone's file quietly not taking effect, and the record is what
	// lets /skills say so. Recorded at resolution, where both halves are
	// still in hand.
	Shadows []string
}

// Enabled reports whether this skill does anything at all.
//
// False only for a foreign project skill awaiting approval. Every consumer
// that would put a skill in front of the model or the keyboard asks this
// first, so a skill that has not been agreed to cannot reach either by a path
// someone forgot to guard.
func (s Skill) Enabled() bool { return !s.NeedsApproval }

// ModelInvocable reports whether the model may select this skill. False when
// the header says `disable-model-invocation: true`; such a skill is also kept
// out of the index, so the model is never told about something it cannot use.
func (s Skill) ModelInvocable() bool { return !s.Invocation.DisableModelInvocation }

// UserInvocable reports whether a person may trigger this skill directly.
// False when the header says `user-invocable: false` -- background knowledge
// the model may consult but nobody types.
func (s Skill) UserInvocable() bool { return !s.Invocation.DisableUserInvocation }

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
	// warnings are resolved-but-surprising outcomes: a shadowed skill, a
	// header value that did not parse. See Warnings.
	warnings []string
	// approvals and workspace are kept so a caller can approve a pending
	// skill without re-deriving where the registry came from. canonicalWS is
	// the approval-key form of workspace, resolved once at load rather than
	// per skill.
	approvals   *Approvals
	workspace   string
	canonicalWS string
}

// Workspace is the working directory this registry was resolved for.
func (r *Registry) Workspace() string {
	if r == nil {
		return ""
	}
	return r.workspace
}

// Approve records agreement to load one pending skill and returns a registry
// reflecting it. The caller replaces its registry with the result.
//
// Reloading rather than mutating in place is deliberate: approval changes
// which skills win names and which are in the index, and recomputing that from
// discovery is the only way to be sure the answer matches what a fresh start
// would produce.
func (r *Registry) Approve(name string) (*Registry, error) {
	if r == nil {
		return nil, fmt.Errorf("no registry")
	}
	s, ok := r.Lookup(name)
	if !ok {
		return r, fmt.Errorf("no skill named %q", name)
	}
	if !s.NeedsApproval {
		return r, fmt.Errorf("skill %q does not need approval", name)
	}
	if err := r.approvals.Approve(r.workspace, s.Name, s.Origin, s.Body); err != nil {
		return r, err
	}
	return Load(r.workspace), nil
}

// Revoke withdraws a previously granted approval and returns a registry
// reflecting it.
func (r *Registry) Revoke(name string) (*Registry, error) {
	if r == nil {
		return nil, fmt.Errorf("no registry")
	}
	if err := r.approvals.Revoke(r.workspace, strings.TrimSpace(name)); err != nil {
		return r, err
	}
	return Load(r.workspace), nil
}

// Load discovers skills for workingDir.
//
// Precedence is user > project > builtin: a skill in your home directory wins
// the name against a skill in the repository you happen to have open.
//
// That ordering is not a matter of taste. A project scope is repository
// content and a user scope is not, so only one direction of this choice has a
// victim: if the repo wins, cloning a hostile repository silently replaces the
// `deploy` skill you wrote for yourself with one you have never read, and you
// invoke it by the same name and the same muscle memory as before. If the home
// directory wins, the worst case is that a repository's own skill does not
// take effect, which is visible, recoverable, and reported through Warnings.
//
// It also matches Claude Code, which orders its scopes the same way and for
// the same reason. Skills published for that ecosystem are written expecting
// it, so disagreeing would be a compatibility bug on top of a security one.
//
// This inverts what packetcode did until 2026-08-28, when the ordering was
// project > user by analogy with workflows -- the analogy was wrong, because a
// workflow is something you run deliberately after reading it and a skill is
// something loaded by name mid-turn.
//
// Discovery never fails: an unreadable scope contributes to Errors and the
// other scopes still resolve.
func Load(workingDir string) *Registry {
	approvals, err := LoadApprovals()
	// A store that failed to load is empty, and the error is reported below.
	// An unreadable approval file must read as "nothing is approved" rather
	// than as "everything is".
	r := &Registry{byName: make(map[string]int), approvals: approvals, workspace: workingDir}
	if strings.TrimSpace(workingDir) != "" {
		if ws, cerr := CanonicalWorkspace(workingDir); cerr == nil {
			r.canonicalWS = ws
		} else {
			// Without a canonical workspace no approval can match, so every
			// foreign project skill stays pending. That is the safe direction,
			// but it is not one to take silently.
			r.errors = append(r.errors, fmt.Sprintf(
				"skills: cannot resolve the working directory, so no project skill approvals apply: %s", cerr))
		}
	}
	if err != nil {
		r.errors = append(r.errors, fmt.Sprintf("skills: %s", err))
	}
	// Weakest first: upsert lets a later scope displace an earlier one.
	r.loadBuiltins()

	userDirs, err := userScopeDirs()
	if err != nil {
		r.errors = append(r.errors, fmt.Sprintf("user skills: %s", err))
	}
	// Deduplicated by resolved path, keeping the strongest scope for each.
	//
	// Running packetcode from your home directory makes <cwd>/.agents/skills
	// and ~/.agents/skills the same directory. Scanning it twice reported
	// every malformed skill twice, made every skill in it "shadow" itself with
	// a warning naming a project skill that was the same file, and labelled
	// the user's own skills untrusted repository content on the project pass.
	// One directory is one scope, and when the two claim it the user scope
	// wins -- it is the truthful one, since those really are the user's files.
	for _, d := range dedupeScopes(append(projectScopeDirs(workingDir), userDirs...)) {
		r.loadScopeDir(d)
	}
	// Sorted once, here, now that upsert no longer does it per insert. A
	// stable prompt prefix is what makes provider-side prompt caching work.
	sortSkills(r)
	return r
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

// Warnings returns things that resolved but not as their author wrote them:
// a skill displaced by a narrower scope, a header value that did not parse.
//
// Kept apart from Errors because the two mean different things to a caller.
// An error is a skill that is not there; a warning is a skill that is there
// and is not quite what was asked for. `packetcode skills list` exits non-zero
// on the first and not the second, and shadowing is an ordinary, documented
// outcome that must not start failing anyone's build.
func (r *Registry) Warnings() []string {
	if r == nil {
		return nil
	}
	return append([]string(nil), r.warnings...)
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
	listed := 0
	b.WriteString("<available_skills>\n")
	b.WriteString("Reference material you can load during a turn. Each line is a name and what it covers; the instructions themselves are not in context. When one matches the task, call the skill tool with that name before acting.\n")
	omitted := 0
	for _, s := range r.ordered {
		if !s.Enabled() {
			// Never named to the model. Its description is repository text
			// found by opening a directory, and the system prompt is the
			// highest-authority position in the conversation -- putting it
			// there is most of the exposure the approval gate exists to stop.
			continue
		}
		if !s.ModelInvocable() {
			// Not counted as omitted: `omitted` reports skills the budget cut,
			// and the model can act on that ("ask for the rest"). This one is
			// not truncation, it is a decision by the skill's author, and
			// advertising it would spend tokens describing something the model
			// is then refused -- and invite it to keep trying.
			continue
		}
		// The source is named on every line. A project skill's description is
		// repository content, and an unlabelled line in the system prompt
		// reads with the same authority as the sentence above it.
		var line strings.Builder
		line.WriteString("- ")
		line.WriteString(s.Name)
		line.WriteString(" (")
		line.WriteString(s.Source)
		line.WriteString("): ")
		// Escaped, not rejected. A description is copied into the system
		// prompt -- the highest-authority position in the conversation -- and
		// a project skill's is attacker-controllable in a hostile repo, so it
		// must not be able to close <available_skills> and continue as if it
		// were operator text. Refusing angle brackets outright would also
		// refuse legitimate <name> placeholders, which several builtins use.
		line.WriteString(escapeIndexText(s.Description))
		line.WriteString("\n")
		// Whole entries are dropped, never truncated mid-description. A
		// half-description is worse than an absent one: it still costs its
		// bytes and still claims the skill is available, while having lost
		// the part that says when to choose it.
		if b.Len()+line.Len() > MaxIndexBytes {
			omitted++
			continue
		}
		b.WriteString(line.String())
		listed++
	}
	if listed == 0 {
		// Every skill was model-disabled. A header and a sentence promising
		// reference material, followed by nothing, is worse than no block:
		// it asserts a capability and then names none of it.
		return ""
	}
	if omitted > 0 {
		// Named, not silent. A model that cannot see a skill and is not told
		// so will confidently report that packetcode cannot do the thing.
		fmt.Fprintf(&b, "(%d further skills were omitted to bound this list; run `packetcode skills list` to see them.)\n", omitted)
	}
	b.WriteString("</available_skills>")
	return b.String()
}

// escapeIndexText neutralises markup in text destined for the system prompt.
func escapeIndexText(s string) string {
	s = strings.ReplaceAll(s, "<", "&lt;")
	return strings.ReplaceAll(s, ">", "&gt;")
}

// noteSkillWarnings surfaces header values the parser could not understand.
//
// These are not load failures -- the skill resolved and works -- but each one
// means a line the author wrote had no effect, and for the invocation flags
// the no-effect answer is the permissive one. Same channel as a malformed
// skill, for the same reason: a rule that silently did not apply is
// indistinguishable from one that did.
func (r *Registry) noteSkillWarnings(s Skill) {
	for _, w := range s.Invocation.Warnings {
		r.warnings = append(r.warnings, fmt.Sprintf("%s skill %q: %s", s.Source, s.Name, w))
	}
}

func (r *Registry) upsert(s Skill) {
	// Warned here rather than in each loader: upsert is the one call every
	// loader makes, the same reason newSkill carries the name validation.
	r.noteSkillWarnings(s)
	if idx, ok := r.byName[s.Name]; ok {
		prev := r.ordered[idx]
		s.Shadows = append(append([]string(nil), prev.Shadows...), prev.Source)
		r.ordered[idx] = s
		// Reported, not merely recorded. Since the precedence inversion the
		// dangerous direction of this is unreachable -- a repository can no
		// longer take over a name in the user's home directory -- but the
		// remaining direction is still a surprise worth one line: someone
		// wrote a skill into the repo expecting their teammates to get it,
		// and a teammate with a same-named personal skill never sees it.
		r.warnings = append(r.warnings, fmt.Sprintf(
			"%s skill %q shadows the %s skill of the same name; the %s one is not loaded",
			s.Source, s.Name, prev.Source, prev.Source))
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

// dedupeScopes collapses directories that resolve to the same place.
//
// Input is weakest-first, so a later entry claiming a path replaces an earlier
// one and the returned order is preserved. Paths are compared after cleaning
// and case-folding: Windows paths differ in case for the same directory, and a
// duplicate that slipped through would reintroduce exactly the double-scan
// this exists to prevent.
func dedupeScopes(in []skillScope) []skillScope {
	at := make(map[string]int, len(in))
	out := make([]skillScope, 0, len(in))
	for _, d := range in {
		key := scopePathKey(d.path)
		if idx, seen := at[key]; seen {
			out[idx] = d
			continue
		}
		at[key] = len(out)
		out = append(out, d)
	}
	return out
}

// scopePathKey identifies a directory for deduplication. Symlinks are not
// resolved: the directories may not exist, which is the ordinary case, and
// EvalSymlinks fails on a path that does not.
func scopePathKey(path string) string {
	return strings.ToLower(filepath.Clean(path))
}

// loadScopeDir loads one directory, labelling what it finds with the layout it
// was found under and marking foreign project skills pending approval.
func (r *Registry) loadScopeDir(d skillScope) {
	r.loadDirWith(d.path, d.source, d.origin, d.foreignProject)
}

func (r *Registry) loadDir(dir, source string) {
	r.loadDirWith(dir, source, OriginNative, false)
}

func (r *Registry) loadDirWith(dir, source, origin string, foreignProject bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// An absent scope is the normal case. Any other failure is unreadable
		// state and gets reported.
		if !os.IsNotExist(err) {
			r.errors = append(r.errors, fmt.Sprintf("%s skills (%s): %s", source, dir, err))
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
		skill.Origin = origin
		// Bound to this exact body: a repository that rewrites an approved
		// skill is asked again, or the first benign version buys permanent
		// trust for every version after it.
		skill.NeedsApproval = foreignProject && !r.approvals.Approved(r.canonicalWS, skill.Name, skill.Body)
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
	data, err := readCapped(path, maxFileBytes)
	if err != nil {
		return Skill{}, err
	}
	return newSkill(name, string(data), source, path, dir)
}

// readCapped reads at most limit+1 bytes and refuses anything longer.
// os.ReadFile treats the stat size as a capacity hint and reads to EOF
// regardless, so it cannot be trusted to honour a cap that Stat reported --
// including when the file grows between the two calls.
func readCapped(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file is over the %d byte cap", limit)
	}
	return data, nil
}

func newSkill(name, raw, source, path, dir string) (Skill, error) {
	// Checked here, not only in loadSkillDir. loadBuiltins builds skills from
	// embedded directory names and never went through that check, so the one
	// loader whose names are compiled in was the one loader with no gate.
	// That is backwards, and it is the path a bad name would survive longest
	// on. Every constructor of a Skill goes through this function.
	if !ValidName(name) {
		return Skill{}, fmt.Errorf("skill name %q is not a valid name", name)
	}
	body, fm := ParseFrontmatterFields(raw)
	description := fm.Description
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
		Invocation:  fm,
		Path:        path,
		Dir:         dir,
	}, nil
}
