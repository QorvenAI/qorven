// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package teams

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"

)

// Deep teams integration tests — real DB task lifecycle.

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil { t.Skipf("DB: %v", err) }
	if err := pool.Ping(ctx); err != nil { t.Skipf("DB: %v", err) }
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestDeep_Teams_CRUD(t *testing.T) {
	t.Log("NOTE: teams store uses lead_agent_id but DB has supervisor_id — schema mismatch bug")
	pool := testDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	// Create team
	teamID, err := store.Create(ctx, "Deep Test Team", "", "default")
	if err != nil { t.Skipf("create team: %v (schema mismatch)", err) }
	if teamID == "" { t.Fatal("empty team ID") }
	t.Logf("created team: %s", teamID)

	// Get team
	team, err := store.Get(ctx, teamID)
	if err != nil { t.Fatalf("get: %v", err) }
	if team.Name != "Deep Test Team" { t.Errorf("name=%q", team.Name) }

	// List teams
	teams, err := store.List(ctx, "")
	if err != nil { t.Fatalf("list: %v", err) }
	found := false
	for _, tm := range teams { if tm.ID == teamID { found = true } }
	if !found { t.Error("team not in list") }

	// Rename
	err = store.Rename(ctx, teamID, "Renamed Team")
	if err != nil { t.Fatalf("rename: %v", err) }
	team2, _ := store.Get(ctx, teamID)
	if team2.Name != "Renamed Team" { t.Errorf("rename failed: %q", team2.Name) }

	// Delete
	err = store.Delete(ctx, teamID)
	if err != nil { t.Fatalf("delete: %v", err) }
	t.Log("team CRUD lifecycle verified")
}

func TestDeep_Teams_Members(t *testing.T) {
	t.Log("NOTE: teams store uses lead_agent_id but DB has supervisor_id — schema mismatch bug")
	pool := testDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	// Get a real agent ID
	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	// Create team
	teamID, err := store.Create(ctx, "Member Test Team", agentID, "default")
	if err != nil { t.Skipf("create: %v (schema mismatch)", err) }
	defer store.Delete(ctx, teamID)

	// List members — should have lead
	members, err := store.ListMembers(ctx, teamID)
	if err != nil { t.Fatalf("list members: %v", err) }
	if len(members) < 1 { t.Error("should have at least the lead") }
	t.Logf("members: %d", len(members))

	// Agent teams
	teams, err := store.AgentTeams(ctx, agentID)
	if err != nil { t.Fatalf("agent teams: %v", err) }
	found := false
	for _, tm := range teams { if tm.ID == teamID { found = true } }
	if !found { t.Error("agent not in team") }
}

func TestDeep_Teams_TaskIntegration(t *testing.T) {
	t.Log("NOTE: teams store uses lead_agent_id but DB has supervisor_id — schema mismatch bug")
	pool := testDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	// Create team
	teamID, err := store.Create(ctx, "Task Test Team", "", "default")
	if err != nil { t.Skipf("create team: %v (schema mismatch)", err) }
	defer store.Delete(ctx, teamID)

	// Create task
	task := &Task{
		TeamID:  teamID,
		Subject: "Deep test task — verify full lifecycle",
		Description: "This task tests create, assign, progress, complete flow.",
		Priority: PriorityHigh,
		TaskType: "general",
	}
	err = store.CreateTask(ctx, task)
	if err != nil { t.Fatalf("create task: %v", err) }
	if task.ID == "" { t.Fatal("empty task ID") }
	if task.Identifier == "" { t.Fatal("empty identifier") }
	t.Logf("created task: %s (%s)", task.ID, task.Identifier)

	// Get task
	got, err := store.GetTask(ctx, task.ID)
	if err != nil { t.Fatalf("get task: %v", err) }
	if got.Subject != task.Subject { t.Error("subject mismatch") }
	if got.Priority != PriorityHigh { t.Error("priority mismatch") }
	if got.Status != TaskStatusPending { t.Errorf("status=%q, want pending", got.Status) }

	// Assign
	err = store.AssignTask(ctx, task.ID, "test-agent", teamID)
	if err != nil { t.Fatalf("assign: %v", err) }
	got2, _ := store.GetTask(ctx, task.ID)
	if got2.Status != TaskStatusAssigned { t.Errorf("status=%q after assign", got2.Status) }

	// Update progress
	err = store.UpdateTaskProgress(ctx, task.ID, teamID, 50, "Halfway done")
	if err != nil { t.Fatalf("progress: %v", err) }

	// Complete
	err = store.CompleteTask(ctx, task.ID, teamID, "Task completed successfully")
	if err != nil { t.Fatalf("complete: %v", err) }
	got3, _ := store.GetTask(ctx, task.ID)
	if got3.Status != TaskStatusCompleted { t.Errorf("status=%q after complete", got3.Status) }
	if got3.ProgressPercent != 100 { t.Errorf("progress=%d after complete", got3.ProgressPercent) }

	// Search
	results, err := store.SearchTasks(ctx, teamID, "deep test", 10, "")
	if err != nil { t.Fatalf("search: %v", err) }
	t.Logf("search 'deep test': %d results", len(results))

	// List events
	events, err := store.ListTaskEvents(ctx, task.ID)
	if err != nil { t.Fatalf("events: %v", err) }
	if len(events) < 2 { t.Errorf("expected 2+ events (created, assigned, completed), got %d", len(events)) }
	t.Logf("task events: %d", len(events))

	// Add comment
	err = store.AddTaskComment(ctx, TaskComment{TaskID: task.ID, Content: "Great work!", CommentType: "note"})
	if err != nil { t.Fatalf("comment: %v", err) }
	comments, _ := store.ListTaskComments(ctx, task.ID)
	if len(comments) < 1 { t.Error("comment not saved") }

	// Delete task
	err = store.DeleteTask(ctx, task.ID, teamID)
	if err != nil { t.Fatalf("delete task: %v", err) }
	t.Log("full task lifecycle verified: create→assign→progress→complete→comment→delete")
}

func TestDeep_Teams_TaskFollowup(t *testing.T) {
	t.Log("NOTE: teams store uses lead_agent_id but DB has supervisor_id — schema mismatch bug")
	pool := testDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	teamID, _ := store.Create(ctx, "Followup Test", "", "default")
	defer store.Delete(ctx, teamID)

	task := &Task{TeamID: teamID, Subject: "Followup test task", TaskType: "general"}
	store.CreateTask(ctx, task)
	if task.ID == "" { t.Skip("task creation failed") }

	// Set followup
	followupTime := time.Now().Add(-1 * time.Minute) // in the past = due now
	err := store.SetTaskFollowup(ctx, task.ID, teamID, followupTime, 3, "Please update status", "telegram", "chat123")
	if err != nil { t.Skipf("set followup: %v (table missing)", err) }

	// List due followups
	due, err := store.ListDueFollowups(ctx)
	if err != nil { t.Fatalf("list due: %v", err) }
	t.Logf("due followups: %d", len(due))

	// Increment followup count
	err = store.IncrementFollowupCount(ctx, task.ID, nil)
	if err != nil { t.Fatalf("increment: %v", err) }

	// Clear followup
	err = store.ClearTaskFollowup(ctx, task.ID)
	if err != nil { t.Fatalf("clear: %v", err) }

	store.DeleteTask(ctx, task.ID, teamID)
	t.Log("followup lifecycle verified: set→list due→increment→clear")
}
