package app

import (
	"strings"
	"testing"
	"time"
)

func TestParseWorkflowInputOverridesQuotedValues(t *testing.T) {
	got := parseInputOverrides([]string{`target="the`, `staged`, `diff"`, `note='two`, `words'`, "plain=value"})
	if got["target"] != "the staged diff" || got["note"] != "two words" || got["plain"] != "value" {
		t.Fatalf("parseInputOverrides() = %#v", got)
	}
}

func TestParseLoopArgs(t *testing.T) {
	cases := []struct {
		args     []string
		mode     loopMode
		interval time.Duration
		body     string
		wantErr  bool
	}{
		{[]string{"5m", "/cost"}, loopInterval, 5 * time.Minute, "/cost", false},
		{[]string{"30s", "check", "the", "build"}, loopInterval, 30 * time.Second, "check the build", false},
		{[]string{"keep", "improving", "the", "readme"}, loopSelfPaced, 0, "keep improving the readme", false},
		{[]string{"2h"}, 0, 0, "", true},         // interval but no body
		{[]string{"500ms", "x"}, 0, 0, "", true}, // interval too small
	}
	for _, c := range cases {
		mode, iv, body, err := parseLoopArgs(c.args)
		if c.wantErr {
			if err == nil {
				t.Fatalf("args %v: expected error", c.args)
			}
			continue
		}
		if err != nil {
			t.Fatalf("args %v: %v", c.args, err)
		}
		if mode != c.mode || iv != c.interval || body != c.body {
			t.Fatalf("args %v => (%d,%v,%q), want (%d,%v,%q)", c.args, mode, iv, body, c.mode, c.interval, c.body)
		}
	}
}

func TestLoopCommand_CreateListStop(t *testing.T) {
	r := newTestApp(t)
	// An interval loop with a /command body (dispatches, doesn't hit the agent).
	r.app.handleSlashCommand("loop", []string{"5m", "/cost"}, "/loop 5m /cost")
	if len(r.app.loops) != 1 {
		t.Fatalf("expected 1 loop, got %d", len(r.app.loops))
	}
	if !strings.Contains(r.app.renderLoops(), "every 5m") {
		t.Fatalf("list missing interval:\n%s", r.app.renderLoops())
	}
	r.app.handleSlashCommand("loop", []string{"stop", "all"}, "/loop stop all")
	if len(r.app.loops) != 0 {
		t.Fatalf("expected 0 loops after stop, got %d", len(r.app.loops))
	}
}

func TestLoopSelfPaced_StopsOnSentinel(t *testing.T) {
	r := newTestApp(t)
	if r.app.loops == nil {
		r.app.loops = map[string]*loopState{}
	}
	ls := &loopState{id: "loop1", mode: loopSelfPaced, body: "do a thing", maxIters: 25, iterations: 1}
	r.app.loops["loop1"] = ls
	r.app.activeLoopID = "loop1"
	// Model emitted the done sentinel this turn → loop should finish, not re-run.
	r.app.lastAgentText = "all done here\nLOOP_DONE"
	if cmd := r.app.onLoopTurnDone(); cmd != nil {
		t.Fatalf("expected loop to stop on sentinel (nil cmd)")
	}
	if _, ok := r.app.loops["loop1"]; ok {
		t.Fatalf("loop should be removed after sentinel")
	}
}

func TestLoopSelfPaced_CapsIterations(t *testing.T) {
	r := newTestApp(t)
	if r.app.loops == nil {
		r.app.loops = map[string]*loopState{}
	}
	ls := &loopState{id: "loop1", mode: loopSelfPaced, body: "x", maxIters: 3, iterations: 3}
	r.app.loops["loop1"] = ls
	r.app.activeLoopID = "loop1"
	r.app.lastAgentText = "still going" // no sentinel
	if cmd := r.app.onLoopTurnDone(); cmd != nil {
		t.Fatalf("expected loop to stop at iteration cap")
	}
	if _, ok := r.app.loops["loop1"]; ok {
		t.Fatalf("loop should be removed at cap")
	}
}

func TestLoopSelfPaced_StopsOnStructuredDecision(t *testing.T) {
	r := newTestApp(t)
	r.app.loops = map[string]*loopState{}
	ls := &loopState{id: "loop1", mode: loopSelfPaced, body: "do a thing", maxIters: 25, iterations: 2}
	r.app.loops["loop1"] = ls
	r.app.activeLoopID = "loop1"
	r.app.lastAgentText = `done
<packetcode-loop-decision>{"version":1,"decision":"stop","reason":"tests pass"}</packetcode-loop-decision>`

	if cmd := r.app.onLoopTurnDone(); cmd != nil {
		t.Fatalf("expected structured stop to finish the loop")
	}
	if _, ok := r.app.loops["loop1"]; ok {
		t.Fatalf("loop should be removed after structured stop")
	}
}

func TestParseLoopDecision_RejectsMalformedOrUnknownVersions(t *testing.T) {
	for _, text := range []string{
		`<packetcode-loop-decision>not json</packetcode-loop-decision>`,
		`<packetcode-loop-decision>{"version":2,"decision":"stop"}</packetcode-loop-decision>`,
		`<packetcode-loop-decision>{"version":1,"decision":"maybe"}</packetcode-loop-decision>`,
	} {
		if _, _, ok := parseLoopDecision(text); ok {
			t.Fatalf("unexpected valid decision for %q", text)
		}
	}
	stop, _, ok := parseLoopDecision(
		`<packetcode-loop-decision>{"version":1,"decision":"continue"}</packetcode-loop-decision>`,
	)
	if !ok || stop {
		t.Fatalf("continue decision should be valid without stopping")
	}
}
