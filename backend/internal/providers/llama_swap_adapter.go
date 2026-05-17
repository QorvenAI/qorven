// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LlamaSwapAdapter wraps a *LlamaSwap so it satisfies the Provider interface.
// The model name is taken from ChatRequest.Model at call time, enabling hot-swap
// across requests without a separate Provider instance per model.
type LlamaSwapAdapter struct {
	swap *LlamaSwap
	name string
}

// NewLlamaSwapAdapter creates a Provider adapter around an existing LlamaSwap.
func NewLlamaSwapAdapter(swap *LlamaSwap, name string) *LlamaSwapAdapter {
	return &LlamaSwapAdapter{swap: swap, name: name}
}

func (a *LlamaSwapAdapter) Name() string { return a.name }

func (a *LlamaSwapAdapter) DefaultModel() string {
	models := a.swap.ListModels()
	if len(models) > 0 {
		return models[0]
	}
	return ""
}

func (a *LlamaSwapAdapter) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if req.Model == "" {
		req.Model = a.DefaultModel()
	}
	if req.Model == "" {
		return nil, fmt.Errorf("llama_swap: no model specified and none configured")
	}
	return a.swap.Chat(ctx, req.Model, req)
}

// ChatStream delivers true SSE streaming by posting directly to the llama-server
// proxy URL returned by EnsureRunning. This avoids the non-streaming fallback
// that would buffer the entire response before the first token reaches the UI.
func (a *LlamaSwapAdapter) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	if req.Model == "" {
		req.Model = a.DefaultModel()
	}
	if req.Model == "" {
		return nil, fmt.Errorf("llama_swap: no model specified and none configured")
	}

	proxyURL, err := a.swap.EnsureRunning(ctx, req.Model)
	if err != nil {
		return nil, fmt.Errorf("llama_swap ensure running: %w", err)
	}

	msgs := make([]map[string]any, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = map[string]any{"role": m.Role, "content": m.Content}
	}
	bodyMap := map[string]any{
		"model":          req.Model,
		"messages":       msgs,
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	if t, ok := req.Options["temperature"]; ok {
		bodyMap["temperature"] = t
	}
	body, _ := json.Marshal(bodyMap)

	endpoint := strings.TrimRight(proxyURL.String(), "/") + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		// llama-server passes EnsureRunning health check before returning but may
		// still be binding its listen socket — one retry after 500ms closes the race.
		time.Sleep(500 * time.Millisecond)
		httpReq2, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
		httpReq2.Header.Set("Content-Type", "application/json")
		resp, err = http.DefaultClient.Do(httpReq2)
		if err != nil {
			return nil, fmt.Errorf("llama_swap stream request: %w", err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("llama_swap %d: %s", resp.StatusCode, string(b))
	}

	// Reuse the OpenAI-compat SSE parser — llama-server emits standard OpenAI SSE.
	oai := &OpenAI{}
	return oai.readStream(resp.Body, onChunk)
}
