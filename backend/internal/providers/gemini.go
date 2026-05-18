// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Gemini implements the Provider interface for Google's Generative AI API.
// Uses generateContent format with Parts, function calling, and SSE streaming.
type Gemini struct {
	apiBase string
	apiKey  string
	model   string
	name    string
	client  *http.Client
}

func NewGemini(cfg ProviderConfig) *Gemini {
	base := cfg.APIBase
	if base == "" {
		base = DefaultAPIBase(TypeGeminiNative)
	}
	base = strings.TrimRight(base, "/")

	model := "gemini-2.5-flash"
	if cfg.Settings != nil {
		var s struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(cfg.Settings, &s) == nil && s.Model != "" {
			model = s.Model
		}
	}

	return &Gemini{
		apiBase: base,
		apiKey:  cfg.APIKey,
		model:   model,
		name:    cfg.Name,
		client:  &http.Client{},
	}
}

func (g *Gemini) Name() string        { return g.name }
func (g *Gemini) DefaultModel() string { return g.model }

func (g *Gemini) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = g.model
	}
	url := fmt.Sprintf("%s/models/%s:generateContent", g.apiBase, model)
	body := g.buildBody(req)

	raw, err := g.doRequest(ctx, url, body)
	if err != nil {
		return nil, err
	}
	return g.parseResponse(raw)
}

func (g *Gemini) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = g.model
	}
	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse", g.apiBase, model)
	body := g.buildBody(req)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", g.apiKey)

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini %d: %s", resp.StatusCode, string(b))
	}

	return g.readStream(resp.Body, onChunk)
}

func (g *Gemini) buildBody(req ChatRequest) []byte {
	// Gemini: system instruction is separate, messages are "contents"
	var systemParts []map[string]any
	contents := make([]map[string]any, 0, len(req.Messages))

	msgs := CollapseToolCallsWithoutSignature(req.Messages)
	for _, m := range msgs {
		if m.Role == "system" {
			systemParts = append(systemParts, map[string]any{"text": m.Content})
			continue
		}
		contents = append(contents, g.convertMessage(m))
	}

	body := map[string]any{
		"contents": contents,
	}
	if len(systemParts) > 0 {
		body["systemInstruction"] = map[string]any{
			"parts": systemParts,
		}
	}

	// Tools → Gemini function declarations + AUTO function calling mode
	if len(req.Tools) > 0 {
		funcs := make([]map[string]any, len(req.Tools))
		for i, t := range req.Tools {
			funcs[i] = map[string]any{
				"name":        t.Function.Name,
				"description": t.Function.Description,
				"parameters":  CleanSchemaForProvider("gemini", t.Function.Parameters),
			}
		}
		body["tools"] = []map[string]any{{
			"functionDeclarations": funcs,
		}}
		// Required for Gemini to honour tool calls; without this some models ignore tools.
		body["tool_config"] = map[string]any{
			"function_calling_config": map[string]any{"mode": "AUTO"},
		}
	}

	// Safety settings — disable all category filters.
	// Gemini's defaults aggressively block legitimate coding/security prompts.
	// litellm and llmgateway both set BLOCK_NONE for all categories.
	body["safetySettings"] = []map[string]any{
		{"category": "HARM_CATEGORY_HARASSMENT", "threshold": "BLOCK_NONE"},
		{"category": "HARM_CATEGORY_HATE_SPEECH", "threshold": "BLOCK_NONE"},
		{"category": "HARM_CATEGORY_SEXUALLY_EXPLICIT", "threshold": "BLOCK_NONE"},
		{"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "BLOCK_NONE"},
		{"category": "HARM_CATEGORY_CIVIC_INTEGRITY", "threshold": "BLOCK_NONE"},
	}

	// Generation config from options
	genConfig := map[string]any{}
	for k, v := range req.Options {
		switch k {
		case "temperature", "top_p", "top_k", "max_tokens":
			key := k
			if k == "max_tokens" {
				key = "maxOutputTokens"
			}
			genConfig[key] = v
		}
	}
	if len(genConfig) > 0 {
		body["generationConfig"] = genConfig
	}

	b, _ := json.Marshal(body)
	return b
}

func (g *Gemini) convertMessage(m Message) map[string]any {
	role := m.Role
	if role == "assistant" {
		role = "model"
	}

	parts := []map[string]any{}

	// Tool call results — Gemini requires the function *name*, not the call ID.
	// Our IDs are synthesized as "call_{functionName}" (see streaming parse),
	// so strip the prefix to recover the actual function name.
	if m.ToolCallID != "" {
		fnName := strings.TrimPrefix(m.ToolCallID, "call_")
		parts = append(parts, map[string]any{
			"functionResponse": map[string]any{
				"name":     fnName,
				"response": map[string]any{"result": m.Content},
			},
		})
		return map[string]any{"role": role, "parts": parts}
	}

	// Text
	if m.Content != "" {
		parts = append(parts, map[string]any{"text": m.Content})
	}

	// Images
	for _, img := range m.Images {
		parts = append(parts, map[string]any{
			"inlineData": map[string]string{
				"mimeType": img.MimeType,
				"data":     img.Data,
			},
		})
	}

	// Tool calls (function calls from assistant)
	for _, tc := range m.ToolCalls {
		parts = append(parts, map[string]any{
			"functionCall": map[string]any{
				"name": tc.Name,
				"args": tc.Arguments,
			},
		})
	}

	if len(parts) == 0 {
		parts = append(parts, map[string]any{"text": ""})
	}

	return map[string]any{"role": role, "parts": parts}
}

func (g *Gemini) doRequest(ctx context.Context, url string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("gemini %d: %s", resp.StatusCode, string(b))
	}
	return b, nil
}

