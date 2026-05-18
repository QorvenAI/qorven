// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	cronpkg "github.com/qorvenai/qorven/internal/cron"
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/council"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/session"
	"github.com/qorvenai/qorven/internal/tools"
)

func (gw *Gateway) interceptCommand(ctx context.Context, agentID, sessionID, userMsg string) (bool, string) {
	if gw.db == nil {
		return false, ""
	}
	msg := strings.TrimSpace(userMsg)

	// 0. Scheduling request → create cron job deterministically
	if agent.IsSchedulingRequest(msg) {
		if expr, task, ok := agent.ParseSchedule(msg); ok {
			slog.Info("dm.scheduling_deterministic", "agent", agentID, "expr", expr, "task", task)
			var jobID string
			delivery := agent.DetectDeliveryChannel(msg)
			if delivery == "group_chat" {
				delivery = "dm"
			}
			payloadJSON, _ := json.Marshal(map[string]string{"instruction": task, "executor_agent_id": agentID})
			gw.db.Pool.QueryRow(ctx,
				`INSERT INTO cron_jobs (tenant_id, agent_id, name, cron_expression, payload, executor_agent_id, delivery_channel, enabled)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, true) RETURNING id`,
				defaultTenant, agentID, "scheduled_task", expr, payloadJSON, agentID, delivery).Scan(&jobID)
			// Register with in-memory scheduler so the job actually fires
			if jobID != "" && tools.OnCronSchedule != nil {
				tools.OnCronSchedule(ctx, defaultTenant, agentID, jobID, "scheduled_task", expr, task)
			}
			humanTime := cronpkg.FormatNextRunExpr(expr)
			confirm := fmt.Sprintf("✅ Done! I'll send you %s, %s.", task, humanTime)
			gw.saveCommandToSession(ctx, sessionID, agentID, userMsg, confirm)
			return true, confirm
		}
	}

	// 1. @mention → delegate_to_soul directly (skip if it looks like an email address)
	if m := cmdMentionRe.FindStringSubmatch(msg); len(m) == 3 {
		soulKey := m[1]
		task := strings.TrimSpace(m[2])
		if !isEmailLike(soulKey) && soulKey != "prime" && gw.soulDesk != nil {
			toolCtx := tools.WithAgentID(ctx, agentID)
			toolCtx = tools.WithSessionID(toolCtx, sessionID)

			// Include recent conversation context so the delegated Qor knows what "this" refers to
			var recentCtx string
			if sessionID != "" {
				rows, err := gw.db.Pool.Query(ctx,
					`SELECT role, content FROM agent_messages WHERE session_id = $1 ORDER BY created_at DESC LIMIT 5`, sessionID)
				if err == nil {
					msgs := []string{}
					for rows.Next() {
						var role, content string
						rows.Scan(&role, &content)
						msgs = append([]string{fmt.Sprintf("[%s] %s", role, content)}, msgs...)
					}
					rows.Close()
					for _, m := range msgs {
						recentCtx += m + "\n"
					}
				}
			}

			fullTask := task
			if recentCtx != "" {
				fullTask = fmt.Sprintf("Context from conversation:\n%s\nTask: %s", recentCtx, task)
			}

			result := gw.toolReg.Execute(toolCtx, "delegate_to_soul", map[string]any{
				"soul_key": soulKey,
				"task":     fullTask,
			})
			slog.Info("command.intercept.mention", "soul", soulKey, "task", task[:min(len(task), 80)])

			// Save to session so the current Qor sees what happened
			response := fmt.Sprintf("📋 Delegated to @%s: %s\n\n%s", soulKey, task, result.ForLLM)
			gw.saveCommandToSession(ctx, sessionID, agentID, userMsg, response)
			return true, response
		}
	}

	// 2. /command → execute tool directly
	if m := cmdSlashRe.FindStringSubmatch(msg); len(m) >= 2 {
		toolName := m[1]
		toolArgs := ""
		if len(m) > 2 {
			toolArgs = strings.TrimSpace(m[2])
		}
		if _, ok := gw.toolReg.Get(toolName); ok {
			toolCtx := tools.WithAgentID(ctx, agentID)
			toolCtx = tools.WithSessionID(toolCtx, sessionID)
			toolCtx = tools.WithWorkspace(toolCtx, "/tmp/qorven-workspace")
			// Parse args: if JSON, use as-is; otherwise pass as "input"
			args := map[string]any{}
			if toolArgs != "" {
				if err := json.Unmarshal([]byte(toolArgs), &args); err != nil {
					args = map[string]any{"input": toolArgs, "command": toolArgs, "query": toolArgs}
				}
			}
			result := gw.toolReg.Execute(toolCtx, toolName, args)
			slog.Info("command.intercept.slash", "tool", toolName, "args_len", len(toolArgs))
			gw.saveCommandToSession(ctx, sessionID, agentID, userMsg, result.ForLLM)
			return true, result.ForLLM
		}
	}

	return false, ""
}

