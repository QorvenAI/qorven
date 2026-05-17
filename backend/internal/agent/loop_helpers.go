// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/memory"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/session"
)

func (l *Loop) resolveProvider(ag *Agent) providers.Provider {
	// Try agent's configured provider first
	if ag.ProviderID != nil {
		if p, ok := l.providerReg.Get(*ag.ProviderID); ok {
			return p
		}
	}
	// Fallback to default provider
	return l.providerReg.Default()
}

func (l *Loop) loadHistory(ctx context.Context, sessionID string) []providers.Message {
	if l.sessionStore == nil || sessionID == "" {
		return nil
	}
	// Try UUID lookup first, then session_key fallback
	sess, err := l.sessionStore.Get(ctx, sessionID)
	if err != nil {
		return nil
	}

	var dbMessages []struct {
		Role       string `json:"role"`
		Content    string `json:"content"`
		ToolCallID string `json:"tool_call_id,omitempty"`
	}
	if err := json.Unmarshal(sess.Messages, &dbMessages); err != nil {
		return nil
	}

	msgs := make([]providers.Message, 0, len(dbMessages))
	for _, m := range dbMessages {
		msgs = append(msgs, providers.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		})
	}
	return msgs
}

func (l *Loop) saveToSession(ctx context.Context, req RunRequest, result *RunResult) {
	if l.sessionStore == nil || req.SessionID == "" {
		return
	}
	// Skip saving user message for internal follow-ups (delegation results),
	// or when the caller explicitly pre-persisted it (NoPersist).
	// Empty channel = web/API call — always save unless NoPersist.
	if req.Channel != "internal" && !req.NoPersist {
		l.sessionStore.AppendMessage(ctx, req.SessionID, session.Message{
			Role: "user", Content: cleanUserMessage(req), Timestamp: time.Now().UnixMilli(),
		}, 0, 0)
		// Broadcast user message too (for live chat sync)
		if l.OnMessage != nil {
			l.OnMessage(req.SessionID, req.AgentID, "user", cleanUserMessage(req), req.Channel)
		}
	}

	// Save assistant response
	partsJSON, _ := json.Marshal(result.Parts)
	l.sessionStore.AppendMessage(ctx, req.SessionID, session.Message{
		Role: "assistant", Content: result.Content, Timestamp: time.Now().UnixMilli(),
		Metadata: result.Metadata, Parts: partsJSON,
	}, result.InputTokens, result.OutputTokens)

	// Broadcast to all connected clients (real-time sync)
	if l.OnMessage != nil {
		l.OnMessage(req.SessionID, req.AgentID, "assistant", result.Content, req.Channel)
	}
}

func (l *Loop) extractMemories(ctx context.Context, agentID, userMsg, assistantMsg, sessionKey string) {
	if l.memStore == nil {
		return
	}

	// Heuristic extraction (fast, no LLM)
	for _, m := range memory.ExtractMemories(agentID, userMsg, sessionKey) {
		l.memStore.Save(ctx, l.tenantID, m)
	}
	for _, m := range memory.ExtractMemories(agentID, assistantMsg, sessionKey) {
		l.memStore.Save(ctx, l.tenantID, m)
	}

	// LLM-based extraction with certainty levels (Neuromancer)
	// Runs async — don't block the response
	if defaultProv := l.providerReg.Default(); defaultProv != nil && userMsg != "" {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			conclusions := memory.LLMExtractConclusions(bgCtx, defaultProv, "", userMsg, assistantMsg)
			for _, c := range conclusions {
				c.AgentID = agentID
				c.Source = sessionKey
				c.Weight = memory.CertaintyWeight(c.Certainty)
				m := memory.Memory{
					AgentID: agentID,
					Content: c.Content,
					Type:    string(c.Certainty),
					Source:  sessionKey,
				}
				l.memStore.Save(bgCtx, l.tenantID, m)
			}

			// Knowledge Graph: extract entities and relationships
			entities, rels := memory.ExtractEntities(bgCtx, defaultProv, "", userMsg, assistantMsg)
			if l.KnowledgeGraph != nil && len(entities) > 0 {
				for _, e := range entities {
					e.AgentID = agentID
					l.KnowledgeGraph.UpsertEntity(bgCtx, e)
				}
				for _, r := range rels {
					l.KnowledgeGraph.UpsertRelationship(bgCtx, r)
				}
				slog.Debug("kg.extracted", "entities", len(entities), "rels", len(rels), "agent", agentID)
			}
		}()
	}
}

