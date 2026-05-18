// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package teams

import (
	"context"
	"testing"
	"time"
)

// Teams + Tasks tests — types, constants, helpers, lifecycle logic.

func TestTaskStatus_Constants(t *testing.T) {
	statuses := []string{TaskStatusPending, TaskStatusAssigned, TaskStatusInProgress,
		TaskStatusReview, TaskStatusCompleted, TaskStatusCancelled, TaskStatusFailed, TaskStatusBlocked}
	seen := map[string]bool{}
	for _, s := range statuses {
		if s == "" { t.Error("empty status constant") }
		if seen[s] { t.Errorf("duplicate status: %s", s) }
		seen[s] = true
	}
	if len(statuses) != 8 { t.Errorf("expected 8 statuses, got %d", len(statuses)) }
}

func TestPriority_Constants(t *testing.T) {
	if PriorityLow != 0 { t.Error("low should be 0") }
	if PriorityNormal != 1 { t.Error("normal should be 1") }
	if PriorityHigh != 2 { t.Error("high should be 2") }
	if PriorityUrgent != 3 { t.Error("urgent should be 3") }
	if PriorityUrgent <= PriorityHigh { t.Error("urgent should be > high") }
}

func TestTask_Fields(t *testing.T) {
	now := time.Now()
	task := Task{
		ID: "t1", TeamID: "team1", Subject: "Fix bug", Description: "Critical bug",
		Status: TaskStatusPending, Priority: PriorityHigh, TaskType: "bug",
		TaskNumber: 1, Identifier: "T-001-ab3f", CreatedAt: now,
	}
	if task.ID != "t1" { t.Error("wrong id") }
	if task.Priority != PriorityHigh { t.Error("wrong priority") }
	if task.Identifier != "T-001-ab3f" { t.Error("wrong identifier") }
}

func TestTask_FollowupFields(t *testing.T) {
	followup := time.Now().Add(24 * time.Hour)
	task := Task{
		FollowupAt: &followup, FollowupCount: 2, FollowupMax: 5,
		FollowupMsg: "Please update", FollowupChannel: "telegram", FollowupChatID: "chat123",
	}
	if task.FollowupCount != 2 { t.Error("wrong count") }
	if task.FollowupMax != 5 { t.Error("wrong max") }
}

func TestTask_LockFields(t *testing.T) {
	now := time.Now()
	expires := now.Add(30 * time.Minute)
	task := Task{LockedAt: &now, LockExpiresAt: &expires}
	if task.LockedAt == nil { t.Error("locked_at nil") }
	if task.LockExpiresAt.Before(now) { t.Error("expiry should be after lock") }
}

func TestTaskComment_Fields(t *testing.T) {
	c := TaskComment{ID: "c1", TaskID: "t1", AgentID: "agent1", Content: "Working on it", CommentType: "note"}
	if c.CommentType != "note" { t.Error("wrong type") }
}

func TestTaskComment_BlockerType(t *testing.T) {
	c := TaskComment{CommentType: "blocker", Content: "Waiting for API access"}
	if c.CommentType != "blocker" { t.Error("should be blocker") }
}

func TestTaskEvent_Fields(t *testing.T) {
	e := TaskEvent{TaskID: "t1", EventType: "assigned", ActorType: "agent", ActorID: "agent1"}
	if e.EventType != "assigned" { t.Error("wrong event type") }
	if e.ActorType != "agent" { t.Error("wrong actor type") }
}

func TestTaskAttachment_Fields(t *testing.T) {
	a := TaskAttachment{TaskID: "t1", TeamID: "team1", Path: "/docs/spec.pdf", FileSize: 1024, MimeType: "application/pdf"}
	if a.FileSize != 1024 { t.Error("wrong size") }
	if a.MimeType != "application/pdf" { t.Error("wrong mime") }
}

func TestScopeEntry_Fields(t *testing.T) {
	s := ScopeEntry{Channel: "telegram", ChatID: "chat123"}
	if s.Channel != "telegram" { t.Error("wrong channel") }
}

func TestRecoveredTask_Fields(t *testing.T) {
	r := RecoveredTask{ID: "t1", TeamID: "team1", TaskNumber: 5, Subject: "Stale task"}
	if r.TaskNumber != 5 { t.Error("wrong number") }
}

func TestTaskListOpts_Defaults(t *testing.T) {
	opts := TaskListOpts{}
	if opts.Limit != 0 { t.Error("default limit should be 0 (handled in ListTasks)") }
	if opts.Status != "" { t.Error("default status should be empty") }
}

func TestNullStr_Empty(t *testing.T) {
	result := nullStr("")
	if result != nil { t.Error("empty should return nil") }
}

func TestNullStr_Value(t *testing.T) {
	result := nullStr("hello")
	if result == nil { t.Fatal("non-empty should return pointer") }
	if *result != "hello" { t.Errorf("got %q", *result) }
}

func TestCoalesce_Empty(t *testing.T) {
	if coalesce("", "default") != "default" { t.Error("empty should use default") }
}

func TestCoalesce_Value(t *testing.T) {
	if coalesce("value", "default") != "value" { t.Error("non-empty should use value") }
}

func TestTeam_Fields(t *testing.T) {
	team := Team{ID: "t1", Name: "Engineering", LeadID: "agent1", TenantID: "tenant1"}
	if team.Name != "Engineering" { t.Error("wrong name") }
}

func TestMember_Fields(t *testing.T) {
	m := Member{AgentID: "agent1", Role: "lead"}
	if m.Role != "lead" { t.Error("wrong role") }
}

func TestEmbeddingProvider_Interface(t *testing.T) {
	// Verify the interface exists and has the right method
	var _ EmbeddingProvider = (*mockEmbedder)(nil)
}

type mockEmbedder struct{}
func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) { return nil, nil }
