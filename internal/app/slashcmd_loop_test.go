package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/packetcode/packetcode/internal/workflow"
)

func TestWorkflowValidateCommand_DoesNotRequireExecutionEngine(t *testing.T) {
	r := newTestApp(t)
	wfDir := filepath.Join(r.tmp, ".packetcode", "workflows")
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const spec = `schema_version = 1
name = "checked"
[[phases]]
name = "p"
[[phases.steps]]
name = "work"
prompt = "do work"
`
	if err := os.WriteFile(filepath.Join(wfDir, "checked.toml"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	r.app.workflowLoader = workflow.NewLoader(r.tmp)
	r.app.workflow = nil

	_, cmd := r.app.handleWorkflowCommand([]string{"validate", "checked"})
	if cmd != nil {
		t.Fatal("validation should not start asynchronous work")
	}
	convContains(t, r.app, "checked is valid (schema v1")
	convContains(t, r.app, "1 unverified")
}

func TestParseWorkflowInputOverridesQuotedValues(t *testing.T) {
	got := parseInputOverrides([]string{`target="the`, `staged`, `diff"`, `note='two`, `words'`, "plain=value"})
	if got["target"] != "the staged diff" || got["note"] != "two words" || got["plain"] != "value" {
		t.Fatalf("parseInputOverrides() = %#v", got)
	}
}

func TestParseWorkflowRunArgsComputerPlacement(t *testing.T) {
	name, opts, inputs, err := parseWorkflowRunArgs([]string{
		"--computer", "production", "review", `target="the`, "staged", `diff"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "review" || opts.Computer != "production" {
		t.Fatalf("parseWorkflowRunArgs target = (%q, %q)", name, opts.Computer)
	}
	if got := parseInputOverrides(inputs)["target"]; got != "the staged diff" {
		t.Fatalf("target override = %q", got)
	}
}

func TestParseWorkflowRunArgsCompatibilityAndFlagErrors(t *testing.T) {
	name, opts, inputs, err := parseWorkflowRunArgs([]string{"review", "target=tree"})
	if err != nil || name != "review" || opts.Computer != "" || len(inputs) != 1 {
		t.Fatalf("legacy syntax parsed as (%q, %#v, %#v, %v)", name, opts, inputs, err)
	}
	if _, _, _, err := parseWorkflowRunArgs([]string{"--computer"}); err == nil {
		t.Fatal("expected missing --computer value error")
	}
	if _, _, _, err := parseWorkflowRunArgs([]string{"review", "--computer", "production"}); err == nil {
		t.Fatal("expected placement-after-name error")
	}
	if _, _, _, err := parseWorkflowRunArgs([]string{"--mystery", "review"}); err == nil {
		t.Fatal("expected unknown flag error")
	}
}

func TestWorkflowRunTargetPrefersResolvedName(t *testing.T) {
	run := workflow.RunSnapshot{
		Computer:     "prod-alias",
		ComputerID:   "pc_123",
		ComputerName: "production",
		WorkingDir:   "/srv/app",
	}
	if got := workflowRunTarget(run); got != "production" {
		t.Fatalf("workflowRunTarget = %q", got)
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

// a.loops is a map, so /loop list has to impose an order of its own. Twelve
// entries make both failure modes visible: Go's randomised map iteration, and
// a lexical sort that would place loop10 between loop1 and loop2.
func TestRenderLoops_ListsInCreationOrder(t *testing.T) {
	r := newTestApp(t)
	r.app.loops = map[string]*loopState{}
	for i := 1; i <= 12; i++ {
		id := fmt.Sprintf("loop%d", i)
		r.app.loops[id] = &loopState{
			id:       id,
			seq:      i,
			mode:     loopInterval,
			interval: time.Minute,
			body:     "check the build",
		}
	}

	rows := strings.Split(r.app.renderLoops(), "\n")[1:]
	if len(rows) != 12 {
		t.Fatalf("expected 12 rows, got %d", len(rows))
	}
	for i, row := range rows {
		want := fmt.Sprintf("loop%d", i+1)
		if got := strings.Fields(row)[0]; got != want {
			t.Fatalf("row %d = %q, want %q (full list:\n%s)", i, got, want, r.app.renderLoops())
		}
	}
}

// A self-paced loop started during a streaming turn used to register, appear
// in /loop list forever, and do nothing. runLoopBody's streaming guard sat
// before the line that claims loop ownership, so agentDoneMsg had nothing to
// re-run -- and slash commands dispatch during a stream, so typing /loop
// mid-turn hit this every time.
func TestLoopSelfPaced_StartedWhileStreamingKeepsOwnership(t *testing.T) {
	r := newTestApp(t)
	if r.app.loops == nil {
		r.app.loops = map[string]*loopState{}
	}
	ls := &loopState{id: "loop1", mode: loopSelfPaced, body: "do a thing", maxIters: 25}
	r.app.loops["loop1"] = ls

	// A turn is already running, which is the case this test exists for.
	r.app.streaming = true
	r.app.runLoopBody(ls)

	if len(r.app.queuedInputs) != 1 {
		t.Fatalf("expected the body to be queued, got %d queued", len(r.app.queuedInputs))
	}
	q := r.app.queuedInputs[0]
	if q.LoopID != "loop1" {
		t.Fatalf("queued turn does not carry loop ownership: LoopID = %q", q.LoopID)
	}
	// The iteration instruction has to travel with it too. Without it the model
	// is never told it is in a loop or how to declare the work finished, so
	// even a loop that resumed would run to its iteration cap every time.
	if !strings.Contains(q.Text, "Loop iteration 1") {
		t.Fatalf("queued turn lost the loop instruction:\n%s", q.Text)
	}
	// But the transcript shows what the user asked to loop, not the protocol.
	if q.Label() != "do a thing" {
		t.Fatalf("Label = %q, want the plain body", q.Label())
	}

	// Draining the queue must claim ownership, or agentDoneMsg still has
	// nothing to re-run. A real turn has to start for that, so the provider
	// hangs rather than completing: the claim happens as the turn begins.
	wireAgent(r, &hangingProvider{})
	r.app.streaming = false
	_, cmd := r.app.startNextQueuedInput()
	if r.app.activeLoopID != "loop1" {
		t.Fatalf("activeLoopID = %q after the queued turn started; the loop is orphaned", r.app.activeLoopID)
	}
	drainCancelledTurn(t, r.app, cmd)
}

// The immediate path must behave identically -- one builder, two callers.
func TestLoopSelfPaced_StartedIdleClaimsOwnership(t *testing.T) {
	r := newTestApp(t)
	if r.app.loops == nil {
		r.app.loops = map[string]*loopState{}
	}
	ls := &loopState{id: "loop2", mode: loopSelfPaced, body: "do a thing", maxIters: 25}
	r.app.loops["loop2"] = ls

	wireAgent(r, &hangingProvider{})
	r.app.streaming = false
	cmd := r.app.runLoopBody(ls)

	if r.app.activeLoopID != "loop2" {
		t.Fatalf("activeLoopID = %q, want loop2", r.app.activeLoopID)
	}
	if len(r.app.queuedInputs) != 0 {
		t.Fatalf("an idle start should not queue: %d queued", len(r.app.queuedInputs))
	}
	drainCancelledTurn(t, r.app, cmd)
}

// An interval loop is driven by its ticker, not by turn completion, so it must
// not claim ownership and hand itself to agentDoneMsg as well.
func TestLoopInterval_DoesNotClaimTurnOwnership(t *testing.T) {
	r := newTestApp(t)
	if r.app.loops == nil {
		r.app.loops = map[string]*loopState{}
	}
	ls := &loopState{id: "loop3", mode: loopInterval, body: "tick", maxIters: 25}
	r.app.loops["loop3"] = ls

	r.app.streaming = true
	r.app.runLoopBody(ls)

	if len(r.app.queuedInputs) != 1 {
		t.Fatalf("expected one queued turn, got %d", len(r.app.queuedInputs))
	}
	if got := r.app.queuedInputs[0].LoopID; got != "" {
		t.Fatalf("an interval loop claimed turn ownership: LoopID = %q", got)
	}
}

// Ctrl+C or a provider error ends the turn through EventError. A self-paced
// loop used to treat that like any other turn end and start the next
// iteration at once, so Ctrl+C could not stop a loop and a 401 became
// twenty-five back-to-back failing requests.
func TestLoopSelfPaced_StopsWhenTurnFailed(t *testing.T) {
	r := newTestApp(t)
	if r.app.loops == nil {
		r.app.loops = map[string]*loopState{}
	}
	ls := &loopState{id: "loop1", mode: loopSelfPaced, body: "fix the tests", maxIters: 25, iterations: 1}
	r.app.loops["loop1"] = ls
	r.app.activeLoopID = "loop1"
	r.app.turnFailed = true
	r.app.lastAgentText = "" // nothing came back: the turn was cancelled

	r.app.stopLoopAfterFailedTurn()

	if _, ok := r.app.loops["loop1"]; ok {
		t.Fatalf("loop should be removed after a failed turn")
	}
	if r.app.activeLoopID != "" {
		t.Fatalf("activeLoopID should be cleared, got %q", r.app.activeLoopID)
	}
}
