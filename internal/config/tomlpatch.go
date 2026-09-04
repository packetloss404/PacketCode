package config

// Surgical, in-place editing of a TOML document.
//
// config.toml is a file a person types. Its comments, key order, and spacing
// are theirs, and it may hold settings written by a build newer than this one.
// Round-tripping it through an encoder loses all of that at once: the encoder
// emits what the struct knows and nothing else, so every comment goes and
// every key this build never heard of is deleted permanently. This edits the
// text instead, replacing only the bytes that hold a value packetcode meant to
// change.
//
// It is deliberately small. Anything it cannot express -- a key inside an
// array of tables, a path that names a table rather than a setting, a value
// type TOML has no literal for -- is refused by name rather than approximated,
// because a wrong patch corrupts the file the user was trying to protect.

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// tomlEdit is one leaf assignment addressed by its full key path from the
// document root. remove clears the key rather than assigning to it, which is
// what an `omitempty` field going empty means on disk.
type tomlEdit struct {
	path   []string
	value  any
	remove bool
}

func (e tomlEdit) name() string { return renderTOMLKeyPath(e.path) }

// tomlEntry is one `key = value` found in the source, located by byte offset
// so its value can be replaced without disturbing the line around it.
type tomlEntry struct {
	path       []string
	valueStart int
	valueEnd   int
	lineStart  int
	lineEnd    int
}

type tomlHeader struct {
	path      []string
	isArray   bool
	lineStart int
	bodyStart int
}

type tomlDoc struct {
	src     string
	headers []tomlHeader
	entries []tomlEntry
	// arrays records the paths declared with [[double brackets]]. A key at or
	// below one of them occurs once per element, so "the" value to replace
	// does not exist; edits landing there are refused rather than guessed at.
	arrays  map[string]bool
	newline string
}

// tomlPathKey is a map key for a key path. NUL is used as the separator
// because a quoted TOML key may legally contain a dot, and joining on one
// would make providers."a.b" and providers.a.b the same setting.
func tomlPathKey(path []string) string { return strings.Join(path, "\x00") }

// patchTOML applies edits to src one at a time, re-scanning between each so an
// insertion that creates a table is visible to the edit that follows it.
// Configs are kilobytes; the repeated scan costs nothing and removes a whole
// class of stale-offset bugs.
func patchTOML(src string, edits []tomlEdit) (string, error) {
	out := src
	for _, edit := range edits {
		next, err := applyTOMLEdit(out, edit)
		if err != nil {
			return "", err
		}
		out = next
	}
	return out, nil
}

func applyTOMLEdit(src string, edit tomlEdit) (string, error) {
	doc, err := scanTOMLDoc(src)
	if err != nil {
		return "", err
	}
	target := tomlPathKey(edit.path)
	for prefix := range doc.arrays {
		if target == prefix || strings.HasPrefix(target, prefix+"\x00") {
			return "", fmt.Errorf("%s lives in an array of tables and cannot be edited in place", edit.name())
		}
	}

	var found []tomlEntry
	for _, entry := range doc.entries {
		if tomlPathKey(entry.path) == target {
			found = append(found, entry)
		}
	}
	if len(found) > 1 {
		// Two assignments to one key is not valid TOML, so the file was
		// already broken or the scanner misread it. Either way, editing one
		// of them at random is the wrong answer.
		return "", fmt.Errorf("%s is assigned more than once", edit.name())
	}
	if len(found) == 1 {
		entry := found[0]
		if edit.remove {
			return src[:entry.lineStart] + src[entry.lineEnd:], nil
		}
		text, err := renderTOMLValue(edit.value)
		if err != nil {
			return "", fmt.Errorf("%s: %w", edit.name(), err)
		}
		return src[:entry.valueStart] + text + src[entry.valueEnd:], nil
	}
	if edit.remove {
		return src, nil
	}
	// A path that already names a table cannot also hold a value; writing one
	// would produce a file that no longer parses.
	for _, header := range doc.headers {
		if tomlPathKey(header.path) == target {
			return "", fmt.Errorf("%s is a table, not a setting", edit.name())
		}
	}
	for _, entry := range doc.entries {
		if strings.HasPrefix(tomlPathKey(entry.path), target+"\x00") {
			return "", fmt.Errorf("%s is a table, not a setting", edit.name())
		}
	}
	return insertTOMLKey(doc, edit)
}

