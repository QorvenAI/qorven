// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// todoRegistry stores session-scoped TODO lists in memory.
// Keyed by session ID; evicted lazily (no expiry needed for agent sessions).
var todoRegistry = &todoStore{items: make(map[string][]todoItem)}

type todoItem struct {
	ID     int    `json:"id"`
	Text   string `json:"text"`
	Status string `json:"status"` // "pending" | "in_progress" | "done" | "skipped"
}

type todoStore struct {
	mu    sync.Mutex
	items map[string][]todoItem // sessionID → items
	seqs  map[string]int        // sessionID → next ID
}

func (s *todoStore) get(sessionID string) []todoItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.items[sessionID]
	if items == nil {
		return []todoItem{}
	}
	out := make([]todoItem, len(items))
	copy(out, items)
	return out
}

func (s *todoStore) set(sessionID string, items []todoItem) {
	s.mu.Lock()
	s.items[sessionID] = items
	s.mu.Unlock()
}

func (s *todoStore) nextID(sessionID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seqs == nil {
		s.seqs = make(map[string]int)
	}
	s.seqs[sessionID]++
	return s.seqs[sessionID]
}

// ── todo_write ────────────────────────────────────────────────────────────────

// TodoWriteTool replaces or updates the session's TODO list.
type TodoWriteTool struct{}

func NewTodoWriteTool() *TodoWriteTool { return &TodoWriteTool{} }
func (t *TodoWriteTool) Name() string  { return "todo_write" }
func (t *TodoWriteTool) Description() string {
	return `Manage your task list for this session. Use this to track what you need to do, what you're working on, and what's done.

Actions:
- set: replace the entire list with new items (use at the start of a multi-step task)
- update: change the status of one item by ID
- add: append a new item

Statuses: pending | in_progress | done | skipped`
}
func (t *TodoWriteTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string", "enum": []string{"set", "update", "add"},
				"description": "set=replace list, update=change one item status, add=append item",
			},
			"items": map[string]any{
				"type":        "array",
				"description": "For action=set: list of task strings to create as pending items",
				"items":       map[string]any{"type": "string"},
			},
			"id": map[string]any{
				"type":        "integer",
				"description": "For action=update: the item ID to update",
			},
			"status": map[string]any{
				"type": "string", "enum": []string{"pending", "in_progress", "done", "skipped"},
				"description": "For action=update: new status for the item",
			},
			"text": map[string]any{
				"type":        "string",
				"description": "For action=add: text of the new item",
			},
		},
		"required": []string{"action"},
	}
}

func (t *TodoWriteTool) Execute(ctx context.Context, args map[string]any) *Result {
	sessionID := SessionIDFromCtx(ctx)
	action, _ := args["action"].(string)

	switch action {
	case "set":
		rawItems, _ := args["items"].([]any)
		if len(rawItems) == 0 {
			return ErrorResult("items array is required for action=set")
		}
		newItems := make([]todoItem, 0, len(rawItems))
		for _, r := range rawItems {
			text, _ := r.(string)
			if text == "" {
				continue
			}
			id := todoRegistry.nextID(sessionID)
			newItems = append(newItems, todoItem{ID: id, Text: text, Status: "pending"})
		}
		todoRegistry.set(sessionID, newItems)
		return TextResult(fmt.Sprintf("TODO list set with %d items.", len(newItems)))

	case "add":
		text, _ := args["text"].(string)
		if text == "" {
			return ErrorResult("text is required for action=add")
		}
		items := todoRegistry.get(sessionID)
		id := todoRegistry.nextID(sessionID)
		items = append(items, todoItem{ID: id, Text: text, Status: "pending"})
		todoRegistry.set(sessionID, items)
		return TextResult(fmt.Sprintf("Added item %d: %s", id, text))

	case "update":
		var id int
		switch v := args["id"].(type) {
		case float64:
			id = int(v)
		case int:
			id = v
		default:
			return ErrorResult("id is required for action=update")
		}
		status, _ := args["status"].(string)
		if status == "" {
			return ErrorResult("status is required for action=update")
		}
		items := todoRegistry.get(sessionID)
		found := false
		for i := range items {
			if items[i].ID == id {
				items[i].Status = status
				found = true
				break
			}
		}
		if !found {
			return ErrorResult(fmt.Sprintf("item %d not found", id))
		}
		todoRegistry.set(sessionID, items)
		return TextResult(fmt.Sprintf("Item %d marked %s.", id, status))

	default:
		return ErrorResult("action must be set, add, or update")
	}
}

// ── todo_read ─────────────────────────────────────────────────────────────────

// TodoReadTool reads the current session TODO list.
type TodoReadTool struct{}

func NewTodoReadTool() *TodoReadTool { return &TodoReadTool{} }
func (t *TodoReadTool) Name() string { return "todo_read" }
func (t *TodoReadTool) Description() string {
	return "Read your current task list for this session. Use this to check what's pending, in progress, and done."
}
func (t *TodoReadTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"format": map[string]any{
				"type": "string", "enum": []string{"text", "json"},
				"description": "Output format. Default: text",
			},
		},
	}
}

func (t *TodoReadTool) Execute(ctx context.Context, args map[string]any) *Result {
	sessionID := SessionIDFromCtx(ctx)
	items := todoRegistry.get(sessionID)

	if len(items) == 0 {
		return TextResult("No TODO items. Use todo_write to create your task list.")
	}

	format, _ := args["format"].(string)
	if format == "json" {
		b, _ := json.Marshal(items)
		return TextResult(string(b))
	}

	icons := map[string]string{
		"pending":     "[ ]",
		"in_progress": "[→]",
		"done":        "[✓]",
		"skipped":     "[–]",
	}
	var sb strings.Builder
	counts := map[string]int{}
	for _, item := range items {
		icon := icons[item.Status]
		if icon == "" {
			icon = "[ ]"
		}
		sb.WriteString(fmt.Sprintf("  %s #%d %s\n", icon, item.ID, item.Text))
		counts[item.Status]++
	}
	sb.WriteString(fmt.Sprintf("\n%d total — %d pending, %d in progress, %d done",
		len(items), counts["pending"], counts["in_progress"], counts["done"]))
	return TextResult(sb.String())
}
