package openaicompat

import "strings"

// Interleaved-thinking models on OpenAI-compatible endpoints (MiniMax M2.x/M3)
// return their reasoning chain inline in the assistant `content` field, wrapped
// in <think>...</think>. Two things follow from that:
//
//   - The reasoning must not be rendered as ordinary assistant text.
//   - The reasoning must survive the turn. MiniMax's tool-use guide is explicit
//     that the complete response — thinking included — has to be appended to the
//     conversation history, and that the <think> tags are preserved exactly when
//     using the OpenAI-native format. Dropping it degrades multi-turn tool use.
//
// thinkFilter splits a token stream into visible text and reasoning text so the
// agent can route each to the right place. Tags are not guaranteed to arrive
// whole: "<thi" and "nk>" can land in separate SSE frames, so any suffix that
// could still grow into a tag is held back until the next chunk resolves it.
type thinkFilter struct {
	inThink bool
	buf     string
}

const (
	openTag  = "<think>"
	closeTag = "</think>"
)

// Write consumes one content delta and returns the portions that belong to the
// visible transcript and to the reasoning stream. Either may be empty. Text
// held back as a possible partial tag stays buffered until Write or Flush
// resolves it.
func (f *thinkFilter) Write(chunk string) (visible, reasoning string) {
	if chunk == "" {
		return "", ""
	}
	f.buf += chunk

	var vis, reas strings.Builder
	for {
		if !f.inThink {
			i := strings.Index(f.buf, openTag)
			if i >= 0 {
				vis.WriteString(f.buf[:i])
				f.buf = f.buf[i+len(openTag):]
				f.inThink = true
				continue
			}
			hold := partialTagSuffix(f.buf, openTag)
			vis.WriteString(f.buf[:len(f.buf)-hold])
			f.buf = f.buf[len(f.buf)-hold:]
			break
		}

		i := strings.Index(f.buf, closeTag)
		if i >= 0 {
			reas.WriteString(f.buf[:i])
			f.buf = f.buf[i+len(closeTag):]
			f.inThink = false
			continue
		}
		hold := partialTagSuffix(f.buf, closeTag)
		reas.WriteString(f.buf[:len(f.buf)-hold])
		f.buf = f.buf[len(f.buf)-hold:]
		break
	}
	return vis.String(), reas.String()
}

// Flush releases whatever is still buffered at end of stream. A model that
// stops mid-tag leaves text that never resolved; emit it rather than lose it,
// attributed to whichever side of the boundary we were on.
func (f *thinkFilter) Flush() (visible, reasoning string) {
	if f.buf == "" {
		return "", ""
	}
	rest := f.buf
	f.buf = ""
	if f.inThink {
		return "", rest
	}
	return rest, ""
}

// partialTagSuffix returns the length of the longest suffix of s that is a
// proper prefix of tag — the bytes that cannot yet be classified because the
// rest of the tag may arrive in the next frame.
func partialTagSuffix(s, tag string) int {
	max := len(tag) - 1
	if len(s) < max {
		max = len(s)
	}
	for n := max; n > 0; n-- {
		if strings.HasPrefix(tag, s[len(s)-n:]) {
			return n
		}
	}
	return 0
}
