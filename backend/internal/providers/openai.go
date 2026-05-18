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

// OpenAI implements the Provider interface for any OpenAI-compatible API.
// Covers: OpenAI, OpenRouter, Groq, DeepSeek, Mistral, xAI, Together,
// Fireworks, Cohere, Perplexity, Ollama, LM Studio, vLLM, MiniMax, etc.
type OpenAI struct {
	apiBase      string
	apiKey       string
	model        string
	name         string
	providerType string
	orgID        string // OpenAI-Organization header
	projectID    string // OpenAI-Project header
	apiVersion   string // Azure api-version query param
	accountID    string // Cloudflare account ID
	authStyle    string // "bearer" (default) | "api-key" (Azure) | "snowflake" | "none"
	extraHeaders map[string]string
	client       *http.Client
}

func NewOpenAI(cfg ProviderConfig) *OpenAI {
	base := cfg.APIBase
	if base == "" {
		base = DefaultAPIBase(cfg.ProviderType)
	}
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	base = strings.TrimRight(base, "/")

	model := "gpt-4o"
	var extra map[string]string
	var orgID, projectID, apiVersion, accountID string
	if cfg.Settings != nil {
		var s struct {
			Model        string            `json:"model"`
			ExtraHeaders map[string]string `json:"extra_headers"`
			OrgID        string            `json:"org_id"`
			ProjectID    string            `json:"project_id"`
			APIVersion   string            `json:"api_version"`
			AccountID    string            `json:"account_id"` // Cloudflare
		}
		if json.Unmarshal(cfg.Settings, &s) == nil {
			if s.Model != "" {
				model = s.Model
			}
			extra = s.ExtraHeaders
			orgID = s.OrgID
			projectID = s.ProjectID
			apiVersion = s.APIVersion
			accountID = s.AccountID
		}
	}

	if extra == nil {
		extra = map[string]string{}
	}

	// Per-provider auth style and required header overrides.
	authStyle := "bearer"
	switch cfg.ProviderType {
	case TypeOpenRouter:
		// OpenRouter requires HTTP-Referer for attribution (returns 403 without it).
		if _, ok := extra["HTTP-Referer"]; !ok {
			extra["HTTP-Referer"] = "https://qorven.ai"
		}
		if _, ok := extra["X-Title"]; !ok {
			extra["X-Title"] = "Qorven"
		}
	case TypeAzureOpenAI, TypeAzureAI:
		// Azure uses api-key header, not Authorization: Bearer.
		authStyle = "api-key"
		if apiVersion == "" {
			apiVersion = "2024-12-01-preview"
		}
	case TypeSnowflake:
		// Snowflake uses: Authorization: Snowflake Token="$JWT"
		authStyle = "snowflake"
	case TypeOllama:
		authStyle = "none"
	}

	return &OpenAI{
		apiBase:      base,
		apiKey:       cfg.APIKey,
		model:        model,
		name:         cfg.Name,
		providerType: cfg.ProviderType,
		orgID:        orgID,
		projectID:    projectID,
		apiVersion:   apiVersion,
		accountID:    accountID,
		authStyle:    authStyle,
		extraHeaders: extra,
		client:       &http.Client{},
	}
}

func (o *OpenAI) Name() string         { return o.name }
func (o *OpenAI) DefaultModel() string  { return o.model }

// Chat sends a non-streaming request.
func (o *OpenAI) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body := o.buildBody(req, false)
	return RetryDo(ctx, DefaultRetryConfig(), func() (*ChatResponse, error) {
		rr, err := o.doRaw(ctx, "/chat/completions", body)
		if err != nil {
			return nil, err
		}
		result, err := o.parseResponse(rr.body)
		if err != nil {
			return nil, err
		}
		result.RequestID = rr.requestID
		result.RateLimitRemaining = rr.rateLimit
		return result, nil
	})
}

// ChatStream sends a streaming request, calling onChunk for each SSE event.
func (o *OpenAI) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	body := o.buildBody(req, true)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.buildURL("/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	o.setHeaders(httpReq)

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai %d: %s", resp.StatusCode, string(b))
	}

	result, err := o.readStream(resp.Body, onChunk)
	if err != nil {
		return nil, err
	}
	result.RequestID = resp.Header.Get("x-request-id")
	if v := resp.Header.Get("x-ratelimit-remaining-requests"); v != "" {
		fmt.Sscanf(v, "%d", &result.RateLimitRemaining)
	}
	return result, nil
}

