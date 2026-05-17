// Copyright 2026 Tekky AI Academy LLP. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// GitHubTask represents an autonomous development task driven by a GitHub issue.
// An agent registers a task when it starts working on an issue. The task queue
// tracks state across QOROS ticks so the agent can retry after test failures
// without human intervention.
type GitHubTask struct {
	// Identity
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// GitHub context
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	IssueNumber int    `json:"issue_number"`
	Branch      string `json:"branch"`
	PRURL       string `json:"pr_url,omitempty"`
	PRNumber    int    `json:"pr_number,omitempty"`

	// Execution state
	Phase      GitHubTaskPhase `json:"phase"`
	RetryCount int             `json:"retry_count"`
	MaxRetries int             `json:"max_retries"`

	// Last test output — fed back to agent on retry
	LastTestOutput string `json:"last_test_output,omitempty"`
	LastError      string `json:"last_error,omitempty"`

	// Room to post progress updates to
	RoomID string `json:"room_id,omitempty"`
}

// GitHubTaskPhase tracks where in the dev loop an agent is.
type GitHubTaskPhase string

const (
	PhaseReading    GitHubTaskPhase = "reading"     // reading the issue
	PhaseBranching  GitHubTaskPhase = "branching"   // creating branch
	PhaseCoding     GitHubTaskPhase = "coding"      // writing code
	PhaseTesting    GitHubTaskPhase = "testing"     // running tests
	PhaseFixing     GitHubTaskPhase = "fixing"      // fixing failing tests
	PhaseOpeningPR  GitHubTaskPhase = "opening_pr"  // opening pull request
	PhaseAwaitingCI GitHubTaskPhase = "awaiting_ci" // waiting for CI
	PhaseComplete   GitHubTaskPhase = "complete"    // merged or closed
	PhaseBlocked    GitHubTaskPhase = "blocked"     // needs human (escalated)
)

// PhasePrompt returns the QOROS tick prompt for a given phase.
// This is what the agent sees when the tick fires and it has active tasks.
func (t *GitHubTask) PhasePrompt() string {
	base := fmt.Sprintf(
		"## Active GitHub Task\nIssue: %s/%s#%d | Branch: `%s` | Phase: %s | Retries: %d/%d",
		t.Owner, t.Repo, t.IssueNumber, t.Branch, t.Phase, t.RetryCount, t.MaxRetries,
	)
	if t.PRURL != "" {
		base += fmt.Sprintf("\nPR: %s", t.PRURL)
	}

	switch t.Phase {
	case PhaseReading:
		return base + "\n\nCall gh_read_issue to fully understand the requirements before writing any code."

	case PhaseBranching:
		return base + fmt.Sprintf(
			"\n\nCreate branch `%s` using gh_create_branch, then switch to the coding phase.",
			t.Branch,
		)

	case PhaseCoding:
		return base + "\n\nWrite the code. Use exec to run tests when done. Update task phase to 'testing'."

	case PhaseTesting:
		if t.LastTestOutput != "" {
			return base + fmt.Sprintf(
				"\n\nTest run failed. Output:\n```\n%s\n```\n\nFix the failing tests. You have %d retries remaining.",
				truncateOutput(t.LastTestOutput, 2000), t.MaxRetries-t.RetryCount,
			)
		}
		return base + "\n\nRun tests with exec. If they pass, open the PR. If they fail, fix and retry."

	case PhaseFixing:
		return base + fmt.Sprintf(
			"\n\nFix the test failures from the last run:\n```\n%s\n```",
			truncateOutput(t.LastTestOutput, 2000),
		)

	case PhaseOpeningPR:
		return base + fmt.Sprintf(
			"\n\nTests are passing. Open a PR with gh_open_pr: head=`%s`, body should include 'Closes #%d'. Post the PR URL to the room.",
			t.Branch, t.IssueNumber,
		)

	case PhaseAwaitingCI:
		return base + fmt.Sprintf(
			"\n\nPR #%d is open. Check CI status with gh_list_pr_checks(pr=%d). If all checks pass and coordinator approved in room, call gh_merge_pr.",
			t.PRNumber, t.PRNumber,
		)

	case PhaseBlocked:
		return base + fmt.Sprintf(
			"\n\nThis task is blocked: %s\n\nPost a detailed status update to the room and wait for human input.",
			t.LastError,
		)

	case PhaseComplete:
		return base + "\n\nTask complete. Clean up and look for the next open issue to work on."
	}

	return base
}

func truncateOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... (truncated)"
}

// GitHubTaskQueue manages autonomous GitHub tasks across all agents.
// Thread-safe. Designed to integrate with QOROS tick callbacks.
type GitHubTaskQueue struct {
	mu    sync.RWMutex
	tasks map[string]*GitHubTask // id → task
}

// NewGitHubTaskQueue creates a new task queue.
func NewGitHubTaskQueue() *GitHubTaskQueue {
	return &GitHubTaskQueue{
		tasks: make(map[string]*GitHubTask),
	}
}

