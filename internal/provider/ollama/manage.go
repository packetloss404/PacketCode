package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/packetcode/packetcode/internal/provider"
)

// This file adds the model-management surface a local-first coding agent needs
// beyond chat: pulling models with progress, seeing what's loaded and whether
// it's on the GPU, and warming a model so the first turn isn't a cold reload.

// keepAlive resolves the effective keep_alive: the user's config value, else
// the 30-minute default.
func (p *Provider) keepAlive() string {
	if p.opts.KeepAlive != "" {
		return p.opts.KeepAlive
	}
	return defaultKeepAlive
}

// ---- Pull ---------------------------------------------------------------

// PullProgress is one frame of a model download. Total/Completed are populated
// only on layer-download frames; other frames carry just a Status phase marker
// ("pulling manifest", "verifying sha256 digest", "success"). Err is non-empty
// if the daemon reported an error mid-stream.
type PullProgress struct {
	Status    string
	Digest    string
	Total     int64
	Completed int64
	Err       string
}

// Percent returns the download percentage for the current layer, or 0 when the
// frame carries no size information.
func (pp PullProgress) Percent() int {
	if pp.Total > 0 {
		return int(pp.Completed * 100 / pp.Total)
	}
	return 0
}

type pullFrame struct {
	Status    string `json:"status"`
	Digest    string `json:"digest"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
	Error     string `json:"error"`
}

// PullModel downloads a model, streaming progress frames on the returned
// channel. The channel closes when the pull finishes, errors, or ctx is
// cancelled. Errors before the first byte are returned synchronously; a
// mid-stream daemon error arrives as a final frame with Err set.
func (p *Provider) PullModel(ctx context.Context, model string) (<-chan PullProgress, error) {
	reqBody, err := json.Marshal(map[string]any{"model": model, "stream": true})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/pull", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pull %s: %w", model, err)
	}
	if resp.StatusCode/100 != 2 {
		body := provider.ReadErrorBody(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("pull %s: status %d: %s", model, resp.StatusCode, extractOllamaError(body))
	}

	ch := make(chan PullProgress, 8)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			if ctx.Err() != nil {
				return
			}
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var f pullFrame
			if err := json.Unmarshal(line, &f); err != nil {
				continue
			}
			pp := PullProgress{Status: f.Status, Digest: f.Digest, Total: f.Total, Completed: f.Completed, Err: f.Error}
			select {
			case ch <- pp:
			case <-ctx.Done():
				return
			}
			if f.Error != "" {
				return
			}
		}
	}()
	return ch, nil
}

// ---- Loaded models (/api/ps) -------------------------------------------

// LoadedModel is a currently-resident model and how it's split across GPU/CPU.
type LoadedModel struct {
	Name     string
	Size     int64 // total resident bytes
	SizeVRAM int64 // bytes in GPU VRAM
}

// OnGPU reports whether any of the model is offloaded to the GPU.
func (m LoadedModel) OnGPU() bool { return m.SizeVRAM > 0 }

// FullyOnGPU reports whether the whole model fits in VRAM (no CPU offload).
// Partial offload is the top cause of unexpectedly slow local inference, so
// surfacing this to the user is valuable.
func (m LoadedModel) FullyOnGPU() bool { return m.Size > 0 && m.SizeVRAM >= m.Size }

type psResponse struct {
	Models []struct {
		Name     string `json:"name"`
		Model    string `json:"model"`
		Size     int64  `json:"size"`
		SizeVRAM int64  `json:"size_vram"`
	} `json:"models"`
}

// LoadedModels returns the models currently held in memory by the daemon.
func (p *Provider) LoadedModels(ctx context.Context) ([]LoadedModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/api/ps", nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama ps: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body := provider.ReadErrorBody(resp.Body)
		return nil, fmt.Errorf("ollama ps: status %d: %s", resp.StatusCode, extractOllamaError(body))
	}
	var parsed psResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode ollama ps: %w", err)
	}
	out := make([]LoadedModel, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		name := m.Name
		if name == "" {
			name = m.Model
		}
		out = append(out, LoadedModel{Name: name, Size: m.Size, SizeVRAM: m.SizeVRAM})
	}
	return out, nil
}

// ---- Warmup -------------------------------------------------------------

// Warmup loads a model into memory (with the configured keep_alive) without
// generating anything, so the first real turn doesn't pay a cold-start reload.
// A bodyless /api/generate returns as soon as the model is resident.
func (p *Provider) Warmup(ctx context.Context, model string) error {
	reqBody, err := json.Marshal(map[string]any{"model": model, "keep_alive": p.keepAlive()})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/generate", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("warm up %s: %w", model, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body := provider.ReadErrorBody(resp.Body)
		return fmt.Errorf("warm up %s: status %d: %s", model, resp.StatusCode, extractOllamaError(body))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// extractOllamaError pulls the message out of Ollama's {"error":"..."} body,
// falling back to the trimmed raw body.
func extractOllamaError(body []byte) string {
	var wrapper struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil && wrapper.Error != "" {
		return wrapper.Error
	}
	return string(bytes.TrimSpace(body))
}
