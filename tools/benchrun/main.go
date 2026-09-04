// Command benchrun compares PacketCode's headless run path with its ACP path.
// It is a developer tool, not part of PacketCode's supported command surface.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
)

const defaultPrompt = "Call read_file exactly once with path go.mod, start_line 1, and end_line 12. Do not call another tool and do not call read_file again. Then reply with exactly two lines and no other text: the first line must be module=<module path from the file>; the second must be go=<version from the go directive>."

type usage struct {
	Input         int `json:"input"`
	Output        int `json:"output"`
	CacheCreation int `json:"cache_creation"`
	CacheRead     int `json:"cache_read"`
}

type sample struct {
	Pair            int     `json:"pair"`
	Order           int     `json:"order"`
	Path            string  `json:"path"`
	OK              bool    `json:"ok"`
	WallMS          int64   `json:"wall_ms"`
	ReportedMS      int64   `json:"reported_ms,omitempty"`
	InitializeMS    int64   `json:"initialize_ms,omitempty"`
	SessionNewMS    int64   `json:"session_new_ms,omitempty"`
	PromptMS        int64   `json:"prompt_ms,omitempty"`
	SessionID       string  `json:"session_id,omitempty"`
	Provider        string  `json:"provider,omitempty"`
	Model           string  `json:"model,omitempty"`
	Usage           usage   `json:"usage"`
	ProviderCalls   int     `json:"provider_calls"`
	ToolCalls       int     `json:"tool_calls"`
	Approvals       int     `json:"approvals"`
	OutputBytes     int     `json:"output_bytes"`
	OutputSHA256    string  `json:"output_sha256,omitempty"`
	CostUSD         float64 `json:"cost_usd"`
	Error           string  `json:"error,omitempty"`
	DiagnosticBytes int     `json:"diagnostic_bytes"`
}

type summary struct {
	ComparablePairs    int     `json:"comparable_pairs"`
	RunMedianWallMS    int64   `json:"run_median_wall_ms"`
	ACPMedianWallMS    int64   `json:"acp_median_wall_ms"`
	MedianPairedDelta  int64   `json:"median_paired_delta_ms"`
	ACPRunWallRatio    float64 `json:"acp_run_wall_ratio"`
	RunMedianReported  int64   `json:"run_median_reported_ms"`
	ACPMedianInit      int64   `json:"acp_median_initialize_ms"`
	ACPMedianSession   int64   `json:"acp_median_session_new_ms"`
	ACPMedianPrompt    int64   `json:"acp_median_prompt_ms"`
	TotalApprovals     int     `json:"total_approvals"`
	UsageMatchedByPair bool    `json:"usage_matched_by_pair"`
	CallsMatchedByPair bool    `json:"calls_matched_by_pair"`
}

type report struct {
	SchemaVersion  int       `json:"schema_version"`
	CapturedAt     time.Time `json:"captured_at"`
	Commit         string    `json:"commit"`
	WorktreeDirty  bool      `json:"worktree_dirty"`
	Platform       string    `json:"platform"`
	PacketCode     string    `json:"packetcode"`
	Provider       string    `json:"provider"`
	Model          string    `json:"model"`
	PermissionMode string    `json:"permission_mode"`
	Prompt         string    `json:"prompt"`
	PromptSHA256   string    `json:"prompt_sha256"`
	RunsPerPath    int       `json:"runs_per_path"`
	Samples        []sample  `json:"samples"`
	Summary        summary   `json:"summary"`
}

type options struct {
	packetcode     string
	sourceHome     string
	workspace      string
	provider       string
	model          string
	permissionMode string
	prompt         string
	runs           int
	timeout        time.Duration
	output         string
}

func main() {
	opts, err := parseFlags()
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchrun:", err)
		os.Exit(2)
	}
	if err := execute(opts); err != nil {
		fmt.Fprintln(os.Stderr, "benchrun:", err)
		os.Exit(1)
	}
}

