// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RoomPostTool lets agents post messages to rooms autonomously.
// This is the critical gap fix — the original was a stub that never actually posted.
// Now agents can proactively post to rooms without user triggers.
type RoomPostTool struct {
	apiBase  string
	getToken func() string
}

func NewRoomPostTool() *RoomPostTool {
	return &RoomPostTool{apiBase: "http://localhost:4200"}
}

func NewRoomPostToolWithAuth(apiBase string, getToken func() string) *RoomPostTool {
	if apiBase == "" {
		apiBase = "http://localhost:4200"
	}
	return &RoomPostTool{apiBase: apiBase, getToken: getToken}
}

func (t *RoomPostTool) Name() string { return "room_post" }

func (t *RoomPostTool) Description() string {
	return `Post a message to a room. Agents use this to communicate autonomously without user triggers.

Use cases:
- Report task completion: "Analysis done. Found 3 key insights: ..."
- Ask another agent: "@researcher can you verify this claim?"
- Share findings: "I've detected an anomaly in the sales data..."
- Pin a decision: "DECISION: We'll use approach B based on cost analysis."
- Assign a task: "TASK @developer: implement the auth fix by Friday"

Use @agent_key to mention specific agents. They will be triggered to respond.
Start with "DECISION:" to pin this as a room decision.
Start with "TASK @agent:" to create a tracked task assignment.`
}

func (t *RoomPostTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room_id": map[string]any{
				"type":        "string",
				"description": "The room ID to post to",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "Message content. Use @agent_key to mention agents. Prefix with DECISION: or TASK @agent: for special handling.",
			},
			"message_type": map[string]any{
				"type":        "string",
				"enum":        []string{"text", "decision", "task", "summary", "alert"},
				"description": "Type of message. 'decision' pins it, 'task' creates a tracked assignment.",
			},
		},
		"required": []string{"room_id", "message"},
	}
}

func (t *RoomPostTool) Execute(ctx context.Context, args map[string]any) *Result {
	roomID, _ := args["room_id"].(string)
	message, _ := args["message"].(string)
	msgType, _ := args["message_type"].(string)

	if roomID == "" || message == "" {
		return ErrorResult("room_id and message required")
	}
	if msgType == "" {
		msgType = "text"
		// Auto-detect type from content
		upper := strings.ToUpper(strings.TrimSpace(message))
		if strings.HasPrefix(upper, "DECISION:") {
			msgType = "decision"
		} else if strings.HasPrefix(upper, "TASK ") {
			msgType = "task"
		} else if strings.Contains(upper, "SUMMARY:") {
			msgType = "summary"
		}
	}

	// Get agent key from context for sender identification
	senderKey := "agent"
	if key, ok := ctx.Value("agent_key").(string); ok && key != "" {
		senderKey = key
	}

	body, _ := json.Marshal(map[string]any{
		"sender_id":    senderKey,
		"sender_type":  "soul",
		"content":      message,
		"message_type": msgType,
	})

	url := fmt.Sprintf("%s/v1/rooms/%s/messages", t.apiBase, roomID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return ErrorResult("failed to create request: " + err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	if t.getToken != nil {
		if tok := t.getToken(); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ErrorResult("room post failed: " + err.Error())
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body) // drain

	if resp.StatusCode >= 400 {
		return ErrorResult(fmt.Sprintf("room post failed: HTTP %d", resp.StatusCode))
	}

	preview := message
	if len(preview) > 80 {
		preview = preview[:80] + "…"
	}
	return TextResult(fmt.Sprintf("✅ Posted to room [%s]: %s", roomID[:min(len(roomID), 8)], preview))
}

// RoomListTool lets agents see what rooms they're in.
type RoomListTool struct {
	apiBase  string
	getToken func() string
}

func NewRoomListTool(apiBase string, getToken func() string) *RoomListTool {
	return &RoomListTool{apiBase: apiBase, getToken: getToken}
}

func (t *RoomListTool) Name() string { return "room_list" }
func (t *RoomListTool) Description() string {
	return "List available rooms and their recent activity. Use to find which room to post to."
}
func (t *RoomListTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}}
}

func (t *RoomListTool) Execute(ctx context.Context, args map[string]any) *Result {
	url := t.apiBase + "/v1/rooms"
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	if t.getToken != nil {
		if tok := t.getToken(); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ErrorResult("failed to list rooms: " + err.Error())
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	var result map[string]any
	json.Unmarshal(data, &result)

	rooms, _ := result["rooms"].([]any)
	if len(rooms) == 0 {
		return TextResult("No rooms available. A room can be created from the web UI or via Prime.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**%d rooms available:**\n\n", len(rooms)))
	for _, r := range rooms {
		rm, _ := r.(map[string]any)
		id, _ := rm["id"].(string)
		name, _ := rm["display_name"].(string)
		if name == "" {
			name, _ = rm["name"].(string)
		}
		memberCount, _ := rm["member_count"].(float64)
		msgCount, _ := rm["message_count"].(float64)
		if id != "" {
			shortID := id
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			sb.WriteString(fmt.Sprintf("- **%s** (ID: `%s`) — %d agents, %d messages\n",
				name, shortID, int(memberCount), int(msgCount)))
		}
	}
	return TextResult(sb.String())
}

// RoomDecideTool pins a decision in a room — persists it separately from message stream.
type RoomDecideTool struct {
	apiBase  string
	getToken func() string
}

func NewRoomDecideTool(apiBase string, getToken func() string) *RoomDecideTool {
	return &RoomDecideTool{apiBase: apiBase, getToken: getToken}
}

func (t *RoomDecideTool) Name() string { return "room_decide" }
func (t *RoomDecideTool) Description() string {
	return "Record a formal decision in a room. Decisions are pinned and persist independently of message history. Use when the team reaches a conclusion that should be remembered."
}
func (t *RoomDecideTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room_id":  map[string]any{"type": "string"},
			"decision": map[string]any{"type": "string", "description": "The decision text — be specific and actionable"},
		},
		"required": []string{"room_id", "decision"},
	}
}

