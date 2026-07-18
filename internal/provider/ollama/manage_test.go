package ollama

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPullModel_Progress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pull" {
			t.Errorf("path %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"status":"pulling manifest"}
{"status":"pulling abc","digest":"sha256:abc","total":1000,"completed":250}
{"status":"pulling abc","digest":"sha256:abc","total":1000,"completed":1000}
{"status":"success"}
`)
	}))
	defer srv.Close()

	ch, err := New(srv.URL).PullModel(context.Background(), "qwen3-coder:30b")
	if err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	var frames []PullProgress
	for f := range ch {
		frames = append(frames, f)
	}
	if len(frames) != 4 {
		t.Fatalf("want 4 frames, got %d: %+v", len(frames), frames)
	}
	if frames[1].Percent() != 25 {
		t.Fatalf("mid frame percent = %d, want 25", frames[1].Percent())
	}
	if frames[3].Status != "success" {
		t.Fatalf("last frame = %+v, want success", frames[3])
	}
}

func TestPullModel_ErrorFrame(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"status":"pulling manifest"}
{"error":"model not found"}
`)
	}))
	defer srv.Close()
	ch, err := New(srv.URL).PullModel(context.Background(), "nope")
	if err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	var lastErr string
	for f := range ch {
		if f.Err != "" {
			lastErr = f.Err
		}
	}
	if lastErr != "model not found" {
		t.Fatalf("expected error frame, got %q", lastErr)
	}
}

func TestLoadedModels_GPUSplit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ps" {
			t.Errorf("path %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"models":[
			{"name":"qwen3-coder:30b","size":20000000000,"size_vram":20000000000},
			{"name":"llama3.3:70b","size":40000000000,"size_vram":24000000000},
			{"name":"cpuonly:7b","size":5000000000,"size_vram":0}
		]}`)
	}))
	defer srv.Close()

	models, err := New(srv.URL).LoadedModels(context.Background())
	if err != nil {
		t.Fatalf("LoadedModels: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("want 3, got %d", len(models))
	}
	if !models[0].FullyOnGPU() {
		t.Fatal("model 0 should be fully on GPU")
	}
	if models[1].FullyOnGPU() || !models[1].OnGPU() {
		t.Fatal("model 1 should be partial GPU offload")
	}
	if models[2].OnGPU() {
		t.Fatal("model 2 should be CPU-only")
	}
}

func TestWarmup(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("path %q", r.URL.Path)
		}
		defer r.Body.Close()
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = io.WriteString(w, `{"model":"m","done":true}`)
	}))
	defer srv.Close()

	if err := NewWithOptions(srv.URL, Options{KeepAlive: "-1"}).Warmup(context.Background(), "qwen3-coder"); err != nil {
		t.Fatalf("Warmup: %v", err)
	}
	if got["model"] != "qwen3-coder" || got["keep_alive"] != "-1" {
		t.Fatalf("warmup body wrong: %+v", got)
	}
}