func (gw *Gateway) saveCommandToSession(ctx context.Context, sessionID, agentID, userMsg, response string) {
	if gw.sessions == nil || sessionID == "" {
		return
	}
	gw.sessions.AppendMessage(ctx, sessionID, session.Message{
		Role: "user", Content: userMsg, Timestamp: time.Now().Unix(),
	}, 0, 0)
	gw.sessions.AppendMessage(ctx, sessionID, session.Message{
		Role: "assistant", Content: response, Timestamp: time.Now().Unix(),
	}, 0, 0)
	if gw.rtHub != nil {
		gw.rtHub.BroadcastNewMessage(sessionID, agentID, "assistant", response)
	}
}

// pendingDelegation holds a deferred @soul task waiting for user confirmation.
type pendingDelegation struct {
	SoulKey string
	Task    string
}

// handleManualDelegation implements the two-phase confirmation flow for manual
// delegation mode. Returns true if the request was fully handled (either held
// for confirmation or resolved from a pending confirmation), false otherwise.
func (gw *Gateway) handleManualDelegation(ctx context.Context, w http.ResponseWriter, agentID, sessionID, model, userMsg string) bool {
	normalized := strings.TrimSpace(strings.ToLower(userMsg))

	// Phase 2: user replied to a pending confirmation.
	if v, ok := gw.pendingDelegations.Load(sessionID); ok {
		pending := v.(pendingDelegation)
		switch normalized {
		case "yes", "y", "confirm", "ok", "proceed", "go", "do it":
			gw.pendingDelegations.Delete(sessionID)
			toolCtx := tools.WithAgentID(ctx, agentID)
			toolCtx = tools.WithSessionID(toolCtx, sessionID)
			result := gw.toolReg.Execute(toolCtx, "delegate_to_soul", map[string]any{
				"soul_key": pending.SoulKey,
				"task":     pending.Task,
			})
			response := fmt.Sprintf("Delegating to @%s...\n\n%s", pending.SoulKey, result.ForLLM)
			gw.saveCommandToSession(ctx, sessionID, agentID, userMsg, response)
			writeJSON(w, 200, map[string]any{
				"object": "chat.completion", "model": model,
				"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": response}, "finish_reason": "stop"}},
			})
			return true
		case "no", "n", "cancel", "abort", "stop", "nope":
			gw.pendingDelegations.Delete(sessionID)
			response := fmt.Sprintf("Cancelled. Task to @%s was not delegated.", pending.SoulKey)
			gw.saveCommandToSession(ctx, sessionID, agentID, userMsg, response)
			writeJSON(w, 200, map[string]any{
				"object": "chat.completion", "model": model,
				"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": response}, "finish_reason": "stop"}},
			})
			return true
		default:
			// User is editing — replace the pending task with the new message if it's another @mention.
			if m := cmdMentionRe.FindStringSubmatch(strings.TrimSpace(userMsg)); len(m) == 3 {
				soulKey := m[1]
				task := strings.TrimSpace(m[2])
				if !isEmailLike(soulKey) {
					gw.pendingDelegations.Store(sessionID, pendingDelegation{SoulKey: soulKey, Task: task})
					response := fmt.Sprintf("Updated. Confirm to delegate to @%s:\n\n> %s\n\nReply `yes` to proceed or `no` to cancel.", soulKey, task)
					gw.saveCommandToSession(ctx, sessionID, agentID, userMsg, response)
					writeJSON(w, 200, map[string]any{
						"object": "chat.completion", "model": model,
						"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": response}, "finish_reason": "stop"}},
					})
					return true
				}
			}
			// Not a confirmation and not a new @mention — clear pending and let it fall through to the agent.
			gw.pendingDelegations.Delete(sessionID)
		}
	}

	// Phase 1: intercept a new @mention and hold it for confirmation.
	if m := cmdMentionRe.FindStringSubmatch(strings.TrimSpace(userMsg)); len(m) == 3 {
		soulKey := m[1]
		task := strings.TrimSpace(m[2])
		if !isEmailLike(soulKey) {
			gw.pendingDelegations.Store(sessionID, pendingDelegation{SoulKey: soulKey, Task: task})
			response := fmt.Sprintf("Manual delegation mode — confirm to delegate to @%s:\n\n> %s\n\nReply `yes` to proceed or `no` to cancel.", soulKey, task)
			gw.saveCommandToSession(ctx, sessionID, agentID, userMsg, response)
			writeJSON(w, 200, map[string]any{
				"object": "chat.completion", "model": model,
				"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": response}, "finish_reason": "stop"}},
			})
			return true
		}
	}

	return false
}