func (o *OpenAI) buildBody(req ChatRequest, stream bool) []byte {
	model := req.Model
	if model == "" {
		model = o.model
	}

	body := map[string]any{
		"model":    model,
		"messages": o.convertMessages(req.Messages),
		"stream":   stream,
	}
	if len(req.Tools) > 0 {
		body["tools"] = CleanToolSchemas("openai", req.Tools)
		// Only set tool_choice when tools are present; some providers (Groq, older Mistral)
		// reject the field entirely when no tools are in the request.
		if req.ToolChoice != "" {
			body["tool_choice"] = req.ToolChoice
		} else {
			body["tool_choice"] = "auto"
		}
	}
	if stream {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	// Pass through options like temperature, max_tokens, etc.
	for k, v := range req.Options {
		if k != "model" && k != "messages" && k != "tools" && k != "stream" {
			body[k] = v
		}
	}
	b, _ := json.Marshal(body)
	return b
}

func (o *OpenAI) convertMessages(msgs []Message) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		msg := map[string]any{"role": m.Role}

		// Vision: images + text as content array
		if len(m.Images) > 0 {
			parts := []map[string]any{}
			if m.Content != "" {
				parts = append(parts, map[string]any{"type": "text", "text": m.Content})
			}
			for _, img := range m.Images {
				parts = append(parts, map[string]any{
					"type": "image_url",
					"image_url": map[string]string{
						"url": "data:" + img.MimeType + ";base64," + img.Data,
					},
				})
			}
			msg["content"] = parts
		} else {
			msg["content"] = m.Content
		}

		if len(m.ToolCalls) > 0 {
			tc := make([]map[string]any, len(m.ToolCalls))
			for i, t := range m.ToolCalls {
				args, _ := json.Marshal(t.Arguments)
				tc[i] = map[string]any{
					"id":   t.ID,
					"type": "function",
					"function": map[string]any{
						"name":      t.Name,
						"arguments": string(args),
					},
				}
			}
			msg["tool_calls"] = tc
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		out = append(out, msg)
	}
	return out
}

func (o *OpenAI) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	switch o.authStyle {
	case "api-key":
		// Azure OpenAI uses api-key header instead of Authorization: Bearer.
		if o.apiKey != "" {
			req.Header.Set("api-key", o.apiKey)
		}
	case "snowflake":
		// Snowflake Cortex uses: Authorization: Snowflake Token="$JWT"
		if o.apiKey != "" {
			req.Header.Set("Authorization", `Snowflake Token="`+o.apiKey+`"`)
			req.Header.Set("X-Snowflake-Authorization-Token-Type", "KEYPAIR_JWT")
		}
	case "none":
		// Ollama / local endpoints: no auth header.
	default:
		// Standard Bearer (OpenAI, Groq, Mistral, etc.)
		if o.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+o.apiKey)
		}
	}
	// OpenAI-specific optional org/project scoping headers.
	if o.orgID != "" {
		req.Header.Set("OpenAI-Organization", o.orgID)
	}
	if o.projectID != "" {
		req.Header.Set("OpenAI-Project", o.projectID)
	}
	for k, v := range o.extraHeaders {
		req.Header.Set(k, v)
	}
}

type rawResponse struct {
	body      []byte
	requestID string
	rateLimit int // x-ratelimit-remaining-requests
}

// buildURL constructs the full request URL for the given path, injecting
// Azure api-version or Cloudflare account ID where needed.
func (o *OpenAI) buildURL(path string) string {
	base := o.apiBase

	// Cloudflare Workers AI: base is https://api.cloudflare.com/client/v4/accounts
	// and the actual endpoint is /accounts/{account_id}/ai/run/{model}
	// For chat/completions we route through their OpenAI-compat endpoint.
	if o.providerType == TypeCloudflare && o.accountID != "" {
		base = fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai", o.accountID)
	}

	url := base + path

	if o.apiVersion != "" {
		if strings.Contains(url, "?") {
			url += "&api-version=" + o.apiVersion
		} else {
			url += "?api-version=" + o.apiVersion
		}
	}
	return url
}

func (o *OpenAI) doRaw(ctx context.Context, path string, body []byte) (*rawResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", o.buildURL(path), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	o.setHeaders(req)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		retryAfter := ParseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, &HTTPError{Status: resp.StatusCode, Body: string(b), RetryAfter: retryAfter}
	}

	rr := &rawResponse{
		body:      b,
		requestID: resp.Header.Get("x-request-id"),
	}
	if v := resp.Header.Get("x-ratelimit-remaining-requests"); v != "" {
		fmt.Sscanf(v, "%d", &rr.rateLimit)
	}
	return rr, nil
}

