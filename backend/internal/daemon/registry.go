// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package daemon manages the multi-agent task daemon: a pool of registered
// external agent instances (Kiro CLI, Claude Code, etc.) that receive tasks
// via SSE push, execute them, and report back results.
//
// Architecture:
//   - 1 task per agent instance (no context pollution / file conflicts)
//   - N instances of the same provider = N concurrent tasks
//   - SSE push for task dispatch; one-time poll on reconnect for recovery
//   - Plans with >1 task require human approval via the approvals system
package daemon

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── Provider interface ───────────────────────────────────────────────────────

// AgentProvider is the interface all external agent integrations satisfy.
// An implementation exists per provider type (kiro_cli, claude_code, etc.).
type AgentProvider interface {
	// ProviderID returns the canonical provider type, e.g. "kiro_cli".
	ProviderID() string

	// Name returns a human-readable display name, e.g. "Kiro CLI".
	Name() string

	// Capabilities returns the set of task types this provider handles.
	// Known values: "code", "review", "plan", "test", "research", "frontend", "backend"
	Capabilities() []string

	// Dispatch sends a task to the agent. Non-blocking; progress arrives
	// via the channel returned by the registry's Subscribe method.
	Dispatch(ctx context.Context, task *Task) error

	// Cancel stops an in-progress task. No-op if task is not running.
	Cancel(ctx context.Context, taskID string) error

	// Ping checks whether the agent process is alive. Returns nil if reachable.
	Ping(ctx context.Context) error
}

// ─── Core types ───────────────────────────────────────────────────────────────

// AgentStatus is the live snapshot for a registered agent instance.
type AgentStatus string

const (
	StatusIdle    AgentStatus = "idle"
	StatusWorking AgentStatus = "working"
	StatusError   AgentStatus = "error"
	StatusOffline AgentStatus = "offline"
)

// AgentInstance is a registered external agent with its runtime state.
type AgentInstance struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Provider     string      `json:"provider"`      // provider type: "kiro_cli", "claude_code", etc.
	Model        string      `json:"model"`         // e.g. "claude-opus-4-7"
	Capabilities []string    `json:"capabilities"`  // "code", "review", "frontend", "backend", …
	Status       AgentStatus `json:"status"`
	CurrentTask  string      `json:"current_task_id,omitempty"`
	RegisteredAt time.Time   `json:"registered_at"`
	LastSeenAt   time.Time   `json:"last_seen_at"`
	TasksDone    int         `json:"tasks_completed"`
	TasksFailed  int         `json:"tasks_failed"`

	provider AgentProvider // implementation; nil for SSE-only agents
}

