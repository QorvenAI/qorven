// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"
)

// OllamaSwap orchestrates model loading on a local Ollama daemon.
// Ollama already supports lazy-load and auto-unload via the
// `keep_alive` request parameter, but it doesn't know anything about:
//
//   - Per-model priority (don't evict the user's default to make room
//     for a one-off translation task)
//   - Queue serialization (parallel requests for two different models
//     on a single GPU cause a load/unload thrash)
//   - VRAM budget (Ollama will happily OOM if you ask for a 70B model
//     on a 24GB GPU and no eviction policy is set)
//
// This orchestrator sits in front of Ollama and tracks which model is
// resident, serialises model-change requests through a queue, and
// enforces a VRAM budget the operator configures.
//
// Why not extend LlamaSwap? LlamaSwap spawns llama-server processes
// directly — it owns the model lifecycle. OllamaSwap defers to the
// Ollama daemon and only signals intent (load / unload / keep_alive).
// Separate concerns, separate struct, less code to break when either
// upstream changes.

// OllamaSwapConfig is the operator-supplied setup.
type OllamaSwapConfig struct {
	// BaseURL of the Ollama HTTP API. Default http://127.0.0.1:11434.
	BaseURL string

	// VRAMBudgetMB is the maximum combined VRAM the orchestrator will
	// allow loaded. Requests that would exceed this trigger eviction
	// of the least-recently-used model. Zero = unbounded (leave
	// eviction to Ollama's default).
	VRAMBudgetMB int

	// ModelSizes lets the operator pre-declare each model's VRAM
	// footprint. Ollama's /api/ps returns loaded model size, so we
	// learn over time — but we need a starting number to decide
	// whether to load a model we haven't seen. Key: Ollama model ID
	// ("llama3.1:8b-instruct-q4_K_M"). Value: MB.
	//
	// If unset, we assume 8 GB per model as a conservative default.
	ModelSizes map[string]int

	// KeepAlive is the default `keep_alive` value sent on every
	// generate call. Negative = infinite; 0 = unload immediately after
	// response; positive = seconds. Default 5 minutes.
	KeepAlive time.Duration

	// PriorityModels lists models the orchestrator refuses to evict
	// even under VRAM pressure. If the budget can't fit a new load
	// without evicting a priority model, the new load fails — the
	// operator chose a stable primary over opportunistic swap.
	PriorityModels []string
}

// OllamaSwap is the orchestrator. Construct one per local Ollama daemon.
type OllamaSwap struct {
	cfg    OllamaSwapConfig
	http   *http.Client
	mu     sync.Mutex // serialises load/unload decisions

	// loaded tracks what we believe Ollama currently has resident.
	// Refreshed from /api/ps on every serialised operation so we
	// don't drift from Ollama's view.
	loaded map[string]*ollamaResident

	// swapQueue serialises concurrent EnsureLoaded calls for
	// different models. Ollama itself handles same-model concurrency,
	// but back-to-back loads of A, B, A, B thrash the GPU.
	swapQueue chan struct{}
}

type ollamaResident struct {
	Name     string
	SizeMB   int
	LastUsed time.Time
}

// ollama /api/ps response shape.
type ollamaPS struct {
	Models []struct {
		Name      string `json:"name"`
		Model     string `json:"model"`
		Size      int64  `json:"size"`
		SizeVRAM  int64  `json:"size_vram"`
		ExpiresAt string `json:"expires_at"`
	} `json:"models"`
}

// NewOllamaSwap constructs the orchestrator. Defaults kick in when
// the config fields are zero-valued.
func NewOllamaSwap(cfg OllamaSwapConfig) *OllamaSwap {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://127.0.0.1:11434"
	}
	if cfg.KeepAlive == 0 {
		cfg.KeepAlive = 5 * time.Minute
	}
	if cfg.ModelSizes == nil {
		cfg.ModelSizes = make(map[string]int)
	}
	return &OllamaSwap{
		cfg:       cfg,
		http:      &http.Client{Timeout: 300 * time.Second},
		loaded:    make(map[string]*ollamaResident),
		swapQueue: make(chan struct{}, 1),
	}
}