func (g *Gemini) parseResponse(raw []byte) (*ChatResponse, error) {
	var resp geminiResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse gemini: %w", err)
	}
	return g.extractFromCandidates(resp)
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text         string          `json:"text"`
				FunctionCall *struct {
					Name string         `json:"name"`
					Args map[string]any `json:"args"`
				} `json:"functionCall"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func (g *Gemini) extractFromCandidates(resp geminiResponse) (*ChatResponse, error) {
	result := &ChatResponse{}

	if len(resp.Candidates) > 0 {
		cand := resp.Candidates[0]
		result.FinishReason = cand.FinishReason

		var content strings.Builder
		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				content.WriteString(part.Text)
			}
			if part.FunctionCall != nil {
				result.ToolCalls = append(result.ToolCalls, ToolCall{
					ID:        fmt.Sprintf("call_%s", part.FunctionCall.Name),
					Name:      part.FunctionCall.Name,
					Arguments: part.FunctionCall.Args,
				})
			}
		}
		result.Content = content.String()
	}

	if resp.UsageMetadata != nil {
		result.Usage = &Usage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		}
	}
	return result, nil
}

func (g *Gemini) readStream(r io.Reader, onChunk func(StreamChunk)) (*ChatResponse, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	result := &ChatResponse{}
	var content strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var resp geminiResponse
		if json.Unmarshal([]byte(data), &resp) != nil {
			continue
		}

		partial, _ := g.extractFromCandidates(resp)
		if partial == nil {
			continue
		}

		if partial.Content != "" {
			content.WriteString(partial.Content)
			onChunk(StreamChunk{Content: partial.Content})
		}
		if len(partial.ToolCalls) > 0 {
			result.ToolCalls = append(result.ToolCalls, partial.ToolCalls...)
		}
		if partial.FinishReason != "" {
			result.FinishReason = partial.FinishReason
		}
		if partial.Usage != nil {
			result.Usage = partial.Usage
		}
	}

	result.Content = content.String()
	onChunk(StreamChunk{Done: true})
	return result, scanner.Err()
}

// ListModels fetches available Gemini models.
func (g *Gemini) ListModels(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf("%s/models", g.apiBase)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-goog-api-key", g.apiKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("gemini list models %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, err
	}

	models := make([]string, 0, len(result.Models))
	for _, m := range result.Models {
		// "models/gemini-2.5-flash" → "gemini-2.5-flash"
		name := strings.TrimPrefix(m.Name, "models/")
		models = append(models, name)
	}
	return models, nil
}
