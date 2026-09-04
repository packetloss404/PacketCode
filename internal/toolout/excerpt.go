package toolout

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// markerReserve holds back enough of the budget for the elision marker so
	// the excerpt as a whole still respects the caller's limit.
	markerReserve = 512
	// lineWindow is how far a cut may move to land on a line boundary. Cutting
	// mid-line costs the model a garbled first/last line for no benefit; 1 KiB
	// is small enough that nothing meaningful is given up to avoid it.
	lineWindow = 1024
)

// Excerpt renders content down to at most limit bytes as head + marker + tail.
//
// Head alone would be the wrong shape for the outputs that overflow in
// practice: a failing test run, a compiler dump, or a long build all carry the
// verdict at the end. The marker between the two halves states how many bytes
// were withheld and, when handle is non-empty, exactly how to retrieve them —
// so the model learns a handle exists from the result itself rather than from
// out-of-band prompting.
func Excerpt(content string, limit int, handle string) string {
	if limit <= 0 || len(content) <= limit {
		return content
	}
	kept := limit - markerReserve
	if kept < 2 {
		kept = 2
	}
	head := alignHeadEnd(content, kept*2/3)
	tailStart := alignTailStart(content, len(content)-(kept-head))
	if tailStart < head {
		tailStart = head
	}
	omitted := tailStart - head
	return content[:head] + marker(len(content), head, len(content)-tailStart, head, tailStart, omitted, handle) + content[tailStart:]
}

func marker(total, headBytes, tailBytes, from, to, omitted int, handle string) string {
	var b strings.Builder
	b.WriteString("\n\n[packetcode: tool output was ")
	fmt.Fprintf(&b, "%d bytes; showing the first %d and the last %d. %d bytes at offsets %d-%d are omitted here", total, headBytes, tailBytes, omitted, from, to)
	if handle == "" {
		b.WriteString(" and were not retained, so they cannot be retrieved.]\n\n")
		return b.String()
	}
	fmt.Fprintf(&b, " but kept on disk.\nRetrieve them with read_tool_output {\"handle\": %q, \"offset\": %d, \"limit\": %d}, then repeat with the next_offset it reports until it says end of output.]\n\n", handle, from, DefaultPageBytes)
	return b.String()
}

// alignHeadEnd moves a head cut back to a rune boundary, then to the start of
// the line after the last complete line when one is close by.
func alignHeadEnd(content string, head int) int {
	if head <= 0 {
		return 0
	}
	if head > len(content) {
		head = len(content)
	}
	for head > 0 && head < len(content) && !utf8.RuneStart(content[head]) {
		head--
	}
	if idx := strings.LastIndexByte(content[:head], '\n'); idx >= 0 && head-idx <= lineWindow {
		return idx + 1
	}
	return head
}

// alignTailStart moves a tail cut forward to a rune boundary, then past the
// partial line it landed in when the next newline is close by.
func alignTailStart(content string, start int) int {
	if start <= 0 {
		return 0
	}
	if start > len(content) {
		return len(content)
	}
	for start < len(content) && !utf8.RuneStart(content[start]) {
		start++
	}
	if idx := strings.IndexByte(content[start:], '\n'); idx >= 0 && idx <= lineWindow {
		return start + idx + 1
	}
	return start
}