// insertTOMLKey adds a key the file does not have yet, into the deepest table
// that already contains it, or into a new table appended at the end.
func insertTOMLKey(doc *tomlDoc, edit tomlEdit) (string, error) {
	text, err := renderTOMLValue(edit.value)
	if err != nil {
		return "", fmt.Errorf("%s: %w", edit.name(), err)
	}
	newline := doc.newline

	best, bestDepth := -1, -1
	for i, header := range doc.headers {
		if header.isArray || len(header.path) >= len(edit.path) {
			continue
		}
		if !tomlPathHasPrefix(edit.path, header.path) || doc.inArrayContext(header.path) {
			continue
		}
		if len(header.path) > bestDepth {
			best, bestDepth = i, len(header.path)
		}
	}
	// A root key must go above the first table header: written below one it
	// would silently become a member of that table instead.
	if best >= 0 || len(edit.path) == 1 {
		at := doc.blockInsertPoint(best)
		prefixLen := 0
		if best >= 0 {
			prefixLen = len(doc.headers[best].path)
		}
		line := renderTOMLKeyPath(edit.path[prefixLen:]) + " = " + text + newline
		// Only a file whose last line has no newline can put the insertion
		// point mid-line; without this the new key lands on the end of the
		// old one and the file stops parsing.
		if head := doc.src[:at]; head != "" && !strings.HasSuffix(head, "\n") {
			line = newline + line
		}
		return doc.src[:at] + line + doc.src[at:], nil
	}

	body := doc.src
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += newline
	}
	if strings.TrimSpace(body) != "" && !strings.HasSuffix(body, newline+newline) {
		body += newline
	}
	body += "[" + renderTOMLKeyPath(edit.path[:len(edit.path)-1]) + "]" + newline
	body += renderTOMLKeyPath(edit.path[len(edit.path)-1:]) + " = " + text + newline
	return body, nil
}

// blockInsertPoint is the offset where a new key belongs in the table with the
// given header index (-1 for the document root): after the table's last
// setting, and before the blank lines and comments that lead into whatever
// comes next. A key appended after those would read as though it belonged to
// the following table.
func (d *tomlDoc) blockInsertPoint(headerIdx int) int {
	start, next := 0, 0
	if headerIdx >= 0 {
		start = d.headers[headerIdx].bodyStart
		next = headerIdx + 1
	}
	end := len(d.src)
	if next < len(d.headers) {
		end = d.headers[next].lineStart
	}
	for end > start {
		lineStart := tomlLineStartBefore(d.src, end)
		if lineStart < start {
			break
		}
		line := strings.TrimSpace(d.src[lineStart:end])
		if line != "" && !strings.HasPrefix(line, "#") {
			break
		}
		end = lineStart
	}
	return end
}

func (d *tomlDoc) inArrayContext(path []string) bool {
	for i := 1; i <= len(path); i++ {
		if d.arrays[tomlPathKey(path[:i])] {
			return true
		}
	}
	return false
}

