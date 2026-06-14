// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"
)

// TestResolveTaskProject_NilSafe verifies nil-safety guards without needing a DB.
func TestResolveTaskProject_NilSafe(t *testing.T) {
	l := &Loop{} // no store, no tenant
	if got := l.resolveTaskProject(context.Background(), "task-1"); got != "" {
		t.Fatalf("nil store should yield empty, got %q", got)
	}
	if got := l.resolveTaskProject(context.Background(), ""); got != "" {
		t.Fatalf("empty taskID should yield empty, got %q", got)
	}
}

// TestResolveTaskProject_DB exercises the resolver against a real DB.
// It seeds a project, a task linked to it, and one task without a project,
// then checks the resolver returns the right values.
func TestResolveTaskProject_DB(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil {
		t.Skipf("DB unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("DB ping failed: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	const tenantID = "00000000-0000-0000-0000-000000000001"
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	// Create a budget_projects row to act as the project.
	var projectID string
	err = pool.QueryRow(ctx,
		`INSERT INTO budget_projects (tenant_id, name) VALUES ($1::uuid, $2) RETURNING id::text`,
		tenantID, "test-proj-"+suffix,
	).Scan(&projectID)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	t.Logf("project: %s", projectID)

	// Create an agent to satisfy tasks FK (if tasks has agent_id).
	store := NewStore(pool)
	ag, err := store.Create(ctx, tenantID, CreateAgentInput{
		AgentKey:     "resolve-test-" + suffix,
		Model:        "gpt-4o-mini",
		SystemPrompt: "test",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Seed a task linked to the project.
	var taskWithProject string
	err = pool.QueryRow(ctx,
		`INSERT INTO tasks (tenant_id, agent_id, title, state, project_id)
		 VALUES ($1::uuid, $2::uuid, $3, 'pending', $4::uuid)
		 RETURNING id::text`,
		tenantID, ag.ID, "task-with-proj-"+suffix, projectID,
	).Scan(&taskWithProject)
	if err != nil {
		t.Fatalf("insert task with project: %v", err)
	}

	// Seed a task with no project.
	var taskNoProject string
	err = pool.QueryRow(ctx,
		`INSERT INTO tasks (tenant_id, agent_id, title, state)
		 VALUES ($1::uuid, $2::uuid, $3, 'pending')
		 RETURNING id::text`,
		tenantID, ag.ID, "task-no-proj-"+suffix,
	).Scan(&taskNoProject)
	if err != nil {
		t.Fatalf("insert task without project: %v", err)
	}

	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM tasks WHERE id = $1 OR id = $2`, taskWithProject, taskNoProject)
		pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, ag.ID)
		pool.Exec(context.Background(), `DELETE FROM budget_projects WHERE id = $1::uuid`, projectID)
	})

	l := &Loop{agentStore: store, tenantID: tenantID}

	// Task with project → should return the project ID.
	got := l.resolveTaskProject(ctx, taskWithProject)
	if got != projectID {
		t.Errorf("task with project: got %q, want %q", got, projectID)
	}

	// Task without project → should return "".
	got = l.resolveTaskProject(ctx, taskNoProject)
	if got != "" {
		t.Errorf("task without project: got %q, want empty", got)
	}

	// Unknown task ID → should return "".
	got = l.resolveTaskProject(ctx, "00000000-0000-0000-0000-000000000000")
	if got != "" {
		t.Errorf("unknown task: got %q, want empty", got)
	}
}