// Register creates a new GitHub task for an agent. Returns the task ID.
func (q *GitHubTaskQueue) Register(agentID, owner, repo string, issueNumber int, branch, roomID string) string {
	q.mu.Lock()
	defer q.mu.Unlock()

	id := fmt.Sprintf("gh-%s-%s-%s-%d", agentID[:min8(agentID)], owner, repo, issueNumber)

	// Overwrite if already exists (idempotent re-registration)
	task := &GitHubTask{
		ID:          id,
		AgentID:     agentID,
		Owner:       owner,
		Repo:        repo,
		IssueNumber: issueNumber,
		Branch:      branch,
		Phase:       PhaseReading,
		MaxRetries:  3,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		RoomID:      roomID,
	}
	q.tasks[id] = task

	slog.Info("github_task.registered",
		"agent", agentID, "repo", owner+"/"+repo,
		"issue", issueNumber, "branch", branch)
	return id
}

// Advance moves a task to the next phase. Call this when an agent completes a step.
func (q *GitHubTaskQueue) Advance(taskID string, phase GitHubTaskPhase) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if t, ok := q.tasks[taskID]; ok {
		t.Phase = phase
		t.UpdatedAt = time.Now()
		slog.Info("github_task.advanced", "task", taskID, "phase", phase)
	}
}

// RecordTestFailure logs failing test output and increments retry count.
// Returns true if retries remain, false if the task should be escalated.
func (q *GitHubTaskQueue) RecordTestFailure(taskID, output string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	t, ok := q.tasks[taskID]
	if !ok {
		return false
	}
	t.LastTestOutput = output
	t.RetryCount++
	t.UpdatedAt = time.Now()

	if t.RetryCount >= t.MaxRetries {
		t.Phase = PhaseBlocked
		t.LastError = fmt.Sprintf("Tests still failing after %d retries. Last output:\n%s",
			t.MaxRetries, truncateOutput(output, 500))
		slog.Warn("github_task.blocked", "task", taskID, "retries", t.RetryCount)
		return false
	}

	t.Phase = PhaseFixing
	slog.Info("github_task.retry", "task", taskID, "retry", t.RetryCount, "max", t.MaxRetries)
	return true
}

// RecordPR records an opened PR for a task.
func (q *GitHubTaskQueue) RecordPR(taskID, prURL string, prNumber int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if t, ok := q.tasks[taskID]; ok {
		t.PRURL = prURL
		t.PRNumber = prNumber
		t.Phase = PhaseAwaitingCI
		t.UpdatedAt = time.Now()
	}
}

// Complete marks a task finished.
func (q *GitHubTaskQueue) Complete(taskID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if t, ok := q.tasks[taskID]; ok {
		t.Phase = PhaseComplete
		t.UpdatedAt = time.Now()
	}
}

// ActiveTasksForAgent returns all non-complete tasks for a given agent, sorted
// by priority: blocked first (needs escalation), then by phase order.
func (q *GitHubTaskQueue) ActiveTasksForAgent(agentID string) []*GitHubTask {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var tasks []*GitHubTask
	for _, t := range q.tasks {
		if t.AgentID == agentID && t.Phase != PhaseComplete {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

// BuildTickContext returns a prompt section injected into every QOROS tick for
// an agent that has active GitHub tasks. Returns "" if no active tasks.
func (q *GitHubTaskQueue) BuildTickContext(agentID string) string {
	tasks := q.ActiveTasksForAgent(agentID)
	if len(tasks) == 0 {
		return ""
	}

	// Show the most urgent task (blocked > awaiting_ci > coding > others)
	priority := map[GitHubTaskPhase]int{
		PhaseBlocked:    0,
		PhaseFixing:     1,
		PhaseTesting:    2,
		PhaseCoding:     3,
		PhaseOpeningPR:  4,
		PhaseAwaitingCI: 5,
		PhaseBranching:  6,
		PhaseReading:    7,
	}
	best := tasks[0]
	for _, t := range tasks[1:] {
		if priority[t.Phase] < priority[best.Phase] {
			best = t
		}
	}

	return best.PhasePrompt()
}

// GlobalGitHubTaskQueue is the singleton queue used by the agent loop and tools.
// Initialized once in gateway.go.
var GlobalGitHubTaskQueue = NewGitHubTaskQueue()

// ─── gh_task_register tool ────────────────────────────────────────────────────
// This is a special agent-facing tool that lets agents register GitHub tasks
// so the QOROS loop knows to keep nudging them. Agents call this after reading
// a GitHub issue — it's the formal commitment to work on the issue.

func min8(s string) int {
	if len(s) < 8 {
		return len(s)
	}
	return 8
}

// ListAll returns a snapshot of all tasks (any phase, any agent).
func (q *GitHubTaskQueue) ListAll() []*GitHubTask {
	q.mu.RLock()
	defer q.mu.RUnlock()
	out := make([]*GitHubTask, 0, len(q.tasks))
	for _, t := range q.tasks {
		out = append(out, t)
	}
	return out
}

// Get returns a single task by ID, or nil if not found.
func (q *GitHubTaskQueue) Get(id string) *GitHubTask {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.tasks[id]
}

// GitHubTaskContext is injected into every QOROS tick prompt when active tasks exist.
// It's appended to the agent's system prompt section so the agent knows what to do next.
func GitHubTaskContext(ctx context.Context, agentID string) string {
	return GlobalGitHubTaskQueue.BuildTickContext(agentID)
}