// EnsureLoaded makes sure `model` is resident, evicting others if
// needed. Returns the model name (usable as-is in chat requests) and
// any error. Safe for concurrent callers — internally serialised.
//
// Call patterns:
//   err := swap.EnsureLoaded(ctx, "llama3.1:8b-instruct-q4_K_M")
//   // model is now resident; send chat requests at cfg.BaseURL
func (s *OllamaSwap) EnsureLoaded(ctx context.Context, model string) error {
	// Grab the single-slot swap queue so only one decision runs at
	// a time. Don't block the actual inference — just the decision.
	select {
	case s.swapQueue <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.swapQueue }()

	// Refresh our view from Ollama. Cheap (< 10ms) and it's the only
	// way to notice that Ollama unloaded a model out from under us
	// (TTL expiry, manual unload, daemon restart).
	if err := s.refreshLoaded(ctx); err != nil {
		return fmt.Errorf("refresh ollama state: %w", err)
	}

	// Already loaded? Touch LRU and return.
	s.mu.Lock()
	if r, ok := s.loaded[model]; ok {
		r.LastUsed = time.Now()
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	// Need to load. Check budget first.
	sizeMB := s.sizeForModel(model)
	if s.cfg.VRAMBudgetMB > 0 {
		if err := s.makeRoomFor(ctx, sizeMB); err != nil {
			return err
		}
	}

	// Trigger load by calling /api/generate with an empty prompt
	// and keep_alive. Ollama responds as soon as the model is
	// loaded, so this doubles as a ready signal.
	if err := s.preloadModel(ctx, model); err != nil {
		return fmt.Errorf("preload %s: %w", model, err)
	}

	s.mu.Lock()
	s.loaded[model] = &ollamaResident{
		Name:     model,
		SizeMB:   sizeMB,
		LastUsed: time.Now(),
	}
	s.mu.Unlock()
	slog.Info("ollama_swap.loaded", "model", model, "size_mb", sizeMB)
	return nil
}

// makeRoomFor evicts least-recently-used non-priority models until
// the projected total (loaded + incoming) fits in the budget. Errors
// when the only eviction candidates are priority models.
func (s *OllamaSwap) makeRoomFor(ctx context.Context, incomingMB int) error {
	s.mu.Lock()
	// Build an eviction-candidate list sorted by LastUsed asc. Priority
	// models are excluded entirely. We snapshot under the lock and
	// release it before doing the (slow) HTTP eviction calls.
	var total int
	for _, r := range s.loaded {
		total += r.SizeMB
	}
	needed := (total + incomingMB) - s.cfg.VRAMBudgetMB
	if needed <= 0 {
		s.mu.Unlock()
		return nil
	}

	candidates := make([]*ollamaResident, 0, len(s.loaded))
	for _, r := range s.loaded {
		if s.isPriority(r.Name) {
			continue
		}
		candidates = append(candidates, r)
	}
	s.mu.Unlock()

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].LastUsed.Before(candidates[j].LastUsed)
	})

	freed := 0
	for _, c := range candidates {
		if freed >= needed {
			break
		}
		if err := s.unloadModel(ctx, c.Name); err != nil {
			slog.Warn("ollama_swap.evict_failed", "model", c.Name, "error", err)
			continue
		}
		freed += c.SizeMB
		s.mu.Lock()
		delete(s.loaded, c.Name)
		s.mu.Unlock()
		slog.Info("ollama_swap.evicted", "model", c.Name, "freed_mb", c.SizeMB)
	}
	if freed < needed {
		return fmt.Errorf("VRAM budget %d MB exceeded and can't evict enough "+
			"(needed %d more MB; only non-priority candidates considered)",
			s.cfg.VRAMBudgetMB, needed-freed)
	}
	return nil
}

func (s *OllamaSwap) isPriority(name string) bool {
	for _, p := range s.cfg.PriorityModels {
		if p == name {
			return true
		}
	}
	return false
}