func (o *OpenAI) doRequest(ctx context.Context, body []byte) ([]byte, error) {
	rr, err := o.doRaw(ctx, "/chat/completions", body)
	if err != nil {
		return nil, err
	}
	return rr.body, nil
}

func (o *OpenAI) parseResponse(raw []byte) (*ChatResponse, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := resp.Choices[0]
	result := &ChatResponse{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		RawContent:   raw,
	}

	for _, tc := range choice.Message.ToolCalls {
		args, ok := parseToolArgsString(tc.Function.Arguments, tc.Function.Name)
		call := ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		}
		if !ok {
			call.ArgsParseError = "arguments JSON invalid or truncated"
		}
		result.ToolCalls = append(result.ToolCalls, call)
	}

	if resp.Usage != nil {
		result.Usage = &Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}
	return result, nil
}

func (o *OpenAI) readStream(r io.Reader, onChunk func(StreamChunk)) (*ChatResponse, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	result := &ChatResponse{}
	var content strings.Builder
	var toolCalls []ToolCall
	tcArgs := map[int]*strings.Builder{} // accumulate streamed tool call args

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}

		if event.Usage != nil {
			result.Usage = &Usage{
				PromptTokens:     event.Usage.PromptTokens,
				CompletionTokens: event.Usage.CompletionTokens,
				TotalTokens:      event.Usage.TotalTokens,
			}
		}

		if len(event.Choices) == 0 {
			continue
		}
		delta := event.Choices[0].Delta

		if delta.Content != "" {
			content.WriteString(delta.Content)
			onChunk(StreamChunk{Content: delta.Content})
		}

		for _, tc := range delta.ToolCalls {
			if tc.ID != "" {
				// New tool call
				for len(toolCalls) <= tc.Index {
					toolCalls = append(toolCalls, ToolCall{})
				}
				toolCalls[tc.Index].ID = tc.ID
				toolCalls[tc.Index].Name = tc.Function.Name
				tcArgs[tc.Index] = &strings.Builder{}
			}
			if tc.Function.Arguments != "" {
				if _, ok := tcArgs[tc.Index]; !ok {
					tcArgs[tc.Index] = &strings.Builder{}
				}
				tcArgs[tc.Index].WriteString(tc.Function.Arguments)
			}
		}

		if event.Choices[0].FinishReason != nil {
			result.FinishReason = *event.Choices[0].FinishReason
		}
	}

	// Finalize tool calls
	for i, tc := range toolCalls {
		if b, ok := tcArgs[i]; ok {
			args, parsed := parseToolArgsString(b.String(), tc.Name)
			tc.Arguments = args
			if !parsed {
				tc.ArgsParseError = "streaming arguments JSON invalid or truncated"
			}
			toolCalls[i] = tc
		}
	}

	result.Content = content.String()
	result.ToolCalls = toolCalls
	onChunk(StreamChunk{Done: true})
	return result, scanner.Err()
}

// ResponsesAPI calls the OpenAI Responses API (POST /v1/responses).
// This is the newer stateful API used by gpt-4.1 and o-series models.
// input is the user message string; previousResponseID chains a multi-turn session.
// Returns the assistant text and the response ID for the next turn.
func (o *OpenAI) ResponsesAPI(ctx context.Context, model, input, previousResponseID string, tools []ToolDefinition) (text, responseID string, err error) {
	if model == "" {
		model = o.model
	}
	payload := map[string]any{
		"model": model,
		"input": input,
	}
	if previousResponseID != "" {
		payload["previous_response_id"] = previousResponseID
	}
	if len(tools) > 0 {
		payload["tools"] = CleanToolSchemas("openai", tools)
	}

	body, _ := json.Marshal(payload)
	rr, err := o.doRaw(ctx, "/responses", body)
	if err != nil {
		return "", "", err
	}

	var resp struct {
		ID     string `json:"id"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(rr.body, &resp); err != nil {
		return "", "", fmt.Errorf("parse responses api: %w", err)
	}

	var sb strings.Builder
	for _, out := range resp.Output {
		if out.Type != "message" {
			continue
		}
		for _, c := range out.Content {
			if c.Type == "output_text" {
				sb.WriteString(c.Text)
			}
		}
	}
	return sb.String(), resp.ID, nil
}

// ListModels fetches available models from the provider.
func (o *OpenAI) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", o.buildURL("/models"), nil)
	if err != nil {
		return nil, err
	}
	o.setHeaders(req)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("list models %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, err
	}

	models := make([]string, len(result.Data))
	for i, m := range result.Data {
		models[i] = m.ID
	}
	return models, nil
}
