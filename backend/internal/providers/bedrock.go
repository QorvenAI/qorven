// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// BedrockProvider calls models via AWS Bedrock.
// Supports both Anthropic format (Claude) and OpenAI-compatible format (Nemotron, Qwen, DeepSeek, etc.)
type BedrockProvider struct {
	client  *bedrockruntime.Client
	modelID string
	region  string
	name    string
}

// BedrockCreds holds optional static AWS credentials. When non-empty they
// take precedence over the default credential chain (env vars, ~/.aws, IMDS).
type BedrockCreds struct {
	AccessKey    string
	SecretKey    string
	SessionToken string
}

func NewBedrockProvider(name, modelID, region string, creds ...BedrockCreds) (*BedrockProvider, error) {
	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if len(creds) > 0 && creds[0].AccessKey != "" && creds[0].SecretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(creds[0].AccessKey, creds[0].SecretKey, creds[0].SessionToken),
		))
	}
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("bedrock: load AWS config: %w", err)
	}
	return &BedrockProvider{
		client:  bedrockruntime.NewFromConfig(cfg),
		modelID: modelID,
		region:  region,
		name:    name,
	}, nil
}

func (p *BedrockProvider) Name() string { return p.name }

// SupportsThinking implements ThinkingCapable. BedrockProvider routes
// Anthropic Claude models via InvokeModel which supports the thinking
// parameter natively in the request body.
func (p *BedrockProvider) SupportsThinking() bool { return true }

// DefaultModel returns the configured model, or a curated Bedrock default
// (Haiku 4.5) when no model was bound to the provider instance. Previously
// this returned an empty string when constructed with modelID="", which
// then caused InvokeModel("") to fail with a serialization error during
// verify/health checks.
func (p *BedrockProvider) DefaultModel() string {
	if p.modelID != "" { return p.modelID }
	return BedrockDefaultModel
}

// BedrockDefaultModel — the fallback Bedrock inference-profile used when
// a request carries no explicit model. Haiku 4.5 is cheap, fast, and
// widely available in us-east-1.
const BedrockDefaultModel = "us.anthropic.claude-haiku-4-5-20251001-v1:0"

// BedrockCuratedModels — the inference-profile IDs we surface in the
// setup wizard and model picker. Reflects the "decisions" doc: Anthropic
// 4.x via inference profiles + the open-weight alternatives that don't
// need a Marketplace subscription.
var BedrockCuratedModels = []string{
	// Anthropic (inference profiles — the us.* prefix matters)
	"us.anthropic.claude-opus-4-7",
	"us.anthropic.claude-sonnet-4-6",
	"us.anthropic.claude-haiku-4-5-20251001-v1:0",
	"us.anthropic.claude-opus-4-5-20251101-v1:0",
	"us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	// Open models — no subscription required
	"deepseek.v3.2",
	"us.deepseek.r1-v1:0",
	"qwen.qwen3-coder-next",
	"qwen.qwen3-next-80b-a3b",
	"us.meta.llama4-maverick-17b-instruct-v1:0",
	"us.meta.llama4-scout-17b-instruct-v1:0",
	"nvidia.nemotron-super-3-120b",
	"moonshotai.kimi-k2.5",
	"minimax.minimax-m2",
	"amazon.nova-pro-v1:0",
	"amazon.nova-lite-v1:0",
}

// ListModels returns the curated Bedrock model list. We deliberately
// don't call bedrock:ListFoundationModels here — the wizard needs the
// small, user-meaningful subset, not all ~200 profiles including
// embedding models, image models, and legacy Claude 3 variants.
func (p *BedrockProvider) ListModels(ctx context.Context) ([]string, error) {
	out := make([]string, len(BedrockCuratedModels))
	copy(out, BedrockCuratedModels)
	return out, nil
}

func (p *BedrockProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	start := time.Now()
	model := req.Model
	if model == "" { model = p.modelID }
	if model == "" { model = BedrockDefaultModel }

	body := p.buildBody(req, model)
	bodyBytes, _ := json.Marshal(body)

	resp, err := p.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(model),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
		Body:        bodyBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("bedrock [%s]: %w", model, err)
	}

	return p.parseResponse(resp.Body, model, time.Since(start))
}

