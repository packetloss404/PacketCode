package mcp

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/packetcode/packetcode/internal/config"
)

// spawnServerProcess starts the MCP server child described by cfg, wires
// up stdin/stdout pipes, opens the per-server log file under logDir, and
// launches a goroutine that tees stderr into that log file. The caller
// owns the returned cmd / pipes / log file and must close them.
//
// The merged environment is os.Environ() overlaid with cfg.Env (cfg
// wins on key collision).
func spawnServerProcess(cfg ServerConfig, logDir string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, *os.File, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)
	cmd.Env = mergeEnv(os.Environ(), cfg.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, nil, nil, nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := os.MkdirAll(logDir, 0o700); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, nil, nil, nil, fmt.Errorf("create log dir: %w", err)
	}
	logFileName, err := config.MCPLogFileName(cfg.Name)
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, nil, nil, nil, err
	}
	logPath := filepath.Join(logDir, logFileName)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, nil, nil, nil, fmt.Errorf("open log file: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		_ = logFile.Close()
		return nil, nil, nil, nil, fmt.Errorf("start: %w", err)
	}

	// Stderr-tee runs until the child closes its stderr (typically on
	// exit). io.Copy returning is also our signal that no more
	// diagnostics are coming.
	go func() {
		_, _ = io.Copy(logFile, stderr)
	}()

	return cmd, stdin, stdout, logFile, nil
}

// mergeEnv overlays overlay onto base. Base entries are "KEY=VALUE"
// strings (i.e. os.Environ() format). Keys present in overlay replace
// any matching entry in base.
func mergeEnv(base []string, overlay map[string]string) []string {
	if len(overlay) == 0 {
		return base
	}
	idx := map[string]int{}
	for i, kv := range base {
		for j := 0; j < len(kv); j++ {
			if kv[j] == '=' {
				idx[kv[:j]] = i
				break
			}
		}
	}
	out := make([]string, len(base))
	copy(out, base)
	for k, v := range overlay {
		entry := k + "=" + v
		if i, ok := idx[k]; ok {
			out[i] = entry
		} else {
			out = append(out, entry)
		}
	}
	return out
}
