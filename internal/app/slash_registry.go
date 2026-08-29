package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/skills"
)

// SlashCommand describes either a built-in UI command or a markdown-backed
// prompt command loaded from a user/project commands directory.
type SlashCommand struct {
	Name        string
	Usage       string
	Description string
	Body        string
	Source      string
	Builtin     bool
	// Hidden keeps a command out of /help and autocomplete while still
	// reserving its name. Used for builtin verbs that are aliases of a listed
	// one -- they are dispatched, so they must be reserved, but listing every
	// spelling twice is noise.
	Hidden bool
	// Skill marks a command whose body is a skill, expanded into the turn
	// rather than run by the program. Kept so the surfaces that must not
	// treat the two alike -- the ACP catalogue, /help's grouping -- can tell
	// them apart without string-matching Source.
	Skill bool
}

// SlashCommandRegistry keeps commands in display order while allowing quick
// lookup during parse/dispatch.
type SlashCommandRegistry struct {
	ordered []SlashCommand
	byName  map[string]int
	errors  []string
	// shadowed collects displaced commands during loading. The message cannot
	// be written at displacement time because the eventual winner of the name
	// is not known until loading ends.
	shadowed []shadowedCommand
}

// shadowedCommand is one verb that lost its name to a later registration.
type shadowedCommand struct {
	name   string
	source string
	skill  bool
}

func (s shadowedCommand) kind() string {
	if s.skill {
		return "skill"
	}
	return "command"
}

func NewBuiltinSlashRegistry() *SlashCommandRegistry {
	r := &SlashCommandRegistry{byName: make(map[string]int)}
	for _, row := range SlashCommands {
		name := verbOf(row.Key)
		if name == "" {
			continue
		}
		r.add(SlashCommand{
			Name:        name,
			Usage:       row.Key,
			Description: row.Desc,
			Source:      "built-in",
			Builtin:     true,
		})
	}
	for _, name := range builtinAliases {
		r.add(SlashCommand{
			Name:        name,
			Usage:       "/" + name,
			Description: "",
			Source:      "built-in",
			Builtin:     true,
			Hidden:      true,
		})
	}
	return r
}

// builtinAliases are verbs handleSlashCommand dispatches that SlashCommands
// does not list separately.
//
// They have to be reserved even though they are not advertised. A name the
// dispatch switch consumes but the registry does not know about is a name a
// skill or a command file can register, appear in /help under, and never run:
// the switch swallows it first, and nothing reports the collision because
// nothing knew there was one. TestBuiltinRegistryReservesEveryDispatchedVerb
// keeps this list honest against the switch itself.
var builtinAliases = []string{"workflow"}

// LoadSlashRegistry resolves every typed verb: builtins, then user-invocable
// skills, then markdown command files.
//
// The order is the precedence, weakest last-registered wins among customs:
// builtins are never displaced (a file must not redefine /model), and a
// markdown command displaces a same-named skill. That second rule is the
// arguable one, and it is deliberate -- a command file is one prompt the user
// wrote for this project, a skill is a library entry that may have arrived
// with a dependency, and the more specific, more local, more deliberate thing
// should win the name. Both displacements are reported through Errors(), so
// the loser is never silently absent.
//
// sk may be nil; callers that have no skill registry get the previous
// behaviour exactly.
func LoadSlashRegistry(workingDir string, sk *skills.Registry) *SlashCommandRegistry {
	r := NewBuiltinSlashRegistry()
	r.registerSkills(sk)

	userDir, err := config.UserCommandsDir()
	if err == nil {
		r.loadDir(userDir, "user")
	} else {
		r.errors = append(r.errors, fmt.Sprintf("user commands: %s", err))
	}
	if workingDir != "" {
		r.loadDir(config.ProjectCommandsDir(workingDir), "project")
	}
	r.noteShadowedNames()
	return r
}

// noteShadowedNames reports every verb that lost its name, naming the command
// that actually holds it now.
//
// Written here rather than at displacement because only here is the winner
// final. A skill displaced by a user command file which is then displaced by a
// project one produced, at displacement time, a message pointing at the user
// file -- which is not what /name runs, and is not the file to edit.
func (r *SlashCommandRegistry) noteShadowedNames() {
	for _, s := range r.shadowed {
		winner, ok := r.Lookup(s.name)
		if !ok {
			continue
		}
		r.errors = append(r.errors, fmt.Sprintf(
			"%s %s %q was shadowed and is not reachable: /%s runs the %s %s",
			s.source, s.kind(), s.name, s.name, winner.Source, kindOf(winner)))
	}
	r.shadowed = nil
}