// sizeForModel returns the MB footprint we assume for `model`. Uses
// the configured map when available, falls back to 8 GB otherwise.
// A Snapshot from /api/ps has the real number — refreshLoaded
// writes it back into the ModelSizes map so subsequent loads are
// accurate.
func (s *OllamaSwap) sizeForModel(model string) int {
	if sz, ok := s.cfg.ModelSizes[model]; ok {
		return sz
	}
	return 8 * 1024 // 8 GB conservative default
}

// refreshLoaded queries Ollama's /api/ps and rebuilds s.loaded.
func (s *OllamaSwap) refreshLoaded(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.BaseURL+"/api/ps", nil)
	if err != nil {
		return err
	}
	res, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("ollama /api/ps: HTTP %d: %s", res.StatusCode, string(body))
	}
	var ps ollamaPS
	if err := json.NewDecoder(res.Body).Decode(&ps); err != nil {
		return fmt.Errorf("decode /api/ps: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Rebuild from ground truth. Any prior entry not in /api/ps was
	// unloaded out-of-band and is gone.
	fresh := make(map[string]*ollamaResident, len(ps.Models))
	for _, m := range ps.Models {
		name := m.Name
		if name == "" {
			name = m.Model
		}
		sizeMB := int(m.SizeVRAM / (1024 * 1024))
		if sizeMB == 0 {
			sizeMB = int(m.Size / (1024 * 1024))
		}
		// Update the sizing hint too — next time we need to decide
		// load feasibility for this model we have a real number.
		if sizeMB > 0 {
			s.cfg.ModelSizes[name] = sizeMB
		}
		// Preserve LastUsed across refreshes — /api/ps doesn't expose it.
		var lastUsed time.Time
		if prev, ok := s.loaded[name]; ok {
			lastUsed = prev.LastUsed
		} else {
			lastUsed = time.Now()
		}
		fresh[name] = &ollamaResident{Name: name, SizeMB: sizeMB, LastUsed: lastUsed}
	}
	s.loaded = fresh
	return nil
}

// preloadModel triggers Ollama to load `model` by sending an empty
// generate with keep_alive. Ollama's load is synchronous — the
// response doesn't come back until the model is GPU-resident.
func (s *OllamaSwap) preloadModel(ctx context.Context, model string) error {
	keepAliveSec := int(s.cfg.KeepAlive.Seconds())
	body, _ := json.Marshal(map[string]any{
		"model":      model,
		"keep_alive": keepAliveSec,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.cfg.BaseURL+"/api/generate",
		io.NopCloser(stringReader(string(body))))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(res.Body)
		return fmt.Errorf("ollama /api/generate: HTTP %d: %s",
			res.StatusCode, string(respBody))
	}
	// Drain response so the connection can be reused.
	_, _ = io.Copy(io.Discard, res.Body)
	return nil
}

// unloadModel forces Ollama to drop a model from VRAM by sending a
// generate with keep_alive=0. Ollama unloads after the (empty)
// request completes.
func (s *OllamaSwap) unloadModel(ctx context.Context, model string) error {
	body, _ := json.Marshal(map[string]any{
		"model":      model,
		"keep_alive": 0,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.cfg.BaseURL+"/api/generate",
		io.NopCloser(stringReader(string(body))))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama unload HTTP %d", res.StatusCode)
	}
	return nil
}

// Status returns a snapshot of what's currently loaded.
func (s *OllamaSwap) Status(ctx context.Context) []map[string]any {
	// Refresh best-effort — if Ollama is unreachable we just report
	// the last-known state.
	_ = s.refreshLoaded(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []map[string]any
	for _, r := range s.loaded {
		out = append(out, map[string]any{
			"model":     r.Name,
			"size_mb":   r.SizeMB,
			"last_used": r.LastUsed,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i]["model"].(string) < out[j]["model"].(string)
	})
	return out
}

// stringReader is a tiny helper so we don't pull in strings in the
// preload-path hot code. bytes.NewBufferString would work too but
// this is a shade faster and we don't need the full buffer API.
type stringReader string

func (s stringReader) Read(p []byte) (int, error) {
	if len(s) == 0 {
		return 0, io.EOF
	}
	n := copy(p, s)
	return n, nil
}
