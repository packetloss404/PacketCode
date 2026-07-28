// Package terminaltext converts untrusted process/provider text into inert
// terminal-safe text. Tool output is data, not terminal control input: cursor,
// clipboard, title, paste, and mouse-mode escape sequences must never reach the
// host terminal through Packetcode's renderer.
package terminaltext

import (
	"strings"
	"unicode/utf8"
)

type parseMode uint8

const (
	modeText parseMode = iota
	modeEscape
	modeEscapeIntermediate
	modeCSI
	modeString
	modeStringEscape
)

// Sanitizer incrementally strips terminal control sequences, validates UTF-8,
// and normalizes carriage-return progress output. maxBytes bounds the retained
// safe tail when positive; zero keeps all safe text.
type Sanitizer struct {
	mode      parseMode
	out       []rune
	utf8Tail  []byte
	pendingCR bool
	maxBytes  int
}

func New(maxBytes int) *Sanitizer {
	return &Sanitizer{maxBytes: maxBytes}
}

// Clean sanitizes a complete string without applying a size cap.
func Clean(value string) string {
	s := New(0)
	s.Append(value)
	return s.String()
}

// Append consumes another raw chunk. Parser and UTF-8 state are retained so
// escape sequences and multibyte runes split across chunks cannot leak.
func (s *Sanitizer) Append(value string) {
	data := append(s.utf8Tail[:0], s.utf8Tail...)
	data = append(data, value...)
	s.utf8Tail = nil

	for i := 0; i < len(data); {
		b := data[i]
		switch s.mode {
		case modeEscape:
			i++
			switch b {
			case '[':
				s.mode = modeCSI
			case ']', 'P', 'X', '^', '_':
				s.mode = modeString
			case 0x1b:
				s.mode = modeEscape
			default:
				if b >= 0x20 && b <= 0x2f {
					s.mode = modeEscapeIntermediate
				} else {
					s.mode = modeText
				}
			}
			continue

		case modeEscapeIntermediate:
			i++
			if b == 0x1b {
				s.mode = modeEscape
			} else if b >= 0x30 && b <= 0x7e {
				s.mode = modeText
			}
			continue

		case modeCSI:
			i++
			if b == 0x1b {
				s.mode = modeEscape
			} else if b >= 0x40 && b <= 0x7e {
				s.mode = modeText
			}
			continue

		case modeString:
			i++
			switch b {
			case 0x07, 0x9c: // BEL or 8-bit ST
				s.mode = modeText
			case 0x1b:
				s.mode = modeStringEscape
			}
			continue

		case modeStringEscape:
			i++
			if b == '\\' {
				s.mode = modeText
			} else if b != 0x1b {
				s.mode = modeString
			}
			continue
		}

		// A bare carriage return is a progress-line rewrite. Delay acting on
		// it until the next byte so CRLF remains an ordinary newline.
		if s.pendingCR {
			if b == '\n' {
				s.out = append(s.out, '\n')
				s.pendingCR = false
				i++
				continue
			}
			s.truncateCurrentLine()
			s.pendingCR = false
		}

		switch b {
		case 0x1b:
			s.mode = modeEscape
			i++
		case 0x9b: // 8-bit CSI
			s.mode = modeCSI
			i++
		case 0x90, 0x98, 0x9d, 0x9e, 0x9f: // DCS/SOS/OSC/PM/APC
			s.mode = modeString
			i++
		case '\r':
			s.pendingCR = true
			i++
		case '\n':
			s.out = append(s.out, '\n')
			i++
		case '\t':
			s.out = append(s.out, ' ', ' ', ' ', ' ')
			i++
		case '\b', 0x7f:
			s.backspace()
			i++
		default:
			if b < 0x20 {
				i++ // discard remaining C0 controls
				continue
			}
			if b < utf8.RuneSelf {
				s.out = append(s.out, rune(b))
				i++
				continue
			}
			if !utf8.FullRune(data[i:]) {
				s.utf8Tail = append(s.utf8Tail, data[i:]...)
				i = len(data)
				continue
			}
			r, size := utf8.DecodeRune(data[i:])
			if r != utf8.RuneError || size > 1 {
				s.out = append(s.out, r)
			}
			i += size
		}
	}
	s.trim()
}

func (s *Sanitizer) String() string { return string(s.out) }

func (s *Sanitizer) truncateCurrentLine() {
	for len(s.out) > 0 && s.out[len(s.out)-1] != '\n' {
		s.out = s.out[:len(s.out)-1]
	}
}

func (s *Sanitizer) backspace() {
	if len(s.out) > 0 && s.out[len(s.out)-1] != '\n' {
		s.out = s.out[:len(s.out)-1]
	}
}

func (s *Sanitizer) trim() {
	if s.maxBytes <= 0 {
		return
	}
	value := string(s.out)
	if len(value) <= s.maxBytes {
		return
	}
	start := len(value) - s.maxBytes
	for start < len(value) && !utf8.RuneStart(value[start]) {
		start++
	}
	tail := value[start:]
	// Prefer starting at a whole output line when doing so does not discard
	// most of the retained tail.
	if newline := strings.IndexByte(tail, '\n'); newline >= 0 && newline < len(tail)/2 {
		tail = tail[newline+1:]
	}
	s.out = []rune(tail)
}
