package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/skills"
)

// writeProjectSkill installs one project-scope skill under root and returns
// the working directory to hand skills.Load. Project scope is the interesting
// one: it is the untrusted case the table has to mark.
func writeProjectSkill(t *testing.T, root, name, description, body string) string {
	t.Helper()
	dir := filepath.Join(root, ".packetcode", "skills", name)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	doc := "---\ndescription: " + description + "\n---\n\n" + body + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, skills.FileName), []byte(doc), 0o600))
	return root
}

// projectWorkspace returns a working directory distinct from the fake HOME so
// project and user scopes do not resolve to the same folder.
func projectWorkspace(t *testing.T, r *testAppRig) string {
	t.Helper()
	dir := filepath.Join(r.tmp, "workspace")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	return dir
}

// useSkills wires a resolved registry into the app under test. One place to
// change if the registry ever moves off Deps.
func useSkills(r *testAppRig, reg *skills.Registry) {
	r.app.deps.Skills = reg
}

func TestSkillsCommandEmptyStateNamesWhereToPutOne(t *testing.T) {
	r := newTestApp(t)
	// An empty registry, not a nil one: nil is "no registry wired", which is a
	// different message.
	useSkills(r, &skills.Registry{})

	r.app.handleSkillsCommand(nil)

	convContains(t, r.app, "no skills resolved")
	convContains(t, r.app, filepath.ToSlash(".packetcode/skills/<name>/SKILL.md"))
}

func TestSkillsCommandNoRegistryWired(t *testing.T) {
	r := newTestApp(t)
	r.app.handleSkillsCommand(nil)
	convContains(t, r.app, "skills: skill registry not available")
}

func TestSkillsCommandTableMarksProjectSkillsUntrusted(t *testing.T) {
	r := newTestApp(t)
	ws := projectWorkspace(t, r)
	writeProjectSkill(t, ws, "deploy", "How this repo ships to production.", "run the deploy script")
	useSkills(r, skills.Load(ws))

	r.app.handleSkillsCommand(nil)

	out := convText(r.app)
	assert.Contains(t, out, "NAME                 SOURCE                DESCRIPTION")
	assert.Contains(t, out, "deploy")
	assert.Contains(t, out, "project (untrusted)")
	// Short fragment: convText wraps to the pane width, so a long needle can
	// straddle a line break.
	assert.Contains(t, out, "How this repo ships to")
	// Builtins resolve alongside project skills and must not be marked.
	assert.Contains(t, out, "builtin")
	assert.NotContains(t, out, "builtin (untrusted)")
	// The body never reaches the conversation, not even in the list.
	assert.NotContains(t, out, "run the deploy script")
}

func TestSkillsCommandTableReportsShadowedBuiltin(t *testing.T) {
	r := newTestApp(t)
	ws := projectWorkspace(t, r)
	builtins := skills.BuiltinNames()
	require.NotEmpty(t, builtins, "expected embedded builtin skills")
	writeProjectSkill(t, ws, builtins[0], "Repo-local replacement.", "repo body")
	useSkills(r, skills.Load(ws))

	r.app.handleSkillsCommand(nil)

	convContains(t, r.app, "shadows the builtin skill")
	convContains(t, r.app, builtins[0])
}

func TestSkillsCommandSurfacesLoadWarnings(t *testing.T) {
	r := newTestApp(t)
	ws := projectWorkspace(t, r)
	// No frontmatter description: discovery drops the skill and records why.
	// Without this section the file the user wrote would simply not exist.
	dir := filepath.Join(ws, ".packetcode", "skills", "broken")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, skills.FileName), []byte("no frontmatter here\n"), 0o600))
	useSkills(r, skills.Load(ws))

	r.app.handleSkillsCommand(nil)

	out := convText(r.app)
	// "could not be loaded", not "warnings": the two are separate sections
	// now. This skill is absent, which is the error section; a warning is a
	// skill that IS here and is not what its author wrote.
	assert.Contains(t, out, "could not be loaded:")
	assert.Contains(t, out, "broken")
}

// A flag that did not parse leaves the permissive default in place, which for
// disable-model-invocation means the model may run a skill whose author said
// it must not. That cannot be silent.
func TestSkillsCommandSurfacesUnparseableFlag(t *testing.T) {
	r := newTestApp(t)
	ws := projectWorkspace(t, r)
	dir := filepath.Join(ws, ".packetcode", "skills", "handsoff")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, skills.FileName),
		[]byte("---\ndescription: d\ndisable-model-invocation: maybe\n---\nbody\n"), 0o600))
	useSkills(r, skills.Load(ws))

	r.app.handleSkillsCommand(nil)

	out := convText(r.app)
	assert.Contains(t, out, "warnings:")
	assert.Contains(t, out, "disable-model-invocation")
	// The skill still loaded -- a bad flag is not a reason to make reference
	// material vanish, only a reason to say so.
	assert.NotContains(t, out, "could not be loaded:")
}

