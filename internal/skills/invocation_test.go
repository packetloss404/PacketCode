package skills

import (
	"strings"
	"testing"
)

// The zero value has to be permissive. Every skill written before these flags
// existed says nothing about either, and the day the fields landed must not be
// the day those skills stopped loading.
func TestParseFrontmatterFields_AbsentFlagsAreInvocableBothWays(t *testing.T) {
	_, fm := ParseFrontmatterFields("---\ndescription: d\n---\nbody\n")
	if fm.DisableModelInvocation {
		t.Fatal("absent disable-model-invocation should leave the model invocation enabled")
	}
	if fm.DisableUserInvocation {
		t.Fatal("absent user-invocable should leave user invocation enabled")
	}
}

func TestParseFrontmatterFields_Flags(t *testing.T) {
	for _, tc := range []struct {
		name             string
		header           string
		wantModelBlocked bool
		wantUserBlocked  bool
	}{
		{"model disabled", "disable-model-invocation: true", true, false},
		{"model disabled yes", "disable-model-invocation: yes", true, false},
		{"model disabled quoted", `disable-model-invocation: "true"`, true, false},
		{"model disabled caps", "disable-model-invocation: TRUE", true, false},
		{"model explicitly enabled", "disable-model-invocation: false", false, false},
		{"user disabled", "user-invocable: false", false, true},
		{"user disabled no", "user-invocable: no", false, true},
		{"user explicitly enabled", "user-invocable: true", false, false},
		{"both", "disable-model-invocation: true\nuser-invocable: false", true, true},
		// An unparseable value must not move a flag off its default, and must
		// not undo a value that already parsed. Each of these pairs a good
		// value with a bad one, so the assertion distinguishes "the default
		// held" from "the key was never handled at all" -- a row asserting
		// false/false proves neither, because false/false is the zero value.
		{"garbage after a restriction", "disable-model-invocation: true\ndisable-model-invocation: maybe", true, false},
		{"garbage after a user restriction", "user-invocable: false\nuser-invocable: maybe", false, true},
		{"garbage alone leaves the model default", "disable-model-invocation: maybe\nuser-invocable: false", false, true},
		{"empty value leaves the user default", "user-invocable:\ndisable-model-invocation: true", true, false},
		// A trailing YAML comment is the likeliest thing an author writes on
		// exactly this line, and reading it as part of the value left the
		// restriction silently unapplied.
		{"inline comment", "disable-model-invocation: true # humans only", true, false},
		{"inline comment false", "user-invocable: false  # background only", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := "---\ndescription: d\n" + tc.header + "\n---\nbody\n"
			t.Logf("header:\n%s", tc.header)
			// Untrimmed: ParseFrontmatterFields returns the body verbatim and
			// newSkill is what trims it.
			body, fm := ParseFrontmatterFields(raw)
			if body != "body\n" {
				t.Fatalf("body = %q, want %q", body, "body\n")
			}
			if fm.Description != "d" {
				t.Fatalf("description = %q, want %q", fm.Description, "d")
			}
			if fm.DisableModelInvocation != tc.wantModelBlocked {
				t.Fatalf("DisableModelInvocation = %v, want %v",
					fm.DisableModelInvocation, tc.wantModelBlocked)
			}
			if fm.DisableUserInvocation != tc.wantUserBlocked {
				t.Fatalf("DisableUserInvocation = %v, want %v",
					fm.DisableUserInvocation, tc.wantUserBlocked)
			}
		})
	}
}

// CRLF is what a header written in Notepad has, and the flags must survive it
// -- otherwise "true\r" falls through to the default and the restriction the
// author asked for silently is not applied.
func TestParseFrontmatterFields_CRLF(t *testing.T) {
	_, fm := ParseFrontmatterFields("---\r\ndescription: d\r\ndisable-model-invocation: true\r\n---\r\nbody\r\n")
	if !fm.DisableModelInvocation {
		t.Fatal("CRLF header should still disable model invocation")
	}
}

func TestSkillInvocationAccessors(t *testing.T) {
	var s Skill
	if !s.ModelInvocable() || !s.UserInvocable() {
		t.Fatal("the zero Skill must be invocable both ways")
	}
	s.Invocation.DisableModelInvocation = true
	if s.ModelInvocable() {
		t.Fatal("ModelInvocable should follow the flag")
	}
	if !s.UserInvocable() {
		t.Fatal("the two flags must be independent")
	}
}