func kindOf(c SlashCommand) string {
	if c.Skill {
		return "skill"
	}
	return "command"
}

func (r *SlashCommandRegistry) Parse(text string) (cmd string, args []string, ok bool) {
	name, args, ok := parseSlashCommandFields(text)
	if !ok {
		return "", nil, false
	}
	if _, hit := r.Lookup(name); !hit {
		return "", nil, false
	}
	return name, args, true
}

func (r *SlashCommandRegistry) Lookup(name string) (SlashCommand, bool) {
	if r == nil {
		r = NewBuiltinSlashRegistry()
	}
	idx, ok := r.byName[name]
	if !ok {
		return SlashCommand{}, false
	}
	return r.ordered[idx], true
}

// Commands returns every registered command in display order: built-ins
// first, then markdown commands in load order. Callers outside the TUI (the
// ACP _packetcode/commands/list extension) filter on Builtin to pick the
// subset their surface can honour.
func (r *SlashCommandRegistry) Commands() []SlashCommand {
	if r == nil {
		r = NewBuiltinSlashRegistry()
	}
	out := make([]SlashCommand, 0, len(r.ordered))
	for _, cmd := range r.ordered {
		if cmd.Hidden {
			continue
		}
		out = append(out, cmd)
	}
	return out
}

func (r *SlashCommandRegistry) HelpRows() []KeyHelp {
	if r == nil {
		r = NewBuiltinSlashRegistry()
	}
	rows := make([]KeyHelp, 0, len(r.ordered))
	for _, cmd := range r.ordered {
		if cmd.Hidden {
			continue
		}
		rows = append(rows, KeyHelp{Key: cmd.Usage, Desc: cmd.Description})
	}
	return rows
}

func (r *SlashCommandRegistry) Errors() []string {
	if r == nil {
		return nil
	}
	out := make([]string, len(r.errors))
	copy(out, r.errors)
	return out
}

// shadowedByBuiltinMessage explains a refused custom verb in terms of the
// thing its author can actually change.
func shadowedByBuiltinMessage(cmd SlashCommand) string {
	fix := "rename the file to use it"
	if cmd.Skill {
		fix = "rename the skill directory to invoke it with a slash command"
	}
	return fmt.Sprintf("%s %s %q was ignored: %q is a built-in command; %s",
		cmd.Source, kindOf(cmd), cmd.Name, cmd.Name, fix)
}

func (r *SlashCommandRegistry) add(cmd SlashCommand) {
	if r.byName == nil {
		r.byName = make(map[string]int)
	}
	if _, exists := r.byName[cmd.Name]; exists {
		r.ordered = append(r.ordered, cmd)
		return
	}
	r.byName[cmd.Name] = len(r.ordered)
	r.ordered = append(r.ordered, cmd)
}

func (r *SlashCommandRegistry) upsertCustom(cmd SlashCommand) {
	if r.byName == nil {
		r.byName = make(map[string]int)
	}
	if idx, exists := r.byName[cmd.Name]; exists {
		if r.ordered[idx].Builtin {
			// Refusing is correct -- a builtin verb is a program affordance
			// and letting a file redefine /model or /permissions would be a
			// hijack. Doing it silently is not: the author wrote a file that
			// now does nothing, with nothing anywhere saying so. Every new
			// builtin verb turns somebody's working command into this case.
			r.errors = append(r.errors, shadowedByBuiltinMessage(cmd))
			return
		}
		if prev := r.ordered[idx]; prev.Skill != cmd.Skill {
			// One of these two is a skill and the other a command file. The
			// loser is a thing its author wrote and can no longer reach, so it
			// is recorded -- but only recorded. Naming the winner here would
			// be a guess: a project command file can still displace the user
			// command file that just displaced this skill, and a message
			// written now would send its reader to edit the wrong file.
			// Resolved once loading is finished, in noteShadowedNames.
			r.shadowed = append(r.shadowed, shadowedCommand{
				name: prev.Name, source: prev.Source, skill: prev.Skill,
			})
		}
		r.ordered[idx] = cmd
		return
	}
	r.byName[cmd.Name] = len(r.ordered)
	r.ordered = append(r.ordered, cmd)
}

