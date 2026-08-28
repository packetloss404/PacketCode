package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Defaults for the sliding-window loop detector. A window of 10 turns with a
// threshold of 5 aborts a fully stuck run around its sixth tool turn — well
// before maxToolIterations, and with a reason attached.
const (
	defaultLoopWindowTurns = 10
	defaultLoopThreshold   = 5
)

// LoopDetectionSettings builds a config from plain values, so each call site
// can translate from its own configuration without internal/agent taking a
// dependency on the config package. Zero values keep the defaults.
func LoopDetectionSettings(disabled bool, window, threshold int) LoopDetectionConfig {
	return LoopDetectionConfig{Disabled: disabled, WindowTurns: window, Threshold: threshold}
}

// LoopDetectionConfig tunes the loop detector. Zero values take the defaults
// above, so callers that do not care can leave it unset. The knobs exist
// because the failure mode of getting this wrong is aborting a legitimate run:
// an operator must be able to loosen the threshold, or switch detection off,
// without a rebuild.
type LoopDetectionConfig struct {
	Disabled    bool
	WindowTurns int // turns retained in the sliding window
	Threshold   int // repeats tolerated inside the window before aborting
}

// toolObservation is what one completed tool call contributed to a turn: the
// tool's name, the arguments that actually ran (post approval edit — signing
// what the model asked for would let an approved edit hide a loop), and the
// authoritative content handed back to the model. Never the live chunk stream:
// a tool whose streamed bytes jitter but whose result is identical is stuck.
type toolObservation struct {
	name      string
	arguments string
	content   string
}

// turnFingerprint is one turn's collapsed signature plus the tool names behind
// it, kept so the abort can say which call repeated. Naming it is the point:
// "exceeded N tool iterations" never explained itself, and for background jobs
// this text is all the user gets in Job.Error.
type turnFingerprint struct {
	signature string
	names     []string
}

// loopDetector aborts a run whose tool calls have stopped making progress.
//
// maxToolIterations cannot tell "25 useful steps" from "the same failing
// read_file 25 times" — it just runs out and reports a count. The detector
// collapses each turn's tool calls into one signature and watches a sliding
// window of recent turns; a signature recurring more than the threshold means
// the model asked for exactly the same work and got exactly the same answer,
// repeatedly.
//
// The signature covers the output as well as the call, which is what keeps a
// legitimate repeat alive: polling a file while a build writes it produces
// identical calls with differing output, and differing output is progress.
//
// State belongs to a single run invocation. Hanging it off callAssembler (or
// the Agent) would leak one user turn's window into the next.
type loopDetector struct {
	windowTurns int
	threshold   int
	recent      []turnFingerprint
}

// newLoopDetector returns nil when detection is disabled; every method is
// nil-safe, so the caller needs no branch.
func newLoopDetector(cfg LoopDetectionConfig) *loopDetector {
	if cfg.Disabled {
		return nil
	}
	d := &loopDetector{windowTurns: cfg.WindowTurns, threshold: cfg.Threshold}
	if d.windowTurns <= 0 {
		d.windowTurns = defaultLoopWindowTurns
	}
	if d.threshold <= 0 {
		d.threshold = defaultLoopThreshold
	}
	return d
}

// observe records one completed turn and returns a non-nil error once that
// turn's signature has recurred too often inside the window. Turns that ran no
// tools carry no progress signal, so they are skipped rather than recorded —
// counting them would let interleaved chatter push a real loop out of view.
func (d *loopDetector) observe(obs []toolObservation) error {
	if d == nil || len(obs) == 0 {
		return nil
	}

	fp := fingerprintTurn(obs)
	d.recent = append(d.recent, fp)
	if len(d.recent) > d.windowTurns {
		d.recent = d.recent[len(d.recent)-d.windowTurns:]
	}

	repeats := 0
	for _, prev := range d.recent {
		if prev.signature == fp.signature {
			repeats++
		}
	}
	if repeats <= d.threshold {
		return nil
	}
	// "was called" rather than "ran": a call the policy or the user keeps
	// refusing never runs, and is just as stuck.
	return fmt.Errorf("loop detected: %s was called %d times in the last %d tool turns with identical arguments and identical output, so this run was stopped for making no progress — check whether the target exists or change the approach before retrying",
		strings.Join(fp.names, ", "), repeats, len(d.recent))
}

// fingerprintTurn hashes the turn's (name, executed arguments, result) triples.
// Triples are sorted so a model reordering its parallel calls between turns
// does not read as fresh work, and every field is length-prefixed so no
// combination of values can be rearranged into another turn's digest.
func fingerprintTurn(obs []toolObservation) turnFingerprint {
	triples := make([]string, 0, len(obs))
	seen := make(map[string]struct{}, len(obs))
	names := make([]string, 0, len(obs))
	for _, o := range obs {
		triples = append(triples, lengthPrefixed(o.name)+lengthPrefixed(o.arguments)+lengthPrefixed(o.content))
		if _, dup := seen[o.name]; !dup {
			seen[o.name] = struct{}{}
			names = append(names, o.name)
		}
	}
	sort.Strings(triples)
	sort.Strings(names)

	h := sha256.New()
	for _, t := range triples {
		h.Write([]byte(t))
	}
	return turnFingerprint{signature: hex.EncodeToString(h.Sum(nil)), names: names}
}

func lengthPrefixed(s string) string {
	return strconv.Itoa(len(s)) + ":" + s
}