func parseFlags() (options, error) {
	var opts options
	defaultHome, err := packetcodeHome()
	if err != nil {
		return opts, err
	}
	defaultWorkspace, err := os.Getwd()
	if err != nil {
		return opts, err
	}
	flag.StringVar(&opts.packetcode, "packetcode", "packetcode", "path to the prebuilt packetcode executable")
	flag.StringVar(&opts.sourceHome, "source-home", defaultHome, "configured PacketCode home copied into an isolated benchmark home")
	flag.StringVar(&opts.workspace, "workspace", defaultWorkspace, "absolute workspace tested by both paths")
	flag.StringVar(&opts.provider, "provider", "openai", "provider used by both paths")
	flag.StringVar(&opts.model, "model", "gpt-5.6-sol", "model used by both paths")
	flag.StringVar(&opts.permissionMode, "permission-mode", "read-only", "permission mode used by both paths")
	flag.StringVar(&opts.prompt, "prompt", defaultPrompt, "identical prompt sent through both paths")
	flag.IntVar(&opts.runs, "runs", 3, "fresh-session pairs to run")
	flag.DurationVar(&opts.timeout, "timeout", 10*time.Minute, "timeout for each process")
	flag.StringVar(&opts.output, "output", "", "optional JSON report path; stdout when empty")
	flag.Parse()
	if flag.NArg() != 0 {
		return opts, errors.New("positional arguments are not accepted")
	}
	if opts.runs < 1 {
		return opts, errors.New("runs must be at least 1")
	}
	if !filepath.IsAbs(opts.workspace) {
		return opts, errors.New("workspace must be absolute")
	}
	return opts, nil
}

func execute(opts options) (retErr error) {
	packetcodePath, err := exec.LookPath(opts.packetcode)
	if err != nil {
		return fmt.Errorf("find packetcode executable: %w", err)
	}
	isolatedHome, err := os.MkdirTemp("", "packetcode-benchrun-")
	if err != nil {
		return fmt.Errorf("create isolated home: %w", err)
	}
	defer func() {
		if cleanupErr := removeIsolatedHome(isolatedHome); cleanupErr != nil && retErr == nil {
			retErr = cleanupErr
		}
	}()
	if err := copyConfiguration(opts.sourceHome, isolatedHome); err != nil {
		return err
	}

	rep := report{
		SchemaVersion:  1,
		CapturedAt:     time.Now().UTC(),
		Commit:         gitCommit(opts.workspace),
		WorktreeDirty:  gitDirty(opts.workspace),
		Platform:       runtime.GOOS + "/" + runtime.GOARCH,
		PacketCode:     packetcodePath,
		Provider:       opts.provider,
		Model:          opts.model,
		PermissionMode: opts.permissionMode,
		Prompt:         opts.prompt,
		PromptSHA256:   digest(opts.prompt),
		RunsPerPath:    opts.runs,
	}

	for pair := 1; pair <= opts.runs; pair++ {
		paths := []string{"run", "acp"}
		if pair%2 == 0 {
			paths = []string{"acp", "run"}
		}
		for order, path := range paths {
			fmt.Fprintf(os.Stderr, "benchrun: pair %d/%d, %s (%d/2)\n", pair, opts.runs, path, order+1)
			var got sample
			if path == "run" {
				got = benchmarkRun(opts, packetcodePath, isolatedHome)
			} else {
				got = benchmarkACP(opts, packetcodePath, isolatedHome)
			}
			got.Pair = pair
			got.Order = order + 1
			rep.Samples = append(rep.Samples, got)
			if !got.OK {
				return fmt.Errorf("pair %d %s failed: %s", pair, path, got.Error)
			}
		}
	}
	rep.Summary = summarize(rep.Samples, opts.runs)
	return writeReport(opts.output, rep)
}

