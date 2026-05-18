// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package coworker

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// skills.go + bus.go + executor.go + queue.go — Core agent infrastructure.

// ── Skills ──

type SkillType string

const (
	SkillBuiltinTools       SkillType = "builtin_tools"
	SkillDeletionGuardrails SkillType = "deletion_guardrails"
	SkillMCPIntegration     SkillType = "mcp_integration"
	SkillWorkflowAuthoring  SkillType = "workflow_authoring"
	SkillWorkflowRunOps     SkillType = "workflow_run_ops"
)

type Skill struct {
	Type        SkillType `json:"type"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	Config      map[string]any `json:"config,omitempty"`
}

type SkillRegistry struct {
	mu     sync.RWMutex
	skills map[SkillType]*Skill
}

func NewSkillRegistry() *SkillRegistry {
	sr := &SkillRegistry{skills: make(map[SkillType]*Skill)}
	sr.registerDefaults()
	return sr
}

func (sr *SkillRegistry) registerDefaults() {
	sr.skills[SkillBuiltinTools] = &Skill{
		Type: SkillBuiltinTools, Name: "Built-in Tools", Enabled: true,
		Description: "File operations, web search, exec — core agent capabilities",
	}
	sr.skills[SkillDeletionGuardrails] = &Skill{
		Type: SkillDeletionGuardrails, Name: "Deletion Guardrails", Enabled: true,
		Description: "Prevents accidental deletion of important files and data",
	}
	sr.skills[SkillMCPIntegration] = &Skill{
		Type: SkillMCPIntegration, Name: "MCP Integration", Enabled: true,
		Description: "Connect to external tools via Model Context Protocol",
	}
	sr.skills[SkillWorkflowAuthoring] = &Skill{
		Type: SkillWorkflowAuthoring, Name: "Workflow Authoring", Enabled: true,
		Description: "Create and edit automated workflows",
	}
	sr.skills[SkillWorkflowRunOps] = &Skill{
		Type: SkillWorkflowRunOps, Name: "Workflow Operations", Enabled: true,
		Description: "Run, monitor, and manage workflow executions",
	}
}

func (sr *SkillRegistry) Get(t SkillType) (*Skill, bool) {
	sr.mu.RLock(); defer sr.mu.RUnlock()
	s, ok := sr.skills[t]; return s, ok
}

func (sr *SkillRegistry) Enable(t SkillType)  { sr.mu.Lock(); if s, ok := sr.skills[t]; ok { s.Enabled = true }; sr.mu.Unlock() }
func (sr *SkillRegistry) Disable(t SkillType) { sr.mu.Lock(); if s, ok := sr.skills[t]; ok { s.Enabled = false }; sr.mu.Unlock() }

func (sr *SkillRegistry) List() []*Skill {
	sr.mu.RLock(); defer sr.mu.RUnlock()
	var out []*Skill
	for _, s := range sr.skills { out = append(out, s) }
	return out
}

// ── Event Bus ──

type EventType string

const (
	EventNoteCreated  EventType = "note.created"
	EventNoteUpdated  EventType = "note.updated"
	EventNoteDeleted  EventType = "note.deleted"
	EventLiveUpdate   EventType = "live.update"
	EventMeetingPrep  EventType = "meeting.prep"
	EventEmailDraft   EventType = "email.draft"
	EventToolExecuted EventType = "tool.executed"
	EventError        EventType = "error"
)

type Event struct {
	Type      EventType      `json:"type"`
	Payload   any            `json:"payload"`
	Timestamp time.Time      `json:"timestamp"`
	Source    string          `json:"source"`
}

type EventHandler func(Event)

type EventBus struct {
	mu       sync.RWMutex
	handlers map[EventType][]EventHandler
}

func NewEventBus() *EventBus {
	return &EventBus{handlers: make(map[EventType][]EventHandler)}
}

func (b *EventBus) Subscribe(eventType EventType, handler EventHandler) {
	b.mu.Lock(); defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

func (b *EventBus) Publish(event Event) {
	event.Timestamp = time.Now()
	b.mu.RLock()
	handlers := b.handlers[event.Type]
	b.mu.RUnlock()
	for _, h := range handlers { go h(event) }
}

// ── Command Executor ──

type CommandResult struct {
	Command  string        `json:"command"`
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
}

type CommandExecutor struct {
	workDir    string
	allowList  []string // allowed command prefixes
	bus        *EventBus
	guardrails *DeletionGuardrails
}

func NewCommandExecutor(workDir string, bus *EventBus) *CommandExecutor {
	return &CommandExecutor{
		workDir: workDir, bus: bus,
		guardrails: NewDeletionGuardrails(),
		allowList: []string{"ls", "cat", "head", "tail", "grep", "find", "wc", "echo", "date", "pwd", "git", "go", "python", "node", "npm", "curl"},
	}
}

func (e *CommandExecutor) Execute(ctx context.Context, command string) (*CommandResult, error) {
	start := time.Now()

	// Check guardrails
	if e.guardrails.IsDangerous(command) {
		return nil, fmt.Errorf("blocked by deletion guardrails: %s", command)
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = e.workDir

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok { exitCode = exitErr.ExitCode() } else { return nil, err }
	}

	result := &CommandResult{
		Command: command, Stdout: stdout.String(), Stderr: stderr.String(),
		ExitCode: exitCode, Duration: time.Since(start),
	}

	if e.bus != nil {
		e.bus.Publish(Event{Type: EventToolExecuted, Payload: result, Source: "executor"})
	}
	return result, nil
}

// ── Deletion Guardrails ──

type DeletionGuardrails struct {
	patterns []string
}

func NewDeletionGuardrails() *DeletionGuardrails {
	return &DeletionGuardrails{
		patterns: []string{
			"rm -rf /", "rm -rf ~", "rm -rf .", "rm -rf *",
			"rmdir /", "del /s /q", "format c:",
			"> /dev/sda", "dd if=/dev/zero",
			"DROP DATABASE", "DROP TABLE", "DELETE FROM",
			"chmod -R 777 /", "chown -R",
		},
	}
}

func (g *DeletionGuardrails) IsDangerous(command string) bool {
	lower := strings.ToLower(command)
	for _, p := range g.patterns {
		if strings.Contains(lower, strings.ToLower(p)) { return true }
	}
	return false
}

// ── Message Queue ──

type Message struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Timestamp time.Time `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type MessageQueue struct {
	mu       sync.Mutex
	messages []Message
	handlers map[string]func(Message)
	maxSize  int
}

func NewMessageQueue(maxSize int) *MessageQueue {
	if maxSize <= 0 { maxSize = 1000 }
	return &MessageQueue{handlers: make(map[string]func(Message)), maxSize: maxSize}
}

func (q *MessageQueue) Push(msg Message) {
	msg.Timestamp = time.Now()
	if msg.ID == "" { msg.ID = fmt.Sprintf("msg_%d", time.Now().UnixNano()) }
	q.mu.Lock()
	q.messages = append(q.messages, msg)
	if len(q.messages) > q.maxSize { q.messages = q.messages[len(q.messages)-q.maxSize:] }
	q.mu.Unlock()
	if h, ok := q.handlers[msg.Type]; ok { go h(msg) }
}

func (q *MessageQueue) Pop() (Message, bool) {
	q.mu.Lock(); defer q.mu.Unlock()
	if len(q.messages) == 0 { return Message{}, false }
	msg := q.messages[0]; q.messages = q.messages[1:]
	return msg, true
}

func (q *MessageQueue) OnMessage(msgType string, handler func(Message)) { q.handlers[msgType] = handler }
func (q *MessageQueue) Len() int { q.mu.Lock(); defer q.mu.Unlock(); return len(q.messages) }
