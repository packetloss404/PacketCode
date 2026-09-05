package diaglog

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDisabledByDefaultAndDiscardIsSafe(t *testing.T) {
	Close()
	if Enabled() {
		t.Fatal("enabled before Init")
	}
	L().Info("nothing", "k", "v") // must not panic or write anywhere
}

func TestInitWritesJSONLinesWithMode0600(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "packetcode.log")
	if err := Init(p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(Close)
	L().Info("provider.http", "method", "POST", "status", 200)
	Close()

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(data))
	var obj map[string]any
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("not one JSON object per line: %q: %v", line, err)
	}
	if obj["msg"] != "provider.http" || obj["method"] != "POST" || obj["status"] != float64(200) {
		t.Fatalf("unexpected record: %v", obj)
	}
	if _, ok := obj["pid"]; !ok {
		t.Fatalf("pid missing: %v", obj)
	}
	if info, err := os.Stat(p); err == nil && !isWindows() && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestInitRefusesRelativePath(t *testing.T) {
	if err := Init("relative.log"); err == nil {
		Close()
		t.Fatal("relative path accepted")
	}
}

func TestInitFromEnv(t *testing.T) {
	t.Setenv(EnvFile, "")
	if p, err := InitFromEnv(); err != nil || p != "" {
		t.Fatalf("unset variable: p=%q err=%v", p, err)
	}
	target := filepath.Join(t.TempDir(), "x.log")
	t.Setenv(EnvFile, target)
	p, err := InitFromEnv()
	t.Cleanup(Close)
	if err != nil || p != target || !Enabled() || Path() != target {
		t.Fatalf("p=%q err=%v enabled=%v path=%q", p, err, Enabled(), Path())
	}
}

func TestRedactURLAndErrText(t *testing.T) {
	u, _ := url.Parse("https://user:pw@api.example/v1/models?key=SECRET&x=1#frag")
	got := RedactURL(u)
	if got != "https://api.example/v1/models" {
		t.Fatalf("RedactURL = %q", got)
	}
	err := &url.Error{Op: "Get", URL: "https://api.example/v1/models?key=SECRET", Err: errors.New("connection refused")}
	text := ErrText(err)
	if strings.Contains(text, "SECRET") || !strings.Contains(text, "connection refused") {
		t.Fatalf("ErrText = %q", text)
	}
	if ErrText(nil) != "" || ErrText(errors.New("plain")) != "plain" {
		t.Fatal("plain errors must pass through")
	}
}