func TestSkillsCommandDetailForKnownName(t *testing.T) {
	r := newTestApp(t)
	ws := projectWorkspace(t, r)
	writeProjectSkill(t, ws, "deploy", "How this repo ships to production.", "SECRET-BODY-MARKER")
	useSkills(r, skills.Load(ws))

	r.app.handleSkillsCommand([]string{"deploy"})

	out := convText(r.app)
	assert.Contains(t, out, "deploy (project)")
	assert.Contains(t, out, "project:deploy")
	assert.Contains(t, out, "untrusted")
	assert.Contains(t, out, filepath.Join("skills", "deploy", skills.FileName))
	assert.Contains(t, out, "byte cap")
	assert.Contains(t, out, "through the skill tool")
	assert.Contains(t, out, "How this repo ships to production.")
	// The whole point of printing the path instead.
	assert.NotContains(t, out, "SECRET-BODY-MARKER")
}

func TestSkillsCommandDetailAcceptsQualifiedName(t *testing.T) {
	r := newTestApp(t)
	ws := projectWorkspace(t, r)
	writeProjectSkill(t, ws, "deploy", "How this repo ships to production.", "body")
	useSkills(r, skills.Load(ws))

	r.app.handleSkillsCommand([]string{"project:deploy"})
	convContains(t, r.app, "deploy (project)")

	// A qualified name naming a scope that did not win is not this skill.
	r.app.handleSkillsCommand([]string{"user:deploy"})
	convContains(t, r.app, `no skill named "user:deploy"`)
}

func TestSkillsCommandDetailForBuiltinReportsEmbeddedPath(t *testing.T) {
	r := newTestApp(t)
	ws := projectWorkspace(t, r)
	reg := skills.Load(ws)
	useSkills(r, reg)
	builtins := skills.BuiltinNames()
	require.NotEmpty(t, builtins)
	skill, ok := reg.Lookup(builtins[0])
	require.True(t, ok)

	r.app.handleSkillsCommand([]string{builtins[0]})

	out := convText(r.app)
	assert.Contains(t, out, "embedded in this binary")
	// The whole phrase: "trusted" alone is a substring of "untrusted", so it
	// passed either way and asserted nothing about which one was rendered.
	assert.Contains(t, out, "trusted — ships with packetcode")
	assert.NotContains(t, out, "untrusted")
	// The body is not printed. This used to assert NotContains "SECRET", a
	// string no builtin contains and no test wrote -- it could not fail under
	// any implementation. Asserted against the real body instead.
	line := firstBodyLine(t, skill.Body)
	assert.NotContains(t, out, line)
}

// firstBodyLine returns a substantial line from a skill body, for asserting
// that the body was NOT rendered.
func firstBodyLine(t *testing.T, body string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		// Long enough that matching it is evidence of the body rather than
		// coincidence with a label or a heading elsewhere in the render.
		if len(line) >= 25 && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	t.Fatalf("no usable line in body:\n%s", body)
	return ""
}

func TestSkillsCommandDetailFlagsSlashCommandCollision(t *testing.T) {
	r := newTestApp(t)
	ws := projectWorkspace(t, r)
	// "cost" is a builtin slash command; a skill of that name is the
	// collision a user would otherwise discover by typing /cost and getting
	// something else entirely.
	writeProjectSkill(t, ws, "cost", "Repo cost conventions.", "body")
	useSkills(r, skills.Load(ws))

	r.app.handleSkillsCommand([]string{"cost"})

	convContains(t, r.app, "/cost is a built-in command")
	convContains(t, r.app, "not this skill")
}

func TestSkillsCommandUnknownNameIsNotAnError(t *testing.T) {
	r := newTestApp(t)
	useSkills(r, skills.Load(projectWorkspace(t, r)))

	model, cmd := r.app.handleSkillsCommand([]string{"nope"})

	assert.Same(t, r.app, model)
	assert.Nil(t, cmd)
	convContains(t, r.app, `skills: no skill named "nope"`)
	convContains(t, r.app, "try /skills to list")
}

func TestSkillsHelpRowsRegistered(t *testing.T) {
	help := renderHelp()
	assert.Contains(t, help, "/skills")
	assert.Contains(t, help, "/skills <name>")

	var listed bool
	for _, row := range SlashCommands {
		if strings.TrimSpace(row.Key) == "/skills" {
			listed = true
		}
	}
	assert.True(t, listed, "/skills must be in SlashCommands so autocomplete offers it")
}
