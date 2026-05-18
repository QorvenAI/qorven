// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// LlamaSwap manages local model inference servers with on-demand start/stop.
// Inspired by github.com/mostlygeek/llama-swap — hot-swap models on a single GPU.
type LlamaSwap struct {
	mu      sync.RWMutex
	models  map[string]*LocalModel
	active  string // currently loaded model
	baseDir string
	config  LlamaSwapConfig
}

// LlamaSwapConfig is the top-level configuration.
type LlamaSwapConfig struct {
	HealthCheckTimeout int                    `yaml:"healthCheckTimeout" json:"health_check_timeout"` // seconds
	Models             map[string]LocalModelConfig `yaml:"models" json:"models"`
}

// LocalModelConfig defines how to run a local model.
type LocalModelConfig struct {
	Command       string   `yaml:"cmd" json:"command"`          // e.g. "llama-server -m model.gguf -c 8192"
	StopCommand   string   `yaml:"cmdStop" json:"stop_command"` // optional graceful stop
	Proxy         string   `yaml:"proxy" json:"proxy"`          // e.g. "http://127.0.0.1:8081"
	Env           []string `yaml:"env" json:"env"`
	CheckEndpoint string   `yaml:"checkEndpoint" json:"check_endpoint"` // e.g. "/health"
	TTL           int      `yaml:"ttl" json:"ttl"`                      // seconds before auto-unload (0 = never)
	Aliases       []string `yaml:"aliases" json:"aliases"`
}

// LocalModel is a running model instance.
type LocalModel struct {
	Name      string           `json:"name"`
	Config    LocalModelConfig `json:"config"`
	State     ModelState       `json:"state"`
	PID       int              `json:"pid,omitempty"`
	StartedAt time.Time        `json:"started_at,omitempty"`
	LastUsed  time.Time        `json:"last_used,omitempty"`
	Requests  int64            `json:"requests"`

	cmd       *exec.Cmd
	proxy     *httputil.ReverseProxy
	proxyURL  *url.URL
	cancel    context.CancelFunc
	ttlTimer  *time.Timer
}

// ModelState represents the lifecycle state of a model.
type ModelState string

const (
	ModelStopped  ModelState = "stopped"
	ModelStarting ModelState = "starting"
	ModelReady    ModelState = "ready"
	ModelStopping ModelState = "stopping"
)

// NewLlamaSwap creates a new model manager.
func NewLlamaSwap(cfg LlamaSwapConfig) *LlamaSwap {
	ls := &LlamaSwap{
		models: make(map[string]*LocalModel),
		config: cfg,
	}
	if cfg.HealthCheckTimeout == 0 {
		ls.config.HealthCheckTimeout = 120
	}

	// Initialize models from config
	for name, mcfg := range cfg.Models {
		proxyURL, _ := url.Parse(mcfg.Proxy)
		ls.models[name] = &LocalModel{
			Name:     name,
			Config:   mcfg,
			State:    ModelStopped,
			proxyURL: proxyURL,
		}
		// Register aliases
		for _, alias := range mcfg.Aliases {
			ls.models[alias] = ls.models[name]
		}
	}
	return ls
}

// EnsureRunning starts a model if it's not already running, and returns the proxy URL.
func (ls *LlamaSwap) EnsureRunning(ctx context.Context, modelName string) (*url.URL, error) {
	ls.mu.Lock()
	model, ok := ls.models[modelName]
	if !ok {
		ls.mu.Unlock()
		return nil, fmt.Errorf("model %q not configured in llama-swap", modelName)
	}

	if model.State == ModelReady {
		model.LastUsed = time.Now()
		model.Requests++
		ls.resetTTL(model)
		ls.mu.Unlock()
		return model.proxyURL, nil
	}

	if model.State == ModelStarting {
		ls.mu.Unlock()
		// Wait for it to become ready
		return ls.waitForReady(ctx, modelName)
	}

	// Need to start it — first stop any other active model (single GPU)
	if ls.active != "" && ls.active != modelName {
		if activeModel, ok := ls.models[ls.active]; ok && activeModel.State == ModelReady {
			slog.Info("llama_swap.unloading", "model", ls.active, "reason", "swap for "+modelName)
			ls.stopModelLocked(activeModel)
		}
	}

	model.State = ModelStarting
	ls.active = modelName
	ls.mu.Unlock()

	// Start the model process
	if err := ls.startModel(ctx, model); err != nil {
		ls.mu.Lock()
		model.State = ModelStopped
		ls.mu.Unlock()
		return nil, fmt.Errorf("start %s: %w", modelName, err)
	}

	return model.proxyURL, nil
}

