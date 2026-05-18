// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"log/slog"
)

// DashScope provider for Alibaba Cloud's Qwen models.
// Wraps OpenAI-compatible API with DashScope-specific thinking controls.
// Critical: DashScope does NOT support tools + streaming simultaneously.

const (
	dashscopeDefaultBase  = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
	dashscopeDefaultModel = "qwen3-max"
)

var dashscopeThinkingModels = map[string]bool{
	"qwen3.5-plus": true, "qwen3.5-turbo": true,
	"qwen3-max": true, "qwen3-235b-a22b": true,
	"qwen3-32b": true, "qwen3-14b": true, "qwen3-8b": true,
}

type DashScope struct {
	*OpenAI
}

func NewDashScope(name, apiKey, defaultModel string) *DashScope {
	cfg := ProviderConfig{Name: name, APIKey: apiKey, APIBase: dashscopeDefaultBase}
	o := NewOpenAI(cfg)
	o.model = orDefault(defaultModel, dashscopeDefaultModel)
	return &DashScope{OpenAI: o}
}

func (d *DashScope) SupportsThinking() bool { return true }

func (d *DashScope) ModelSupportsThinking(model string) bool {
	if model == "" { model = d.model }
	return dashscopeThinkingModels[model]
}

func (d *DashScope) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return d.OpenAI.Chat(ctx, d.applyThinkingGuard(req))
}

// ChatStream handles DashScope's limitation: tools + streaming cannot coexist.
func (d *DashScope) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	req = d.applyThinkingGuard(req)

	if len(req.Tools) > 0 {
		slog.Debug("dashscope: tools present, falling back to non-streaming")
		resp, err := d.OpenAI.Chat(ctx, req)
		if err != nil { return nil, err }
		if onChunk != nil {
			if resp.Content != "" { onChunk(StreamChunk{Content: resp.Content}) }
			onChunk(StreamChunk{Done: true})
		}
		return resp, nil
	}
	return d.OpenAI.ChatStream(ctx, req, onChunk)
}

func (d *DashScope) applyThinkingGuard(req ChatRequest) ChatRequest {
	level, _ := req.Options["thinking"].(string)
	if level == "" || level == "off" { return req }

	if !d.ModelSupportsThinking(req.Model) {
		slog.Debug("dashscope: model does not support thinking", "model", req.Model)
		return req
	}

	opts := make(map[string]any, len(req.Options)+2)
	for k, v := range req.Options { opts[k] = v }
	opts["enable_thinking"] = true
	opts["thinking_budget"] = dashscopeThinkingBudget(level)
	delete(opts, "thinking")
	req.Options = opts
	return req
}

func dashscopeThinkingBudget(level string) int {
	switch level {
	case "low": return 4096
	case "medium": return 16384
	case "high": return 32768
	default: return 16384
	}
}

func orDefault(s, def string) string { if s != "" { return s }; return def }
