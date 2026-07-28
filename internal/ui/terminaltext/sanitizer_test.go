package terminaltext

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizerStripsSplitTerminalControlSequences(t *testing.T) {
	s := New(0)
	s.Append("safe\x1b[?10")
	if got := s.String(); got != "safe" {
		t.Fatalf("first chunk = %q", got)
	}
	s.Append("03htext\x1b]52;c;Y2xpcGJvYXJk")
	s.Append("\x07done")
	if got, want := s.String(), "safetextdone"; got != want {
		t.Fatalf("sanitized = %q, want %q", got, want)
	}
}

func TestSanitizerPreservesSplitUTF8(t *testing.T) {
	raw := []byte("start 🧪 end")
	s := New(0)
	s.Append(string(raw[:8]))
	s.Append(string(raw[8:]))
	if got, want := s.String(), string(raw); got != want {
		t.Fatalf("sanitized = %q, want %q", got, want)
	}
}

func TestSanitizerNormalizesProgressAndBackspace(t *testing.T) {
	s := New(0)
	s.Append("progress 10%\r")
	s.Append("progress 20%\r\nabc\bD")
	if got, want := s.String(), "progress 20%\nabD"; got != want {
		t.Fatalf("sanitized = %q, want %q", got, want)
	}
}

func TestSanitizerTailCapKeepsValidUTF8(t *testing.T) {
	s := New(32)
	s.Append(strings.Repeat("🧪", 30) + "TAIL")
	got := s.String()
	if !utf8.ValidString(got) {
		t.Fatalf("tail is invalid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "TAIL") || len(got) > 32 {
		t.Fatalf("unexpected bounded tail: %q (%d bytes)", got, len(got))
	}
}