// startModel launches the inference server process and waits for health check.
func (ls *LlamaSwap) startModel(ctx context.Context, model *LocalModel) error {
	slog.Info("llama_swap.starting", "model", model.Name, "cmd", model.Config.Command)

	cmdCtx, cancel := context.WithCancel(context.Background())
	parts := strings.Fields(model.Config.Command)
	if len(parts) == 0 {
		cancel()
		return fmt.Errorf("empty command for model %s", model.Name)
	}

	cmd := exec.CommandContext(cmdCtx, parts[0], parts[1:]...)
	cmd.Env = append(os.Environ(), model.Config.Env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start process: %w", err)
	}

	model.cmd = cmd
	model.cancel = cancel
	model.PID = cmd.Process.Pid
	model.StartedAt = time.Now()

	// Wait for health check
	checkURL := model.Config.Proxy
	if model.Config.CheckEndpoint != "" {
		checkURL += model.Config.CheckEndpoint
	} else {
		checkURL += "/health"
	}

	timeout := time.Duration(ls.config.HealthCheckTimeout) * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := http.Get(checkURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				ls.mu.Lock()
				model.State = ModelReady
				model.LastUsed = time.Now()
				model.Requests++
				// Set up reverse proxy
				model.proxy = httputil.NewSingleHostReverseProxy(model.proxyURL)
				ls.resetTTL(model)
				ls.mu.Unlock()
				slog.Info("llama_swap.ready", "model", model.Name, "pid", model.PID,
					"startup_ms", time.Since(model.StartedAt).Milliseconds())
				return nil
			}
		}
		select {
		case <-ctx.Done():
			ls.stopModelLocked(model)
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}

	// Timeout — kill the process
	ls.mu.Lock()
	ls.stopModelLocked(model)
	ls.mu.Unlock()
	return fmt.Errorf("health check timeout after %s for %s", timeout, model.Name)
}

// waitForReady waits for a model that's currently starting.
func (ls *LlamaSwap) waitForReady(ctx context.Context, modelName string) (*url.URL, error) {
	timeout := time.Duration(ls.config.HealthCheckTimeout) * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		ls.mu.RLock()
		model, ok := ls.models[modelName]
		if !ok {
			ls.mu.RUnlock()
			return nil, fmt.Errorf("model %q not found", modelName)
		}
		if model.State == ModelReady {
			model.LastUsed = time.Now()
			model.Requests++
			ls.mu.RUnlock()
			return model.proxyURL, nil
		}
		if model.State == ModelStopped {
			ls.mu.RUnlock()
			return nil, fmt.Errorf("model %s failed to start", modelName)
		}
		ls.mu.RUnlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("timeout waiting for %s", modelName)
}

// stopModelLocked stops a model (caller must hold ls.mu).
func (ls *LlamaSwap) stopModelLocked(model *LocalModel) {
	if model.State == ModelStopped || model.State == ModelStopping {
		return
	}
	model.State = ModelStopping

	if model.ttlTimer != nil {
		model.ttlTimer.Stop()
	}

	// Try graceful stop command first
	if model.Config.StopCommand != "" {
		parts := strings.Fields(model.Config.StopCommand)
		exec.Command(parts[0], parts[1:]...).Run()
	}

	// Kill the process
	if model.cancel != nil {
		model.cancel()
	}
	if model.cmd != nil && model.cmd.Process != nil {
		model.cmd.Process.Signal(os.Interrupt)
		go func() {
			time.Sleep(5 * time.Second)
			if model.cmd.Process != nil {
				model.cmd.Process.Kill()
			}
		}()
	}

	model.State = ModelStopped
	model.proxy = nil
	slog.Info("llama_swap.stopped", "model", model.Name)
}