// registerSkills adds each user-invocable skill as a typed verb.
//
// This is text expansion, not a tool call: /<name> puts the skill body into
// the turn as the user's own message, which is what Claude Code does and what
// makes the two ecosystems' skills behave the same way here. The model is not
// asked to choose -- the user already did.
//
// Skills that set `user-invocable: false` are background knowledge for the
// model and are not registered; nothing here consults the model flag, which
// governs a different surface entirely.
func (r *SlashCommandRegistry) registerSkills(sk *skills.Registry) {
	if sk == nil {
		return
	}
	for _, s := range sk.Skills() {
		if !s.Enabled() {
			// A pending foreign project skill registers nothing. Otherwise a
			// repository's chosen name would be in /help and autocomplete
			// before anyone agreed to load it, and the first time the user
			// found out would be when they typed it.
			continue
		}
		if !s.UserInvocable() {
			continue
		}
		r.upsertCustom(SlashCommand{
			Name:        s.Name,
			Usage:       "/" + s.Name + " [arguments]",
			Description: firstLine(s.Description),
			// Framed here, at registration, so every path that later expands
			// this body -- dispatch, queued input, the ACP catalogue -- gets
			// the provenance label and the defanged markers without having to
			// remember to ask for them.
			Body: s.Block(),
			// The scope alone. Skill carries the kind, and baking "skill" into
			// the source made every composed phrase say it twice.
			Source:  s.Source,
			Builtin: false,
			Skill:   true,
		})
	}
}

func (r *SlashCommandRegistry) loadDir(dir, source string) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		r.errors = append(r.errors, fmt.Sprintf("%s commands: %s", source, err))
		return
	}
	if len(matches) == 0 {
		return
	}
	sort.Strings(matches)
	for _, path := range matches {
		cmd, err := loadMarkdownSlashCommand(path, source)
		if err != nil {
			r.errors = append(r.errors, fmt.Sprintf("%s command %s: %s", source, filepath.Base(path), err))
			continue
		}
		r.upsertCustom(cmd)
	}
}

func loadMarkdownSlashCommand(path, source string) (SlashCommand, error) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if !validSlashCommandName(name) {
		return SlashCommand{}, fmt.Errorf("invalid slash command name %q", name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SlashCommand{}, err
	}
	body, desc := parseMarkdownCommandFile(string(data))
	body = strings.TrimSpace(body)
	if body == "" {
		return SlashCommand{}, fmt.Errorf("empty slash command %q", name)
	}
	if desc == "" {
		desc = "Run " + source + " markdown command"
	}
	usage := "/" + name
	if strings.Contains(body, "$ARGUMENTS") {
		usage += " [arguments]"
	}
	return SlashCommand{
		Name:        name,
		Usage:       usage,
		Description: desc,
		Body:        body,
		Source:      source,
		Builtin:     false,
	}, nil
}

func parseMarkdownCommandFile(raw string) (body, description string) {
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

// Expand renders the body for a turn, substituting what the user typed.
//
// A body without a $ARGUMENTS placeholder gets the arguments appended instead
// of dropped. Dropping them was the older behaviour and it is indefensible:
// the user typed words, the turn started, and the words were not in it, with
// nothing said. Skill bodies make this the common case rather than the rare
// one -- a skill is prose, not a template, and nobody writing one thinks to
// add a placeholder.
func (c SlashCommand) Expand(arguments string) string {
	args := strings.TrimSpace(arguments)
	// $ARGUMENTS is honoured only for a body its user wrote. A skill body is
	// framed, and for a project skill it is repository content -- letting it
	// place the placeholder would let it pull the user's typed words inside
	// its own untrusted block, where they read as part of the repository's
	// text. A skill's arguments always land after the closing marker.
	if !c.Skill && strings.Contains(c.Body, "$ARGUMENTS") {
		return strings.ReplaceAll(c.Body, "$ARGUMENTS", args)
	}
	if args == "" {
		return c.Body
	}
	return c.Body + "\n\n" + args
}

func validSlashCommandName(name string) bool {
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
