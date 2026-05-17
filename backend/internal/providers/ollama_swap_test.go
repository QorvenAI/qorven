// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ollamaMock is a minimal stand-in for the Ollama HTTP API. Records
// every request so tests can assert what the orchestrator actually
// sent. Safe for parallel use.
type ollamaMock struct {
	mu        sync.Mutex
	loaded    map[string]int64 // model → size in bytes
	generates []generateCall
}

type generateCall struct {
	Model     string `json:"model"`
	KeepAlive int    `json:"keep_alive"`
}

func newOllamaMock() *ollamaMock {
	return &ollamaMock{loaded: make(map[string]int64)}
}

func (m *ollamaMock) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ps":
			m.mu.Lock()
			var models []map[string]any
			for name, sz := range m.loaded {
				models = append(models, map[string]any{
					"name":      name,
					"size":      sz,
					"size_vram": sz,
				})
			}
			m.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"models": models})

		case "/api/generate":
			var g generateCall
			if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			m.mu.Lock()
			m.generates = append(m.generates, g)
			if g.KeepAlive == 0 {
				delete(m.loaded, g.Model)
			} else {
				// Default sizes for test models. The orchestrator
				// will learn from /api/ps on the next refresh.
				if _, ok := m.loaded[g.Model]; !ok {
					m.loaded[g.Model] = int64(4 * 1024 * 1024 * 1024) // 4 GB
				}
			}
			m.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"","done":true}`))

		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
}

// TestOllamaSwap_EnsureLoaded_Cold: a model not yet loaded triggers
// /api/generate with the configured keep_alive, then appears in the
// loaded set.
func TestOllamaSwap_EnsureLoaded_Cold(t *testing.T) {
	mock := newOllamaMock()
	srv := mock.server()
	defer srv.Close()

	s := NewOllamaSwap(OllamaSwapConfig{
		BaseURL:   srv.URL,
		KeepAlive: 300 * time.Second,
	})

	if err := s.EnsureLoaded(context.Background(), "llama3.1:8b"); err != nil {
		t.Fatalf("ensure loaded: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.generates) != 1 {
		t.Fatalf("expected 1 generate call, got %d: %+v", len(mock.generates), mock.generates)
	}
	if mock.generates[0].Model != "llama3.1:8b" {
		t.Errorf("wrong model: %q", mock.generates[0].Model)
	}
	if mock.generates[0].KeepAlive != 300 {
		t.Errorf("wrong keep_alive: %d", mock.generates[0].KeepAlive)
	}
}

// TestOllamaSwap_EnsureLoaded_Warm: a model already resident in
// Ollama's /api/ps doesn't trigger another generate call.
func TestOllamaSwap_EnsureLoaded_Warm(t *testing.T) {
	mock := newOllamaMock()
	// Pre-populate as if Ollama loaded it out-of-band.
	mock.loaded["llama3.1:8b"] = 4 * 1024 * 1024 * 1024
	srv := mock.server()
	defer srv.Close()

	s := NewOllamaSwap(OllamaSwapConfig{BaseURL: srv.URL})

	if err := s.EnsureLoaded(context.Background(), "llama3.1:8b"); err != nil {
		t.Fatalf("ensure loaded: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.generates) != 0 {
		t.Errorf("warm model should not trigger generate; got %+v", mock.generates)
	}
}

// TestOllamaSwap_VRAMBudget_Eviction: when loading a new model would
// exceed the budget, the LRU non-priority model is evicted.
func TestOllamaSwap_VRAMBudget_Eviction(t *testing.T) {
	mock := newOllamaMock()
	// Pre-load two 8 GB models so the mock starts with 16 GB used.
	mock.loaded["a:latest"] = 8 * 1024 * 1024 * 1024
	mock.loaded["b:latest"] = 8 * 1024 * 1024 * 1024
	srv := mock.server()
	defer srv.Close()

	s := NewOllamaSwap(OllamaSwapConfig{
		BaseURL:      srv.URL,
		VRAMBudgetMB: 20 * 1024, // 20 GB
		ModelSizes: map[string]int{
			"c:latest": 8 * 1024, // 8 GB incoming
		},
	})

	// Prime LRU: touch model b so it's more recent than a.
	if err := s.EnsureLoaded(context.Background(), "b:latest"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond) // ensure distinct LastUsed
	if err := s.EnsureLoaded(context.Background(), "a:latest"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)

	// Load c — projected total 24 GB, budget 20 GB. Must evict.
	// LRU is b (was touched before a's second touch... wait, we touched
	// b first then a, so a is more recent. b should be evicted).
	if err := s.EnsureLoaded(context.Background(), "c:latest"); err != nil {
		t.Fatalf("load c: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if _, stillLoaded := mock.loaded["b:latest"]; stillLoaded {
		t.Errorf("LRU model b should have been evicted; loaded=%+v", mock.loaded)
	}
	if _, loaded := mock.loaded["c:latest"]; !loaded {
		t.Errorf("new model c should have been loaded; loaded=%+v", mock.loaded)
	}
}

// TestOllamaSwap_PriorityModelNotEvicted: a priority model stays
// resident even under pressure — the new load fails instead.
func TestOllamaSwap_PriorityModelNotEvicted(t *testing.T) {
	mock := newOllamaMock()
	mock.loaded["primary:latest"] = 16 * 1024 * 1024 * 1024 // 16 GB
	srv := mock.server()
	defer srv.Close()

	s := NewOllamaSwap(OllamaSwapConfig{
		BaseURL:        srv.URL,
		VRAMBudgetMB:   20 * 1024, // 20 GB
		PriorityModels: []string{"primary:latest"},
		ModelSizes: map[string]int{
			"newcomer:latest": 8 * 1024, // won't fit
		},
	})

	// Prime the orchestrator's view.
	if err := s.EnsureLoaded(context.Background(), "primary:latest"); err != nil {
		t.Fatal(err)
	}

	err := s.EnsureLoaded(context.Background(), "newcomer:latest")
	if err == nil {
		t.Fatal("expected VRAM budget error when only candidate is priority")
	}
	if !strings.Contains(err.Error(), "budget") {
		t.Errorf("error should mention budget; got %q", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if _, stillLoaded := mock.loaded["primary:latest"]; !stillLoaded {
		t.Error("priority model must never be evicted")
	}
}

// TestOllamaSwap_ConcurrentEnsureLoaded: multiple goroutines asking
// for the same model should result in exactly one generate call.
// This guards the swap-queue serialisation.
func TestOllamaSwap_ConcurrentEnsureLoaded(t *testing.T) {
	mock := newOllamaMock()
	srv := mock.server()
	defer srv.Close()

	s := NewOllamaSwap(OllamaSwapConfig{BaseURL: srv.URL})

	const concurrency = 10
	var wg sync.WaitGroup
	var errCount int32
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.EnsureLoaded(context.Background(), "shared:latest"); err != nil {
				atomic.AddInt32(&errCount, 1)
				t.Errorf("concurrent load failed: %v", err)
			}
		}()
	}
	wg.Wait()

	if errCount != 0 {
		t.Fatalf("%d concurrent calls failed", errCount)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	// Exactly 1 generate: the first caller loads, the rest see it in loaded.
	if len(mock.generates) != 1 {
		t.Errorf("expected 1 generate for %d concurrent callers, got %d",
			concurrency, len(mock.generates))
	}
}

// TestOllamaSwap_ContextCancellation: the swap queue must respect
// the caller's context so a stuck request doesn't wedge the hot path.
func TestOllamaSwap_ContextCancellation(t *testing.T) {
	// Ollama that blocks forever on /api/generate.
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/generate" {
			<-block
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()
	defer close(block)

	s := NewOllamaSwap(OllamaSwapConfig{BaseURL: srv.URL})

	// First caller will get stuck in preloadModel. Second caller
	// should be cancellable via context. We need to give the first
	// one time to actually grab the queue slot.
	go func() { _ = s.EnsureLoaded(context.Background(), "stuck:latest") }()
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := s.EnsureLoaded(ctx, "second:latest")
	if err == nil {
		t.Fatal("expected context-deadline error")
	}
	if !strings.Contains(err.Error(), "deadline") && !strings.Contains(err.Error(), "canceled") {
		t.Errorf("expected context error, got %q", err)
	}
}

// TestOllamaSwap_UnboundedBudget: with VRAMBudgetMB=0, no eviction
// ever happens — we defer entirely to Ollama's own TTL.
func TestOllamaSwap_UnboundedBudget(t *testing.T) {
	mock := newOllamaMock()
	srv := mock.server()
	defer srv.Close()

	s := NewOllamaSwap(OllamaSwapConfig{BaseURL: srv.URL}) // VRAMBudgetMB=0

	for i := 0; i < 5; i++ {
		name := "m:" + fmt.Sprint(i)
		if err := s.EnsureLoaded(context.Background(), name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	unloads := 0
	for _, g := range mock.generates {
		if g.KeepAlive == 0 {
			unloads++
		}
	}
	if unloads != 0 {
		t.Errorf("unbounded budget should not evict anything; saw %d unloads", unloads)
	}
}

// TestOllamaSwap_Status_Empty: with no loaded models and Ollama
// unreachable, Status returns empty, not panic.
func TestOllamaSwap_Status_Empty(t *testing.T) {
	s := NewOllamaSwap(OllamaSwapConfig{BaseURL: "http://127.0.0.1:1"}) // invalid port
	out := s.Status(context.Background())
	if len(out) != 0 {
		t.Errorf("expected empty status; got %+v", out)
	}
}

// TestOllamaSwap_PreRefreshLearnsSize: the orchestrator learns real
// sizes from /api/ps. A second call with a bigger incoming model
// should use the learned size, not the 8 GB default.
func TestOllamaSwap_PreRefreshLearnsSize(t *testing.T) {
	mock := newOllamaMock()
	// 2 GB model, well under the default 8 GB assumption.
	mock.loaded["small:latest"] = 2 * 1024 * 1024 * 1024
	srv := mock.server()
	defer srv.Close()

	s := NewOllamaSwap(OllamaSwapConfig{
		BaseURL:      srv.URL,
		VRAMBudgetMB: 5 * 1024, // 5 GB — wouldn't fit 2 defaults (16 GB) but fits two 2 GB
	})

	// Trigger refresh — mock reports 2 GB.
	if err := s.EnsureLoaded(context.Background(), "small:latest"); err != nil {
		t.Fatal(err)
	}

	// Learned size should be in ModelSizes now.
	learned, ok := s.cfg.ModelSizes["small:latest"]
	if !ok {
		t.Fatal("learned size not recorded")
	}
	if learned != 2048 {
		t.Errorf("learned size = %d MB, want ~2048", learned)
	}
}
