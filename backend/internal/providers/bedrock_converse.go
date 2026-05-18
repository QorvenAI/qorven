// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brdoc "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// TypeBedrockConverse and TypeBedrockMantle are defined in types.go.

// BedrockConverseDriver calls Amazon Bedrock via the Converse / ConverseStream API.
type BedrockConverseDriver struct {
	client        *bedrockruntime.Client
	providerName  string
	forceStreaming bool // true for bedrock_mantle
}

// NewBedrockConverseDriver creates a BedrockConverseDriver.
// cfg.APIBase holds the AWS region (default "us-east-1").
func NewBedrockConverseDriver(cfg ProviderConfig, forceStreaming bool) (*BedrockConverseDriver, error) {
	region := cfg.APIBase
	if region == "" {
		region = "us-east-1"
	}
	// If caller passed a full URL instead of a bare region name, extract just the region.
	if strings.HasPrefix(region, "http") {
		region = "us-east-1"
	}

	var opts []func(*awsconfig.LoadOptions) error
	opts = append(opts, awsconfig.WithRegion(region))
	if cfg.AWSAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AWSAccessKey, cfg.AWSSecretKey, cfg.AWSSessionToken),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("bedrock_converse: aws config: %w", err)
	}

	return &BedrockConverseDriver{
		client:        bedrockruntime.NewFromConfig(awsCfg),
		providerName:  cfg.Name,
		forceStreaming: forceStreaming,
	}, nil
}

func (d *BedrockConverseDriver) Name() string         { return d.providerName }
func (d *BedrockConverseDriver) DefaultModel() string { return "us.anthropic.claude-sonnet-4-6" }

// ---------- message conversion ----------

func (d *BedrockConverseDriver) buildMessages(req ChatRequest) ([]brtypes.Message, []brtypes.SystemContentBlock) {
	var msgs []brtypes.Message
	var system []brtypes.SystemContentBlock

	// Buffer consecutive tool-result messages so they merge into one user turn.
	var pendingToolResults []brtypes.ContentBlock

	flushToolResults := func() {
		if len(pendingToolResults) > 0 {
			msgs = append(msgs, brtypes.Message{
				Role:    brtypes.ConversationRoleUser,
				Content: pendingToolResults,
			})
			pendingToolResults = nil
		}
	}

	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			system = append(system, &brtypes.SystemContentBlockMemberText{Value: m.Content})

		case "tool":
			// Tool result → ContentBlockMemberToolResult
			pendingToolResults = append(pendingToolResults, &brtypes.ContentBlockMemberToolResult{
				Value: brtypes.ToolResultBlock{
					ToolUseId: aws.String(m.ToolCallID),
					Content: []brtypes.ToolResultContentBlock{
						&brtypes.ToolResultContentBlockMemberText{Value: m.Content},
					},
				},
			})

		case "assistant":
			flushToolResults()
			var content []brtypes.ContentBlock
			if m.Content != "" {
				content = append(content, &brtypes.ContentBlockMemberText{Value: m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := tc.Arguments
				if input == nil {
					input = map[string]any{}
				}
				content = append(content, &brtypes.ContentBlockMemberToolUse{
					Value: brtypes.ToolUseBlock{
						ToolUseId: aws.String(tc.ID),
						Name:      aws.String(tc.Name),
						Input:     brdoc.NewLazyDocument(input),
					},
				})
			}
			if len(content) == 0 {
				continue
			}
			msgs = append(msgs, brtypes.Message{Role: brtypes.ConversationRoleAssistant, Content: content})

		default: // "user"
			flushToolResults()
			msgs = append(msgs, brtypes.Message{
				Role:    brtypes.ConversationRoleUser,
				Content: []brtypes.ContentBlock{&brtypes.ContentBlockMemberText{Value: m.Content}},
			})
		}
	}
	flushToolResults()
	return msgs, system
}

func (d *BedrockConverseDriver) buildInferenceConfig(req ChatRequest) *brtypes.InferenceConfiguration {
	cfg := &brtypes.InferenceConfiguration{MaxTokens: aws.Int32(4096)}
	if t, ok := req.Options["temperature"]; ok {
		if f, ok2 := toFloat32(t); ok2 {
			cfg.Temperature = aws.Float32(f)
		}
	}
	return cfg
}

func (d *BedrockConverseDriver) buildToolConfig(req ChatRequest) *brtypes.ToolConfiguration {
	if len(req.Tools) == 0 {
		return nil
	}
	var tools []brtypes.Tool
	for _, t := range req.Tools {
		tools = append(tools, &brtypes.ToolMemberToolSpec{
			Value: brtypes.ToolSpecification{
				Name:        aws.String(t.Function.Name),
				Description: aws.String(t.Function.Description),
				InputSchema: &brtypes.ToolInputSchemaMemberJson{
					Value: brdoc.NewLazyDocument(t.Function.Parameters),
				},
			},
		})
	}
	return &brtypes.ToolConfiguration{Tools: tools}
}

// ---------- Chat (non-streaming) ----------