// Task is a unit of work dispatched to one agent instance.
type Task struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Owner       string            `json:"owner"`       // AgentInstance.ID
	Priority    string            `json:"priority"`    // "high" | "normal" | "low"
	Status      TaskStatus        `json:"status"`
	DependsOn   []string          `json:"depends_on"`
	CreatedBy   string            `json:"created_by"`  // agent id or "human"
	Context     map[string]any    `json:"context,omitempty"`
	Summary     string            `json:"summary,omitempty"`
	Error       string            `json:"error,omitempty"`
	FilesChanged []string         `json:"files_changed,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	DurationMS  int64             `json:"duration_ms,omitempty"`
}

// TaskStatus is the lifecycle state of a task.
type TaskStatus string

const (
	TaskQueued     TaskStatus = "queued"
	TaskInProgress TaskStatus = "in_progress"
	TaskDone       TaskStatus = "done"
	TaskFailed     TaskStatus = "failed"
	TaskCancelled  TaskStatus = "cancelled"
)

// Plan is a proposed set of tasks requiring human approval before execution.
type Plan struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	ProposedBy  string      `json:"proposed_by"`
	Status      PlanStatus  `json:"status"`
	Tasks       []PlanTask  `json:"tasks"`
	CreatedAt   time.Time   `json:"created_at"`
	DecidedAt   *time.Time  `json:"decided_at,omitempty"`
	DecidedBy   string      `json:"decided_by,omitempty"`
	Modifications string    `json:"modifications,omitempty"` // human edits at approval time
}

// PlanStatus is the lifecycle of a plan.
type PlanStatus string

const (
	PlanPending   PlanStatus = "pending"
	PlanApproved  PlanStatus = "approved"
	PlanRejected  PlanStatus = "rejected"
	PlanExecuting PlanStatus = "executing"
	PlanDone      PlanStatus = "done"
)

// PlanTask is a task stub inside a Plan (not yet a full Task until approved).
type PlanTask struct {
	ID                string         `json:"id"`
	Title             string         `json:"title"`
	OwnerCapability   string         `json:"owner_capability"` // "frontend" | "backend" | "code" | …
	Priority          string         `json:"priority"`
	EstimatedMinutes  int            `json:"estimated_minutes,omitempty"`
	// Routing metadata — forwarded into Task.Context so external agents
	// (Kiro, Claude Code) know why they were spawned without extra API calls.
	TicketID          string         `json:"ticket_id,omitempty"`
	OriginID          string         `json:"origin_id,omitempty"`   // project_brief_id or plan_node_id
	OriginType        string         `json:"origin_type,omitempty"` // "project_brief" | "plan_node"
	ExtraContext      map[string]any `json:"extra_context,omitempty"`
}

// ─── Events ───────────────────────────────────────────────────────────────────

// EventType classifies a registry event for SSE dispatch.
type EventType string

const (
	EvtAgentRegistered   EventType = "agent_registered"
	EvtAgentStatus       EventType = "agent_status"
	EvtAgentUnregistered EventType = "agent_unregistered"
	EvtTaskCreated       EventType = "task_created"
	EvtTaskAssigned      EventType = "task_assigned"
	EvtTaskProgress      EventType = "task_progress"
	EvtTaskFile          EventType = "task_file"
	EvtTaskDone          EventType = "task_done"
	EvtTaskFailed        EventType = "task_failed"
	EvtPlanProposed      EventType = "plan_proposed"
	EvtPlanApproved      EventType = "plan_approved"
	EvtPlanRejected      EventType = "plan_rejected"
)

// Event is a single notification broadcast to all SSE subscribers.
type Event struct {
	Type EventType `json:"type"`
	Data any       `json:"data"`
}

// TaskProgress is an in-flight progress update from an agent.
type TaskProgress struct {
	TaskID   string `json:"task_id"`
	AgentID  string `json:"agent_id"`
	Message  string `json:"message"`
	Percent  int    `json:"percent,omitempty"`
	FilePath string `json:"file_path,omitempty"` // populated when a file is touched
	Action   string `json:"action,omitempty"`    // "created" | "modified" | "deleted"
}

// ─── Registry ────────────────────────────────────────────────────────────────

// SuspensionChecker is satisfied by supervisor.Supervisor. The Registry calls
// this before dispatching tasks so suspended agents cannot receive new work.
type SuspensionChecker interface {
	IsSuspended(agentID string) bool
}

// Registry is the central daemon that manages agent instances, the task queue,
// and fan-out of events to SSE subscribers.
type Registry struct {
	mu               sync.RWMutex
	agents           map[string]*AgentInstance // id → instance
	tasks            map[string]*Task          // id → task
	plans            map[string]*Plan          // id → plan
	subs             map[string]chan Event      // subscriber id → channel
	subsMu           sync.RWMutex
	db               *store             // optional DB persistence; nil = memory-only
	suspensionCheck  SuspensionChecker  // optional; enforces supervisor suspension
}

// SetSuspensionChecker wires in the supervisor so the registry can skip
// suspended agents during task dispatch.
func (reg *Registry) SetSuspensionChecker(sc SuspensionChecker) {
	reg.mu.Lock()
	reg.suspensionCheck = sc
	reg.mu.Unlock()
}

// New creates a ready-to-use Registry.
func New() *Registry {
	return &Registry{
		agents: make(map[string]*AgentInstance),
		tasks:  make(map[string]*Task),
		plans:  make(map[string]*Plan),
		subs:   make(map[string]chan Event),
	}
}

// SetPool attaches a DB pool for durable plan/task persistence and then
// restores any non-terminal plans and tasks from the database into memory.
// Must be called before the gateway serves requests (typically at startup).
func (reg *Registry) SetPool(pool *pgxpool.Pool, tenantID string) {
	reg.db = &store{pool: pool, tenantID: tenantID}
	reg.LoadState(context.Background())
}

// ─── Agent registration ───────────────────────────────────────────────────────

// Register adds a new agent instance. Returns the created instance.
// After registration it attempts to dispatch any queued tasks that were
// waiting for an agent with matching capabilities.
func (reg *Registry) Register(name, providerType, model string, capabilities []string, impl AgentProvider) *AgentInstance {
	id := uuid.New().String()
	inst := &AgentInstance{
		ID:           id,
		Name:         name,
		Provider:     providerType,
		Model:        model,
		Capabilities: capabilities,
		Status:       StatusIdle,
		RegisteredAt: time.Now(),
		LastSeenAt:   time.Now(),
		provider:     impl,
	}

	reg.mu.Lock()
	reg.agents[id] = inst
	reg.mu.Unlock()

	slog.Info("daemon.agent.registered", "id", id, "name", name, "provider", providerType, "model", model)
	reg.broadcast(Event{Type: EvtAgentRegistered, Data: inst})

	// New agent is idle — retry any orphaned queued tasks it can handle.
	go reg.tryDispatchQueued(context2Background(), capabilities)
	return inst
}

// Unregister removes an agent. Any in-progress task is left in the queue
// (TaskQueued) for reassignment by the dispatcher.
func (reg *Registry) Unregister(id, reason string) {
	reg.mu.Lock()
	inst, ok := reg.agents[id]
	if !ok {
		reg.mu.Unlock()
		return
	}
	// Re-queue in-progress task so the dispatcher can reassign it.
	if inst.CurrentTask != "" {
		if t, ok := reg.tasks[inst.CurrentTask]; ok && t.Status == TaskInProgress {
			t.Status = TaskQueued
			t.Owner = ""
		}
	}
	delete(reg.agents, id)
	reg.mu.Unlock()

	slog.Info("daemon.agent.unregistered", "id", id, "reason", reason)
	reg.broadcast(Event{Type: EvtAgentUnregistered, Data: map[string]string{"id": id, "reason": reason}})
}

// Heartbeat updates the agent's last-seen timestamp and optionally its status.
// When an agent transitions to idle, it retries any queued tasks that were
// waiting for an agent with matching capabilities.
func (reg *Registry) Heartbeat(id string, status AgentStatus) bool {
	reg.mu.Lock()
	inst, ok := reg.agents[id]
	if !ok {
		reg.mu.Unlock()
		return false
	}
	wasIdle := inst.Status == StatusIdle
	changed := inst.Status != status
	inst.LastSeenAt = time.Now()
	inst.Status = status
	caps := inst.Capabilities
	reg.mu.Unlock()

	if changed {
		reg.broadcast(Event{Type: EvtAgentStatus, Data: map[string]any{
			"id": id, "status": status,
		}})
	}

	// Agent just became idle — retry orphaned queued tasks it can handle.
	if !wasIdle && status == StatusIdle {
		go reg.tryDispatchQueued(context2Background(), caps)
	}
	return true
}

// ListAgents returns a snapshot of all registered agents.
func (reg *Registry) ListAgents() []*AgentInstance {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	out := make([]*AgentInstance, 0, len(reg.agents))
	for _, inst := range reg.agents {
		cp := *inst
		cp.provider = nil // don't expose internal impl
		out = append(out, &cp)
	}
	return out
}

// GetAgent returns a single agent by ID.
func (reg *Registry) GetAgent(id string) *AgentInstance {
	reg.mu.RLock()
	inst, ok := reg.agents[id]
	reg.mu.RUnlock()
	if !ok {
		return nil
	}
	cp := *inst
	cp.provider = nil
	return &cp
}

// ─── Task queue ───────────────────────────────────────────────────────────────

// CreateTask adds a new task to the queue. If ownerID is empty, Dispatch is
// called immediately to find the best available agent.
func (reg *Registry) CreateTask(title, description, ownerID, priority, createdBy string, dependsOn []string, context map[string]any) *Task {
	if priority == "" {
		priority = "normal"
	}
	t := &Task{
		ID:          uuid.New().String(),
		Title:       title,
		Description: description,
		Owner:       ownerID,
		Priority:    priority,
		Status:      TaskQueued,
		DependsOn:   dependsOn,
		CreatedBy:   createdBy,
		Context:     context,
		CreatedAt:   time.Now(),
	}

	reg.mu.Lock()
	reg.tasks[t.ID] = t
	reg.mu.Unlock()

	go reg.db.saveTask(context2Background(), t)

	slog.Info("daemon.task.created", "id", t.ID, "title", title, "owner", ownerID)
	reg.broadcast(Event{Type: EvtTaskCreated, Data: t})

	// Auto-dispatch if owner already assigned.
	if ownerID != "" {
		go reg.dispatchTo(context2Background(), t.ID, ownerID)
	} else {
		go reg.autoDispatch(context2Background(), t.ID)
	}
	return t
}

// AssignTask re-assigns a queued task to a specific agent.
func (reg *Registry) AssignTask(taskID, agentID string) bool {
	reg.mu.Lock()
	t, ok := reg.tasks[taskID]
	if !ok || t.Status != TaskQueued {
		reg.mu.Unlock()
		return false
	}
	prev := t.Owner
	t.Owner = agentID
	reg.mu.Unlock()

	reg.broadcast(Event{Type: EvtTaskAssigned, Data: map[string]string{
		"task_id": taskID, "agent_id": agentID, "reassigned_from": prev,
	}})
	go reg.dispatchTo(context2Background(), taskID, agentID)
	return true
}

// Progress records an in-flight update from an agent and fans it out.
func (reg *Registry) Progress(p TaskProgress) {
	reg.broadcast(Event{Type: EvtTaskProgress, Data: p})
	if p.FilePath != "" {
		reg.broadcast(Event{Type: EvtTaskFile, Data: map[string]string{
			"task_id": p.TaskID, "agent_id": p.AgentID,
			"path": p.FilePath, "action": p.Action,
		}})
	}
}

// Complete marks a task done and frees the agent.
func (reg *Registry) Complete(taskID, summary string, files []string) bool {
	reg.mu.Lock()
	t, ok := reg.tasks[taskID]
	if !ok {
		reg.mu.Unlock()
		return false
	}
	now := time.Now()
	t.Status = TaskDone
	t.Summary = summary
	t.FilesChanged = files
	t.CompletedAt = &now
	if t.StartedAt != nil {
		t.DurationMS = now.Sub(*t.StartedAt).Milliseconds()
	}
	agentID := t.Owner
	reg.mu.Unlock()

	go reg.db.updateTaskStatus(context2Background(), taskID, TaskDone, summary, "", &now)

	reg.freeAgent(agentID, taskID, true)
	reg.broadcast(Event{Type: EvtTaskDone, Data: map[string]any{
		"task_id": taskID, "agent_id": agentID,
		"summary": summary, "files_changed": files, "duration_ms": t.DurationMS,
	}})
	slog.Info("daemon.task.done", "id", taskID, "agent", agentID, "files", len(files))
	return true
}

// Fail marks a task failed and frees the agent.
func (reg *Registry) Fail(taskID, errMsg string, retryable bool) bool {
	reg.mu.Lock()
	t, ok := reg.tasks[taskID]
	if !ok {
		reg.mu.Unlock()
		return false
	}
	t.Status = TaskFailed
	t.Error = errMsg
	agentID := t.Owner
	reg.mu.Unlock()

	go reg.db.updateTaskStatus(context2Background(), taskID, TaskFailed, "", errMsg, nil)

	reg.freeAgent(agentID, taskID, false)
	reg.broadcast(Event{Type: EvtTaskFailed, Data: map[string]any{
		"task_id": taskID, "agent_id": agentID,
		"error": errMsg, "retryable": retryable,
	}})
	slog.Warn("daemon.task.failed", "id", taskID, "agent", agentID, "error", errMsg)
	return true
}

// ListTasks returns tasks filtered by optional agent id and status,
// sorted by CreatedAt DESC (newest first) for deterministic ordering.
func (reg *Registry) ListTasks(agentID, status string, limit int) []*Task {
	reg.mu.RLock()
	all := make([]*Task, 0, len(reg.tasks))
	for _, t := range reg.tasks {
		if agentID != "" && t.Owner != agentID {
			continue
		}
		if status != "" && string(t.Status) != status {
			continue
		}
		cp := *t
		all = append(all, &cp)
	}
	reg.mu.RUnlock()

	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})
	if limit > 0 && len(all) > limit {
		return all[:limit]
	}
	return all
}

// GetTask returns a single task by ID.
func (reg *Registry) GetTask(id string) *Task {
	reg.mu.RLock()
	t, ok := reg.tasks[id]
	reg.mu.RUnlock()
	if !ok {
		return nil
	}
	cp := *t
	return &cp
}

// ─── Plan approval ────────────────────────────────────────────────────────────

// ProposePlan creates a pending plan. Single-task plans auto-approve;
// multi-task plans wait for human approval.
func (reg *Registry) ProposePlan(title, description, proposedBy string, tasks []PlanTask) *Plan {
	p := &Plan{
		ID:          uuid.New().String(),
		Title:       title,
		Description: description,
		ProposedBy:  proposedBy,
		Status:      PlanPending,
		Tasks:       tasks,
		CreatedAt:   time.Now(),
	}

	reg.mu.Lock()
	reg.plans[p.ID] = p
	reg.mu.Unlock()

	go reg.db.savePlan(context2Background(), p)

	reg.broadcast(Event{Type: EvtPlanProposed, Data: p})
	slog.Info("daemon.plan.proposed", "id", p.ID, "tasks", len(tasks), "by", proposedBy)

	// Single-task plans auto-approve to avoid friction for small work items.
	if len(tasks) == 1 {
		go func() {
			reg.ApprovePlan(p.ID, "system", "")
		}()
	}
	return p
}

// ApprovePlan approves a plan and queues its tasks.
func (reg *Registry) ApprovePlan(planID, approvedBy, modifications string) bool {
	reg.mu.Lock()
	p, ok := reg.plans[planID]
	if !ok || p.Status != PlanPending {
		reg.mu.Unlock()
		return false
	}
	now := time.Now()
	p.Status = PlanApproved
	p.DecidedAt = &now
	p.DecidedBy = approvedBy
	p.Modifications = modifications
	tasks := p.Tasks
	reg.mu.Unlock()

	go reg.db.updatePlanStatus(context2Background(), planID, PlanApproved, approvedBy, modifications, "")

	reg.broadcast(Event{Type: EvtPlanApproved, Data: map[string]string{
		"plan_id": planID, "approved_by": approvedBy, "modifications": modifications,
	}})
	slog.Info("daemon.plan.approved", "id", planID, "by", approvedBy, "tasks", len(tasks))

	// Queue each task — include routing metadata so external agents know
	// which ticket to update and where the task originated.
	for _, pt := range tasks {
		ctx := map[string]any{
			"plan_id":           planID,
			"owner_capability":  pt.OwnerCapability,
			"estimated_minutes": pt.EstimatedMinutes,
		}
		if pt.TicketID != "" {
			ctx["ticket_id"] = pt.TicketID
		}
		if pt.OriginID != "" {
			ctx["origin_id"] = pt.OriginID
			ctx["origin_type"] = pt.OriginType
		}
		for k, v := range pt.ExtraContext {
			ctx[k] = v
		}
		desc := pt.Title
		if origin, ok := ctx["brief_title"].(string); ok && origin != "" {
			desc = origin + " — " + pt.Title
		}
		reg.CreateTask(pt.Title, desc, "", pt.Priority, "plan:"+planID, nil, ctx)
	}
	return true
}

// RejectPlan rejects a plan.
func (reg *Registry) RejectPlan(planID, rejectedBy, reason string) bool {
	reg.mu.Lock()
	p, ok := reg.plans[planID]
	if !ok || p.Status != PlanPending {
		reg.mu.Unlock()
		return false
	}
	now := time.Now()
	p.Status = PlanRejected
	p.DecidedAt = &now
	p.DecidedBy = rejectedBy
	reg.mu.Unlock()

	go reg.db.updatePlanStatus(context2Background(), planID, PlanRejected, rejectedBy, "", reason)

	reg.broadcast(Event{Type: EvtPlanRejected, Data: map[string]string{
		"plan_id": planID, "rejected_by": rejectedBy, "reason": reason,
	}})
	slog.Info("daemon.plan.rejected", "id", planID, "by", rejectedBy)
	return true
}

// ListPlans returns all plans, newest first.
func (reg *Registry) ListPlans() []*Plan {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	out := make([]*Plan, 0, len(reg.plans))
	for _, p := range reg.plans {
		cp := *p
		out = append(out, &cp)
	}
	return out
}

// GetPlan returns a single plan by ID.
func (reg *Registry) GetPlan(id string) *Plan {
	reg.mu.RLock()
	p, ok := reg.plans[id]
	reg.mu.RUnlock()
	if !ok {
		return nil
	}
	cp := *p
	return &cp
}

// ─── SSE fan-out ─────────────────────────────────────────────────────────────

// Subscribe creates an event channel for a new SSE subscriber.
// The caller must call Unsubscribe when the connection closes.
func (reg *Registry) Subscribe() (string, <-chan Event) {
	id := uuid.New().String()
	ch := make(chan Event, 64)
	reg.subsMu.Lock()
	reg.subs[id] = ch
	reg.subsMu.Unlock()
	return id, ch
}

// Unsubscribe removes an SSE subscriber and closes its channel.
func (reg *Registry) Unsubscribe(id string) {
	reg.subsMu.Lock()
	ch, ok := reg.subs[id]
	if ok {
		delete(reg.subs, id)
		close(ch)
	}
	reg.subsMu.Unlock()
}

// broadcast fans an event out to all SSE subscribers. Non-blocking — slow
// subscribers are skipped (channel full = dropped event, not stall).
func (reg *Registry) broadcast(e Event) {
	reg.subsMu.RLock()
	defer reg.subsMu.RUnlock()
	for _, ch := range reg.subs {
		select {
		case ch <- e:
		default:
			// Subscriber too slow; drop rather than block.
		}
	}
}

// ─── Internal dispatch ────────────────────────────────────────────────────────

// tryDispatchQueued scans for queued tasks that match agentCaps and dispatches
// them via autoDispatch. Called when a newly registered or newly-idle agent
// can absorb work that had no match at creation time.
func (reg *Registry) tryDispatchQueued(ctx context.Context, agentCaps []string) {
	reg.mu.RLock()
	var candidates []string
	for id, t := range reg.tasks {
		if t.Status != TaskQueued {
			continue
		}
		// If the task has no capability requirement, any agent can take it.
		reqCap := ""
		if t.Context != nil {
			reqCap, _ = t.Context["owner_capability"].(string)
		}
		if reqCap == "" || hasCapability(agentCaps, reqCap) {
			candidates = append(candidates, id)
		}
	}
	reg.mu.RUnlock()

	for _, taskID := range candidates {
		reg.autoDispatch(ctx, taskID)
	}
}

// autoDispatch finds the best available idle agent that matches the task's
// capability requirements and dispatches to it.
func (reg *Registry) autoDispatch(ctx context.Context, taskID string) {
	reg.mu.RLock()
	t, ok := reg.tasks[taskID]
	reg.mu.RUnlock()
	if !ok {
		return
	}

	// Determine required capability from context["owner_capability"] if present.
	requiredCap := ""
	if t.Context != nil {
		if cap, ok := t.Context["owner_capability"].(string); ok {
			requiredCap = cap
		}
	}

	// Find first idle, non-suspended agent matching capability.
	reg.mu.RLock()
	sc := reg.suspensionCheck
	var targetID string
	for id, inst := range reg.agents {
		if inst.Status != StatusIdle {
			continue
		}
		if sc != nil && sc.IsSuspended(id) {
			slog.Info("daemon.task.skip_suspended", "agent", id, "task", taskID)
			continue
		}
		if requiredCap == "" || hasCapability(inst.Capabilities, requiredCap) {
			targetID = id
			break
		}
	}
	reg.mu.RUnlock()

	if targetID == "" {
		slog.Info("daemon.task.no_agent", "task", taskID, "cap", requiredCap)
		return // will be retried when an agent sends a heartbeat with idle status
	}

	reg.dispatchTo(ctx, taskID, targetID)
}

// dispatchTo assigns a task to a specific agent and invokes its provider.
func (reg *Registry) dispatchTo(ctx context.Context, taskID, agentID string) {
	// Refuse dispatch to a supervisor-suspended agent.
	reg.mu.RLock()
	sc := reg.suspensionCheck
	reg.mu.RUnlock()
	if sc != nil && sc.IsSuspended(agentID) {
		slog.Warn("daemon.dispatch.blocked_suspended", "agent", agentID, "task", taskID)
		return
	}

	reg.mu.Lock()
	t, ok := reg.tasks[taskID]
	if !ok {
		reg.mu.Unlock()
		return
	}
	inst, ok2 := reg.agents[agentID]
	if !ok2 || inst.Status != StatusIdle {
		reg.mu.Unlock()
		return
	}
	now := time.Now()
	t.Owner = agentID
	t.Status = TaskInProgress
	t.StartedAt = &now
	inst.Status = StatusWorking
	inst.CurrentTask = taskID
	taskCopy := *t
	provider := inst.provider
	reg.mu.Unlock()

	reg.broadcast(Event{Type: EvtTaskAssigned, Data: map[string]string{
		"task_id": taskID, "agent_id": agentID,
	}})

	// Push to provider if it has one (for programmatic dispatch).
	// SSE-connected agents (like kiro-cli) will pick it up via the SSE stream.
	if provider != nil {
		if err := provider.Dispatch(ctx, &taskCopy); err != nil {
			slog.Warn("daemon.task.dispatch_failed", "task", taskID, "agent", agentID, "error", err)
			reg.Fail(taskID, err.Error(), true)
		}
	}
}

// freeAgent sets an agent back to idle after a task finishes.
// succeeded=true increments TasksDone; false increments TasksFailed.
func (reg *Registry) freeAgent(agentID, taskID string, succeeded bool) {
	reg.mu.Lock()
	inst, ok := reg.agents[agentID]
	if ok && inst.CurrentTask == taskID {
		wasWorking := inst.Status == StatusWorking
		inst.Status = StatusIdle
		inst.CurrentTask = ""
		if wasWorking {
			if succeeded {
				inst.TasksDone++
			} else {
				inst.TasksFailed++
			}
		}
	}
	reg.mu.Unlock()
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func hasCapability(caps []string, required string) bool {
	for _, c := range caps {
		if c == required {
			return true
		}
	}
	return false
}

// context2Background is a named alias to make callsites self-documenting.
// Dispatch goroutines detach from the HTTP request context on purpose.
func context2Background() context.Context {
	return context.Background()
}

// MarshalEvent returns the JSON-encoded event data for SSE framing.
func MarshalEvent(e Event) ([]byte, error) {
	return json.Marshal(e)
}