func (t *RoomDecideTool) Execute(ctx context.Context, args map[string]any) *Result {
	roomID, _ := args["room_id"].(string)
	decision, _ := args["decision"].(string)
	if roomID == "" || decision == "" {
		return ErrorResult("room_id and decision required")
	}

	senderKey := "agent"
	if key, ok := ctx.Value("agent_key").(string); ok && key != "" {
		senderKey = key
	}

	body, _ := json.Marshal(map[string]any{
		"content":    decision,
		"decided_by": senderKey,
	})

	url := fmt.Sprintf("%s/v1/rooms/%s/decisions", t.apiBase, roomID)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if t.getToken != nil {
		if tok := t.getToken(); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode >= 400 {
		// Fall back to posting as message with DECISION: prefix
		fallbackMsg := "📌 DECISION: " + decision
		fallbackBody, _ := json.Marshal(map[string]any{
			"sender_id":    senderKey,
			"sender_type":  "soul",
			"content":      fallbackMsg,
			"message_type": "decision",
		})
		msgURL := fmt.Sprintf("%s/v1/rooms/%s/messages", t.apiBase, roomID)
		msgReq, _ := http.NewRequestWithContext(ctx, "POST", msgURL, bytes.NewReader(fallbackBody))
		msgReq.Header.Set("Content-Type", "application/json")
		if t.getToken != nil {
			if tok := t.getToken(); tok != "" {
				msgReq.Header.Set("Authorization", "Bearer "+tok)
			}
		}
		http.DefaultClient.Do(msgReq)
		return TextResult("📌 Decision recorded: " + decision)
	}
	if resp != nil {
		resp.Body.Close()
	}
	return TextResult("📌 Decision pinned: " + decision)
}

// RoomAssignTool assigns a task to an agent within a room context.
type RoomAssignTool struct {
	apiBase  string
	getToken func() string
}

func NewRoomAssignTool(apiBase string, getToken func() string) *RoomAssignTool {
	return &RoomAssignTool{apiBase: apiBase, getToken: getToken}
}

func (t *RoomAssignTool) Name() string { return "room_assign" }
func (t *RoomAssignTool) Description() string {
	return "Assign a task to another agent within a room. The assigned agent will be notified and expected to complete it."
}
func (t *RoomAssignTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room_id":     map[string]any{"type": "string"},
			"assign_to":   map[string]any{"type": "string", "description": "agent_key of the agent to assign to"},
			"task":        map[string]any{"type": "string", "description": "Clear task description"},
			"due":         map[string]any{"type": "string", "description": "Optional: when should this be done (e.g. 'by end of day', 'Friday 5pm')"},
		},
		"required": []string{"room_id", "assign_to", "task"},
	}
}

func (t *RoomAssignTool) Execute(ctx context.Context, args map[string]any) *Result {
	roomID, _ := args["room_id"].(string)
	assignTo, _ := args["assign_to"].(string)
	task, _ := args["task"].(string)
	due, _ := args["due"].(string)

	if roomID == "" || assignTo == "" || task == "" {
		return ErrorResult("room_id, assign_to, and task required")
	}

	senderKey := "agent"
	if key, ok := ctx.Value("agent_key").(string); ok && key != "" {
		senderKey = key
	}

	// Write to room_tasks table so the Tasks tab shows this assignment
	taskPayload := map[string]any{
		"title":       task,
		"assigned_by": senderKey,
		"assigned_to": assignTo,
	}
	if due != "" {
		taskPayload["due_at"] = due
	}
	taskBody, _ := json.Marshal(taskPayload)
	taskURL := fmt.Sprintf("%s/v1/rooms/%s/tasks", t.apiBase, roomID)
	taskReq, _ := http.NewRequestWithContext(ctx, "POST", taskURL, bytes.NewReader(taskBody))
	taskReq.Header.Set("Content-Type", "application/json")
	if t.getToken != nil {
		if tok := t.getToken(); tok != "" {
			taskReq.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	taskResp, taskErr := http.DefaultClient.Do(taskReq)
	if taskResp != nil {
		io.ReadAll(taskResp.Body)
		taskResp.Body.Close()
	}

	// Fall back to message-only if the tasks endpoint failed
	if taskErr != nil || taskResp.StatusCode >= 400 {
		taskMsg := fmt.Sprintf("📋 TASK → @%s: %s", assignTo, task)
		if due != "" {
			taskMsg += fmt.Sprintf(" (due: %s)", due)
		}
		msgBody, _ := json.Marshal(map[string]any{
			"sender_id":    senderKey,
			"sender_type":  "soul",
			"content":      taskMsg,
			"message_type": "task",
		})
		msgURL := fmt.Sprintf("%s/v1/rooms/%s/messages", t.apiBase, roomID)
		msgReq, _ := http.NewRequestWithContext(ctx, "POST", msgURL, bytes.NewReader(msgBody))
		msgReq.Header.Set("Content-Type", "application/json")
		if t.getToken != nil {
			if tok := t.getToken(); tok != "" {
				msgReq.Header.Set("Authorization", "Bearer "+tok)
			}
		}
		http.DefaultClient.Do(msgReq)
	}

	dueStr := ""
	if due != "" {
		dueStr = " (due: " + due + ")"
	}
	return TextResult(fmt.Sprintf("✅ Task assigned to @%s: %s%s", assignTo, task, dueStr))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Unused import prevention
var _ = time.Now
