// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/tools"
)

func (l *Loop) ChatStream(ctx context.Context, agentID, message string, onDelta func(delta string)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	ag, err := l.agentStore.Get(ctx, agentID)
	if err != nil {
		return "", err
	}

	provider := l.resolveProvider(ag)
	if provider == nil {
		return "", errNoProvider
	}

	rc := RuntimeContext{Mode: PromptFull, Channel: "room", TriggerBy: "mention", NoTools: true}
	pb := NewPromptBuilder(ag, rc)
	if l.agentStore != nil {
		if roster, err := l.agentStore.List(ctx, l.tenantID); err == nil {
			pb.SetTeam(roster)
		}
	}
	if l.skillStore != nil {
		pb.SetSkillStore(l.skillStore)
	}
	systemPrompt := pb.Build()

	var fullText strings.Builder
	_, err = provider.ChatStream(ctx, providers.ChatRequest{
		Model: ag.Model,
		Messages: []providers.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: message},
		},
		Options: map[string]any{"temperature": 0.5, "max_tokens": 4000},
	}, func(chunk providers.StreamChunk) {
		if chunk.Content != "" {
			fullText.WriteString(chunk.Content)
			onDelta(chunk.Content)
		}
	})
	if err != nil {
		return fullText.String(), err
	}

	result := fullText.String()
	stripped := stripHallucinatedToolCalls(result)
	if stripped == "" && result != "" {
		slog.Warn("agent.loop.stripped_hallucination", "original_len", len(result), "preview", result[:min(len(result), 100)])
		// Don't return empty — return the original with XML tags removed
		stripped = strings.TrimSpace(stripped)
	}
	return stripped, nil
}

func (l *Loop) Chat(ctx context.Context, agentID, message string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return l.chatInternal(ctx, agentID, message, nil)
}

func (l *Loop) ChatWithEnv(ctx context.Context, agentID, message string, env *EnvironmentPayload) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// If delivery is NOT group_chat, use the full agent loop (needs tools for DM/email/etc)
	if env != nil && env.DeliveryChannel != "group_chat" {
		result, err := l.Run(ctx, RunRequest{
			AgentID: agentID, UserMessage: message, Channel: "room",
		}, func(event StreamEvent) {})
		if err != nil {
			return "", err
		}
		if result != nil {
			return result.Content, nil
		}
		return "", nil
	}

	return l.chatInternal(ctx, agentID, message, env)
}

func (l *Loop) chatInternal(ctx context.Context, agentID, message string, env *EnvironmentPayload) (string, error) {
	ag, err := l.agentStore.Get(ctx, agentID)
	if err != nil {
		return "", err
	}

	provider := l.resolveProvider(ag)
	if provider == nil {
		return "", errNoProvider
	}

	// Build system prompt via PromptBuilder
	rc := RuntimeContext{Mode: PromptFull, Channel: "room", TriggerBy: "mention", Environment: env, NoTools: true}
	pb := NewPromptBuilder(ag, rc)
	if l.agentStore != nil {
		if roster, err := l.agentStore.List(ctx, l.tenantID); err == nil {
			pb.SetTeam(roster)
		}
	}
	if l.skillStore != nil {
		pb.SetSkillStore(l.skillStore)
	}
	systemPrompt := pb.Build()

	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Model: ag.Model,
		Messages: []providers.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: message},
		},
		Options: map[string]any{"temperature": 0.5, "max_tokens": 4000},
	})
	if err != nil {
		return "", err
	}

	// Sanitize: strip hallucinated tool XML tags from response
	content := resp.Content
	content = stripHallucinatedToolCalls(content)

	// If hallucinated tool calls were stripped, retry with explicit instruction
	if content == "" && resp.Content != "" {
		slog.Info("agent.chat.hallucination_retry", "agent", agentID)
		retryResp, retryErr := provider.Chat(ctx, providers.ChatRequest{
			Model: ag.Model,
			Messages: []providers.Message{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: message},
				{Role: "assistant", Content: resp.Content},
				{Role: "user", Content: "That response contained XML tool tags which are not valid here. You have NO tools. Please write your complete answer as plain text. Write the actual content the user asked for."},
			},
			Options: map[string]any{"temperature": 0.7, "max_tokens": 4000},
		})
		if retryErr == nil && retryResp.Content != "" {
			content = stripHallucinatedToolCalls(retryResp.Content)
		}
	}

	return content, nil
}

func stripHallucinatedToolCalls(s string) string {
	// Common hallucinated patterns
	patterns := []string{
		`<tool>`, `</tool>`, `<soul>`, `</soul>`, `<task>`, `</task>`,
		`<think>`, `</think>`, `<reasoning>`, `</reasoning>`,
		`<reflection>`, `</reflection>`, `<inner_monologue>`, `</inner_monologue>`,
		`<function_call>`, `</function_call>`, `<tool_call>`, `</tool_call>`,
		`<tool_use>`, `</tool_use>`, `<parameter`, `</parameter>`,
	}
	lower := strings.ToLower(s)
	hasXML := false
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			hasXML = true
			break
		}
	}
	if !hasXML {
		return s
	}

	// If the entire response is tool XML, it's a hallucination — return empty
	// so the caller can handle it
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "<tool>") || strings.HasPrefix(trimmed, "<function_call>") || strings.HasPrefix(trimmed, "<tool_call>") {
		return ""
	}
	return s
}