func tomlPathHasPrefix(path, prefix []string) bool {
	if len(prefix) > len(path) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

// scanTOMLDoc locates every table header and every key assignment in src.
//
// It is a locator, not a parser: it learns where values start and end without
// interpreting them, which is all an in-place edit needs. Correctness rests on
// skipping the constructs where a '[', '=', '#' or newline does not mean what
// it looks like -- strings of all four kinds, comments, and bracketed values
// that run across lines.
func scanTOMLDoc(src string) (*tomlDoc, error) {
	doc := &tomlDoc{src: src, arrays: map[string]bool{}, newline: detectTOMLNewline(src)}
	var table []string
	i := 0
	for i < len(src) {
		lineStart := i
		j := skipTOMLInlineSpace(src, i)
		if j >= len(src) {
			break
		}
		if src[j] == '\n' || src[j] == '\r' || src[j] == '#' {
			i = tomlLineEndAfter(src, j)
			continue
		}
		if src[j] == '[' {
			isArray := j+1 < len(src) && src[j+1] == '['
			k := j + 1
			if isArray {
				k = j + 2
			}
			path, k, err := parseTOMLKey(src, k, ']')
			if err != nil {
				return nil, err
			}
			k++
			if isArray {
				if k >= len(src) || src[k] != ']' {
					return nil, fmt.Errorf("unterminated [[table]] header at offset %d", lineStart)
				}
				k++
			}
			body := tomlLineEndAfter(src, k)
			doc.headers = append(doc.headers, tomlHeader{path: path, isArray: isArray, lineStart: lineStart, bodyStart: body})
			if isArray {
				doc.arrays[tomlPathKey(path)] = true
			}
			table = path
			i = body
			continue
		}
		path, k, err := parseTOMLKey(src, j, '=')
		if err != nil {
			return nil, err
		}
		valueStart := skipTOMLInlineSpace(src, k+1)
		valueEnd, err := scanTOMLValue(src, valueStart)
		if err != nil {
			return nil, err
		}
		full := append(append([]string{}, table...), path...)
		lineEnd := tomlLineEndAfter(src, valueEnd)
		doc.entries = append(doc.entries, tomlEntry{
			path:       full,
			valueStart: valueStart,
			valueEnd:   valueEnd,
			lineStart:  lineStart,
			lineEnd:    lineEnd,
		})
		i = lineEnd
	}
	return doc, nil
}

// parseTOMLKey reads a bare, quoted, or dotted key and returns the offset of
// stop ('=' for an assignment, ']' for a table header).
func parseTOMLKey(src string, i int, stop byte) ([]string, int, error) {
	var parts []string
	for {
		i = skipTOMLInlineSpace(src, i)
		if i >= len(src) {
			return nil, i, fmt.Errorf("unterminated key at offset %d", i)
		}
		var (
			part string
			err  error
		)
		switch src[i] {
		case '"', '\'':
			part, i, err = scanTOMLQuotedString(src, i)
		default:
			start := i
			for i < len(src) && isTOMLBareKeyByte(src[i]) {
				i++
			}
			if i == start {
				return nil, i, fmt.Errorf("expected a key at offset %d", start)
			}
			part = src[start:i]
		}
		if err != nil {
			return nil, i, err
		}
		parts = append(parts, part)
		i = skipTOMLInlineSpace(src, i)
		if i < len(src) && src[i] == '.' {
			i++
			continue
		}
		if i < len(src) && src[i] == stop {
			return parts, i, nil
		}
		return nil, i, fmt.Errorf("expected %q after key at offset %d", string(stop), i)
	}
}

// scanTOMLValue returns the offset just past the value beginning at i, having
// stepped over any newlines the value legitimately spans.
func scanTOMLValue(src string, i int) (int, error) {
	if i >= len(src) {
		return i, fmt.Errorf("missing value at offset %d", i)
	}
	switch {
	case strings.HasPrefix(src[i:], `"""`), strings.HasPrefix(src[i:], `'''`):
		return scanTOMLMultilineString(src, i)
	case src[i] == '"', src[i] == '\'':
		_, end, err := scanTOMLQuotedString(src, i)
		return end, err
	case src[i] == '[':
		return scanTOMLBracketed(src, i, '[', ']')
	case src[i] == '{':
		return scanTOMLBracketed(src, i, '{', '}')
	}
	// Numbers, booleans and datetimes run to the end of the line or to a
	// comment. None of them may contain '#', so cutting there is safe.
	end := i
	for end < len(src) && src[end] != '\n' && src[end] != '#' {
		end++
	}
	for end > i && isTOMLSpaceByte(src[end-1]) {
		end--
	}
	if end == i {
		return i, fmt.Errorf("missing value at offset %d", i)
	}
	return end, nil
}

// scanTOMLQuotedString returns the decoded single-line string at i and the
// offset just past its closing quote.
func scanTOMLQuotedString(src string, i int) (string, int, error) {
	quote := src[i]
	start := i
	i++
	for i < len(src) {
		switch {
		case src[i] == '\n':
			return "", i, fmt.Errorf("unterminated string at offset %d", start)
		case quote == '"' && src[i] == '\\':
			i += 2
		case src[i] == quote:
			raw := src[start : i+1]
			if quote == '\'' {
				return raw[1 : len(raw)-1], i + 1, nil
			}
			// Go and TOML spell the same escapes the same way, so Unquote is
			// the decoder; anything it rejects is refused rather than guessed.
			decoded, err := strconv.Unquote(raw)
			if err != nil {
				return "", i, fmt.Errorf("unsupported escape in %s", raw)
			}
			return decoded, i + 1, nil
		default:
			i++
		}
	}
	return "", i, fmt.Errorf("unterminated string at offset %d", start)
}

func scanTOMLMultilineString(src string, i int) (int, error) {
	quote := src[i]
	start := i
	i += 3
	for i < len(src) {
		if quote == '"' && src[i] == '\\' {
			i += 2
			continue
		}
		if src[i] != quote {
			i++
			continue
		}
		run := 0
		for i+run < len(src) && src[i+run] == quote {
			run++
		}
		if run >= 3 {
			// Up to two quotes may sit against the closing delimiter, so a
			// run of three to five ends the string and anything longer is not
			// valid TOML.
			if run > 5 {
				return i, fmt.Errorf("malformed multi-line string at offset %d", start)
			}
			return i + run, nil
		}
		i += run
	}
	return i, fmt.Errorf("unterminated multi-line string at offset %d", start)
}

// scanTOMLBracketed steps over an array or inline table, including the
// strings, comments, and newlines an array may contain.
func scanTOMLBracketed(src string, i int, open, closer byte) (int, error) {
	start := i
	depth := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '#':
			i = tomlLineEndAfter(src, i)
		case strings.HasPrefix(src[i:], `"""`), strings.HasPrefix(src[i:], `'''`):
			end, err := scanTOMLMultilineString(src, i)
			if err != nil {
				return i, err
			}
			i = end
		case c == '"', c == '\'':
			_, end, err := scanTOMLQuotedString(src, i)
			if err != nil {
				return i, err
			}
			i = end
		case c == open:
			depth++
			i++
		case c == closer:
			depth--
			i++
			if depth == 0 {
				return i, nil
			}
		default:
			i++
		}
	}
	return i, fmt.Errorf("unterminated %q at offset %d", string(open), start)
}

