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

// Anthropic implements the Provider interface for Claude's native API.
// Anthropic uses a different message format: content blocks, tool_use blocks,
// system as top-level param (not a message), and thinking/reasoning support.
type Anthropic struct {
	apiBase string
	apiKey  string
	model   string
	name    string
	version string
	client  *http.Client
}

func NewAnthropic(cfg ProviderConfig) *Anthropic {
	base := cfg.APIBase
	if base == "" {
		base = DefaultAPIBase(TypeAnthropicNative)
	}
	base = strings.TrimRight(base, "/")

	model := "claude-sonnet-4-6"
	version := "2023-06-01"
	if cfg.Settings != nil {
		var s struct {
			Model   string `json:"model"`
			Version string `json:"version"`
		}
		if json.Unmarshal(cfg.Settings, &s) == nil {
			if s.Model != "" {
				model = s.Model
			}
			if s.Version != "" {
				version = s.Version
			}
		}
	}

	return &Anthropic{
		apiBase: base,
		apiKey:  cfg.APIKey,
		model:   model,
		name:    cfg.Name,
		version: version,
		client:  &http.Client{},
	}
}

func (a *Anthropic) Name() string         { return a.name }
func (a *Anthropic) DefaultModel() string  { return a.model }
func (a *Anthropic) SupportsThinking() bool { return true }

func (a *Anthropic) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body := a.buildBody(req, false)
	raw, reqID, err := a.doRequestWithID(ctx, a.apiBase+"/v1/messages", body, betaHeaders(req)...)
	if err != nil {
		return nil, err
	}
	result, err := a.parseResponse(raw)
	if err != nil {
		return nil, err
	}
	result.RequestID = reqID
	return result, nil
}

func (a *Anthropic) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	body := a.buildBody(req, true)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.apiBase+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	a.setHeaders(httpReq, betaHeaders(req)...)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic %d: %s", resp.StatusCode, string(b))
	}

	result, err := a.readStream(resp.Body, onChunk)
	if err != nil {
		return nil, err
	}
	result.RequestID = resp.Header.Get("request-id")
	return result, nil
}

// betaHeaders returns the anthropic-beta values required for this request.
// Claude 4.x extended thinking no longer requires interleaved-thinking-2025-05-14;
// that header was for Claude 3.x. Tool prompt caching still requires the beta opt-in.
func betaHeaders(req ChatRequest) []string {
	var betas []string
	if len(req.Tools) > 0 {
		betas = append(betas, "prompt-caching-2024-07-31")
	}
	return betas
}

func (a *Anthropic) buildBody(req ChatRequest, stream bool) []byte {
	model := req.Model
	if model == "" {
		model = a.model
	}

	// Anthropic: system is a top-level param array, not in messages.
	// Multiple system messages are supported — each becomes a text block.
	// A message with CacheControl="ephemeral" gets a cache_control marker on its block,
	// enabling Anthropic's prompt-cache feature for stable prefixes.
	var systemBlocks []map[string]any
	msgs := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == "system" {
			block := map[string]any{"type": "text", "text": m.Content}
			if m.CacheControl == "ephemeral" {
				block["cache_control"] = map[string]string{"type": "ephemeral"}
			}
			systemBlocks = append(systemBlocks, block)
			continue
		}
		msgs = append(msgs, a.convertMessage(m))
	}
	// Backward-compat: if only one system block and no explicit cache_control was set,
	// add ephemeral caching (preserves pre-existing behaviour).
	if len(systemBlocks) == 1 {
		if _, hasCacheCtrl := systemBlocks[0]["cache_control"]; !hasCacheCtrl {
			systemBlocks[0]["cache_control"] = map[string]string{"type": "ephemeral"}
		}
	}

	maxTokens := 8192
	if v, ok := req.Options["max_tokens"]; ok {
		switch n := v.(type) {
		case int:
			maxTokens = n
		case float64:
			maxTokens = int(n)
		}
	}
	body := map[string]any{
		"model":      model,
		"messages":   msgs,
		"max_tokens": maxTokens,
		"stream":     stream,
	}
	if len(systemBlocks) > 0 {
		body["system"] = systemBlocks
	}

	// Tools → Anthropic format
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = map[string]any{
				"name":         t.Function.Name,
				"description":  t.Function.Description,
				"input_schema": CleanSchemaForProvider("anthropic", t.Function.Parameters),
			}
		}
		body["tools"] = tools
	}

	// Extended thinking
	if v, ok := req.Options["thinking"]; ok {
		body["thinking"] = v
	}

	for k, v := range req.Options {
		if k == "thinking" || k == "model" || k == "messages" || k == "tools" || k == "stream" || k == "max_tokens" || k == "system" {
			continue
		}
		body[k] = v
	}

	b, _ := json.Marshal(body)
	return b
}

// convertMessage converts our Message to Anthropic's content block format.
func (a *Anthropic) convertMessage(m Message) map[string]any {
	msg := map[string]any{"role": m.Role}

	// Tool result message
	if m.ToolCallID != "" {
		msg["role"] = "user"
		msg["content"] = []map[string]any{{
			"type":        "tool_result",
			"tool_use_id": m.ToolCallID,
			"content":     m.Content,
		}}
		return msg
	}

	// Assistant with tool calls
	if m.Role == "assistant" && len(m.ToolCalls) > 0 {
		blocks := []map[string]any{}
		if m.Content != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
		}
		for _, tc := range m.ToolCalls {
			blocks = append(blocks, map[string]any{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Name,
				"input": tc.Arguments,
			})
		}
		msg["content"] = blocks
		return msg
	}

	// Vision
	if len(m.Images) > 0 {
		blocks := []map[string]any{}
		for _, img := range m.Images {
			blocks = append(blocks, map[string]any{
				"type": "image",
				"source": map[string]string{
					"type":       "base64",
					"media_type": img.MimeType,
					"data":       img.Data,
				},
			})
		}
		if m.Content != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
		}
		msg["content"] = blocks
		return msg
	}

	msg["content"] = m.Content
	return msg
}