// The flags have to survive discovery, not just the parser. A field parsed and
// then dropped on the floor between newSkill and the registry reads as working
// in every unit test of the parser and protects nothing.
func TestLoad_CarriesInvocationFlags(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	writeSkill(t, ProjectSkillsDir(project), "handsoff",
		"---\ndescription: no touching\ndisable-model-invocation: true\n---\nbody\n")
	writeSkill(t, ProjectSkillsDir(project), "background",
		"---\ndescription: reference only\nuser-invocable: false\n---\nbody\n")

	reg := Load(project)
	handsoff, ok := reg.Lookup("handsoff")
	if !ok {
		t.Fatal("handsoff did not resolve")
	}
	if handsoff.ModelInvocable() {
		t.Fatal("disable-model-invocation did not survive discovery")
	}
	if !handsoff.UserInvocable() {
		t.Fatal("disable-model-invocation must not touch user invocation")
	}

	background, ok := reg.Lookup("background")
	if !ok {
		t.Fatal("background did not resolve")
	}
	if background.UserInvocable() {
		t.Fatal("user-invocable: false did not survive discovery")
	}
	if !background.ModelInvocable() {
		t.Fatal("user-invocable: false must not touch model invocation")
	}
}

// The index is what the model is told exists. Advertising a skill it will then
// be refused spends tokens every turn and invites it to keep trying.
func TestIndexBlock_OmitsModelDisabledSkills(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	writeSkill(t, ProjectSkillsDir(project), "visible",
		"---\ndescription: the model may pick this\n---\nbody\n")
	writeSkill(t, ProjectSkillsDir(project), "handsoff",
		"---\ndescription: only a human runs this\ndisable-model-invocation: true\n---\nbody\n")

	index := Load(project).IndexBlock()
	if !strings.Contains(index, "visible") {
		t.Fatalf("expected the model-invocable skill in the index:\n%s", index)
	}
	if strings.Contains(index, "handsoff") {
		t.Fatalf("model-disabled skill leaked into the index:\n%s", index)
	}
	// Not reported as omitted either: "omitted" means the byte budget cut it,
	// which the model can act on by asking. This is a decision it cannot.
	if strings.Contains(index, "omitted") {
		t.Fatalf("a model-disabled skill was counted as budget-omitted:\n%s", index)
	}
}

// A header with a promise and no entries under it is worse than no block: it
// asserts a capability and then names none of it.
func TestIndexBlock_EmptyWhenEverySkillIsModelDisabled(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	// Shadow every builtin too, or the builtins alone would keep the index
	// non-empty and this would assert nothing.
	for _, name := range BuiltinNames() {
		writeSkill(t, ProjectSkillsDir(project), name,
			"---\ndescription: d\ndisable-model-invocation: true\n---\nbody\n")
	}
	writeSkill(t, ProjectSkillsDir(project), "handsoff",
		"---\ndescription: d\ndisable-model-invocation: true\n---\nbody\n")

	if got := Load(project).IndexBlock(); got != "" {
		t.Fatalf("expected no block at all, got:\n%s", got)
	}
}

// loadBuiltins builds skills straight from embedded directory names and never
// called ValidName, so the one loader whose names are compiled in was the one
// loader with no gate. newSkill is now the chokepoint every loader shares.
func TestNewSkill_RejectsInvalidName(t *testing.T) {
	for _, name := range []string{"", "has space", "a/b", "a:b", "..", "a.b"} {
		if _, err := newSkill(name, "---\ndescription: d\n---\nbody\n", SourceBuiltin, "", ""); err == nil {
			t.Fatalf("newSkill accepted invalid name %q", name)
		}
	}
	if _, err := newSkill("ok-name_1", "---\ndescription: d\n---\nbody\n", SourceBuiltin, "", ""); err != nil {
		t.Fatalf("newSkill rejected a valid name: %v", err)
	}
}

// Every shipped builtin must be reachable. A builtin that set either flag
// would be a build-time mistake nothing else would catch.
func TestBuiltins_AreInvocableBothWays(t *testing.T) {
	isolate(t)
	for _, s := range Load("").Skills() {
		if s.Source != SourceBuiltin {
			continue
		}
		if !s.ModelInvocable() || !s.UserInvocable() {
			t.Fatalf("builtin skill %q restricts invocation: %+v", s.Name, s.Invocation)
		}
	}
}