func (d *BedrockConverseDriver) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if d.forceStreaming {
		var resp *ChatResponse
		var err error
		_, err = d.ChatStream(ctx, req, func(chunk StreamChunk) {
			if resp == nil {
				resp = &ChatResponse{}
			}
			resp.Content += chunk.Content
		})
		return resp, err
	}

	msgs, system := d.buildMessages(req)
	input := &bedrockruntime.ConverseInput{
		ModelId:            aws.String(req.Model),
		Messages:           msgs,
		System:             system,
		InferenceConfig:    d.buildInferenceConfig(req),
		ToolConfig:         d.buildToolConfig(req),
	}

	out, err := d.client.Converse(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("bedrock_converse: %w", err)
	}

	resp := &ChatResponse{FinishReason: string(out.StopReason)}
	if out.Usage != nil {
		resp.Usage = &Usage{
			PromptTokens:     int(aws.ToInt32(out.Usage.InputTokens)),
			CompletionTokens: int(aws.ToInt32(out.Usage.OutputTokens)),
			TotalTokens:      int(aws.ToInt32(out.Usage.TotalTokens)),
		}
	}

	if msg, ok := out.Output.(*brtypes.ConverseOutputMemberMessage); ok {
		for _, block := range msg.Value.Content {
			switch b := block.(type) {
			case *brtypes.ContentBlockMemberText:
				resp.Content += b.Value
			case *brtypes.ContentBlockMemberToolUse:
				tc := ToolCall{
					ID:   aws.ToString(b.Value.ToolUseId),
					Name: aws.ToString(b.Value.Name),
				}
				if b.Value.Input != nil {
					raw, err := json.Marshal(b.Value.Input)
					if err != nil {
						tc.ArgsParseError = fmt.Sprintf("bedrock_converse: marshal tool input: %v", err)
					} else if err := json.Unmarshal(raw, &tc.Arguments); err != nil {
						tc.ArgsParseError = fmt.Sprintf("bedrock_converse: unmarshal tool input: %v", err)
					}
				}
				resp.ToolCalls = append(resp.ToolCalls, tc)
			}
		}
	}

	if len(resp.ToolCalls) > 0 {
		resp.FinishReason = "tool_calls"
	}
	return resp, nil
}

// ---------- ChatStream ----------

func (d *BedrockConverseDriver) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	msgs, system := d.buildMessages(req)
	input := &bedrockruntime.ConverseStreamInput{
		ModelId:         aws.String(req.Model),
		Messages:        msgs,
		System:          system,
		InferenceConfig: d.buildInferenceConfig(req),
		ToolConfig:      d.buildToolConfig(req),
	}

	stream, err := d.client.ConverseStream(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("bedrock_converse stream: %w", err)
	}
	defer stream.GetStream().Close()

	resp := &ChatResponse{}

	// Track in-progress tool use by block index.
	type pendingTool struct {
		id   string
		name string
		args strings.Builder
	}
	toolsByBlock := map[int32]*pendingTool{}

	for event := range stream.GetStream().Events() {
		switch ev := event.(type) {

		case *brtypes.ConverseStreamOutputMemberContentBlockStart:
			if tu, ok := ev.Value.Start.(*brtypes.ContentBlockStartMemberToolUse); ok {
				idx := aws.ToInt32(ev.Value.ContentBlockIndex)
				toolsByBlock[idx] = &pendingTool{
					id:   aws.ToString(tu.Value.ToolUseId),
					name: aws.ToString(tu.Value.Name),
				}
			}

		case *brtypes.ConverseStreamOutputMemberContentBlockDelta:
			idx := aws.ToInt32(ev.Value.ContentBlockIndex)
			switch delta := ev.Value.Delta.(type) {
			case *brtypes.ContentBlockDeltaMemberText:
				resp.Content += delta.Value
				onChunk(StreamChunk{Content: delta.Value})
			case *brtypes.ContentBlockDeltaMemberToolUse:
				if pt, ok := toolsByBlock[idx]; ok && delta.Value.Input != nil {
					pt.args.WriteString(*delta.Value.Input)
				}
			}

		case *brtypes.ConverseStreamOutputMemberContentBlockStop:
			idx := aws.ToInt32(ev.Value.ContentBlockIndex)
			if pt, ok := toolsByBlock[idx]; ok {
				var args map[string]any
				tc := ToolCall{ID: pt.id, Name: pt.name}
				if raw := pt.args.String(); raw == "" {
					tc.Arguments = map[string]any{} // no-arg tool call — explicit empty, not nil
				} else if err := json.Unmarshal([]byte(raw), &args); err != nil {
					tc.ArgsParseError = fmt.Sprintf("bedrock_converse: truncated tool args: %v", err)
				} else {
					tc.Arguments = args
				}
				resp.ToolCalls = append(resp.ToolCalls, tc)
				delete(toolsByBlock, idx)
			}

		case *brtypes.ConverseStreamOutputMemberMessageStop:
			resp.FinishReason = string(ev.Value.StopReason)

		case *brtypes.ConverseStreamOutputMemberMetadata:
			if ev.Value.Usage != nil {
				resp.Usage = &Usage{
					PromptTokens:     int(aws.ToInt32(ev.Value.Usage.InputTokens)),
					CompletionTokens: int(aws.ToInt32(ev.Value.Usage.OutputTokens)),
					TotalTokens:      int(aws.ToInt32(ev.Value.Usage.TotalTokens)),
				}
			}
		}
	}

	// Flush any tool-use blocks that never received ContentBlockStop (stream cancelled
	// mid-response). Mark them with ArgsParseError so the agent loop can signal failure
	// instead of silently executing a tool call with nil arguments.
	for _, pt := range toolsByBlock {
		resp.ToolCalls = append(resp.ToolCalls, ToolCall{
			ID:             pt.id,
			Name:           pt.name,
			ArgsParseError: "bedrock_converse: stream cancelled before ContentBlockStop",
		})
	}

	if err := stream.GetStream().Err(); err != nil {
		return nil, fmt.Errorf("bedrock_converse stream read: %w", err)
	}

	if len(resp.ToolCalls) > 0 {
		resp.FinishReason = "tool_calls"
	}
	return resp, nil
}

// ---------- helpers ----------

func toFloat32(v any) (float32, bool) {
	switch f := v.(type) {
	case float64:
		return float32(f), true
	case float32:
		return f, true
	case int:
		return float32(f), true
	}
	return 0, false
}
