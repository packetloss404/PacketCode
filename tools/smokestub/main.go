// Command smokestub is a credential-free OpenAI-compatible server used by
// smoke.sh to drive packetcode's real agent loop without a provider account.
// It is a development tool, not part of packetcode's supported surface.
//
// It exists because the useful smoke assertions are about the loop, not about
// a model: that the configured credential actually reaches the wire, that an
// approved write lands on disk, that an unapproved one does not, and that the
// permission floors hold. All four need a provider that answers; none of them
// need a real one.
//
// Two behaviours make the assertions airtight:
//
//   - It refuses any request whose bearer token is not the expected one, with
//     401. A run that succeeds therefore proves the key was resolved from the
//     configured source and sent.
//   - On its second turn it echoes every tool-role message back as assistant
//     text. Anything a tool handed the model therefore lands on packetcode's
//     stdout, so a test can assert that a secret or a denied command's output
//     is NOT there.
//
// Stdlib only: the smoke test adds no module dependency.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

type wireRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
		Name    string `json:"name"`
	} `json:"messages"`
	Tools []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tools"`
}

func main() {
	addrFile := flag.String("addr-file", "", "write the listening address here once bound")
	token := flag.String("token", "", "the bearer token every request must carry")
	flag.Parse()
	if *token == "" {
		fmt.Fprintln(os.Stderr, "smokestub: -token is required")
		os.Exit(2)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r, *token) {
			return
		}
		writeJSON(w, map[string]any{"data": []any{
			map[string]any{"id": "smoke-model", "object": "model", "owned_by": "smoke"},
		}})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r, *token) {
			return
		}
		var req wireRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":{"message":"bad request"}}`, http.StatusBadRequest)
			return
		}
		stream(w, plan(req))
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "smokestub: listen: %v\n", err)
		os.Exit(1)
	}
	addr := "http://" + listener.Addr().String()
	if *addrFile != "" {
		if err := os.WriteFile(*addrFile, []byte(addr), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "smokestub: write addr file: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Fprintln(os.Stderr, "smokestub listening on "+addr)
	if err := http.Serve(listener, mux); err != nil {
		fmt.Fprintf(os.Stderr, "smokestub: serve: %v\n", err)
		os.Exit(1)
	}
}

func authorized(w http.ResponseWriter, r *http.Request, want string) bool {
	if r.Header.Get("Authorization") == "Bearer "+want {
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"message":"smokestub: bearer token missing or wrong"}}`))
	return false
}

// step is one scripted assistant turn: either a tool call or final text.
type step struct {
	toolName string
	toolArgs string
	text     string
}

// plan decides what this turn should be. A request carrying tool-role messages
// is the follow-up turn, and is answered by echoing those results so a test can
// assert on what the tools actually handed the model.
func plan(req wireRequest) step {
	var toolResults []string
	lastUser := ""
	for _, m := range req.Messages {
		switch m.Role {
		case "tool":
			toolResults = append(toolResults, m.Content)
		case "user":
			lastUser = m.Content
		}
	}
	if len(toolResults) > 0 {
		return step{text: "TOOLRESULT " + strings.Join(toolResults, " | ")}
	}
	switch {
	case strings.Contains(lastUser, "SMOKE_WRITE"):
		return step{toolName: "write_file", toolArgs: `{"path":"smoke-artifact.txt","content":"written-by-smoke"}`}
	case strings.Contains(lastUser, "SMOKE_READ_ENV"):
		return step{toolName: "read_file", toolArgs: `{"path":".env"}`}
	case strings.Contains(lastUser, "SMOKE_DENY_SHELL"):
		// Compound on purpose. A deny rule on the `echo SMOKE_DENIED_MARKER`
		// prefix has to see through the `; :` tail; before the deny-floor fix
		// that trailing no-op was enough to fall through to allow.
		return step{toolName: "execute_command", toolArgs: `{"command":"echo SMOKE_DENIED_MARKER; :"}`}
	default:
		return step{text: "SMOKE_PLAIN_REPLY"}
	}
}

func stream(w http.ResponseWriter, s step) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)

	send := func(v any) {
		buf, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", buf)
		flusher.Flush()
	}

	finish := "stop"
	if s.toolName != "" {
		finish = "tool_calls"
		send(map[string]any{"choices": []any{map[string]any{
			"index": 0,
			"delta": map[string]any{"tool_calls": []any{map[string]any{
				"index": 0, "id": "smoke_call_1", "type": "function",
				"function": map[string]any{"name": s.toolName, "arguments": s.toolArgs},
			}}},
		}}})
	} else {
		send(map[string]any{"choices": []any{map[string]any{
			"index": 0,
			"delta": map[string]any{"role": "assistant", "content": s.text},
		}}})
	}
	send(map[string]any{"choices": []any{map[string]any{
		"index": 0, "delta": map[string]any{}, "finish_reason": finish,
	}}})
	// A usage frame ends the stream for packetcode's parser and gives the cost
	// tally something to record, which exercises that path too.
	send(map[string]any{
		"choices": []any{},
		"usage":   map[string]any{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18},
	})
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
