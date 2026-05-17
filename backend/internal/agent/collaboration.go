// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent
import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	
	"github.com/qorvenai/qorven/internal/tools"
)
// CollabSession represents a shared workspace where multiple agents collaborate.
// Unlike delegation (one-way), collaboration is bidirectional — agents see each
// other's work, share context, and can review/modify each other's output.
type CollabSession struct {
	ID        string
	ProjectID string
	Agents    []CollabAgent
	SharedCtx []SharedMessage // shared context visible to all agents
	mu        sync.Mutex
}
type CollabAgent struct {
	AgentID string
	Role    string // "lead", "reviewer", "specialist"
	Status  string // "working", "reviewing", "idle"
}
type SharedMessage struct {
	AgentID   string    `json:"agent_id"`
	AgentRole string    `json:"role"`
	Content   string    `json:"content"`
	Type      string    `json:"type"` // "code", "review", "question", "decision"
	Timestamp time.Time `json:"timestamp"`
}
// CollabManager orchestrates multi-agent collaboration sessions.
type CollabManager struct {
	sessions map[string]*CollabSession
	loop     *Loop
	mu       sync.Mutex
}
func NewCollabManager(loop *Loop) *CollabManager {
	return &CollabManager{sessions: make(map[string]*CollabSession), loop: loop}
}
// StartCollab creates a collaboration session with multiple agents.
func (cm *CollabManager) StartCollab(ctx context.Context, projectID string, agents []CollabAgent) *CollabSession {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	sess := &CollabSession{
		ID:        fmt.Sprintf("collab_%d", time.Now().Unix()),
		ProjectID: projectID,
		Agents:    agents,
	}
	cm.sessions[sess.ID] = sess
	slog.Info("collab.started", "id", sess.ID, "project", projectID, "agents", len(agents))
	return sess
}
// PostMessage adds a message to the shared context.
func (cs *CollabSession) PostMessage(agentID, role, content, msgType string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.SharedCtx = append(cs.SharedCtx, SharedMessage{
		AgentID: agentID, AgentRole: role, Content: content,
		Type: msgType, Timestamp: time.Now(),
	})
}
// GetContext returns the shared context formatted for injection into an agent's prompt.
func (cs *CollabSession) GetContext(forAgentID string) string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if len(cs.SharedCtx) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Collaboration Context\n\nOther agents' work on this project:\n\n")
	for _, m := range cs.SharedCtx {
		if m.AgentID == forAgentID {
			continue // skip own messages
		}
		sb.WriteString(fmt.Sprintf("**%s** (%s): %s\n\n", m.AgentRole, m.Type, truncCollab(m.Content, 500)))
	}
	return sb.String()
}
// RunCollabTask runs a task within a collaboration session.
// The agent sees all other agents' work as context.
func (cm *CollabManager) RunCollabTask(ctx context.Context, collabID, agentID, task string) (*RunResult, error) {
	cm.mu.Lock()
	sess, ok := cm.sessions[collabID]
	cm.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("collab session not found: %s", collabID)
	}
	// Get shared context from other agents
	sharedCtx := sess.GetContext(agentID)
	// Run the agent with shared context injected
	result, err := cm.loop.Run(ctx, RunRequest{
		AgentID:           agentID,
		UserMessage:       task,
		ExtraSystemPrompt: sharedCtx,
		ChannelType:       "collab",
	}, nil)
	if err != nil {
		return nil, err
	}
	// Post the result to shared context
	role := "agent"
	for _, a := range sess.Agents {
		if a.AgentID == agentID {
			role = a.Role
		}
	}
	sess.PostMessage(agentID, role, result.Content, "work")
	return result, nil
}
// ReviewWork has one agent review another agent's work.
func (cm *CollabManager) ReviewWork(ctx context.Context, collabID, reviewerID, authorID string) (*RunResult, error) {
	cm.mu.Lock()
	sess, ok := cm.sessions[collabID]
	cm.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("collab session not found: %s", collabID)
	}
	// Find the author's latest work
	var authorWork string
	sess.mu.Lock()
	for i := len(sess.SharedCtx) - 1; i >= 0; i-- {
		if sess.SharedCtx[i].AgentID == authorID {
			authorWork = sess.SharedCtx[i].Content
			break
		}
	}
	sess.mu.Unlock()
	if authorWork == "" {
		return nil, fmt.Errorf("no work found from agent %s", authorID)
	}
	reviewPrompt := fmt.Sprintf("Review this work from another agent. Identify issues, suggest improvements, and rate quality (1-10):\n\n%s", truncCollab(authorWork, 2000))
	result, err := cm.loop.Run(ctx, RunRequest{
		AgentID:     reviewerID,
		UserMessage: reviewPrompt,
		ChannelType: "collab",
	}, nil)
	if err != nil {
		return nil, err
	}
	sess.PostMessage(reviewerID, "reviewer", result.Content, "review")
	return result, nil
}
func truncCollab(s string, max int) string {
	if len(s) <= max { return s }
	return s[:max] + "..."
}
// --- Collab Tool (for agents to initiate collaboration) ---
type CollabTool struct {
	manager *CollabManager
}
func NewCollabTool(mgr *CollabManager) *CollabTool {
	return &CollabTool{manager: mgr}
}
func (t *CollabTool) Name() string { return "collaborate" }
func (t *CollabTool) Description() string {
	return "Start a collaboration session with other agents. Actions: start, post, review."
}
func (t *CollabTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":    map[string]any{"type": "string", "enum": []string{"start", "post", "review"}},
			"agents":    map[string]any{"type": "string", "description": "Comma-separated agent IDs for start"},
			"collab_id": map[string]any{"type": "string", "description": "Collaboration session ID"},
			"message":   map[string]any{"type": "string", "description": "Message to post"},
			"target":    map[string]any{"type": "string", "description": "Agent ID to review"},
		},
		"required": []string{"action"},
	}
}
func (t *CollabTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	action, _ := args["action"].(string)
	agentID := tools.AgentIDFromCtx(ctx)
	switch action {
	case "start":
		agentList, _ := args["agents"].(string)
		ids := strings.Split(agentList, ",")
		var agents []CollabAgent
		agents = append(agents, CollabAgent{AgentID: agentID, Role: "lead"})
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id != "" && id != agentID {
				agents = append(agents, CollabAgent{AgentID: id, Role: "specialist"})
			}
		}
		sess := t.manager.StartCollab(ctx, "", agents)
		return tools.TextResult(fmt.Sprintf("Collaboration started: %s with %d agents", sess.ID, len(agents)))
	case "post":
		collabID, _ := args["collab_id"].(string)
		message, _ := args["message"].(string)
		t.manager.mu.Lock()
		sess, ok := t.manager.sessions[collabID]
		t.manager.mu.Unlock()
		if !ok { return tools.ErrorResult("session not found") }
		sess.PostMessage(agentID, "agent", message, "work")
		return tools.TextResult("Posted to collaboration")
	case "review":
		collabID, _ := args["collab_id"].(string)
		target, _ := args["target"].(string)
		result, err := t.manager.ReviewWork(ctx, collabID, agentID, target)
		if err != nil { return tools.ErrorResult(err.Error()) }
		return tools.TextResult(result.Content)
	default:
		return tools.ErrorResult("unknown action: " + action)
	}
}
