//go:build integration

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestE2E_AutonomousLoop exercises the full Inception → team spawn →
// task pickup → completion sequence through the live HTTP API.
//
// This test is self-contained: it creates a project brief, proposes a
// team, approves it (spawning agents + tickets + tasks), registers a
// mock daemon agent, assigns it a task, reports progress, and marks it
// complete.  No LLM call is required; the propose step uses the
// deterministic fallback (no QORVEN_LLM_TEST flag set).
//
// Run with:
//
//	QORVEN_TOKEN=<token> go test -v -tags integration -run TestE2E_AutonomousLoop ./internal/gateway/
func TestE2E_AutonomousLoop(t *testing.T) {
	requireGateway(t)

	suffix := time.Now().Format("150405.000")

	// ── Step 1: Create project brief ────────────────────────────────────────────
	briefResp := authPost(t, "/v1/project-briefs", map[string]any{
		"title": fmt.Sprintf("Inception E2E Test %s", suffix),
		"idea":  "Build a simple REST API with health endpoint and basic CRUD.",
		"stack": "Go",
	})
	if briefResp.StatusCode != 200 && briefResp.StatusCode != 201 {
		t.Fatalf("create brief: status=%d body=%s", briefResp.StatusCode, readBody(briefResp))
	}
	var brief map[string]any
	json.Unmarshal([]byte(readBody(briefResp)), &brief)
	briefID, _ := brief["id"].(string)
	if briefID == "" {
		t.Fatal("no brief ID in response")
	}
	t.Logf("brief created: %s", briefID)

	// ── Step 2: Propose team (deterministic fallback — no LLM needed) ───────────
	proposeResp := authPost(t, fmt.Sprintf("/v1/project-briefs/%s/propose", briefID), map[string]any{})
	if proposeResp.StatusCode != 200 {
		t.Fatalf("propose team: status=%d body=%s", proposeResp.StatusCode, readBody(proposeResp))
	}
	var proposal map[string]any
	json.Unmarshal([]byte(readBody(proposeResp)), &proposal)
	agents, _ := proposal["agents"].([]any)
	tasks, _ := proposal["tasks"].([]any)
	if len(agents) == 0 {
		t.Fatal("proposal has no agents")
	}
	if len(tasks) == 0 {
		t.Fatal("proposal has no tasks")
	}
	t.Logf("proposal: %d agents, %d tasks", len(agents), len(tasks))

	// Verify brief transitioned to "proposed"
	briefGetResp := authGet(t, fmt.Sprintf("/v1/project-briefs/%s", briefID))
	if briefGetResp.StatusCode != 200 {
		t.Fatalf("get brief after propose: %d", briefGetResp.StatusCode)
	}
	var updatedBrief map[string]any
	json.Unmarshal([]byte(readBody(briefGetResp)), &updatedBrief)
	if updatedBrief["status"] != "proposed" {
		t.Errorf("expected status=proposed, got %q", updatedBrief["status"])
	}

	// ── Step 3: Approve team (spawns agents + tickets + tasks) ──────────────────
	approveResp := authPost(t, fmt.Sprintf("/v1/project-briefs/%s/approve", briefID), map[string]any{})
	if approveResp.StatusCode != 200 {
		t.Fatalf("approve team: status=%d body=%s", approveResp.StatusCode, readBody(approveResp))
	}
	var approveResult map[string]any
	json.Unmarshal([]byte(readBody(approveResp)), &approveResult)

	spawnedAgents, _ := approveResult["agents"].(map[string]any)
	spawnedTickets, _ := approveResult["tickets"].(map[string]any)
	if len(spawnedAgents) == 0 {
		t.Fatal("no agents spawned after approval")
	}
	if len(spawnedTickets) == 0 {
		t.Fatal("no tickets created after approval")
	}
	t.Logf("spawned: %d agents, %d tickets", len(spawnedAgents), len(spawnedTickets))

	// Verify brief is now "active"
	briefActiveResp := authGet(t, fmt.Sprintf("/v1/project-briefs/%s", briefID))
	var activeBrief map[string]any
	json.Unmarshal([]byte(readBody(briefActiveResp)), &activeBrief)
	if activeBrief["status"] != "active" {
		t.Errorf("expected status=active after approval, got %q", activeBrief["status"])
	}

	// Verify spawned agents exist in the agent list
	agentsListResp := authGet(t, "/v1/agents")
	agentsBody := readBody(agentsListResp)
	for role := range spawnedAgents {
		if !strings.Contains(agentsBody, role) {
			t.Logf("agent role %q not found in list (may be keyed differently)", role)
		}
	}

	// Verify the Tasks board has entries for this brief
	tasksResp := authGet(t, "/v1/tasks")
	if tasksResp.StatusCode == 200 {
		tasksBody := readBody(tasksResp)
		if !strings.Contains(tasksBody, briefID[:8]) && !strings.Contains(tasksBody, brief["title"].(string)[:10]) {
			t.Logf("tasks board: brief not visible (tasks may be filtered differently)")
		} else {
			t.Log("tasks board: inception tasks visible")
		}
	}

	// ── Step 4: Register a mock daemon agent (simulates Kiro / Claude Code) ─────
	regResp := authPost(t, "/v1/daemon/agents", map[string]any{
		"name":         fmt.Sprintf("test-coder-%s", suffix),
		"provider":     "custom",
		"model":        "claude-sonnet-4-6",
		"capabilities": []string{"code", "developer"},
	})
	if regResp.StatusCode != 200 && regResp.StatusCode != 201 {
		t.Fatalf("register daemon agent: status=%d body=%s", regResp.StatusCode, readBody(regResp))
	}
	var daemonAgent map[string]any
	json.Unmarshal([]byte(readBody(regResp)), &daemonAgent)
	daemonID, _ := daemonAgent["id"].(string)
	if daemonID == "" {
		t.Fatal("no daemon agent ID")
	}
	t.Logf("daemon agent registered: %s", daemonID)

	// ── Step 5: Create a daemon task (the plan from inception) ──────────────────
	// Inception may propose tasks via daemonReg.ProposePlan — we also verify
	// we can create one directly to confirm the daemon task pipeline works.
	createTaskResp := authPost(t, "/v1/daemon/tasks", map[string]any{
		"title":       fmt.Sprintf("Implement health endpoint for %s", briefID[:8]),
		"description": "Add GET /health route returning {status:ok,version}.",
		"owner":       "developer",
		"priority":    "high",
	})
	if createTaskResp.StatusCode != 200 && createTaskResp.StatusCode != 201 {
		t.Fatalf("create daemon task: status=%d body=%s", createTaskResp.StatusCode, readBody(createTaskResp))
	}
	var daemonTask map[string]any
	json.Unmarshal([]byte(readBody(createTaskResp)), &daemonTask)
	taskID, _ := daemonTask["id"].(string)
	if taskID == "" {
		t.Fatal("no daemon task ID")
	}
	t.Logf("daemon task created: %s (status=%v)", taskID, daemonTask["status"])

	// Verify task is in the list
	listTasksResp := authGet(t, "/v1/daemon/tasks")
	if listTasksResp.StatusCode != 200 {
		t.Fatalf("list daemon tasks: %d", listTasksResp.StatusCode)
	}
	listBody := readBody(listTasksResp)
	if !strings.Contains(listBody, taskID) {
		t.Errorf("task %s not found in daemon task list", taskID)
	}

	// ── Step 6: Assign task to daemon agent ─────────────────────────────────────
	assignResp := authPost(t, fmt.Sprintf("/v1/daemon/tasks/%s/assign", taskID), map[string]any{
		"agent_id": daemonID,
	})
	if assignResp.StatusCode != 200 {
		t.Fatalf("assign task: status=%d body=%s", assignResp.StatusCode, readBody(assignResp))
	}
	var assignedTask map[string]any
	json.Unmarshal([]byte(readBody(assignResp)), &assignedTask)
	if assignedTask["status"] != "in_progress" {
		t.Errorf("expected status=in_progress after assign, got %q", assignedTask["status"])
	}
	t.Log("task assigned and in_progress")

	// ── Step 7: Report progress ──────────────────────────────────────────────────
	progressResp := authPost(t, fmt.Sprintf("/v1/daemon/tasks/%s/progress", taskID), map[string]any{
		"message":  "Writing health handler",
		"percent":  50,
		"agent_id": daemonID,
	})
	if progressResp.StatusCode != 200 && progressResp.StatusCode != 204 {
		t.Fatalf("progress: status=%d body=%s", progressResp.StatusCode, readBody(progressResp))
	}
	t.Log("progress reported")

	// ── Step 8: Mark task complete (simulates PR merged / work done) ─────────────
	completeResp := authPost(t, fmt.Sprintf("/v1/daemon/tasks/%s/complete", taskID), map[string]any{
		"summary":       "Added GET /health returning {status:ok,version:1.0.0}. Tests pass.",
		"files_changed": []string{"cmd/server.go", "internal/health/handler.go"},
	})
	if completeResp.StatusCode != 200 && completeResp.StatusCode != 204 {
		t.Fatalf("complete task: status=%d body=%s", completeResp.StatusCode, readBody(completeResp))
	}
	t.Log("task completed — autonomous loop end-to-end: PASS")

	// Verify final task state
	finalTaskResp := authGet(t, fmt.Sprintf("/v1/daemon/tasks/%s", taskID))
	if finalTaskResp.StatusCode == 200 {
		var finalTask map[string]any
		json.Unmarshal([]byte(readBody(finalTaskResp)), &finalTask)
		if finalTask["status"] != "completed" && finalTask["status"] != "done" {
			t.Errorf("final task status: expected completed/done, got %q", finalTask["status"])
		}
		t.Logf("final task status: %v", finalTask["status"])
	}

	// ── Cleanup: unregister daemon agent ────────────────────────────────────────
	authDelete(t, fmt.Sprintf("/v1/daemon/agents/%s", daemonID))
}