// resetTTL resets the TTL timer for auto-unloading.
func (ls *LlamaSwap) resetTTL(model *LocalModel) {
	if model.Config.TTL <= 0 {
		return
	}
	if model.ttlTimer != nil {
		model.ttlTimer.Stop()
	}
	model.ttlTimer = time.AfterFunc(time.Duration(model.Config.TTL)*time.Second, func() {
		ls.mu.Lock()
		defer ls.mu.Unlock()
		if model.State == ModelReady {
			slog.Info("llama_swap.ttl_unload", "model", model.Name, "ttl", model.Config.TTL)
			ls.stopModelLocked(model)
			if ls.active == model.Name {
				ls.active = ""
			}
		}
	})
}

// Proxy returns the reverse proxy for a model (nil if not running).
func (ls *LlamaSwap) Proxy(modelName string) *httputil.ReverseProxy {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	if model, ok := ls.models[modelName]; ok && model.State == ModelReady {
		return model.proxy
	}
	return nil
}

// StopAll stops all running models.
func (ls *LlamaSwap) StopAll() {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for _, model := range ls.models {
		if model.State == ModelReady || model.State == ModelStarting {
			ls.stopModelLocked(model)
		}
	}
	ls.active = ""
}

// Status returns the status of all configured models.
func (ls *LlamaSwap) Status() []map[string]any {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	seen := make(map[string]bool)
	status := []map[string]any{}
	for name, model := range ls.models {
		if seen[model.Name] {
			continue
		}
		seen[model.Name] = true
		s := map[string]any{
			"name":     name,
			"state":    model.State,
			"proxy":    model.Config.Proxy,
			"requests": model.Requests,
		}
		if model.State == ModelReady {
			s["pid"] = model.PID
			s["uptime_seconds"] = int(time.Since(model.StartedAt).Seconds())
			s["last_used"] = model.LastUsed
		}
		status = append(status, s)
	}
	return status
}

// ListModels returns the names of all configured models.
func (ls *LlamaSwap) ListModels() []string {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	seen := make(map[string]bool)
	names := []string{}
	for _, model := range ls.models {
		if !seen[model.Name] {
			seen[model.Name] = true
			names = append(names, model.Name)
		}
	}
	return names
}

// LoadConfig loads llama-swap config from a YAML file.
func LoadLlamaSwapConfig(path string) (*LlamaSwapConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg LlamaSwapConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// ChatViaLlamaSwap sends a chat request through the llama-swap proxy.
func (ls *LlamaSwap) Chat(ctx context.Context, modelName string, req ChatRequest) (*ChatResponse, error) {
	proxyURL, err := ls.EnsureRunning(ctx, modelName)
	if err != nil {
		return nil, err
	}

	// Build OpenAI-compatible request
	body, _ := json.Marshal(map[string]any{
		"model":    modelName,
		"messages": req.Messages,
		"stream":   false,
	})

	httpReq, _ := http.NewRequestWithContext(ctx, "POST", proxyURL.String()+"/v1/chat/completions", strings.NewReader(string(body)))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llama-swap request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	// Parse OpenAI response
	var oaiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &oaiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if len(oaiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &ChatResponse{
		Content:      oaiResp.Choices[0].Message.Content,
		FinishReason: oaiResp.Choices[0].FinishReason,
		Usage: &Usage{
			PromptTokens:     oaiResp.Usage.PromptTokens,
			CompletionTokens: oaiResp.Usage.CompletionTokens,
			TotalTokens:      oaiResp.Usage.PromptTokens + oaiResp.Usage.CompletionTokens,
		},
	}, nil
}
