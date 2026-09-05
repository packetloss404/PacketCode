// Package diaglog is packetcode's opt-in structured diagnostic log.
//
// It is off unless PACKETCODE_LOG_FILE names an absolute path, in which case
// every process (TUI, run, acp, doctor) appends one JSON object per line to
// that file, mode 0600. The events are the ones an operator needs when a
// session went wrong and nobody was watching the terminal: which provider
// endpoint was called and what it answered, when a Codex token was refreshed,
// what the permission policy decided for each tool call and what the user
// answered, which MCP servers were spawned, which URLs fetch reached, which
// SSH computers were connected, and how each hook exited.
//
// What is never written here, by construction rather than by care at each
// call site: request and response bodies, tool arguments, file contents,
// prompt text, API keys, bearer tokens, and URL query strings (RedactURL
// strips them, and ErrText strips them from the URLs net/http embeds in its
// errors). A line is metadata about a call, not a copy of it.
//
// Discarded when disabled: L() returns a logger backed by slog.DiscardHandler,
// so a call site costs one interface call and no allocation.
package diaglog

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// EnvFile names the environment variable that turns the log on.
const EnvFile = "PACKETCODE_LOG_FILE"

var (
	mu      sync.RWMutex
	current = slog.New(slog.DiscardHandler)
	sink    *os.File
	path    string
)

// L returns the active logger. Safe to call before Init; it discards.
func L() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Enabled reports whether a file sink is open.
func Enabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return sink != nil
}

// Path reports the open log file, or "" when logging is off.
func Path() string {
	mu.RLock()
	defer mu.RUnlock()
	return path
}

// InitFromEnv opens the file named by PACKETCODE_LOG_FILE, if set. It
// returns the path it opened (empty when the variable is unset) and any error
// opening it. Callers report the error and continue: a diagnostic that cannot
// be written is not a reason to refuse to run.
func InitFromEnv() (string, error) {
	p := strings.TrimSpace(os.Getenv(EnvFile))
	if p == "" {
		return "", nil
	}
	return p, Init(p)
}

// Init opens p for append and routes L() to it. Relative paths are refused
// so the file cannot drift with the process working directory, which for the
// TUI is whatever project the user happened to start in.
func Init(p string) error {
	if !filepath.IsAbs(p) {
		return fmt.Errorf("%s must be an absolute path: %q", EnvFile, p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("%s: create %s: %w", EnvFile, filepath.Dir(p), err)
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("%s: open %s: %w", EnvFile, p, err)
	}
	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	mu.Lock()
	old := sink
	sink = f
	path = p
	current = slog.New(handler).With("pid", os.Getpid())
	mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

// Close flushes and closes the sink and returns L() to discarding.
func Close() {
	mu.Lock()
	old := sink
	sink = nil
	path = ""
	current = slog.New(slog.DiscardHandler)
	mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
}

// RedactURL renders a URL with its query, fragment, and userinfo removed:
// scheme, host, and path only. Query strings are where API keys have
// travelled (Gemini's ?key=, until audit patch P01) and where a model can put
// anything it likes in a fetch, so they never reach the log.
func RedactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	c := *u
	c.RawQuery = ""
	c.ForceQuery = false
	c.Fragment = ""
	c.RawFragment = ""
	c.User = nil
	return c.String()
}

// ErrText renders an error for the log. net/http wraps transport failures in
// *url.Error, whose message embeds the full request URL; the URL is redacted
// the same way RedactURL does before the text is returned.
func ErrText(err error) string {
	if err == nil {
		return ""
	}
	var ue *url.Error
	if errors.As(err, &ue) && ue != nil {
		target := ue.URL
		if parsed, perr := url.Parse(ue.URL); perr == nil {
			target = RedactURL(parsed)
		}
		inner := ""
		if ue.Err != nil {
			inner = ue.Err.Error()
		}
		return fmt.Sprintf("%s %s: %s", ue.Op, target, inner)
	}
	return err.Error()
}
