package gateway

import (
	"encoding/json"
	"testing"
)

func TestBuildProjectEvent(t *testing.T) {
	row, ev := buildProjectEvent("brief-1", "task_started", "Backend coding started",
		map[string]any{"role": "backend-dev"}, "task-9", "agent-3")
	if row.ProjectBriefID != "brief-1" || row.Type != "task_started" || row.Title == "" {
		t.Fatalf("row not populated: %+v", row)
	}
	if row.TaskID != "task-9" || row.AgentID != "agent-3" {
		t.Errorf("task/agent ids lost: %+v", row)
	}
	b, _ := json.Marshal(ev.Data)
	var d map[string]any
	_ = json.Unmarshal(b, &d)
	if d["project_id"] != "brief-1" {
		t.Errorf("event data missing project_id: %s", b)
	}
	// task_started surfaces as the generic project_updated refresh (which the
	// timeline subscribes to); the original type is preserved in the payload.
	if ev.Type != "project_updated" {
		t.Errorf("event type mismatch: %s", ev.Type)
	}
	if d["type"] != "task_started" {
		t.Errorf("payload should preserve original type: %s", b)
	}
}

func TestBuildProjectEvent_TypeMapsToRealtime(t *testing.T) {
	_, ev := buildProjectEvent("b", "done", "", nil, "", "")
	if ev.Type != "task_done" {
		t.Errorf("done should map to task_done, got %s", ev.Type)
	}
	_, ev2 := buildProjectEvent("b", "gate_decision", "", nil, "", "")
	if ev2.Type != "project_updated" {
		t.Errorf("gate_decision should map to project_updated, got %s", ev2.Type)
	}
}
