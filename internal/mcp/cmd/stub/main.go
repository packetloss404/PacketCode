// Stub MCP server used by the mcp package's manager tests. Compiled on
// demand by TestMain in manager_test.go via `go build`. Reads JSON-RPC
// 2.0 newline-delimited requests on stdin and writes responses on
// stdout.
//
// Behavior is controlled by env vars:
//   - PACKETCODE_STUB_DELAY_MS — sleep this many ms before responding to
//     `initialize` (used to simulate slow startup).
//   - PACKETCODE_STUB_FAIL_INIT=1 — exit immediately on `initialize`.
//   - PACKETCODE_STUB_NO_TOOLS=1 — omit the `tools` capability.
//   - PACKETCODE_STUB_TOOLS=N — number of fake tools to expose (default 1).
//   - PACKETCODE_STUB_NAME=<name> — serverInfo.name (default "stub").
//   - PACKETCODE_STUB_PROTOCOL_VERSION=<version> — initialize protocol version.
//   - PACKETCODE_STUB_EXIT_AFTER_TOOLS=<code> — exit after replying tools/list.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

const protocolVersion = "2025-06-18"

type req struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *json.Number    `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type errObj struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type resp struct {
	JSONRPC string  `json:"jsonrpc"`
	ID      any     `json:"id"`
	Result  any     `json:"result,omitempty"`
	Error   *errObj `json:"error,omitempty"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 8<<20)
	w := bufio.NewWriter(os.Stdout)

	delayMS, _ := strconv.Atoi(os.Getenv("PACKETCODE_STUB_DELAY_MS"))
	failInit := os.Getenv("PACKETCODE_STUB_FAIL_INIT") == "1"
	noTools := os.Getenv("PACKETCODE_STUB_NO_TOOLS") == "1"
	toolCount, _ := strconv.Atoi(os.Getenv("PACKETCODE_STUB_TOOLS"))
	if toolCount <= 0 {
		toolCount = 1
	}
	name := os.Getenv("PACKETCODE_STUB_NAME")
	if name == "" {
		name = "stub"
	}
	proto := os.Getenv("PACKETCODE_STUB_PROTOCOL_VERSION")
	if proto == "" {
		proto = protocolVersion
	}
	exitAfterTools, _ := strconv.Atoi(os.Getenv("PACKETCODE_STUB_EXIT_AFTER_TOOLS"))

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var r req
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		if r.Method == "" || r.ID == nil {
			// Notification (we don't bother replying) or malformed — drop.
			continue
		}
		id, err := r.ID.Int64()
		if err != nil {
			continue
		}
		switch r.Method {
		case "initialize":
			if delayMS > 0 {
				time.Sleep(time.Duration(delayMS) * time.Millisecond)
			}
			if failInit {
				os.Exit(2)
			}
			caps := map[string]any{}
			if !noTools {
				caps["tools"] = map[string]any{}
			}
			writeResp(w, resp{
				JSONRPC: "2.0",
				ID:      id,
				Result: map[string]any{
					"protocolVersion": proto,
					"capabilities":    caps,
					"serverInfo": map[string]any{
						"name":    name,
						"version": "0.0.1-stub",
					},
				},
			})
		case "tools/list":
			tools := make([]map[string]any, 0, toolCount)
			for i := 0; i < toolCount; i++ {
				tools = append(tools, map[string]any{
					"name":        fmt.Sprintf("hello%d", i),
					"description": "stub tool",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{},
					},
				})
			}
			writeResp(w, resp{
				JSONRPC: "2.0",
				ID:      id,
				Result:  map[string]any{"tools": tools},
			})
			if exitAfterTools != 0 {
				os.Exit(exitAfterTools)
			}
		case "tools/call":
			writeResp(w, resp{
				JSONRPC: "2.0",
				ID:      id,
				Result: map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "stub-result"},
					},
					"isError": false,
				},
			})
		default:
			writeResp(w, resp{
				JSONRPC: "2.0",
				ID:      id,
				Error:   &errObj{Code: -32601, Message: "method not found"},
			})
		}
	}
}

func writeResp(w *bufio.Writer, r resp) {
	buf, err := json.Marshal(r)
	if err != nil {
		return
	}
	_, _ = w.Write(buf)
	_, _ = w.Write([]byte{'\n'})
	_ = w.Flush()
}