func isTOMLBareKeyByte(c byte) bool {
	return c == '-' || c == '_' ||
		(c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z')
}

func isTOMLSpaceByte(c byte) bool { return c == ' ' || c == '\t' || c == '\r' }

func skipTOMLInlineSpace(src string, i int) int {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	return i
}

func tomlLineEndAfter(src string, i int) int {
	if j := strings.IndexByte(src[i:], '\n'); j >= 0 {
		return i + j + 1
	}
	return len(src)
}

// tomlLineStartBefore takes an offset that sits on a line boundary and returns
// the start of the line before it.
func tomlLineStartBefore(src string, end int) int {
	if end <= 0 {
		return 0
	}
	i := end - 1
	if src[i] == '\n' {
		i--
	}
	for i >= 0 && src[i] != '\n' {
		i--
	}
	return i + 1
}

// detectTOMLNewline picks the line ending the file already uses, so a config
// written on Windows does not acquire a lone LF line in the middle of it.
func detectTOMLNewline(src string) string {
	if i := strings.IndexByte(src, '\n'); i > 0 && src[i-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

func renderTOMLKeyPath(parts []string) string {
	rendered := make([]string, 0, len(parts))
	for _, part := range parts {
		rendered = append(rendered, renderTOMLKeyPart(part))
	}
	return strings.Join(rendered, ".")
}

func renderTOMLKeyPart(part string) string {
	if part == "" {
		return `""`
	}
	for i := 0; i < len(part); i++ {
		if !isTOMLBareKeyByte(part[i]) {
			return renderTOMLString(part)
		}
	}
	return part
}

// renderTOMLValue writes a decoded TOML value back as a literal. The accepted
// types are exactly what BurntSushi's decoder produces, because every value
// reaching here came from decoding a document this package encoded.
func renderTOMLValue(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", fmt.Errorf("has no value to write")
	case string:
		return renderTOMLString(v), nil
	case bool:
		return strconv.FormatBool(v), nil
	case int:
		return strconv.FormatInt(int64(v), 10), nil
	case int32:
		return strconv.FormatInt(int64(v), 10), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float32:
		return renderTOMLFloat(float64(v)), nil
	case float64:
		return renderTOMLFloat(v), nil
	case time.Time:
		return v.Format(time.RFC3339Nano), nil
	case []any:
		return renderTOMLArray(v)
	case []map[string]any:
		items := make([]any, 0, len(v))
		for _, item := range v {
			items = append(items, item)
		}
		return renderTOMLArray(items)
	case map[string]any:
		return renderTOMLInlineTable(v)
	default:
		return "", fmt.Errorf("has a value of type %T that cannot be written as TOML", value)
	}
}

func renderTOMLArray(items []any) (string, error) {
	rendered := make([]string, 0, len(items))
	for _, item := range items {
		text, err := renderTOMLValue(item)
		if err != nil {
			return "", err
		}
		rendered = append(rendered, text)
	}
	return "[" + strings.Join(rendered, ", ") + "]", nil
}

func renderTOMLInlineTable(table map[string]any) (string, error) {
	keys := make([]string, 0, len(table))
	for key := range table {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		text, err := renderTOMLValue(table[key])
		if err != nil {
			return "", err
		}
		pairs = append(pairs, renderTOMLKeyPart(key)+" = "+text)
	}
	return "{" + strings.Join(pairs, ", ") + "}", nil
}

func renderTOMLFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	text := strconv.FormatFloat(f, 'g', -1, 64)
	// A float that formats without a point or exponent would read back as an
	// integer, which is a different TOML type and a different Go field.
	if !strings.ContainsAny(text, ".eE") {
		text += ".0"
	}
	return text
}

func renderTOMLString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