func truncateForEvent(s string) string {
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}

func isCorrection(msg string) bool {
	lower := strings.ToLower(msg)
	corrections := []string{"no,", "wrong", "incorrect", "that's not", "actually,", "i meant", "not what i", "fix this", "try again", "redo"}
	for _, c := range corrections {
		if strings.Contains(lower, c) {
			return true
		}
	}
	return false
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func autoTemperature(userMsg string, iteration int) float64 {
	if iteration > 0 {
		return 0.2 // tool follow-up iterations need precision
	}
	tier := ScoreComplexity(userMsg)
	switch tier {
	case TierLight:
		return 0.7
	case TierStandard:
		return 0.5
	case TierComplex:
		return 0.3
	case TierReasoning:
		return 0.1 // precision matters most for deep reasoning tasks
	default:
		return 0.5
	}
}

func (e *providerError) Error() string { return e.msg }

func modelToProvider(model string) string {
	switch {
	case strings.Contains(model, "gemini"):
		return "gemini"
	case strings.Contains(model, "claude"):
		return "anthropic"
	case strings.Contains(model, "gpt"):
		return "openai"
	case strings.Contains(model, "deepseek"):
		return "deepseek"
	case strings.Contains(model, "kimi"):
		return "custom"
	case strings.Contains(model, "qwen"):
		return "custom"
	default:
		return ""
	}
}

func extractLocation(query string) string {
	q := query
	lower := strings.ToLower(q)
	// Try known patterns first
	for _, prefix := range []string{"weather in ", "temperature in ", "forecast for ", "weather of ", "climate in ", "weather at ", "weather for ", "what about ", "how about ", "and in ", "and "} {
		if idx := strings.Index(lower, prefix); idx >= 0 {
			loc := strings.TrimSpace(q[idx+len(prefix):])
			loc = strings.TrimRight(loc, "?!.")
			if len(loc) > 1 {
				return loc
			}
		}
	}
	// Try "what's the weather in X" pattern
	if idx := strings.Index(lower, " in "); idx >= 0 {
		loc := strings.TrimSpace(q[idx+4:])
		loc = strings.TrimRight(loc, "?!.")
		if len(loc) > 1 {
			return loc
		}
	}
	return strings.TrimRight(strings.TrimSpace(q), "?!.")
}

func toF64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case float32:
		return float64(n)
	}
	return 0
}