func isEmailLike(s string) bool {
	return strings.Contains(s, ".") && (strings.HasSuffix(s, ".com") || strings.HasSuffix(s, ".io") ||
		strings.HasSuffix(s, ".org") || strings.HasSuffix(s, ".net") || strings.HasSuffix(s, ".ai"))
}

func (gw *Gateway) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model         string                     `json:"model"`
		Messages      []providers.Message        `json:"messages"`
		Stream        bool                       `json:"stream"`
		Tools         []providers.ToolDefinition `json:"tools,omitempty"`
		AgentID        string                     `json:"agent_id,omitempty"`
		SessionID      string                     `json:"session_id,omitempty"`
		Channel        string                     `json:"channel,omitempty"`
		Depth          string                     `json:"depth,omitempty"`
		Message        string                     `json:"message,omitempty"`
		ThinkingLevel  string                     `json:"thinking_level,omitempty"`
		DelegationMode string                     `json:"delegation_mode,omitempty"` // "auto", "explicit", "manual"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}

	// Normalize: if 'message' provided but 'messages' empty, convert
	if req.Message != "" && len(req.Messages) == 0 {
		req.Messages = []providers.Message{{Role: "user", Content: req.Message}}
	}

	// Validate: reject empty messages early (don't send to provider)
	if len(req.Messages) == 0 {
		writeJSON(w, 400, map[string]string{"error": "messages array is required and must not be empty"})
		return
	}

	// Validate: reject obviously invalid model names early
	if req.Model != "" && (len(req.Model) > 100 || strings.ContainsAny(req.Model, " \t\n<>")) {
		writeJSON(w, 400, map[string]string{"error": "invalid model name"})
		return
	}

	// If agent_id provided, use the full agent loop (tools, memory, skills)
	// Intercept @mentions and /commands — execute directly, skip LLM
	if req.AgentID != "" && gw.agentLoop != nil {
		agentID := req.AgentID
		userMsg := req.Message
		if userMsg == "" && len(req.Messages) > 0 {
			userMsg = req.Messages[len(req.Messages)-1].Content
		}

		// manual delegation mode: two-phase confirmation before delegating.
		if req.DelegationMode == "manual" {
			if handled := gw.handleManualDelegation(r.Context(), w, agentID, req.SessionID, req.Model, userMsg); handled {
				return
			}
		}

		// Command interceptor: @mention → delegate, /command → tool exec
		if handled, response := gw.interceptCommand(r.Context(), agentID, req.SessionID, userMsg); handled {
			if req.Stream {
				flusher, ok := w.(http.Flusher)
				if !ok {
					writeJSON(w, 500, map[string]string{"error": "streaming not supported"})
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Connection", "keep-alive")
				w.WriteHeader(200)
				data, _ := json.Marshal(map[string]any{
					"object": "chat.completion.chunk",
					"choices": []map[string]any{{
						"index": 0, "delta": map[string]any{"content": response},
					}},
				})
				fmt.Fprintf(w, "data: %s\n\n", data)
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
			} else {
				writeJSON(w, 200, map[string]any{
					"object": "chat.completion", "model": req.Model,
					"choices": []map[string]any{{
						"index": 0, "message": map[string]any{"role": "assistant", "content": response},
						"finish_reason": "stop",
					}},
				})
			}
			return
		}

		gw.handleAgentChat(w, r, agentID, req.SessionID, req.Model, req.Messages, req.Stream, req.Depth, req.Channel, req.ThinkingLevel, req.DelegationMode)
		return
	}

	// Otherwise: direct provider passthrough — route by model name when possible.
	var p providers.Provider
	if req.Model != "" {
		p = gw.providerReg.ProviderForModel(req.Model)
	}
	if p == nil {
		p = gw.providerReg.Default()
	}
	if p == nil {
		writeJSON(w, 503, map[string]string{"error": "no providers configured"})
		return
	}

	chatReq := providers.ChatRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Tools:    req.Tools,
	}

	if req.Stream {
		gw.streamChat(w, r, p, chatReq)
		return
	}

	resp, err := p.Chat(r.Context(), chatReq)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]any{
		"object": "chat.completion", "model": req.Model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": resp.Content, "tool_calls": formatToolCalls(resp.ToolCalls)},
			"finish_reason": resp.FinishReason,
		}},
		"usage": resp.Usage,
	})
}