func (l *Loop) RunReAct(ctx context.Context, req RunRequest, onEvent func(StreamEvent), maxIter int) (*RunResult, error) {
	if maxIter <= 0 {
		maxIter = 10
	}

	ag, err := l.agentStore.Get(ctx, req.AgentID)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	provider := l.resolveProvider(ag)
	model := req.Model
	if model == "" {
		model = ag.Model
	}

	// Build tool descriptions for the prompt
	cb := NewContextBuilder(ag, l.skillLoader, l.memStore, l.toolReg)
	toolDefs := cb.BuildToolDefs()
	var toolDesc strings.Builder
	for _, t := range toolDefs {
		toolDesc.WriteString(fmt.Sprintf("- %s: %s\n", t.Function.Name, t.Function.Description))
	}

	reactPrompt := fmt.Sprintf(`%s

You have access to these tools:
%s
Use this format for EVERY step:

Thought: <reasoning about what to do>
Action: <tool_name>
Action Input: <JSON arguments>

When done:
Thought: I have enough information.
Final Answer: <your complete answer>

Begin!`, ag.SystemPrompt, toolDesc.String())

	messages := []providers.Message{{Role: "system", Content: reactPrompt}}

	// Load session history
	if req.SessionID != "" {
		sess, _ := l.sessionStore.GetByID(ctx, req.SessionID)
		if sess != nil {
			var history []struct{ Role, Content string }
			json.Unmarshal(sess.Messages, &history)
			for _, m := range history {
				messages = append(messages, providers.Message{Role: m.Role, Content: m.Content})
			}
		}
	}
	messages = append(messages, providers.Message{Role: "user", Content: req.UserMessage})

	var allToolsUsed []string
	var totalIn, totalOut int
	var thinking strings.Builder

	var lastLLMContent string
	_ = lastLLMContent
	for iter := 0; iter < maxIter; iter++ {
		onEvent(ThinkingDelta(fmt.Sprintf("ReAct iteration %d...", iter+1)))

		resp, err := provider.Chat(ctx, providers.ChatRequest{
			Model: model, Messages: messages,
		})
		if err != nil {
			return nil, fmt.Errorf("llm error iter %d: %w", iter, err)
		}

		totalIn += resp.Usage.PromptTokens
		totalOut += resp.Usage.CompletionTokens
		content := strings.TrimSpace(resp.Content)
		thinking.WriteString(fmt.Sprintf("--- Iteration %d ---\n%s\n\n", iter+1, content))

		// Check for Final Answer
		if idx := strings.Index(content, "Final Answer:"); idx >= 0 {
			answer := strings.TrimSpace(content[idx+len("Final Answer:"):])
			onEvent(TextDelta(answer))
			return &RunResult{
				Content: answer, Thinking: thinking.String(),
				ToolsUsed: allToolsUsed, InputTokens: totalIn, OutputTokens: totalOut,
			}, nil
		}

		// Parse Action + Action Input
		actionIdx := strings.Index(content, "Action:")
		inputIdx := strings.Index(content, "Action Input:")
		if actionIdx < 0 || inputIdx < 0 {
			onEvent(TextDelta(content))
			return &RunResult{
				Content: content, Thinking: thinking.String(),
				ToolsUsed: allToolsUsed, InputTokens: totalIn, OutputTokens: totalOut,
			}, nil
		}

		toolName := strings.TrimSpace(content[actionIdx+len("Action:") : inputIdx])
		argsStr := strings.TrimSpace(content[inputIdx+len("Action Input:"):])

		var args map[string]any
		json.Unmarshal([]byte(argsStr), &args)
		if args == nil {
			args = map[string]any{"input": argsStr}
		}

		onEvent(ToolStart(toolName))
		slog.Info("react.tool_call", "agent", ag.AgentKey, "tool", toolName, "iter", iter)
		allToolsUsed = append(allToolsUsed, toolName)

		toolCtx := tools.WithWorkspace(ctx, "/tmp/qorven-workspace")
		toolCtx = tools.WithAgentID(toolCtx, ag.ID)
		result := l.executeTool(toolCtx, req, toolName, args)
		onEvent(ToolResult(toolName, result.ForLLM))

		messages = append(messages,
			providers.Message{Role: "assistant", Content: content},
			providers.Message{Role: "user", Content: fmt.Sprintf("Observation: %s", result.ForLLM)},
		)
	}

	return &RunResult{
		Content: "ReAct loop reached max iterations.", Thinking: thinking.String(),
		ToolsUsed: allToolsUsed, InputTokens: totalIn, OutputTokens: totalOut,
	}, nil
}
