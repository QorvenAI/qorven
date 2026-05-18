// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package llm

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

// Message is a chat message in OpenAI format.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the input to a provider.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	Tools       []any     `json:"tools,omitempty"`
}

// ChatResponse is a non-streaming response.
type ChatResponse struct {
	Content      string `json:"content"`
	Model        string `json:"model"`
	FinishReason string `json:"finish_reason"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall from the LLM.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// StreamChunk is one piece of a streaming response.
type StreamChunk struct {
	Delta        string     `json:"delta"`
	FinishReason string     `json:"finish_reason,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
}

// Provider is the interface all LLM drivers implement.
type Provider interface {
	Name() string
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error)
	ListModels(ctx context.Context) ([]string, error)
}

// --- OpenAI-Compatible Driver (works for OpenAI, Groq, DeepSeek, Together, custom) ---

type OpenAIProvider struct {
	name    string
	apiKey  string
	apiBase string
	client  *http.Client
}

func NewOpenAIProvider(name, apiKey, apiBase string) *OpenAIProvider {
	if apiBase == "" { apiBase = "https://api.openai.com/v1" }
	return &OpenAIProvider{name: name, apiKey: apiKey, apiBase: strings.TrimRight(apiBase, "/"), client: &http.Client{}}
}

func (p *OpenAIProvider) Name() string { return p.name }

func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	req.Stream = false
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai %d: %s", resp.StatusCode, string(b))
	}

	var raw struct {
		Choices []struct {
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		} `json:"choices"`
		Model string `json:"model"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	json.NewDecoder(resp.Body).Decode(&raw)
	if len(raw.Choices) == 0 { return nil, fmt.Errorf("no choices") }

	return &ChatResponse{
		Content: raw.Choices[0].Message.Content, Model: raw.Model,
		FinishReason: raw.Choices[0].FinishReason,
		InputTokens: raw.Usage.PromptTokens, OutputTokens: raw.Usage.CompletionTokens,
	}, nil
}

func (p *OpenAIProvider) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	req.Stream = true
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai stream %d: %s", resp.StatusCode, string(b))
	}

	var full strings.Builder
	var finishReason string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") { continue }
		data := line[6:]
		if data == "[DONE]" { break }

		var chunk struct {
			Choices []struct {
				Delta        struct{ Content string `json:"content"` } `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		json.Unmarshal([]byte(data), &chunk)
		if len(chunk.Choices) == 0 { continue }

		delta := chunk.Choices[0].Delta.Content
		if delta != "" {
			full.WriteString(delta)
			onChunk(StreamChunk{Delta: delta})
		}
		if chunk.Choices[0].FinishReason != nil {
			finishReason = *chunk.Choices[0].FinishReason
		}
	}

	return &ChatResponse{Content: full.String(), FinishReason: finishReason}, nil
}

func (p *OpenAIProvider) ListModels(ctx context.Context) ([]string, error) {
	httpReq, _ := http.NewRequestWithContext(ctx, "GET", p.apiBase+"/models", nil)
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := p.client.Do(httpReq)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	var raw struct {
		Data []struct{ ID string `json:"id"` } `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&raw)
	models := make([]string, len(raw.Data))
	for i, m := range raw.Data { models[i] = m.ID }
	return models, nil
}

// --- Anthropic Driver ---

type AnthropicProvider struct {
	apiKey string
	client *http.Client
}

func NewAnthropicProvider(apiKey string) *AnthropicProvider {
	return &AnthropicProvider{apiKey: apiKey, client: &http.Client{}}
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	system := ""
	msgs := make([]map[string]string, 0)
	for _, m := range req.Messages {
		if m.Role == "system" { system = m.Content; continue }
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}

	payload := map[string]any{"model": req.Model, "messages": msgs, "max_tokens": anthropicMaxTokens(req.Model, req.MaxTokens)}
	if system != "" { payload["system"] = system }

	body, _ := json.Marshal(payload)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic %d: %s", resp.StatusCode, string(b))
	}

	var raw struct {
		Content []struct{ Text string `json:"text"` } `json:"content"`
		Model   string `json:"model"`
		Usage   struct{ InputTokens, OutputTokens int } `json:"usage"`
		StopReason string `json:"stop_reason"`
	}
	json.NewDecoder(resp.Body).Decode(&raw)

	text := ""
	if len(raw.Content) > 0 { text = raw.Content[0].Text }
	return &ChatResponse{
		Content: text, Model: raw.Model, FinishReason: raw.StopReason,
		InputTokens: raw.Usage.InputTokens, OutputTokens: raw.Usage.OutputTokens,
	}, nil
}

func (p *AnthropicProvider) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	system := ""
	msgs := make([]map[string]string, 0)
	for _, m := range req.Messages {
		if m.Role == "system" { system = m.Content; continue }
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}

	payload := map[string]any{"model": req.Model, "messages": msgs, "max_tokens": anthropicMaxTokens(req.Model, req.MaxTokens), "stream": true}
	if system != "" { payload["system"] = system }

	body, _ := json.Marshal(payload)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic stream %d: %s", resp.StatusCode, string(b))
	}

	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") { continue }
		var event struct {
			Type  string `json:"type"`
			Delta struct{ Text string `json:"text"` } `json:"delta"`
		}
		json.Unmarshal([]byte(line[6:]), &event)
		if event.Type == "content_block_delta" && event.Delta.Text != "" {
			full.WriteString(event.Delta.Text)
			onChunk(StreamChunk{Delta: event.Delta.Text})
		}
	}

	return &ChatResponse{Content: full.String(), FinishReason: "end_turn"}, nil
}

func (p *AnthropicProvider) ListModels(ctx context.Context) ([]string, error) {
	return []string{
		"claude-sonnet-4-20250514", "claude-haiku-3-5-20241022",
		"claude-opus-4-20250514", "claude-3-5-sonnet-20241022",
	}, nil
}

// anthropicMaxTokens returns per-model output limits instead of hardcoded values.
// From Qorven: "Replaced hardcoded 16K max_tokens with per-model native output limits"
func anthropicMaxTokens(model string, requested int) int {
	if requested > 0 { return requested }
	switch {
	case strings.Contains(model, "opus"):
		return 128000
	case strings.Contains(model, "sonnet"):
		return 64000
	case strings.Contains(model, "haiku"):
		return 8192
	default:
		return 4096
	}
}