func (a *Anthropic) setHeaders(req *http.Request, betaFeatures ...string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", a.version)
	if len(betaFeatures) > 0 {
		req.Header.Set("anthropic-beta", strings.Join(betaFeatures, ","))
	}
}

func (a *Anthropic) doRequestWithID(ctx context.Context, url string, body []byte, betas ...string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	a.setHeaders(req, betas...)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("anthropic %d: %s", resp.StatusCode, string(b))
	}
	return b, resp.Header.Get("request-id"), nil
}

// CountTokens calls POST /v1/messages/count_tokens to get the token count for a request
// without generating a response. Returns prompt token count.
func (a *Anthropic) CountTokens(ctx context.Context, req ChatRequest) (int, error) {
	// count_tokens uses same body structure as messages but without stream/max_tokens
	body := a.buildBody(req, false)
	// Patch out max_tokens and stream — count_tokens doesn't accept them
	var m map[string]any
	if json.Unmarshal(body, &m) == nil {
		delete(m, "max_tokens")
		delete(m, "stream")
		body, _ = json.Marshal(m)
	}
	raw, _, err := a.doRequestWithID(ctx, a.apiBase+"/v1/messages/count_tokens", body, betaHeaders(req)...)
	if err != nil {
		return 0, err
	}
	var result struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return 0, fmt.Errorf("count_tokens parse: %w", err)
	}
	return result.InputTokens, nil
}

func (a *Anthropic) parseResponse(raw []byte) (*ChatResponse, error) {
	var resp struct {
		Content []struct {
			Type     string          `json:"type"`
			Text     string          `json:"text"`
			ID       string          `json:"id"`
			Name     string          `json:"name"`
			Input    json.RawMessage `json:"input"`
			Thinking string          `json:"thinking"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      *struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse anthropic: %w", err)
	}

	result := &ChatResponse{FinishReason: resp.StopReason, RawContent: raw}

	var content strings.Builder
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			content.WriteString(block.Text)
		case "thinking":
			result.Thinking = block.Thinking
		case "tool_use":
			args, ok := parseToolArgs(block.Input, block.Name)
			call := ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			}
			if !ok {
				call.ArgsParseError = "tool_use input JSON invalid or truncated"
			}
			result.ToolCalls = append(result.ToolCalls, call)
		}
	}
	result.Content = content.String()

	if resp.Usage != nil {
		result.Usage = &Usage{
			PromptTokens:        resp.Usage.InputTokens,
			CompletionTokens:    resp.Usage.OutputTokens,
			TotalTokens:         resp.Usage.InputTokens + resp.Usage.OutputTokens,
			CacheCreationTokens: resp.Usage.CacheCreationInputTokens,
			CacheReadTokens:     resp.Usage.CacheReadInputTokens,
		}
	}
	return result, nil
}

func (a *Anthropic) readStream(r io.Reader, onChunk func(StreamChunk)) (*ChatResponse, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	result := &ChatResponse{}
	var content, thinking strings.Builder
	var toolCalls []ToolCall
	currentToolIdx := -1
	var currentToolArgs strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var event struct {
			Type  string `json:"type"`
			Index int    `json:"index"`
			Delta *struct {
				Type         string          `json:"type"`
				Text         string          `json:"text"`
				Thinking     string          `json:"thinking"`
				PartialJSON  string          `json:"partial_json"`
				StopReason   string          `json:"stop_reason"`
			} `json:"delta"`
			ContentBlock *struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Message *struct {
				Usage *struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage *struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}

		switch event.Type {
		case "message_start":
			if event.Message != nil && event.Message.Usage != nil {
				result.Usage = &Usage{
					PromptTokens: event.Message.Usage.InputTokens,
				}
			}

		case "content_block_start":
			if event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
				currentToolIdx = len(toolCalls)
				toolCalls = append(toolCalls, ToolCall{
					ID:   event.ContentBlock.ID,
					Name: event.ContentBlock.Name,
				})
				currentToolArgs.Reset()
			}

		case "content_block_delta":
			if event.Delta == nil {
				continue
			}
			switch event.Delta.Type {
			case "text_delta":
				content.WriteString(event.Delta.Text)
				onChunk(StreamChunk{Content: event.Delta.Text})
			case "thinking_delta":
				thinking.WriteString(event.Delta.Thinking)
				onChunk(StreamChunk{Thinking: event.Delta.Thinking})
			case "input_json_delta":
				currentToolArgs.WriteString(event.Delta.PartialJSON)
			}

		case "content_block_stop":
			if currentToolIdx >= 0 && currentToolIdx < len(toolCalls) {
				args, ok := parseToolArgsString(currentToolArgs.String(), toolCalls[currentToolIdx].Name)
				toolCalls[currentToolIdx].Arguments = args
				if !ok {
					toolCalls[currentToolIdx].ArgsParseError = "streaming tool_use input JSON invalid or truncated"
				}
				currentToolIdx = -1
			}

		case "message_delta":
			if event.Delta != nil && event.Delta.StopReason != "" {
				result.FinishReason = event.Delta.StopReason
			}
			if event.Usage != nil && result.Usage != nil {
				result.Usage.CompletionTokens = event.Usage.OutputTokens
				result.Usage.TotalTokens = result.Usage.PromptTokens + event.Usage.OutputTokens
			}
		}
	}

	result.Content = content.String()
	result.Thinking = thinking.String()
	result.ToolCalls = toolCalls
	onChunk(StreamChunk{Done: true})
	return result, scanner.Err()
}
