package skills

import (
	"fmt"
	"regexp"
	"strings"
)

// Notes are the lines packetcode adds beneath a skill block to say what it did
// NOT do with the body.
//
// Both of the syntaxes handled here are ecosystem features packetcode does not
// implement, and in both cases the unimplemented version is worse than a
// failure. !`gh pr diff` reaches the model as those exact characters, and a
// model reading "PR diff: !`gh pr diff`" has been handed something that looks
// like a filled slot -- so it answers about a diff it never saw. A `$1` left
// standing reads as an argument someone supplied. Neither errors, neither is
// visibly empty, and both fail in the direction of invention.
//
// So the note is the cheap half of each feature: not running the command is a
// defensible position -- upstream ships a switch to disable it for the same
// reason -- but running nothing while presenting the text as though something
// ran is not a position at all. Saying so costs a sentence and closes the
// class.
//
// The notes are emitted after the closing marker rather than inside the block.
// A body cannot write text there: its own `<skill` and `</skill` sequences are
// defanged, so it cannot close its block early and forge a note that appears to
// come from packetcode. Inside the block that property would be lost, and for a
// project skill -- repository content, the case the framing exists for -- a
// forgeable operator note is worse than none.
//
// What a note must never do is fire on text that means something else. That
// risk is entirely on the placeholder side, and it is not hypothetical: run
// against the 37 skills actually installed on the author's machine, an earlier
// version of this file annotated 23 files, every one of them wrongly -- shell
// scripts whose `$1` is a shell positional parameter, Hyperdrive documentation
// whose `$1` is a SQL bind parameter, and capacity notes priced in dollars.
// Zero were placeholders. The scoping below is what that measurement bought.

// maxNotedItems bounds what one note names before it summarises the rest. Three
// is enough to recognise what was found, and matches the cap the allowed-tools
// refusal uses; a note that lists forty placeholders is one nobody finishes.
const maxNotedItems = 3

// bangCommandRe matches !`command` dynamic injection.
//
// The `!` must abut the backtick, which is what separates the syntax from prose
// ending in an exclamation before an inline code span. A newline inside the
// span is not allowed, so an unmatched backtick cannot swallow the rest of a
// document into one enormous "command".
var bangCommandRe = regexp.MustCompile("!`[^`\n]+`")

// positionalArgRe over-matches on purpose, and the caller narrows it.
//
// The trailing group exists to *reject*: it lets the filter tell `$1` from
// `$100`, `$1.50` and `$1,000`, which are money rather than placeholders.
//
// A separator only continues the run when a digit follows it. Consuming a bare
// `.` or `,` was the first version and it was wrong in the most ordinary way
// available: "Review PR $1 on branch $2." ends a sentence, so `$2.` was read as
// money and the placeholder went unreported -- the exact silence this file
// exists to break, reintroduced by the guard against a rarer mistake.
var positionalArgRe = regexp.MustCompile(`\$[1-9](?:[0-9]|[.,][0-9])*`)

// codeRe matches the spans where a `$1` is somebody else's syntax: fenced
// blocks first, so a fence containing backticks is consumed whole rather than
// being picked apart as inline spans, then inline code.
var codeRe = regexp.MustCompile("(?s)```.*?```|(?s)~~~.*?~~~|`[^`\n]*`")

// BodyNotes reports what packetcode recognised in a skill body and chose not to
// act on. Returns nil for the ordinary body, which is every body that uses
// neither syntax -- the common path adds nothing to the turn.
func BodyNotes(text string) []string {
	var notes []string
	if n, ok := bangNote(text); ok {
		notes = append(notes, n)
	}
	// Only outside code. A body demonstrating a shell script is not declaring
	// placeholders, and telling the model that its `$1` is an unfilled argument
	// would make a correct script read as a broken one.
	if n, ok := placeholderNote(stripCode(text)); ok {
		notes = append(notes, n)
	}
	return notes
}

// ResourceNotes is BodyNotes for a file beside a skill, which gets the command
// check and not the placeholder one.
//
// A resource is never expanded with arguments -- there is no path by which a
// `$1` in one could have been substituted -- so the placeholder note would be
// answering a question nobody asked, about text that is overwhelmingly some
// other language's syntax. The dollar signs in these files belong to sh and
// SQL, and the note has nothing true to say about either.
func ResourceNotes(text string) []string {
	if n, ok := bangNote(text); ok {
		return []string{n}
	}
	return nil
}

func bangNote(text string) (string, bool) {
	cmds := dedupeInOrder(bangCommandRe.FindAllString(text, -1))
	if len(cmds) == 0 {
		return "", false
	}
	one := len(cmds) == 1
	return fmt.Sprintf(
		"packetcode note: %s %s dynamic command injection, which packetcode does not "+
			"execute. Nothing ran, and no output from %s appears above -- the backticked "+
			"text reached you verbatim. Do not answer as though you had seen a result; if "+
			"you need one, run the command yourself through the usual tools, where it is "+
			"approved like anything else.",
		listItems(cmds),
		map[bool]string{true: "is", false: "are"}[one],
		map[bool]string{true: "it", false: "them"}[one]), true
}

func placeholderNote(text string) (string, bool) {
	args := dedupeInOrder(positionalArgs(text))
	if len(args) == 0 {
		return "", false
	}
	one := len(args) == 1
	return fmt.Sprintf(
		"packetcode note: %s %s, which packetcode does not substitute; %s reached you as "+
			"literal text rather than as a value anyone supplied. Anything the user typed "+
			"appears after this block, whole and unsplit.",
		listItems(args),
		map[bool]string{true: "is a positional placeholder", false: "are positional placeholders"}[one],
		map[bool]string{true: "it", false: "they"}[one]), true
}

// stripCode blanks code spans, preserving newlines so nothing on either side of
// a removed block runs together into a match that neither half contained.
func stripCode(text string) string {
	return codeRe.ReplaceAllStringFunc(text, func(m string) string {
		return strings.Repeat("\n", strings.Count(m, "\n"))
	})
}

// positionalArgs keeps only the matches that are a placeholder rather than a
// sum of money: exactly `$` and one digit, nothing trailing.
//
// A bare `$5` in prose remains indistinguishable from a placeholder and is
// still reported. That is the one over-match left standing: there is no signal
// in the text separating them, upstream has the same ambiguity and resolves it
// by substituting anyway, and with code excluded the surviving case is rare
// enough to be worth a surplus sentence.
func positionalArgs(text string) []string {
	var out []string
	for _, m := range positionalArgRe.FindAllString(text, -1) {
		if len(m) == 2 {
			out = append(out, m)
		}
	}
	return out
}

// listItems renders up to maxNotedItems quoted, counting any remainder.
func listItems(items []string) string {
	shown := items
	suffix := ""
	if len(shown) > maxNotedItems {
		shown = shown[:maxNotedItems]
		suffix = fmt.Sprintf(" and %d more", len(items)-maxNotedItems)
	}
	return strings.Join(quoteAll(shown), ", ") + suffix
}

// dedupeInOrder drops repeats while keeping first appearance. A body that uses
// `$1` in eight places is using one placeholder, and naming it eight times
// would spend the cap on a single item.
func dedupeInOrder(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