// The form most published skills actually use. A line-based parser that does
// not understand it read the value as the literal ">-" -- which is non-empty
// and single-line, so it passed every validation and produced a meaningless
// index entry, and once that was detected the skill was dropped instead.
// 22 of the 37 skills in one real ~/.agents/skills directory are written this
// way; refusing the form refuses the ecosystem.
func TestParseFrontmatterFields_FoldedBlockScalarDescription(t *testing.T) {
	raw := "---\n" +
		"name: autopilot\n" +
		"description: >-\n" +
		"  Keep a PR merge-ready by triaging comments, resolving clear conflicts, and\n" +
		"  fixing CI in a loop.\n" +
		"---\n" +
		"# Autopilot\n"

	body, fm := ParseFrontmatterFields(raw)
	want := "Keep a PR merge-ready by triaging comments, resolving clear conflicts, and fixing CI in a loop."
	if fm.Description != want {
		t.Fatalf("Description = %q,\n           want %q", fm.Description, want)
	}
	if len(fm.Warnings) != 0 {
		t.Fatalf("a well-formed block scalar produced warnings: %v", fm.Warnings)
	}
	if body != "# Autopilot\n" {
		t.Fatalf("body = %q", body)
	}
}

// The continuation lines must be consumed, not re-read as keys. A continuation
// line containing a colon would otherwise parse as one -- and a key parsed out
// of prose can set a flag its author never wrote.
func TestParseFrontmatterFields_BlockScalarDoesNotLeakIntoLaterKeys(t *testing.T) {
	raw := "---\n" +
		"description: >-\n" +
		"  Use when: you want a thing done.\n" +
		"  user-invocable: false\n" +
		"disable-model-invocation: true\n" +
		"---\n" +
		"body\n"

	_, fm := ParseFrontmatterFields(raw)
	if !strings.Contains(fm.Description, "Use when: you want a thing done.") {
		t.Fatalf("Description lost its colon-bearing line: %q", fm.Description)
	}
	// "user-invocable: false" was inside the block, so it is prose, not a key.
	if fm.DisableUserInvocation {
		t.Fatal("a line inside a block scalar was parsed as a key")
	}
	// The real key after the block still parses.
	if !fm.DisableModelInvocation {
		t.Fatal("the key following a block scalar was swallowed")
	}
}

func TestParseFrontmatterFields_BlockScalarIndicators(t *testing.T) {
	for _, indicator := range []string{">", ">-", ">+", "|", "|-", "|+"} {
		raw := "---\ndescription: " + indicator + "\n  one\n  two\n---\nbody\n"
		_, fm := ParseFrontmatterFields(raw)
		// Literal blocks fold too: a description is a one-line routing string
		// by contract, and keeping the newlines only to have newSkill reject
		// them would drop a working skill over formatting.
		if fm.Description != "one two" {
			t.Errorf("%s: Description = %q, want %q", indicator, fm.Description, "one two")
		}
	}
}

// A blank line inside a block is a paragraph break, not the end of the block.
func TestParseFrontmatterFields_BlockScalarSpansBlankLines(t *testing.T) {
	raw := "---\ndescription: >-\n  first para\n\n  second para\nuser-invocable: false\n---\nbody\n"
	_, fm := ParseFrontmatterFields(raw)
	if fm.Description != "first para second para" {
		t.Fatalf("Description = %q", fm.Description)
	}
	if !fm.DisableUserInvocation {
		t.Fatal("the key after a block containing a blank line was swallowed")
	}
}

// End to end: a real-shaped skill file must resolve rather than be dropped.
func TestLoad_AcceptsBlockScalarDescriptions(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	writeSkill(t, ProjectSkillsDir(project), "autopilot",
		"---\nname: autopilot\ndescription: >-\n  Keep a PR merge-ready by triaging\n  comments and fixing CI.\n---\nbody\n")

	reg := Load(project)
	if errs := reg.Errors(); len(errs) != 0 {
		t.Fatalf("a block-scalar skill was refused: %v", errs)
	}
	s, ok := reg.Lookup("autopilot")
	if !ok {
		t.Fatal("autopilot did not resolve")
	}
	if s.Description != "Keep a PR merge-ready by triaging comments and fixing CI." {
		t.Fatalf("Description = %q", s.Description)
	}
}