func (p *BedrockProvider) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	model := req.Model
	if model == "" { model = p.modelID }
	if model == "" { model = BedrockDefaultModel }

	body := p.buildBody(req, model)
	bodyBytes, _ := json.Marshal(body)

	stream, err := p.client.InvokeModelWithResponseStream(ctx, &bedrockruntime.InvokeModelWithResponseStreamInput{
		ModelId:     aws.String(model),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
		Body:        bodyBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("bedrock stream [%s]: %w", model, err)
	}

	if isAnthropicModel(model) {
		return p.readAnthropicStream(stream, onChunk)
	}
	return p.readOpenAIStream(stream, onChunk)
}

// readAnthropicStream processes SSE events from InvokeModelWithResponseStream for Claude models.
func (p *BedrockProvider) readAnthropicStream(stream *bedrockruntime.InvokeModelWithResponseStreamOutput, onChunk func(StreamChunk)) (*ChatResponse, error) {
	result := &ChatResponse{}
	var content, thinking strings.Builder
	var toolCalls []ToolCall
	currentToolIdx := -1
	var currentToolArgs strings.Builder

	for event := range stream.GetStream().Events() {
		chunk, ok := event.(*brtypes.ResponseStreamMemberChunk)
		if !ok {
			continue
		}
		var ev struct {
			Type  string `json:"type"`
			Index int    `json:"index"`
			Delta *struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				Thinking    string `json:"thinking"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			ContentBlock *struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(chunk.Value.Bytes, &ev) != nil {
			continue
		}
		switch ev.Type {
		case "content_block_start":
			if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
				currentToolIdx = len(toolCalls)
				toolCalls = append(toolCalls, ToolCall{ID: ev.ContentBlock.ID, Name: ev.ContentBlock.Name})
				currentToolArgs.Reset()
			}
		case "content_block_delta":
			if ev.Delta == nil { continue }
			switch ev.Delta.Type {
			case "text_delta":
				content.WriteString(ev.Delta.Text)
				onChunk(StreamChunk{Content: ev.Delta.Text})
			case "thinking_delta":
				thinking.WriteString(ev.Delta.Thinking)
				onChunk(StreamChunk{Thinking: ev.Delta.Thinking})
			case "input_json_delta":
				currentToolArgs.WriteString(ev.Delta.PartialJSON)
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
			if ev.Delta != nil && ev.Delta.StopReason != "" {
				result.FinishReason = ev.Delta.StopReason
			}
		case "message_start":
			if ev.Usage != nil {
				result.Usage = &Usage{PromptTokens: ev.Usage.InputTokens}
			}
		}
	}
	if err := stream.GetStream().Err(); err != nil {
		return nil, fmt.Errorf("bedrock stream: %w", err)
	}

	result.Content = content.String()
	result.Thinking = thinking.String()
	result.ToolCalls = toolCalls
	onChunk(StreamChunk{Done: true})
	return result, nil
}

// readOpenAIStream processes SSE events for non-Anthropic Bedrock models (Llama, Qwen, DeepSeek, etc.)
func (p *BedrockProvider) readOpenAIStream(stream *bedrockruntime.InvokeModelWithResponseStreamOutput, onChunk func(StreamChunk)) (*ChatResponse, error) {
	result := &ChatResponse{}
	var content strings.Builder
	var toolCalls []ToolCall
	tcArgs := map[int]*strings.Builder{}

	for event := range stream.GetStream().Events() {
		chunk, ok := event.(*brtypes.ResponseStreamMemberChunk)
		if !ok {
			continue
		}
		var ev struct {
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
		if json.Unmarshal(chunk.Value.Bytes, &ev) != nil {
			continue
		}
		if ev.Usage != nil {
			result.Usage = &Usage{
				PromptTokens: ev.Usage.PromptTokens, CompletionTokens: ev.Usage.CompletionTokens, TotalTokens: ev.Usage.TotalTokens,
			}
		}
		if len(ev.Choices) == 0 { continue }
		d := ev.Choices[0].Delta
		if d.Content != "" {
			content.WriteString(d.Content)
			onChunk(StreamChunk{Content: d.Content})
		}
		for _, tc := range d.ToolCalls {
			if tc.ID != "" {
				for len(toolCalls) <= tc.Index { toolCalls = append(toolCalls, ToolCall{}) }
				toolCalls[tc.Index].ID = tc.ID
				toolCalls[tc.Index].Name = tc.Function.Name
				tcArgs[tc.Index] = &strings.Builder{}
			}
			if tc.Function.Arguments != "" {
				if _, ok := tcArgs[tc.Index]; !ok { tcArgs[tc.Index] = &strings.Builder{} }
				tcArgs[tc.Index].WriteString(tc.Function.Arguments)
			}
		}
		if ev.Choices[0].FinishReason != nil {
			result.FinishReason = *ev.Choices[0].FinishReason
		}
	}
	if err := stream.GetStream().Err(); err != nil {
		return nil, fmt.Errorf("bedrock stream: %w", err)
	}

	for i, tc := range toolCalls {
		if b, ok := tcArgs[i]; ok {
			args, parsed := parseToolArgsString(b.String(), tc.Name)
			tc.Arguments = args
			if !parsed { tc.ArgsParseError = "streaming arguments JSON invalid or truncated" }
			toolCalls[i] = tc
		}
	}
	result.Content = content.String()
	result.ToolCalls = toolCalls
	onChunk(StreamChunk{Done: true})
	return result, nil
}

// isAnthropicModel returns true for models that use Anthropic Messages API format.
// Includes Bedrock inference-profile prefixes (us./global./eu.) so Claude 4.x
// profiles like "us.anthropic.claude-sonnet-4-6" are routed through the
// Anthropic Messages body shape. Previously only "anthropic." prefixed
// foundation model IDs were detected, which meant every 4.x inference-profile
// silently went through the OpenAI-compat body and errored.
func isAnthropicModel(model string) bool {
	switch {
	case len(model) >= 10 && model[:10] == "anthropic.":
		return true
	case len(model) >= 13 && model[:13] == "us.anthropic.":
		return true
	case len(model) >= 17 && model[:17] == "global.anthropic.":
		return true
	case len(model) >= 13 && model[:13] == "eu.anthropic.":
		return true
	}
	return false
}

func (p *BedrockProvider) buildBody(req ChatRequest, model string) map[string]any {
	if isAnthropicModel(model) {
		return p.buildAnthropicBody(req)
	}
	return p.buildOpenAIBody(req)
}

// buildOpenAIBody — for Nemotron, Qwen, DeepSeek, Kimi, GLM, MiniMax
func (p *BedrockProvider) buildOpenAIBody(req ChatRequest) map[string]any {
	var msgs []map[string]any
	for _, m := range req.Messages {
		msg := map[string]any{"role": m.Role, "content": m.Content}
		if len(m.ToolCalls) > 0 {
			var tcs []map[string]any
			for _, tc := range m.ToolCalls {
				args, _ := json.Marshal(tc.Arguments)
				tcs = append(tcs, map[string]any{
					"id": tc.ID, "type": "function",
					"function": map[string]string{"name": tc.Name, "arguments": string(args)},
				})
			}
			msg["tool_calls"] = tcs
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		msgs = append(msgs, msg)
	}
	body := map[string]any{"messages": msgs, "max_tokens": 4096}
	if temp, ok := req.Options["temperature"]; ok { body["temperature"] = temp }
	if len(req.Tools) > 0 {
		var tools []map[string]any
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": t.Function.Name, "description": t.Function.Description,
					"parameters": t.Function.Parameters,
				},
			})
		}
		body["tools"] = tools
	}
	return body
}

// buildAnthropicBody — for Claude models
// buildAnthropicBody — for Claude models on Bedrock.
//
// The internal ChatRequest format mirrors OpenAI's (role: user/assistant/
// tool, assistant carries ToolCalls, tool messages reply with ToolCallID +
// string content). The Anthropic Messages API is structurally different:
//
//   - Only "user" and "assistant" roles exist — tool *results* must ride
//     inside a "user" message as [{type:"tool_result", tool_use_id, content}]
//     content blocks. Sending role:"tool" errors with
//     "Unexpected role \"tool\". Allowed roles are \"user\" or \"assistant\"".
//
//   - Assistant tool *calls* must be encoded as [{type:"tool_use", id, name,
//     input}] content blocks on the assistant message, not as a separate
//     "tool_calls" field. Mixed text + tool_use is allowed — emit the text
//     block first, then each tool_use.
//
// Consecutive tool messages (multiple tools in one turn) are merged into a
// single user message carrying one tool_result block per tool call — this
// matches what Claude expects and what the agent loop actually produces.
func (p *BedrockProvider) buildAnthropicBody(req ChatRequest) map[string]any {
	var msgs []map[string]any
	var system string

	flushToolResults := func(pending []map[string]any) []map[string]any {
		if len(pending) > 0 {
			msgs = append(msgs, map[string]any{"role": "user", "content": pending})
		}
		return nil
	}

	var pendingToolResults []map[string]any

	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			system += m.Content + "\n"

		case "tool":
			// Convert an OpenAI-shaped tool reply into an Anthropic
			// tool_result content block, buffering until we hit a
			// non-tool message. Content must be a string.
			pendingToolResults = append(pendingToolResults, map[string]any{
				"type":        "tool_result",
				"tool_use_id": m.ToolCallID,
				"content":     m.Content,
			})

		case "assistant":
			pendingToolResults = flushToolResults(pendingToolResults)
			var content []map[string]any
			if m.Content != "" {
				content = append(content, map[string]any{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := tc.Arguments
				if input == nil { input = map[string]any{} }
				content = append(content, map[string]any{
					"type": "tool_use", "id": tc.ID, "name": tc.Name, "input": input,
				})
			}
			// An assistant message must have at least one block — if the
			// upstream produced an empty assistant turn, skip it rather
			// than sending an invalid empty content array.
			if len(content) == 0 { continue }
			msgs = append(msgs, map[string]any{"role": "assistant", "content": content})

		default: // "user" (and anything else we don't explicitly model)
			pendingToolResults = flushToolResults(pendingToolResults)
			msgs = append(msgs, map[string]any{"role": "user", "content": m.Content})
		}
	}
	// Final flush: a trailing tool message should produce a user turn the
	// model can respond to. Claude will otherwise reject the request.
	pendingToolResults = flushToolResults(pendingToolResults)
	_ = pendingToolResults

	body := map[string]any{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens": 4096, "messages": msgs,
	}
	if system != "" { body["system"] = system }
	if temp, ok := req.Options["temperature"]; ok { body["temperature"] = temp }
	if thinking, ok := req.Options["thinking"]; ok { body["thinking"] = thinking }
	if len(req.Tools) > 0 {
		var tools []map[string]any
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"name": t.Function.Name, "description": t.Function.Description,
				"input_schema": t.Function.Parameters,
			})
		}
		body["tools"] = tools
	}
	return body
}

func (p *BedrockProvider) parseResponse(data []byte, model string, elapsed time.Duration) (*ChatResponse, error) {
	if isAnthropicModel(model) {
		return p.parseAnthropicResponse(data, elapsed)
	}
	return p.parseOpenAIResponse(data, elapsed)
}

// parseOpenAIResponse — for Nemotron, Qwen, DeepSeek, Kimi, GLM, MiniMax
func (p *BedrockProvider) parseOpenAIResponse(data []byte, elapsed time.Duration) (*ChatResponse, error) {
	var result struct {
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
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("bedrock parse: %w", err)
	}
	resp := &ChatResponse{
		Usage: &Usage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		},
	}
	if len(result.Choices) > 0 {
		c := result.Choices[0]
		resp.Content = c.Message.Content
		resp.FinishReason = c.FinishReason
		for _, tc := range c.Message.ToolCalls {
			args, ok := parseToolArgsString(tc.Function.Arguments, tc.Function.Name)
			call := ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: args}
			if !ok {
				call.ArgsParseError = "arguments JSON invalid or truncated"
			}
			resp.ToolCalls = append(resp.ToolCalls, call)
		}
	}
	slog.Debug("bedrock.chat", "model", result.Model, "in", result.Usage.PromptTokens,
		"out", result.Usage.CompletionTokens, "elapsed", elapsed)
	return resp, nil
}

// parseAnthropicResponse — for Claude models
func (p *BedrockProvider) parseAnthropicResponse(data []byte, elapsed time.Duration) (*ChatResponse, error) {
	var result struct {
		Content []struct {
			Type  string `json:"type"`
			Text  string `json:"text,omitempty"`
			ID    string `json:"id,omitempty"`
			Name  string `json:"name,omitempty"`
			Input any    `json:"input,omitempty"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		StopReason string `json:"stop_reason"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("bedrock parse: %w", err)
	}
	resp := &ChatResponse{
		Usage:        &Usage{PromptTokens: result.Usage.InputTokens, CompletionTokens: result.Usage.OutputTokens, TotalTokens: result.Usage.InputTokens + result.Usage.OutputTokens},
		FinishReason: result.StopReason,
	}
	for _, c := range result.Content {
		switch c.Type {
		case "text":
			resp.Content += c.Text
		case "tool_use":
			args, _ := c.Input.(map[string]any)
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{ID: c.ID, Name: c.Name, Arguments: args})
		}
	}
	slog.Debug("bedrock.chat", "model", p.modelID, "in", result.Usage.InputTokens, "out", result.Usage.OutputTokens, "elapsed", elapsed)
	return resp, nil
}