func looksLikeActionClaim(content string) bool {
	lower := strings.ToLower(content)
	actionPatterns := []string{
		"✅ scheduled", "✅ created", "✅ sent", "✅ saved", "✅ done",
		"scheduled!", "created!", "task created", "job created",
		"i've scheduled", "i've created", "i've sent", "i've saved",
		"i have scheduled", "i have created", "i have sent",
		"successfully scheduled", "successfully created",
		"set up a", "set up the",
		"(job:", "(task:", "(id:",
		"message sent", "sent to telegram", "sent to your telegram",
		"i'll send", "will send it", "sending now", "message queued",
	}
	for _, p := range actionPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func cleanUserMessage(req RunRequest) string {
	msg := req.UserMessage
	if req.Channel == "voice" {
		if idx := strings.Index(msg, "User said: \""); idx >= 0 {
			msg = msg[idx+12:]
			if end := strings.LastIndex(msg, "\""); end > 0 {
				msg = msg[:end]
			}
		} else if strings.Contains(msg, "[Voice message") || strings.Contains(msg, "[Voice]") {
			// Strip voice prefix — extract quoted content or everything after prefix
			if idx := strings.LastIndex(msg, "\""); idx > 0 {
				start := strings.LastIndex(msg[:idx], "\"")
				if start >= 0 {
					msg = msg[start+1 : idx]
				}
			} else if idx := strings.Index(msg, "]"); idx >= 0 {
				msg = strings.TrimSpace(msg[idx+1:])
			}
		}
	}
	return strings.TrimSpace(msg)
}

func (l *Loop) flushMemoryBeforeCompaction(ctx context.Context, provider providers.Provider, ag *Agent, messages []providers.Message) {
	if provider == nil || len(messages) < 4 {
		return
	}

	// Build a condensed version of recent messages for the flush prompt
	var sb strings.Builder
	sb.WriteString("Extract the 5 most important facts from this conversation that should be remembered permanently:\n\n")
	count := 0
	for i := len(messages) - 1; i >= 0 && count < 20; i-- {
		m := messages[i]
		if m.Role == "tool" {
			continue
		}
		content := m.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		sb.WriteString(m.Role + ": " + content + "\n")
		count++
	}
	sb.WriteString("\nReturn ONLY a JSON array of strings, each a key fact. Example: [\"User prefers Go\", \"Project uses PostgreSQL\"]")

	fctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := provider.Chat(fctx, providers.ChatRequest{
		Model:    ag.Model,
		Messages: []providers.Message{{Role: "user", Content: sb.String()}},
		Options:  map[string]any{"max_tokens": 500, "temperature": 0.1},
	})
	if err != nil {
		slog.Warn("memory.flush.failed", "error", err)
		return
	}

	// Parse facts and save to memory
	facts := extractJSONArray(resp.Content)
	for _, fact := range facts {
		if len(fact) > 10 {
			l.memStore.Save(ctx, l.tenantID, memory.Memory{
				AgentID: ag.ID, Type: "compaction_flush", Content: fact,
				Source: "pre-compaction", Importance: 0.8,
			})
		}
	}
	slog.Info("memory.flush.complete", "facts", len(facts), "agent", ag.AgentKey)
}

func extractJSONArray(s string) []string {
	// Find JSON array in response
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end <= start {
		return nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(s[start:end+1]), &arr); err != nil {
		return nil
	}
	return arr
}

func (l *Loop) loadSecretPatterns() map[string]string {
	// Load secret patterns from environment and config
	patterns := map[string]string{}
	for _, env := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "OPENROUTER_API_KEY"} {
		if v := os.Getenv(env); v != "" && len(v) > 8 {
			patterns[env] = v
		}
	}
	return patterns
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

func RequestIDFromCtx(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

func (l *Loop) getConfiguredModels() []string {
	if l.providerReg == nil {
		return nil
	}
	seen := map[string]bool{}
	var models []string
	for _, p := range l.providerReg.List() {
		if !p.Enabled {
			continue
		}
		// Map provider type to default model
		m := providerDefaultModel(p.ProviderType, p.Name)
		if m != "" && !seen[m] {
			seen[m] = true
			models = append(models, m)
		}
	}
	return models
}

func providerDefaultModel(provType, name string) string {
	switch {
	case strings.Contains(name, "deepseek"):
		return "deepseek-chat"
	case strings.Contains(name, "gemini") || strings.Contains(name, "google"):
		return "gemini-2.0-flash"
	case strings.Contains(name, "openai"):
		return "gpt-4o-mini"
	case strings.Contains(name, "anthropic") || provType == "bedrock":
		return "claude-sonnet-4-20250514"
	default:
		return ""
	}
}

func truncateForProjectMem(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// applyReasoningToOptions injects provider-specific thinking parameters into chatOpts
// based on the resolved effort level from ResolveReasoningDecision.
func applyReasoningToOptions(opts map[string]any, provider providers.Provider, model, effort string) {
	// Map effort → approximate budget_tokens for Anthropic extended thinking
	budgetMap := map[string]int{
		"minimal": 512,
		"low":     1024,
		"medium":  4096,
		"high":    10000,
		"xhigh":   32000,
	}

	switch p := provider.(type) {
	case interface{ Name() string }:
		name := p.Name()
		switch {
		case name == "anthropic" || name == "anthropic_native":
			if budget, ok := budgetMap[effort]; ok {
				opts["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
			}
		case strings.HasPrefix(name, "bedrock") || strings.Contains(name, "bedrock"):
			// BedrockProvider routes Claude via InvokeModel using the Anthropic
			// Messages body; inject the same thinking object as native Anthropic.
			if budget, ok := budgetMap[effort]; ok {
				opts["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
			}
		case name == "openai" || name == "azure_openai":
			opts["reasoning_effort"] = effort
		case name == "gemini" || name == "google":
			opts["thinking_config"] = map[string]any{"thinking_budget": budgetFromEffort(effort)}
		default:
			opts["reasoning_effort"] = effort
		}
	default:
		_ = p
		opts["reasoning_effort"] = effort
	}
}

func budgetFromEffort(effort string) int {
	switch effort {
	case "minimal": return 512
	case "low":     return 1024
	case "medium":  return 4096
	case "high":    return 10000
	case "xhigh":   return 32000
	default:        return 4096
	}
}
