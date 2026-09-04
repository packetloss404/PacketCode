package tools

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Post-edit diagnostics. A successful write_file or patch_file used to answer
// with nothing but "wrote it", so a model that had just broken the parse found
// out a turn later at best, and handed back code that does not build at worst.
// These helpers re-run the same in-process parser get_diagnostics uses, on the
// bytes that were just committed, and append what it finds to the result.
//
// Three rules keep the appended block from costing more than the silence it
// replaces:
//
//   - Nothing is ever shelled out. execute_command is permission-gated; running
//     "go build" behind a write would execute an unapproved command on every
//     edit and route around the whole permission model. The parser runs
//     in-process, so there is nothing to approve and nothing to install.
//   - Errors the file already had are subtracted. Reporting them blames the
//     model for damage it did not do and invites it to "fix" code nobody asked
//     it to touch.
//   - A file in a language with no in-process checker adds nothing at all --
//     not even a line saying so. The absence of noise is the feature.
const (
	// postEditDiagnosticBudget is a hard wall-clock bound on the analysis.
	// Diagnostics are advisory, so a slow answer is dropped rather than held:
	// delaying the write's own result is the worse failure.
	postEditDiagnosticBudget = 250 * time.Millisecond

	// Output bounds. A file with 400 parse errors must not flood the transcript
	// or evict the model's working context; the first few plus a count carry
	// the same signal.
	maxPostEditDiagnostics     = 10
	maxPostEditDiagnosticBytes = 2 * 1024
	// postEditTrailerReserve keeps room for the "... N more" and pre-existing
	// footer lines inside maxPostEditDiagnosticBytes, so the byte cap holds for
	// the whole block and not just the part written before the footers.
	postEditTrailerReserve = 160

	// maxPostEditSourceBytes skips very large (usually generated) files rather
	// than pay their parse cost on every edit. Parsing runs at roughly 2.5 MB/s
	// on a loaded machine and an edit parses twice (before and after), so this
	// cap keeps a normal file well inside the budget above -- the budget is the
	// backstop, not the everyday path. The largest hand-written file in this
	// repository is under 100 KB.
	maxPostEditSourceBytes = 256 * 1024

	// postEditPriorReadTimeout bounds the pre-edit read that exists only so
	// pre-existing errors can be told from introduced ones.
	postEditPriorReadTimeout = 2 * time.Second
)

// postEditDiagnosticReport is what a successful edit learned about its own
// result. A zero report means there is nothing worth telling the model.
type postEditDiagnosticReport struct {
	Block       string
	Introduced  int
	PreExisting int
}

// postEditDiagnosticsSupported reports whether an in-process checker exists for
// path. Only Go qualifies today. Every other extension returns false so that a
// Markdown or YAML write gains nothing rather than a "no checker available"
// line the model would have to read and discard on every edit.
func postEditDiagnosticsSupported(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".go")
}

// postEditDiagnostics analyses the bytes an edit just committed. prior is the
// file's contents before the edit, or nil when the file did not exist; it is
// used only to subtract errors that were already there. The returned report is
// empty whenever the edit introduced nothing.
func postEditDiagnostics(displayPath string, after, prior []byte) postEditDiagnosticReport {
	if !postEditDiagnosticsSupported(displayPath) || len(after) > maxPostEditSourceBytes {
		return postEditDiagnosticReport{}
	}
	// The goroutine exists purely to make postEditDiagnosticBudget a hard
	// bound. The channel is buffered so an over-budget parse that finishes
	// after the deadline exits instead of blocking forever on a send nobody
	// will receive.
	done := make(chan postEditDiagnosticReport, 1)
	go func() { done <- analyzePostEdit(displayPath, after, prior) }()
	select {
	case report := <-done:
		return report
	case <-time.After(postEditDiagnosticBudget):
		return postEditDiagnosticReport{}
	}
}

func analyzePostEdit(displayPath string, after, prior []byte) postEditDiagnosticReport {
	display := filepath.ToSlash(displayPath)
	afterDiags := goSourceDiagnostics(display, displayPath, string(after))
	if len(afterDiags) == 0 {
		return postEditDiagnosticReport{}
	}
	// prior == nil means the file is new, so every diagnostic is introduced.
	// Parsing must never fall back to reading displayPath from disk here: the
	// new contents are already committed there, which would subtract the
	// post-edit errors from themselves and report nothing.
	var priorDiags []codeDiagnostic
	if prior != nil {
		priorDiags = goSourceDiagnostics(display, displayPath, string(prior))
	}
	introduced, preExisting := splitPreExistingDiagnostics(afterDiags, priorDiags)
	if len(introduced) == 0 {
		return postEditDiagnosticReport{}
	}
	return postEditDiagnosticReport{
		Block:       formatPostEditDiagnostics(introduced, preExisting),
		Introduced:  len(introduced),
		PreExisting: preExisting,
	}
}

// splitPreExistingDiagnostics drops, for every message the file already
// produced before the edit, one occurrence of that message from the post-edit
// set, and returns what is left plus the number dropped.
//
// Matching is by message text and not by position because an edit shifts every
// line below it, so positions cannot be compared across the two parses. The
// trade is deliberate: an introduced error whose message reads exactly like one
// that was already there stays hidden. That is the safe direction to be wrong
// in -- a missed diagnostic costs a turn, a wrongly blamed one sends the model
// editing code it was never asked to change.
func splitPreExistingDiagnostics(after, before []codeDiagnostic) ([]codeDiagnostic, int) {
	if len(before) == 0 {
		return after, 0
	}
	remaining := make(map[string]int, len(before))
	for _, d := range before {
		remaining[d.Message]++
	}
	introduced := make([]codeDiagnostic, 0, len(after))
	dropped := 0
	for _, d := range after {
		if remaining[d.Message] > 0 {
			remaining[d.Message]--
			dropped++
			continue
		}
		introduced = append(introduced, d)
	}
	return introduced, dropped
}

// formatPostEditDiagnostics renders the block appended to a successful result.
// The first line says the edit succeeded, because a model that reads a bare
// list of errors as the whole answer will conclude the write failed and redo
// it.
func formatPostEditDiagnostics(introduced []codeDiagnostic, preExisting int) string {
	var b strings.Builder
	b.WriteString("Diagnostics for the new contents (the edit succeeded; go parser, syntax only):\n")
	shown := 0
	for _, d := range introduced {
		// truncateRunes, not a byte slice: a parser message can quote source
		// text, and cutting UTF-8 mid-rune would put U+FFFD in the transcript.
		line := fmt.Sprintf("%s:%d:%d: %s: %s\n", d.Path, d.Line, d.Column, d.Severity, truncateRunes(d.Message, maxCodeIntelSnippet))
		if shown >= maxPostEditDiagnostics || b.Len()+len(line)+postEditTrailerReserve > maxPostEditDiagnosticBytes {
			break
		}
		b.WriteString(line)
		shown++
	}
	if rest := len(introduced) - shown; rest > 0 {
		fmt.Fprintf(&b, "... %d more diagnostic(s) not shown.\n", rest)
	}
	if preExisting > 0 {
		fmt.Fprintf(&b, "%d diagnostic(s) this file already had before the edit are not listed.\n", preExisting)
	}
	return strings.TrimRight(b.String(), "\n")
}
