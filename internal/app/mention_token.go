package app

import (
	"regexp"
	"strings"
)

// mentionQueryValid matches a run of characters that are all inside the
// @mention character class. An empty query (just "@") passes; any character
// outside the class fails. Built from the same class as mentionPattern so
// the popup's live token and the submit-time expander agree on what a
// path-like mention is.
var mentionQueryValid = regexp.MustCompile(`^[` + mentionCharClass + `]*$`)

// activeMentionToken inspects the text up to the caret and reports the
// @mention token the caret is currently sitting in, if any. It powers the
// interactive @-file autocomplete popup: the popup is open exactly when this
// returns ok=true.
//
// The token is the whitespace-delimited run ending at the caret. It is an
// active mention when it begins with "@" and everything after the "@" is a
// legal path character. A query containing any character outside the class
// (or a token not starting with "@") returns ok=false, which closes the
// popup and lets a literal "@" — or an "@" followed by prose — stand.
//
// start is the byte index of the "@" in textToCaret, so callers can splice
// the accepted path back over [start, caret). query is the text after "@".
func activeMentionToken(textToCaret string) (start int, query string, ok bool) {
	i := strings.LastIndexAny(textToCaret, " \t\n\r")
	tokStart := i + 1
	tok := textToCaret[tokStart:]
	if !strings.HasPrefix(tok, "@") {
		return 0, "", false
	}
	query = tok[1:]
	if !mentionQueryValid.MatchString(query) {
		return 0, "", false
	}
	return tokStart, query, true
}
