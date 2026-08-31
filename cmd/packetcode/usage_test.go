package main

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

// renderUsage builds a real flag set, the way main does, and renders help from
// it. Without registering the flags first the Flags section is empty -- which
// is a bug this test exists to catch, not something to work around here.
func renderUsage(t *testing.T) string {
	t.Helper()
	fs := flag.NewFlagSet("packetcode", flag.ContinueOnError)
	registerFlags(fs)
	var buf bytes.Buffer
	printUsage(&buf, fs)
	return buf.String()
}

// The defect this file exists to prevent, asserted directly.
//
// packetcode shipped four subcommands and `--help` named none of them, because
// dispatch and help were two hand-maintained lists. This walks the one table
// both now read, so a command added to dispatch without a help entry is not
// possible -- and if the table is ever split again, this fails.
func TestUsage_ListsEveryDispatchableCommand(t *testing.T) {
	help := renderUsage(t)
	commands := subcommands()
	if len(commands) == 0 {
		t.Fatal("no subcommands registered; this test would pass by finding nothing")
	}
	for _, c := range commands {
		if !strings.Contains(help, c.name) {
			t.Errorf("`packetcode %s` dispatches but is not named in --help", c.name)
		}
		if c.summary == "" {
			t.Errorf("`packetcode %s` has no summary, so help would list a bare name", c.name)
		}
		if c.run == nil {
			t.Errorf("`packetcode %s` is listed in help but dispatches to nothing", c.name)
		}
	}
}

func TestUsage_MentionsTheThingsAUserNeedsToStart(t *testing.T) {
	help := renderUsage(t)

	for _, want := range []string{
		"Usage:",
		"Commands:",
		"Flags:",
		// The flag list has to be in there. An earlier version of this printed
		// the command table with an empty Flags section, because help ran
		// before the flags were declared -- worse than the original, since it
		// looked complete.
		"-provider",
		"-permission-mode",
		// Subcommands have their own flags and help must say where to find them.
		"--help",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("help does not mention %q:\n%s", want, help)
		}
	}
}

// `packetcode doctor --help` must reach doctor. Answering it at the top level
// would print help for a command the user did not ask about, while looking
// like the answer to the one they did.
func TestWantsTopLevelHelp_OnlyTheFirstArgument(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"--help"}, true},
		{[]string{"-h"}, true},
		{[]string{"-help"}, true},
		{[]string{"help"}, true},
		{nil, false},
		{[]string{}, false},
		{[]string{"doctor"}, false},
		{[]string{"doctor", "--help"}, false},
		{[]string{"skills", "install", "-h"}, false},
		{[]string{"--provider", "openai"}, false},
	} {
		if got := wantsTopLevelHelp(tc.args); got != tc.want {
			t.Errorf("wantsTopLevelHelp(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

// Dispatch must answer for exactly the commands help advertises, and refuse
// anything else so it falls through to starting a session.
func TestDispatchSubcommand_MatchesTheTable(t *testing.T) {
	for _, c := range subcommands() {
		var out, errOut bytes.Buffer
		// "--help" keeps each command from doing real work; what is asserted
		// is that dispatch claimed the name at all.
		_, handled := dispatchSubcommand([]string{c.name, "--help"}, &out, &errOut)
		if !handled {
			t.Errorf("dispatch did not claim %q, which help advertises", c.name)
		}
	}

	var out, errOut bytes.Buffer
	if _, handled := dispatchSubcommand([]string{"definitely-not-a-command"}, &out, &errOut); handled {
		t.Error("dispatch claimed an unknown command instead of falling through to the session")
	}
	if _, handled := dispatchSubcommand(nil, &out, &errOut); handled {
		t.Error("dispatch claimed an empty argument list")
	}
}

// The table is the single source for help and dispatch, which keeps them
// consistent but cannot notice a command going missing from both at once --
// removing one from the table simply removes it from everywhere, and the
// consistency check above still passes.
//
// So the commands packetcode actually ships are named here. Deleting one then
// fails a test, which makes the removal a deliberate act rather than an
// oversight; that is the whole difference, and it is the difference that
// mattered when four of these were reachable but unadvertised.
func TestSubcommands_ShippedCommandsAreAllPresent(t *testing.T) {
	have := map[string]bool{}
	for _, c := range subcommands() {
		have[c.name] = true
	}
	for _, want := range []string{"doctor", "skills", "acp", "sugar"} {
		if !have[want] {
			t.Errorf("`packetcode %s` is gone from the command table; if that was "+
				"deliberate, remove it here too", want)
		}
	}
}
