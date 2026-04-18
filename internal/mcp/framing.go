package mcp

import (
	"bufio"
	"encoding/json"
	"io"
)

// scannerInitialBuf and scannerMaxBuf bound the bufio.Scanner reading
// MCP stdout. 1 MB initial, 8 MB max accommodates the upper bound on
// well-behaved MCP messages while still rejecting runaway servers.
const (
	scannerInitialBuf = 1 << 20
	scannerMaxBuf     = 8 << 20
)

// writeLine marshals msg to JSON, appends a single '\n', and writes the
// result to w in a single Write call. The caller is responsible for
// synchronising concurrent writers.
func writeLine(w io.Writer, msg any) error {
	buf, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	_, err = w.Write(buf)
	return err
}

// newScanner constructs a bufio.Scanner over r with our standard 1 MB
// initial / 8 MB max line buffer. The scanner splits on lines.
func newScanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, scannerInitialBuf), scannerMaxBuf)
	return s
}