func (gw *Gateway) handleAgentChat(w http.ResponseWriter, r *http.Request, agentID, sessionID, model string, messages []providers.Message, stream bool, depthParam, channel, thinkingLevel, delegationMode string) {
	// Extract last user message
	userMsg := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			userMsg = messages[i].Content
			break
		}
	}
	if userMsg == "" {
		writeJSON(w, 400, map[string]string{"error": "no user message found"})
		return
	}
	if gw.agentLoop == nil {
		writeJSON(w, 503, map[string]string{"error": "agent loop not initialized"})
		return
	}

	// Ensure session exists for this key — create if missing
	// AppendMessage handles both UUID and session_key lookups
	if gw.sessions != nil && sessionID != "" {
		if _, err := gw.sessions.Get(r.Context(), sessionID); err != nil {
			// Resolve agent_key → agent UUID for the FK constraint
			agUUID := agentID
			if gw.agents != nil {
				if ags, listErr := gw.agents.List(r.Context(), defaultTenant); listErr == nil {
					for _, a := range ags {
						if a.AgentKey == agentID || a.ID == agentID {
							agUUID = a.ID
							break
						}
					}
				}
			}
			ch := channel
			if ch == "" {
				ch = "web"
			}
			gw.sessions.CreateWithKey(r.Context(), defaultTenant, agUUID, "operator", ch, sessionID) //nolint:errcheck
		}
	}

	var webUserID string
	if u := userFromContext(r.Context()); u != nil {
		webUserID = u.ID
	}

	req := agent.RunRequest{
		AgentID:        agentID,
		SessionID:      sessionID,
		UserMessage:    userMsg,
		Model:          model,
		Channel:        channel,
		ThinkingLevel:  thinkingLevel,
		DelegationMode: delegationMode,
		UserID:         webUserID,
		TenantID:       defaultTenant,
	}

	// Look up the current discussion for this session (fast, indexed)
	var currentDiscussionID string
	if sessionID != "" {
		gw.db.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(discussion_id::text, '') FROM sessions WHERE id = $1`,
			sessionID,
		).Scan(&currentDiscussionID)
	}
	req.DiscussionID = currentDiscussionID
	req.SourceChannel = "web"

	// Depth dial: check if council mode should activate
	depth := council.Depth(depthParam)
	if depth == "" {
		depth = council.DepthBalanced
	}
	depthCfg := council.GetDepthConfig(depth)

	// Apply depth config to run request
	if !depthCfg.ToolsEnabled {
		req.NoTools = true
	}

	// Check if council should run (non-streaming only)
	if !stream && depthCfg.CouncilEnabled {
		dims := providers.ScoreRequest(userMsg, !req.NoTools, false, 0)
		if council.ShouldUseCouncil(depth, dims.Complexity) {
			provider := gw.providerReg.Default()
			if provider != nil {
				c := council.New(provider, council.DefaultConfig())
				councilResult, err := c.Run(r.Context(), userMsg)
				if err == nil {
					writeJSON(w, 200, map[string]any{
						"object": "chat.completion", "model": "council",
						"choices": []map[string]any{{
							"index":         0,
							"message":       map[string]any{"role": "assistant", "content": councilResult.Synthesis},
							"finish_reason": "stop",
						}},
						"council": councilResult,
					})
					return
				}
				slog.Warn("council.failed_fallback_to_single", "error", err)
			}
		}
	}

	if stream {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSON(w, 500, map[string]string{"error": "streaming not supported"})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(200)

		// doneCh is closed the moment [DONE] is written to the client.
		// The agent loop continues post-processing (persist, metrics, memory)
		// in a background goroutine so the HTTP response closes immediately.
		doneCh := make(chan struct{})
		go func() {
			gw.agentLoop.Run(context.Background(), req, func(event agent.StreamEvent) {
				var data []byte
				switch event.Type {
				case "text_delta":
					data, _ = json.Marshal(map[string]any{
						"object": "chat.completion.chunk",
						"choices": []map[string]any{{
							"index": 0, "delta": map[string]any{"content": event.Delta},
						}},
					})
				case "thinking_delta":
					data, _ = json.Marshal(map[string]any{
						"object": "chat.completion.chunk",
						"choices": []map[string]any{{
							"index": 0, "delta": map[string]any{"reasoning_content": event.Delta},
						}},
					})
				case "tool_start":
					data, _ = json.Marshal(map[string]any{"type": "tool_start", "data": event.Data})
				case "tool_result":
					data, _ = json.Marshal(map[string]any{"type": "tool_result", "data": event.Data})
				case "sources":
					data, _ = json.Marshal(map[string]any{"type": "sources", "data": event.Data})
				case "part":
					data, _ = json.Marshal(map[string]any{"type": "part", "data": event.Data})
				case "widget":
					data, _ = json.Marshal(map[string]any{"type": "widget", "data": event.Data})
				case "title":
					data, _ = json.Marshal(map[string]any{"type": "title", "data": event.Data})
				case "tags":
					data, _ = json.Marshal(map[string]any{"type": "tags", "data": event.Data})
				case "follow_up":
					data, _ = json.Marshal(map[string]any{"type": "follow_up", "data": event.Data})
				case "citation":
					data, _ = json.Marshal(map[string]any{"type": "citation", "data": event.Data})
				case "stream_start":
					data, _ = json.Marshal(map[string]any{"type": "stream_start", "data": event.Data})
				case "tool_approval":
					data, _ = json.Marshal(map[string]any{"type": "tool_approval", "data": event.Data})
				case "error":
					data, _ = json.Marshal(map[string]any{"type": "error", "data": event.Data})
				case "done":
					select {
					case <-doneCh: // already closed
					default:
						fmt.Fprintf(w, "data: [DONE]\n\n")
						flusher.Flush()
						close(doneCh)
					}
					return
				default:
					return
				}
				// Only write to response if still open
				select {
				case <-doneCh:
					return
				default:
				}
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			})
			// Ensure doneCh is closed even if loop returns without a "done" event
			select {
			case <-doneCh:
			default:
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				close(doneCh)
			}
			go gw.assignDiscussionAsync(context.Background(), agentID, sessionID, userMsg)
		}()
		<-doneCh
		return
	}

	// Non-streaming
	result, err := gw.agentLoop.Run(r.Context(), req, func(event agent.StreamEvent) {})
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	go gw.assignDiscussionAsync(context.Background(), agentID, sessionID, userMsg)
	writeJSON(w, 200, map[string]any{
		"object": "chat.completion", "model": model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": result.Content},
			"finish_reason": "stop",
		}},
		"usage": map[string]int{
			"prompt_tokens": result.InputTokens, "completion_tokens": result.OutputTokens,
			"total_tokens": result.InputTokens + result.OutputTokens,
		},
		"metadata": map[string]any{
			"tools_used": result.ToolsUsed,
			"iterations": result.Iterations,
			"session_id": sessionID,
			"agent_id":   agentID,
		},
	})
}

func (gw *Gateway) streamChat(w http.ResponseWriter, r *http.Request, p providers.Provider, req providers.ChatRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, 500, map[string]string{"error": "streaming not supported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)

	p.ChatStream(r.Context(), req, func(chunk providers.StreamChunk) {
		if chunk.Done {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}
		delta := map[string]any{}
		if chunk.Content != "" {
			delta["content"] = chunk.Content
		}
		if chunk.Thinking != "" {
			delta["reasoning_content"] = chunk.Thinking
		}
		data, _ := json.Marshal(map[string]any{
			"object": "chat.completion.chunk",
			"choices": []map[string]any{{
				"index": 0, "delta": delta,
			}},
		})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	})
}

// handleAISDKChat is the bridge between the AI SDK v6 useChat/DefaultChatTransport
// protocol and Qorven's agent loop. In static-export builds the Next.js API
// route at /app/api/chat/route.ts is not bundled (no Node.js runtime), so the
// Go binary handles /api/chat directly.
//
// Request (POST, JSON):
//
//	{ messages: [{role, parts:[{type:"text",text:"..."}]}, ...],
//	  agentId: "uuid", sessionId: "uuid", thinkingLevel?: "off"|"medium"|"high" }
//
// Response: SSE with Content-Type text/event-stream and the
// x-vercel-ai-ui-message-stream: v1 header. Each event is a JSON-serialized
// UIMessageChunk. Stream ends with `data: [DONE]\n\n`.
func (gw *Gateway) handleAISDKChat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Messages []struct {
			Role  string `json:"role"`
			Parts []struct {
				Type      string `json:"type"`
				Text      string `json:"text"`
				URL       string `json:"url,omitempty"`
				MediaType string `json:"mediaType,omitempty"`
				Filename  string `json:"filename,omitempty"`
			} `json:"parts"`
			Content interface{} `json:"content"` // v5 compat
		} `json:"messages"`
		AgentID       string `json:"agentId"`
		SessionID     string `json:"sessionId"`
		SystemContext string `json:"systemContext,omitempty"`
		ThinkingLevel string `json:"thinkingLevel,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Extract user message text from the last user message (AI SDK v6 uses parts).
	userMsg := ""
	for i := len(body.Messages) - 1; i >= 0; i-- {
		m := body.Messages[i]
		if m.Role != "user" {
			continue
		}
		for _, p := range m.Parts {
			if p.Type == "text" {
				userMsg += p.Text
			}
		}
		if userMsg == "" {
			if s, ok := m.Content.(string); ok {
				userMsg = s
			}
		}
		break
	}
	if userMsg == "" {
		http.Error(w, `{"error":"no user message found"}`, http.StatusBadRequest)
		return
	}
	if gw.agentLoop == nil {
		http.Error(w, `{"error":"agent loop not initialised"}`, http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("x-vercel-ai-ui-message-stream", "v1")
	w.Header().Set("x-accel-buffering", "no")
	w.WriteHeader(http.StatusOK)

	enq := func(chunk map[string]any) {
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Start marker — AI SDK expects this before any content.
	enq(map[string]any{"type": "start"})

	textID := fmt.Sprintf("t-%d", time.Now().UnixNano())
	textStarted := false

	req := agent.RunRequest{
		AgentID:       body.AgentID,
		SessionID:     body.SessionID,
		UserMessage:   userMsg,
		Channel:       "web",
		ThinkingLevel: body.ThinkingLevel,
	}

	if _, runErr := gw.agentLoop.Run(r.Context(), req, func(event agent.StreamEvent) {
		switch event.Type {
		case "text_delta":
			if !textStarted {
				enq(map[string]any{"type": "text-start", "id": textID})
				textStarted = true
			}
			enq(map[string]any{"type": "text-delta", "id": textID, "delta": event.Delta})
		case "thinking_delta":
			enq(map[string]any{"type": "reasoning-delta", "id": "r-" + textID, "delta": event.Delta})
		case "tool_start":
			if textStarted {
				enq(map[string]any{"type": "text-end", "id": textID})
				textStarted = false
				textID = fmt.Sprintf("t-%d", time.Now().UnixNano())
			}
			if event.Data != nil {
				d, _ := json.Marshal(event.Data)
				var td map[string]any
				if json.Unmarshal(d, &td) == nil {
					toolName, _ := td["name"].(string)
					if toolName == "" {
						toolName, _ = td["tool"].(string)
					}
					toolID, _ := td["id"].(string)
					if toolID == "" {
						toolID = fmt.Sprintf("tc-%d", time.Now().UnixNano())
					}
					enq(map[string]any{
						"type": "tool-input-available", "toolCallId": toolID,
						"toolName": toolName, "input": td["args"], "dynamic": true,
					})
				}
			}
		case "tool_result":
			if event.Data != nil {
				d, _ := json.Marshal(event.Data)
				var td map[string]any
				if json.Unmarshal(d, &td) == nil {
					toolID, _ := td["id"].(string)
					if toolID == "" {
						toolID = fmt.Sprintf("tc-%d", time.Now().UnixNano())
					}
					result := td["result"]
					if result == nil {
						result = td["output"]
					}
					enq(map[string]any{"type": "tool-output-available", "toolCallId": toolID, "output": result})
				}
			}
		case "sources":
			enq(map[string]any{"type": "data-sources", "id": "src-" + textID, "data": event.Data})
		case "follow_up":
			enq(map[string]any{"type": "data-follow_ups", "id": "fu-" + textID, "data": event.Data})
		case "title":
			enq(map[string]any{"type": "data-title", "id": "ti-" + textID, "data": event.Data})
		case "widget":
			enq(map[string]any{"type": "data-widget", "id": "wg-" + textID, "data": event.Data})
		case "error":
			if textStarted {
				enq(map[string]any{"type": "text-end", "id": textID})
				textStarted = false
			}
			msg := ""
			if s, ok := event.Data.(string); ok {
				msg = s
			} else if event.Delta != "" {
				msg = event.Delta
			}
			enq(map[string]any{"type": "error", "errorText": msg})
		case "done":
			// handled below after Run returns
		}
	}); runErr != nil && !textStarted {
		enq(map[string]any{"type": "error", "errorText": runErr.Error()})
	}

	if textStarted {
		enq(map[string]any{"type": "text-end", "id": textID})
	}
	enq(map[string]any{"type": "finish"})
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// handleOpenAIModels returns an OpenAI-compatible /v1/models response scoped
// to the tenant's selected models. Used by OpenAI-compat clients (Claude Code, Cursor, etc.).
func (gw *Gateway) handleOpenAIModels(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"object": "list", "data": []any{}})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT model_id, provider_id, created_at
		 FROM selected_models
		 WHERE tenant_id = $1
		 ORDER BY is_default DESC, display_order, model_id`, defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	type modelObj struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}
	data := []modelObj{}
	for rows.Next() {
		var id, pid string
		var createdAt time.Time
		rows.Scan(&id, &pid, &createdAt)
		data = append(data, modelObj{
			ID:      id,
			Object:  "model",
			Created: createdAt.Unix(),
			OwnedBy: pid,
		})
	}
	if data == nil {
		data = []modelObj{}
	}
	writeJSON(w, 200, map[string]any{"object": "list", "data": data})
}

func formatToolCalls(tcs []providers.ToolCall) any {
	if len(tcs) == 0 {
		return nil
	}
	out := make([]map[string]any, len(tcs))
	for i, tc := range tcs {
		args, _ := json.Marshal(tc.Arguments)
		out[i] = map[string]any{
			"id": tc.ID, "type": "function",
			"function": map[string]any{"name": tc.Name, "arguments": string(args)},
		}
	}
	return out
}