type runJSON struct {
	OK        bool   `json:"ok"`
	SessionID string `json:"session_id"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Output    string `json:"output"`
	ElapsedMS int64  `json:"elapsed_ms"`
	Error     string `json:"error"`
	Usage     struct {
		Input         int `json:"input_tokens"`
		Output        int `json:"output_tokens"`
		CacheCreation int `json:"cache_creation_input_tokens"`
		CacheRead     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

func benchmarkRun(opts options, executable, home string) sample {
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "run", "--provider", opts.provider, "--model", opts.model, "--permission-mode", opts.permissionMode, "--json", opts.prompt)
	cmd.Dir = opts.workspace
	cmd.Env = environmentWithHome(home)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	started := time.Now()
	err := cmd.Run()
	got := sample{Path: "run", WallMS: time.Since(started).Milliseconds(), DiagnosticBytes: stderr.Len()}
	var result runJSON
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		got.Error = fmt.Sprintf("decode run JSON: %v (process: %v)", decodeErr, err)
		return got
	}
	got.OK = err == nil && result.OK
	got.ReportedMS = result.ElapsedMS
	got.SessionID = result.SessionID
	got.Provider = result.Provider
	got.Model = result.Model
	got.Usage = usage{
		Input: result.Usage.Input, Output: result.Usage.Output,
		CacheCreation: result.Usage.CacheCreation, CacheRead: result.Usage.CacheRead,
	}
	got.OutputBytes = len(result.Output)
	got.OutputSHA256 = digest(result.Output)
	if err != nil || result.Error != "" {
		got.Error = strings.TrimSpace(fmt.Sprintf("%v %s", err, result.Error))
	}
	enrichFromSession(&got, home)
	return got
}

type acpClient struct {
	stdin     io.WriteCloser
	scanner   *bufio.Scanner
	approvals int
	toolCalls int
	output    strings.Builder
}

type synchronizedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *synchronizedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

func benchmarkACP(opts options, executable, home string) sample {
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "acp", "--provider", opts.provider, "--model", opts.model, "--permission-mode", opts.permissionMode)
	cmd.Dir = opts.workspace
	cmd.Env = environmentWithHome(home)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return sample{Path: "acp", Error: err.Error()}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return sample{Path: "acp", Error: err.Error()}
	}
	var stderr synchronizedBuffer
	cmd.Stderr = &stderr
	client := &acpClient{stdin: stdin, scanner: bufio.NewScanner(stdout)}
	client.scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	started := time.Now()
	if err := cmd.Start(); err != nil {
		return sample{Path: "acp", Error: err.Error()}
	}
	got := sample{Path: "acp"}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	if err := client.send("init", "initialize", map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}}); err != nil {
		got.Error = err.Error()
		return got
	}
	if _, err := client.receive("init"); err != nil {
		got.Error = err.Error()
		return got
	}
	got.InitializeMS = time.Since(started).Milliseconds()

	sessionStarted := time.Now()
	if err := client.send("new", "session/new", map[string]any{
		"cwd": opts.workspace, "mcpServers": []any{},
		"_packetcode": map[string]any{"provider": opts.provider, "model": opts.model, "permissionMode": opts.permissionMode},
	}); err != nil {
		got.Error = err.Error()
		return got
	}
	newResponse, err := client.receive("new")
	if err != nil {
		got.Error = err.Error()
		return got
	}
	got.SessionNewMS = time.Since(sessionStarted).Milliseconds()
	result, err := responseResult(newResponse)
	if err != nil {
		got.Error = err.Error()
		return got
	}
	got.SessionID, _ = result["sessionId"].(string)

	promptStarted := time.Now()
	if err := client.send("prompt", "session/prompt", map[string]any{
		"sessionId": got.SessionID,
		"prompt":    []any{map[string]any{"type": "text", "text": opts.prompt}},
	}); err != nil {
		got.Error = err.Error()
		return got
	}
	promptResponse, err := client.receive("prompt")
	got.PromptMS = time.Since(promptStarted).Milliseconds()
	got.WallMS = time.Since(started).Milliseconds()
	got.DiagnosticBytes = stderr.Len()
	got.Approvals = client.approvals
	got.ToolCalls = client.toolCalls
	got.OutputBytes = client.output.Len()
	got.OutputSHA256 = digest(client.output.String())
	if err != nil {
		got.Error = err.Error()
		return got
	}
	promptResult, err := responseResult(promptResponse)
	if err != nil {
		got.Error = err.Error()
		return got
	}
	got.OK = promptResult["stopReason"] == "end_turn" && got.Approvals == 0
	if !got.OK {
		got.Error = fmt.Sprintf("stopReason=%v approvals=%d", promptResult["stopReason"], got.Approvals)
	}
	enrichFromSession(&got, home)
	return got
}

func (c *acpClient) send(id, method string, params any) error {
	message := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
	encoded, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(c.stdin, "%s\n", encoded)
	return err
}

func (c *acpClient) receive(target string) (map[string]any, error) {
	for c.scanner.Scan() {
		var message map[string]any
		if err := json.Unmarshal(c.scanner.Bytes(), &message); err != nil {
			return nil, fmt.Errorf("decode ACP message: %w", err)
		}
		if method, _ := message["method"].(string); method == "session/request_permission" {
			c.approvals++
			id := fmt.Sprint(message["id"])
			if err := c.sendResult(id, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "reject_once"}}); err != nil {
				return nil, err
			}
			continue
		}
		c.observeUpdate(message)
		if fmt.Sprint(message["id"]) == target {
			return message, nil
		}
	}
	if err := c.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

func (c *acpClient) sendResult(id string, result any) error {
	encoded, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(c.stdin, "%s\n", encoded)
	return err
}

func (c *acpClient) observeUpdate(message map[string]any) {
	if message["method"] != "session/update" {
		return
	}
	params, _ := message["params"].(map[string]any)
	update, _ := params["update"].(map[string]any)
	switch update["sessionUpdate"] {
	case "tool_call":
		c.toolCalls++
	case "agent_message_chunk":
		content, _ := update["content"].(map[string]any)
		text, _ := content["text"].(string)
		c.output.WriteString(text)
	}
}

func responseResult(message map[string]any) (map[string]any, error) {
	if raw, ok := message["error"]; ok && raw != nil {
		return nil, fmt.Errorf("ACP error: %v", raw)
	}
	result, ok := message["result"].(map[string]any)
	if !ok {
		return nil, errors.New("ACP response has no result object")
	}
	return result, nil
}

func enrichFromSession(got *sample, home string) {
	if got.SessionID == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(home, "sessions", got.SessionID+".json"))
	if err != nil {
		got.Error = appendError(got.Error, "read session: "+err.Error())
		got.OK = false
		return
	}
	var saved session.Session
	if err := json.Unmarshal(data, &saved); err != nil {
		got.Error = appendError(got.Error, "decode session: "+err.Error())
		got.OK = false
		return
	}
	got.Provider = saved.Provider
	got.Model = saved.Model
	got.Usage = usage{
		Input: saved.TokenUsage.TotalInput, Output: saved.TokenUsage.TotalOutput,
		CacheCreation: saved.TokenUsage.TotalCacheCreation, CacheRead: saved.TokenUsage.TotalCacheRead,
	}
	got.CostUSD = saved.Cost.TotalUSD
	got.ProviderCalls = 0
	got.ToolCalls = 0
	for _, message := range saved.Messages {
		if message.Role == provider.RoleAssistant {
			got.ProviderCalls++
			got.ToolCalls += len(message.ToolCalls)
		}
	}
}

func summarize(samples []sample, pairs int) summary {
	var out summary
	var runWall, acpWall, runReported, acpInit, acpSession, acpPrompt, paired []int64
	out.UsageMatchedByPair = true
	out.CallsMatchedByPair = true
	for _, got := range samples {
		out.TotalApprovals += got.Approvals
		if !got.OK {
			continue
		}
		if got.Path == "run" {
			runWall = append(runWall, got.WallMS)
			runReported = append(runReported, got.ReportedMS)
		} else {
			acpWall = append(acpWall, got.WallMS)
			acpInit = append(acpInit, got.InitializeMS)
			acpSession = append(acpSession, got.SessionNewMS)
			acpPrompt = append(acpPrompt, got.PromptMS)
		}
	}
	for pair := 1; pair <= pairs; pair++ {
		var run, acp *sample
		for i := range samples {
			if samples[i].Pair != pair || !samples[i].OK {
				continue
			}
			if samples[i].Path == "run" {
				run = &samples[i]
			} else {
				acp = &samples[i]
			}
		}
		if run == nil || acp == nil || run.Approvals != 0 || acp.Approvals != 0 {
			continue
		}
		if run.Usage != acp.Usage {
			out.UsageMatchedByPair = false
		}
		if run.ProviderCalls != acp.ProviderCalls || run.ToolCalls != acp.ToolCalls {
			out.CallsMatchedByPair = false
			continue
		}
		out.ComparablePairs++
		paired = append(paired, acp.WallMS-run.WallMS)
	}
	out.RunMedianWallMS = median(runWall)
	out.ACPMedianWallMS = median(acpWall)
	out.MedianPairedDelta = median(paired)
	if out.RunMedianWallMS > 0 {
		out.ACPRunWallRatio = float64(out.ACPMedianWallMS) / float64(out.RunMedianWallMS)
	}
	out.RunMedianReported = median(runReported)
	out.ACPMedianInit = median(acpInit)
	out.ACPMedianSession = median(acpSession)
	out.ACPMedianPrompt = median(acpPrompt)
	return out
}

func median(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	values = append([]int64(nil), values...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return (values[middle-1] + values[middle]) / 2
}

func writeReport(path string, rep report) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func copyConfiguration(sourceHome, targetHome string) error {
	for _, name := range []string{"config.toml", ".env"} {
		source := filepath.Join(sourceHome, name)
		data, err := os.ReadFile(source)
		if errors.Is(err, os.ErrNotExist) && name == ".env" {
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", source, err)
		}
		if err := os.WriteFile(filepath.Join(targetHome, name), data, 0o600); err != nil {
			return fmt.Errorf("copy %s: %w", name, err)
		}
	}
	return nil
}

func removeIsolatedHome(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve isolated home: %w", err)
	}
	tempRoot, err := filepath.Abs(os.TempDir())
	if err != nil {
		return fmt.Errorf("resolve temporary directory: %w", err)
	}
	if !strings.EqualFold(filepath.Dir(absolute), filepath.Clean(tempRoot)) ||
		!strings.HasPrefix(filepath.Base(absolute), "packetcode-benchrun-") {
		return fmt.Errorf("refuse to remove unexpected isolated home %s", absolute)
	}
	if err := os.RemoveAll(absolute); err != nil {
		return fmt.Errorf("remove isolated home: %w", err)
	}
	return nil
}

func environmentWithHome(home string) []string {
	const key = "PACKETCODE_HOME"
	out := make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if !strings.EqualFold(name, key) {
			out = append(out, item)
		}
	}
	return append(out, key+"="+home)
}

func packetcodeHome() (string, error) {
	if configured := os.Getenv("PACKETCODE_HOME"); configured != "" {
		return configured, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".packetcode"), nil
}

func gitCommit(workspace string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = workspace
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func gitDirty(workspace string) bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = workspace
	output, err := cmd.Output()
	return err != nil || len(bytes.TrimSpace(output)) > 0
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func appendError(existing, next string) string {
	if existing == "" {
		return next
	}
	return existing + "; " + next
}
