// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package souldesk

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/memory"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/session"
	"github.com/qorvenai/qorven/internal/skills"
	"github.com/qorvenai/qorven/internal/tools"
)

// delegationResult holds the result of a single Soul's work.
type delegationResult struct {
	SoulKey string
	SoulName string
	Task    string
	Result  string
	Err     error
}

// delegationBatch tracks parallel delegations from the same parent.
type delegationBatch struct {
	mu       sync.Mutex
	primeID  string
	expected int
	results  []delegationResult
	done     chan struct{}
}

// SoulDesk orchestrates multiple Souls (agents) working together.
// The Prime Soul delegates tasks to specialist Souls via the delegate_to_soul tool.
// Each Soul runs its own agent loop independently.
// Results flow back: Soul → Prime Soul → User.
// Parallel batching: when multiple Souls work simultaneously, results are collected
// and delivered as ONE combined announcement (Qorven-inspired).
type SoulDesk struct {
	agentStore  *agent.Store
	sessionStore *session.Store
	providerReg *providers.Registry
	toolReg     *tools.Registry
	skillLoader  *skills.Loader
	skillStore   *skills.Store
	memStore    *memory.Store
	smartRouter *providers.SmartRouter
	tenantID    string
	taskInteg   *TaskIntegration
	rtHub       RTHub // real-time hub for live activity streaming

	// Parallel delegation batching
	batchMu  sync.Mutex
	batches  map[string]*delegationBatch // batchID → batch

	// Batched announce queue — merges multiple Soul completions into one announcement
	announceQueue *AnnounceQueue

	// Prime Soul follow-up: when delegation completes, run Prime Soul to present the result
	OnDelegationComplete func(ctx context.Context, primeID, sessionID, soulKey, task, result string)
}

// RTHub is the interface for real-time event broadcasting.
type RTHub interface {
	BroadcastSoulActivity(agentID, soulKey, status, detail string)
	BroadcastSoulCompleted(agentID, soulName, taskTitle, result string)
}

func (d *SoulDesk) SetTaskIntegration(ti *TaskIntegration) { d.taskInteg = ti }
func (d *SoulDesk) SetSmartRouter(r *providers.SmartRouter)  { d.smartRouter = r }

// SetAnnounceQueue enables batched delivery of delegation results.
// When set, deliverResult uses the queue instead of immediate delivery.
func (d *SoulDesk) SetAnnounceQueue(aq *AnnounceQueue) { d.announceQueue = aq }
func (d *SoulDesk) SetSkillStore(store *skills.Store) { d.skillStore = store }

// sendMessage sends an inter-agent message (Soul → Prime Soul)
func (d *SoulDesk) sendMessage(ctx context.Context, fromID, toID, taskID, content string) {
	if d.agentStore == nil {
		return
	}
	d.agentStore.Pool().Exec(ctx,
		`INSERT INTO agent_messages (tenant_id, from_agent, to_agent, task_id, content, message_type)
		 VALUES ($1, $2, $3, $4, $5, 'report')`,
		d.tenantID, fromID, toID, nilIfEmpty(taskID), content)
}

func nilIfEmpty(s string) *string {
	if s == "" { return nil }
	return &s
}

// CheckUpdatesTool lets Prime Soul check for completed delegations
type CheckUpdatesTool struct {
	desk *SoulDesk
}

func NewCheckUpdatesTool(desk *SoulDesk) *CheckUpdatesTool {
	return &CheckUpdatesTool{desk: desk}
}

func (t *CheckUpdatesTool) Name() string        { return "check_updates" }
func (t *CheckUpdatesTool) Description() string {
	return "Check for updates from your Souls — completed tasks, reports, and messages. Call this to see what your team has finished."
}
func (t *CheckUpdatesTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (t *CheckUpdatesTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	primeID := tools.AgentIDFromCtx(ctx)
	if primeID == "" {
		return tools.ErrorResult("no agent context")
	}

	// Get unread messages for Prime Soul
	rows, err := t.desk.agentStore.Pool().Query(ctx,
		`SELECT am.content, am.created_at, a.display_name
		 FROM agent_messages am
		 JOIN agents a ON am.from_agent = a.id
		 WHERE am.to_agent = $1 AND am.read = false
		 ORDER BY am.created_at DESC LIMIT 10`, primeID)
	if err != nil {
		return tools.ErrorResult(err.Error())
	}
	defer rows.Close()

	var updates string
	count := 0
	for rows.Next() {
		var content, name string
		var createdAt interface{}
		rows.Scan(&content, &createdAt, &name)
		updates += fmt.Sprintf("📩 From %s:\n%s\n\n", name, content)
		count++
	}

	// Mark as read
	if count > 0 {
		t.desk.agentStore.Pool().Exec(ctx,
			`UPDATE agent_messages SET read = true WHERE to_agent = $1 AND read = false`, primeID)
	}

	if count == 0 {
		return tools.TextResult("No new updates from your team. All Souls are either idle or still working.")
	}
	return tools.TextResult(fmt.Sprintf("%d update(s):\n\n%s", count, updates))
}

func New(
	agentStore *agent.Store,
	sessionStore *session.Store,
	providerReg *providers.Registry,
	toolReg *tools.Registry,
	skillLoader  *skills.Loader,
	memStore *memory.Store,
	tenantID string,
) *SoulDesk {
	return &SoulDesk{
		agentStore:   agentStore,
		sessionStore: sessionStore,
		providerReg:  providerReg,
		toolReg:      toolReg,
		skillLoader:  skillLoader,
		memStore:     memStore,
		tenantID:     tenantID,
		batches:      make(map[string]*delegationBatch),
	}
}

// SetRTHub sets the real-time hub for live activity streaming.
func (d *SoulDesk) SetRTHub(hub RTHub) { d.rtHub = hub }

// DelegateTool is the tool that allows the Prime Soul to delegate work to other Souls.
// Registered as "delegate_to_soul" in the tool registry.
type DelegateTool struct {
	desk *SoulDesk
}

func NewDelegateTool(desk *SoulDesk) *DelegateTool {
	return &DelegateTool{desk: desk}
}

func (t *DelegateTool) Name() string { return "delegate_to_soul" }
func (t *DelegateTool) Description() string {
	return `Delegate a task to another Soul (specialist agent). Use this when a task requires specialized skills you don't have, or when you want to parallelize work. The Soul will execute the task independently and return the result. Available Souls can be listed with soul_key. If no matching Soul exists, describe what kind of Soul you need.`
}
func (t *DelegateTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"soul_key": map[string]any{
				"type":        "string",
				"description": "The agent_key of the Soul to delegate to (e.g. 'researcher', 'coder', 'writer')",
			},
			"task": map[string]any{
				"type":        "string",
				"description": "Clear description of what the Soul should do. Be specific about expected output.",
			},
			"context": map[string]any{
				"type":        "string",
				"description": "Any context the Soul needs (data, background, constraints)",
			},
		},
		"required": []string{"soul_key", "task"},
	}
}

func (t *DelegateTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	soulKey, _ := args["soul_key"].(string)
	task, _ := args["task"].(string)
	taskContext, _ := args["context"].(string)

	if soulKey == "" || task == "" {
		return tools.ErrorResult("soul_key and task are required")
	}

	// Find the Soul
	soul, err := t.desk.findSoul(ctx, soulKey)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Soul '%s' not found. Available Souls: %s", soulKey, t.desk.listSoulKeys(ctx)))
	}

	// Create task in Kanban
	var taskID string
	primeID := tools.AgentIDFromCtx(ctx)
	sessionID := tools.SessionIDFromCtx(ctx)
	if t.desk.taskInteg != nil {
		taskID, _ = t.desk.taskInteg.CreateDelegationTask(ctx, soul.ID, soulKey, primeID, task)
	}

	// Run async — fire and forget
	// Supports parallel batching: when multiple Souls work simultaneously,
	// results are collected and delivered as ONE combined announcement.
	go func() {
		bgCtx := context.Background()
		slog.Info("souldesk.delegate.async.start", "soul", soulKey, "task", task[:min(len(task), 80)])

		// P3.1: Live activity stream — broadcast that this Soul started working
		if t.desk.rtHub != nil {
			t.desk.rtHub.BroadcastSoulActivity(soul.ID, soulKey, "working", task[:min(len(task), 100)])
		}

		result, err := t.desk.runSoul(bgCtx, soul, task, taskContext)

		dr := delegationResult{SoulKey: soulKey, SoulName: soul.DisplayName, Task: task, Err: err}
		if err != nil {
			slog.Error("souldesk.delegate.async.error", "soul", soulKey, "error", err)
			dr.Result = "Error: " + err.Error()
			if t.desk.taskInteg != nil && taskID != "" {
				t.desk.taskInteg.CompleteDelegationTask(bgCtx, taskID, dr.Result, 0)
			}
		} else {
			slog.Info("souldesk.delegate.async.complete", "soul", soulKey, "result_len", len(result))
			dr.Result = result
			if t.desk.taskInteg != nil && taskID != "" {
				t.desk.taskInteg.CompleteDelegationTask(bgCtx, taskID, result, 0)
			}
		}

		// P3.1: Live activity stream — broadcast completion
		if t.desk.rtHub != nil {
			status := "completed"
			if err != nil {
				status = "failed"
			}
			t.desk.rtHub.BroadcastSoulActivity(soul.ID, soulKey, status, dr.Result[:min(len(dr.Result), 200)])
			t.desk.rtHub.BroadcastSoulCompleted(soul.ID, soulKey, task[:min(len(task), 100)], dr.Result[:min(len(dr.Result), 500)])
		}

		// P1.2: Parallel batching — check if this is part of a batch
		if primeID != "" {
			t.desk.deliverResult(bgCtx, primeID, sessionID, taskID, dr)
		}

		// Save as memory
		if t.desk.memStore != nil {
			t.desk.memStore.Save(bgCtx, t.desk.tenantID, memory.Memory{
				AgentID: soul.ID, Type: memory.TypeEvent,
				Content:    fmt.Sprintf("Soul @%s completed: %s", soulKey, task[:min(len(task), 100)]),
				Source:     "souldesk", SourceType: "delegation", Importance: 0.7,
			})
		}
	}()

	// Respond immediately — don't block
	estimate := "a few minutes"
	if len(task) < 100 {
		estimate = "1-2 minutes"
	}
	return tools.TextResult(fmt.Sprintf("📋 Task assigned to @%s (%s)\n\nTask: %s\n\nEstimated completion: %s. I'll notify you when it's done.",
		soulKey, soul.DisplayName, task[:min(len(task), 200)], estimate))
}

// ListSoulsTool lets the Prime Soul see what Souls are available.
type ListSoulsTool struct {
	desk *SoulDesk
}

func NewListSoulsTool(desk *SoulDesk) *ListSoulsTool {
	return &ListSoulsTool{desk: desk}
}

func (t *ListSoulsTool) Name() string        { return "list_souls" }
func (t *ListSoulsTool) Description() string { return "List all available Souls (specialist agents) you can delegate tasks to." }
func (t *ListSoulsTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (t *ListSoulsTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	agents, err := t.desk.agentStore.List(ctx, t.desk.tenantID)
	if err != nil {
		return tools.ErrorResult(err.Error())
	}
	if len(agents) == 0 {
		return tools.TextResult("No Souls available. You are the only one.")
	}

	var result string
	for _, a := range agents {
		role := ""
		if a.Role != nil { role = *a.Role }
		title := ""
		if a.Title != nil { title = *a.Title }
		result += fmt.Sprintf("- @%s — %s [%s %s] model=%s tools=%s\n",
			a.AgentKey, a.DisplayName, role, title, a.Model, a.ToolProfile)
	}
	return tools.TextResult(result)
}

// --- Internal methods ---

func (d *SoulDesk) findSoul(ctx context.Context, soulKey string) (*agent.Agent, error) {
	agents, err := d.agentStore.List(ctx, d.tenantID)
	if err != nil {
		return nil, err
	}
	for _, a := range agents {
		if strings.EqualFold(a.AgentKey, soulKey) {
			return d.agentStore.Get(ctx, a.ID)
		}
	}
	return nil, fmt.Errorf("not found")
}

func (d *SoulDesk) listSoulKeys(ctx context.Context) string {
	agents, _ := d.agentStore.List(ctx, d.tenantID)
	var keys string
	for _, a := range agents {
		if keys != "" { keys += ", " }
		keys += "@" + a.AgentKey
	}
	return keys
}

// runSoul executes a Soul's agent loop with a specific task.
// Creates a temporary session, runs the loop, returns the result.
func (d *SoulDesk) runSoul(ctx context.Context, soul *agent.Agent, task, taskContext string) (string, error) {
	// Build the message for the Soul
	message := task
	if taskContext != "" {
		message = fmt.Sprintf("Task: %s\n\nContext: %s", task, taskContext)
	}

	// Resolve provider
	provider := d.resolveProvider(soul)
	if provider == nil {
		return "", fmt.Errorf("no provider for soul %s", soul.AgentKey)
	}

	// Build context
	systemPrompt := soul.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = fmt.Sprintf("You are %s, a specialist Soul. Complete the assigned task thoroughly and return the result.", soul.DisplayName)
	}

	// Build tool definitions for this Soul
	var allow, deny []string
	if soul.ToolsAllowed != nil { json.Unmarshal(soul.ToolsAllowed, &allow) }
	if soul.ToolsDenied != nil { json.Unmarshal(soul.ToolsDenied, &deny) }
	toolDefs := tools.FilterTools(d.toolReg, allow, deny, soul.ToolProfile, true) // isSubagent=true

	// Convert to provider format
	provDefs := make([]providers.ToolDefinition, len(toolDefs))
	for i, td := range toolDefs {
		provDefs[i] = providers.ToolDefinition{
			Type: "function",
			Function: providers.ToolFunctionSchema{
				Name: td.Function.Name, Description: td.Function.Description, Parameters: td.Function.Parameters,
			},
		}
	}

	// Prepare tool execution context
	toolCtx := tools.WithWorkspace(ctx, "/tmp/qorven-workspace")
	toolCtx = tools.WithAgentID(toolCtx, soul.ID)

	// Run the think→act→observe loop (max 10 iterations for sub-souls)
	messages := []providers.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: message},
	}

	maxIter := soul.MaxToolIterations
	if maxIter <= 0 || maxIter > 10 { maxIter = 10 }

	model := soul.Model
	if model == "" || model == "default" {
		if d.smartRouter != nil {
			role := ""
			if soul.Role != nil {
				role = *soul.Role
			}
			model = pickSoulModel(d.smartRouter, role)
		}
		if model == "" {
			model = "kimi-k2.5"
		}
	}

	var finalContent string
	start := time.Now()

	for iter := 0; iter < maxIter; iter++ {
		resp, err := provider.Chat(ctx, providers.ChatRequest{
			Model:    model,
			Messages: messages,
			Tools:    provDefs,
		})
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		// No tool calls — this is the final response
		if len(resp.ToolCalls) == 0 {
			finalContent = resp.Content
			break
		}

		// Tool calls — execute and loop
		messages = append(messages, providers.Message{
			Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls,
		})

		for _, tc := range resp.ToolCalls {
			result := d.toolReg.Execute(toolCtx, tc.Name, tc.Arguments)
			content := result.ForLLM
			if len(content) > 20000 { content = content[:20000] + "\n[truncated]" }
			messages = append(messages, providers.Message{
				Role: "tool", Content: content, ToolCallID: tc.ID,
			})
		}
	}

	dur := time.Since(start)
	slog.Info("souldesk.soul.complete", "soul", soul.AgentKey, "iterations", len(messages)/2, "duration_ms", dur.Milliseconds())

	if finalContent == "" {
		finalContent = "(Soul completed but produced no output)"
	}

	// Save as memory (the Prime Soul should remember what this Soul did)
	if d.memStore != nil {
		d.memStore.Save(ctx, d.tenantID, memory.Memory{
			AgentID:    soul.ID,
			Type:       memory.TypeEvent,
			Content:    fmt.Sprintf("Soul @%s completed task: %s → Result: %s", soul.AgentKey, task[:min(len(task), 100)], finalContent[:min(len(finalContent), 200)]),
			Source:     "souldesk",
			SourceType: "delegation",
			Importance: 0.7,
		})
	}

	return finalContent, nil
}

func (d *SoulDesk) resolveProvider(soul *agent.Agent) providers.Provider {
	if soul.ProviderID != nil {
		if p, ok := d.providerReg.Get(*soul.ProviderID); ok { return p }
	}
	return d.providerReg.Default()
}

// CreateSoulTool lets the Prime Soul create new specialist Souls from chat.
type CreateSoulTool struct {
	desk *SoulDesk
}

func NewCreateSoulTool(desk *SoulDesk) *CreateSoulTool {
	return &CreateSoulTool{desk: desk}
}

func (t *CreateSoulTool) Name() string { return "create_soul" }
func (t *CreateSoulTool) Description() string {
	return `Create a new specialist Soul (agent) with optional skills from the marketplace. The Soul will be available for future delegation via delegate_to_soul. Specify a unique key, name, role, specialty, and optionally skills to auto-install (e.g. "web-research,code-review").`
}
func (t *CreateSoulTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"soul_key":     map[string]any{"type": "string", "description": "Unique lowercase key (e.g. 'writer', 'analyst', 'devops')"},
			"display_name": map[string]any{"type": "string", "description": "Display name (e.g. 'Content Writer')"},
			"role":         map[string]any{"type": "string", "description": "Role: Engineer, Researcher, Writer, Designer, Analyst, Support, DevOps, Custom"},
			"specialty":    map[string]any{"type": "string", "description": "What this Soul specializes in — becomes the system prompt"},
			"tool_profile": map[string]any{"type": "string", "description": "Tool access: 'coding' (fs+runtime+web+memory), 'messaging' (channels+web), 'full' (everything), 'minimal' (read-only)"},
			"skills":       map[string]any{"type": "string", "description": "Comma-separated skill slugs to auto-install (e.g. 'web-research,data-analyst,email-composer'). Available: web-research, code-review, email-composer, data-analyst, meeting-notes, content-writer, devops-helper, customer-support, project-manager, api-connector"},
		},
		"required": []string{"soul_key", "display_name", "role", "specialty"},
	}
}

func (t *CreateSoulTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	soulKey, _ := args["soul_key"].(string)
	displayName, _ := args["display_name"].(string)
	role, _ := args["role"].(string)
	specialty, _ := args["specialty"].(string)
	toolProfile, _ := args["tool_profile"].(string)

	if soulKey == "" || displayName == "" {
		return tools.ErrorResult("soul_key and display_name are required")
	}
	if toolProfile == "" {
		toolProfile = "coding"
	}

	// Check if Soul already exists
	if _, err := t.desk.findSoul(ctx, soulKey); err == nil {
		return tools.ErrorResult(fmt.Sprintf("Soul @%s already exists", soulKey))
	}

	primeID := tools.AgentIDFromCtx(ctx)
	systemPrompt := buildSoulSystemPrompt(displayName, role, specialty)
	model := pickSoulModel(t.desk.smartRouter, role)

	input := agent.CreateAgentInput{
		AgentKey:          soulKey,
		DisplayName:       displayName,
		Role:              role,
		Title:             specialty,
		ManagerID:         primeID,
		Model:             model,
		SystemPrompt:      systemPrompt,
		ToolProfile:       toolProfile,
		Temperature:       0.5,
		ContextWindow:     128000,
		MaxToolIterations: 10,
		MemorySharing:     "team",
	}
	memEnabled := true
	input.MemoryEnabled = &memEnabled

	created, err := t.desk.agentStore.Create(ctx, t.desk.tenantID, input)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to create Soul: %v", err))
	}

	slog.Info("souldesk.create_soul", "key", soulKey, "name", displayName, "role", role, "model", model, "id", created.ID)

	// Auto-install skills if specified
	skillsStr, _ := args["skills"].(string)
	installedSkills := []string{}
	if skillsStr != "" && t.desk.skillStore != nil {
		for _, slug := range strings.Split(skillsStr, ",") {
			slug = strings.TrimSpace(slug)
			if slug == "" {
				continue
			}
			if err := t.desk.skillStore.Install(ctx, created.ID, slug); err == nil {
				installedSkills = append(installedSkills, slug)
			}
		}
	}

	msg := fmt.Sprintf("Soul created: @%s — %s [%s] model=%s\nReady for delegation via delegate_to_soul.", soulKey, displayName, role, model)
	if len(installedSkills) > 0 {
		msg += fmt.Sprintf("\nSkills installed: %s", strings.Join(installedSkills, ", "))
	}
	return tools.TextResult(msg)
}

// pickSoulModel selects the best available model for a role using the SmartRouter.
// Falls back to a sensible default when the router is unavailable.
func pickSoulModel(router *providers.SmartRouter, role string) string {
	if router != nil {
		tier := roleToTier(role)
		if m := router.BestModelForTier(tier); m != "" {
			return m
		}
	}
	return "kimi-k2.5"
}

// roleToTier maps a soul role string to a SmartRouter routing tier.
func roleToTier(role string) string {
	switch strings.ToLower(role) {
	case "engineer", "devops":
		return providers.TierCoding
	case "researcher", "analyst":
		return providers.TierComplex
	case "qa", "reviewer":
		return providers.TierReasoning
	default:
		return providers.TierStandard
	}
}

// buildSoulSystemPrompt generates a structured agency-agents-style system prompt
// for a specialist Soul based on its role and specialty.
func buildSoulSystemPrompt(displayName, role, specialty string) string {
	identity := buildIdentity(displayName, role, specialty)
	mission := buildMission(role, specialty)
	rules := buildCriticalRules(role)
	style := buildCommunicationStyle(role)
	metrics := buildSuccessMetrics(role)

	return strings.Join([]string{identity, mission, rules, style, metrics}, "\n\n")
}

func buildIdentity(displayName, role, specialty string) string {
	roleDesc := map[string]string{
		"engineer":   "years building production systems",
		"devops":     "years running infrastructure at scale",
		"researcher": "years synthesizing information into insight",
		"analyst":    "years turning data into decisions",
		"writer":     "years crafting content that resonates",
		"designer":   "years shaping user experiences",
		"qa":         "years finding problems others miss",
		"reviewer":   "years ensuring quality before it ships",
		"support":    "years helping people solve real problems",
	}
	exp, ok := roleDesc[strings.ToLower(role)]
	if !ok {
		exp = "deep expertise in your domain"
	}
	return fmt.Sprintf(`## Your Identity & Memory

You are %s, a specialist Soul with %s. Your focus is %s.

You don't just complete tasks — you bring genuine expertise. You remember your work history within this session and build on prior context rather than starting from scratch. When you receive a task, you draw on everything you know to deliver the best possible result.`, displayName, exp, specialty)
}

func buildMission(role, specialty string) string {
	missionTemplates := map[string]string{
		"engineer":   "Design, implement, and debug software systems. Write production-quality code that is correct, efficient, and maintainable. When given a problem, think through the architecture before writing a line of code. Deliver working solutions, not sketches.",
		"devops":     "Manage infrastructure, automate deployments, and keep systems running. Write scripts and configs that work the first time. Document what you change and why. Think about failure modes before they happen.",
		"researcher": "Find, synthesize, and summarize information from multiple sources. Prioritize primary sources. Identify gaps and contradictions. Deliver structured findings with clear sourcing, not raw dumps of information.",
		"analyst":    "Interpret data, identify patterns, and extract actionable insight. Show your reasoning. Quantify uncertainty. Deliver conclusions with the evidence that supports them.",
		"writer":     "Create clear, compelling content tailored to the audience and purpose. Match tone to context. Edit ruthlessly. Deliver polished output, not first drafts.",
		"designer":   "Define user experiences that are intuitive and purposeful. Ground decisions in user needs. Document design rationale. Deliver specifications that engineers can build from.",
		"qa":         "Find defects before they reach production. Think adversarially — what would a hostile user do? Document issues with clear reproduction steps. Default to 'needs work' rather than 'looks fine'.",
		"reviewer":   "Evaluate work against standards and objectives. Be specific about what is wrong and why. Suggest concrete improvements. Default to 'needs revision' until the work genuinely meets the bar.",
		"support":    "Help people solve problems and understand systems. Prioritize clarity over technical precision when speaking to non-experts. Follow up to confirm the issue is resolved.",
	}

	mission, ok := missionTemplates[strings.ToLower(role)]
	if !ok {
		mission = fmt.Sprintf("Apply your expertise in %s to complete tasks thoroughly and return structured, actionable results.", specialty)
	}
	return "## Your Core Mission\n\n" + mission
}

func buildCriticalRules(role string) string {
	sharedRules := []string{
		"Never fabricate information. If you don't know something, say so and explain how to find out.",
		"When a task is ambiguous, state your interpretation before proceeding.",
		"Return structured output. Use headers, lists, and code blocks where they aid clarity.",
		"If you cannot complete a task with the tools available, explain precisely what is missing.",
	}

	roleRules := map[string][]string{
		"engineer": {
			"Never leave TODOs, placeholders, or incomplete implementations in delivered code.",
			"Always include error handling for operations that can fail.",
			"Test your logic mentally before returning code — walk through the execution path.",
			"Prefer existing patterns and libraries over reinventing the wheel.",
		},
		"devops": {
			"Never run destructive commands (drop, delete, reset) without an explicit instruction to do so.",
			"Always state what a script does before running it.",
			"Idempotency is a requirement, not a nice-to-have.",
			"Document every non-obvious configuration choice.",
		},
		"researcher": {
			"Always cite your sources. No source, no claim.",
			"Distinguish between fact, inference, and opinion.",
			"Flag conflicting information rather than silently picking one version.",
			"Summarise before you elaborate — lead with the answer.",
		},
		"analyst": {
			"Show the data that supports every conclusion.",
			"State your assumptions explicitly.",
			"Quantify confidence levels when you can.",
			"Never extrapolate beyond what the data supports.",
		},
		"qa": {
			"Default to 'needs work' — approval requires explicit evidence of correctness.",
			"Every reported issue must include: what failed, expected vs actual, and reproduction steps.",
			"Test edge cases, not just the happy path.",
			"Never mark something passed because it 'looks right'.",
		},
		"reviewer": {
			"Be specific — 'this is wrong because X' is actionable; 'this looks off' is not.",
			"Separate critical issues from minor suggestions.",
			"Default to requesting changes until quality standards are met.",
			"Reference the standard being violated, not just the violation.",
		},
	}

	rules := append(sharedRules, roleRules[strings.ToLower(role)]...)
	var b strings.Builder
	b.WriteString("## Critical Rules You Must Follow\n")
	for _, r := range rules {
		b.WriteString("\n- ")
		b.WriteString(r)
	}
	return b.String()
}

func buildCommunicationStyle(role string) string {
	styles := map[string]string{
		"engineer":   "Technical and precise. Use code blocks for all code. Lead with the solution, then explain the reasoning. Skip preamble.",
		"devops":     "Operational and direct. Commands go in code blocks. State what you're doing and why in one line before doing it.",
		"researcher": "Academic but accessible. Lead with a summary. Support claims with citations. Use bullet lists for findings, prose for synthesis.",
		"analyst":    "Data-first. Lead with the key number or conclusion. Support with evidence. Use tables for comparisons.",
		"writer":     "Audience-aware. Match the register the task specifies. Deliver finished prose, not commentary about the prose.",
		"designer":   "Visual and structured. Use headers and lists. Describe intent before specifics. Include rationale with every decision.",
		"qa":         "Clinical and systematic. Report issues in a consistent format: title, severity, steps, expected, actual. No ambiguity.",
		"reviewer":   "Authoritative and fair. Be direct about problems. Be specific about improvements. Separate what must change from what could change.",
		"support":    "Clear and patient. Avoid jargon unless the user uses it first. Confirm understanding before diagnosing. Follow up.",
	}
	style, ok := styles[strings.ToLower(role)]
	if !ok {
		style = "Clear and professional. Lead with the answer. Support with evidence. Skip unnecessary preamble."
	}
	return "## Communication Style\n\n" + style
}

func buildSuccessMetrics(role string) string {
	metrics := map[string]string{
		"engineer":   "Code runs correctly the first time. No TODOs. Error paths are handled. The solution fits the existing codebase.",
		"devops":     "Scripts are idempotent and documented. No surprises at runtime. Rollback path is defined.",
		"researcher": "Findings are sourced and structured. Conflicting data is surfaced. The summary answers the original question.",
		"analyst":    "Conclusions are supported by data shown. Assumptions are stated. The output drives a decision.",
		"writer":     "The output is publication-ready. No filler. The reader gets what they need without re-reading.",
		"designer":   "Specifications are buildable. Design decisions have documented rationale. Edge cases are covered.",
		"qa":         "Defects found before production. Reports are actionable. No false positives.",
		"reviewer":   "Work improves as a result of the review. Issues are specific and prioritised. Approval is meaningful.",
		"support":    "The user's problem is solved. They understand why. They can resolve similar issues themselves next time.",
	}
	m, ok := metrics[strings.ToLower(role)]
	if !ok {
		m = "The task is complete, the output is correct, and the result is immediately usable."
	}
	return "## Success Metrics\n\n" + m
}

// AssignTaskTool lets the Prime Soul create and assign tasks to Souls.
type AssignTaskTool struct {
	desk      *SoulDesk
	taskStore interface {
		Create(ctx context.Context, tenantID string, t interface{}) (string, error)
	}
}

// --- P3.2: Soul-to-Soul Direct Messaging ---

// SoulMessageTool lets any Soul send a direct message to another Soul.
// Qorven-inspired mailbox: send, broadcast, read.
type SoulMessageTool struct {
	desk *SoulDesk
}

func NewSoulMessageTool(desk *SoulDesk) *SoulMessageTool {
	return &SoulMessageTool{desk: desk}
}

func (t *SoulMessageTool) Name() string { return "soul_message" }
func (t *SoulMessageTool) Description() string {
	return `Send a direct message to another Soul. Use for coordination: ask questions, share partial results, request help. Actions: send (to one Soul), broadcast (to all Souls), read (check your inbox).`
}
func (t *SoulMessageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string", "enum": []string{"send", "broadcast", "read"},
				"description": "send=message one Soul, broadcast=message all Souls, read=check inbox",
			},
			"to": map[string]any{
				"type": "string", "description": "Soul key to message (for send action)",
			},
			"message": map[string]any{
				"type": "string", "description": "Message content (for send/broadcast)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *SoulMessageTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	action, _ := args["action"].(string)
	senderID := tools.AgentIDFromCtx(ctx)
	if senderID == "" {
		return tools.ErrorResult("no agent context")
	}

	switch action {
	case "send":
		toKey, _ := args["to"].(string)
		msg, _ := args["message"].(string)
		if toKey == "" || msg == "" {
			return tools.ErrorResult("'to' and 'message' required for send")
		}
		target, err := t.desk.findSoul(ctx, toKey)
		if err != nil {
			return tools.ErrorResult(fmt.Sprintf("Soul @%s not found", toKey))
		}
		t.desk.sendMessage(ctx, senderID, target.ID, "", fmt.Sprintf("[Soul message] %s", msg))
		slog.Info("soul_message.send", "from", senderID, "to", toKey)
		return tools.TextResult(fmt.Sprintf("📨 Message sent to @%s", toKey))

	case "broadcast":
		msg, _ := args["message"].(string)
		if msg == "" {
			return tools.ErrorResult("'message' required for broadcast")
		}
		agents, err := t.desk.agentStore.List(ctx, t.desk.tenantID)
		if err != nil {
			return tools.ErrorResult(err.Error())
		}
		count := 0
		for _, a := range agents {
			if a.ID != senderID {
				t.desk.sendMessage(ctx, senderID, a.ID, "", fmt.Sprintf("[Broadcast] %s", msg))
				count++
			}
		}
		return tools.TextResult(fmt.Sprintf("📢 Broadcast sent to %d Souls", count))

	case "read":
		rows, err := t.desk.agentStore.Pool().Query(ctx,
			`SELECT am.content, am.created_at, a.agent_key
			 FROM agent_messages am
			 JOIN agents a ON am.from_agent = a.id
			 WHERE am.to_agent = $1 AND am.read = false
			 ORDER BY am.created_at DESC LIMIT 10`, senderID)
		if err != nil {
			return tools.ErrorResult(err.Error())
		}
		defer rows.Close()
		var inbox string
		count := 0
		for rows.Next() {
			var content, key string
			var at interface{}
			rows.Scan(&content, &at, &key)
			inbox += fmt.Sprintf("📩 @%s: %s\n", key, content)
			count++
		}
		if count > 0 {
			t.desk.agentStore.Pool().Exec(ctx,
				`UPDATE agent_messages SET read = true WHERE to_agent = $1 AND read = false`, senderID)
		}
		if count == 0 {
			return tools.TextResult("📭 No unread messages.")
		}
		return tools.TextResult(fmt.Sprintf("%d message(s):\n\n%s", count, inbox))

	default:
		return tools.ErrorResult("action must be send, broadcast, or read")
	}
}

func min(a, b int) int {
	if a < b { return a }
	return b
}

// deliverResult handles result delivery with parallel batching support.
// Pushes result to: 1) agent_messages table, 2) Prime Soul follow-up OR direct to session, 3) WebSocket.
func (d *SoulDesk) deliverResult(ctx context.Context, primeID, sessionID, taskID string, dr delegationResult) {
	// Use batched announce queue if available (merges multiple completions)
	if d.announceQueue != nil && sessionID != "" {
		d.announceQueue.Enqueue(primeID, sessionID, AnnounceEntry{
			SoulKey:     dr.SoulKey,
			DisplayName: dr.SoulName,
			Content:     dr.Result,
			TaskID:      taskID,
			Failed:      dr.Err != nil,
		})
		return
	}

	// Fallback: immediate delivery (no batching)
	icon := "✅"
	if dr.Err != nil {
		icon = "❌"
	}
	msg := fmt.Sprintf("%s Task completed by @%s\n\nTask: %s\n\nResult:\n%s",
		icon, dr.SoulKey, dr.Task[:min(len(dr.Task), 100)], dr.Result[:min(len(dr.Result), 2000)])

	// 1. Send to agent_messages (inter-agent inbox)
	d.sendMessage(ctx, dr.SoulKey, primeID, taskID, msg)

	// 2. Prime Soul follow-up: let Prime Soul review and present the result
	//    Like a real Chief of Staff — collects report, summarizes, presents to user.
	if d.OnDelegationComplete != nil && sessionID != "" {
		slog.Info("souldesk.deliver.prime_followup", "session", sessionID, "soul", dr.SoulKey)
		d.OnDelegationComplete(ctx, primeID, sessionID, dr.SoulKey, dr.Task, dr.Result)
		return // Prime Soul will handle the presentation
	}

	// Fallback: push raw result to session if no follow-up configured
	if d.sessionStore != nil && sessionID != "" {
		d.sessionStore.AppendMessage(ctx, sessionID, session.Message{
			Role: "assistant", Content: msg, Timestamp: time.Now().Unix(),
		}, 0, 0)
	}

	// 3. Broadcast via WebSocket
	if d.rtHub != nil {
		d.rtHub.BroadcastSoulCompleted(primeID, dr.SoulName, dr.Task[:min(len(dr.Task), 80)], msg)
	}
}

// --- Handoff Tool: Full context transfer between Souls ---

type HandoffTool struct{ desk *SoulDesk }

func NewHandoffTool(desk *SoulDesk) *HandoffTool { return &HandoffTool{desk: desk} }

func (t *HandoffTool) Name() string { return "handoff_to_soul" }
func (t *HandoffTool) Description() string {
	return `Transfer the full conversation to another Soul. Unlike delegation (which sends a task), handoff gives the target Soul the complete conversation history so it can seamlessly continue. Use when the user's request requires a different specialist to take over.`
}
func (t *HandoffTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"soul_key": map[string]any{
				"type":        "string",
				"description": "The agent_key of the Soul to hand off to",
			},
			"instruction": map[string]any{
				"type":        "string",
				"description": "Brief instruction for the receiving Soul about what to focus on",
			},
		},
		"required": []string{"soul_key"},
	}
}

func (t *HandoffTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	soulKey, _ := args["soul_key"].(string)
	instruction, _ := args["instruction"].(string)
	sessionID := tools.SessionIDFromCtx(ctx)

	if soulKey == "" {
		return tools.ErrorResult("soul_key is required")
	}

	soul, err := t.desk.findSoul(ctx, soulKey)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Soul '%s' not found", soulKey))
	}

	// Load full conversation history from session
	history := []providers.Message{}
	if sessionID != "" && t.desk.sessionStore != nil {
		sess, err := t.desk.sessionStore.Get(ctx, sessionID)
		if err == nil && sess != nil {
			msgs := []session.Message{}
			json.Unmarshal(sess.Messages, &msgs)
			for _, m := range msgs {
				history = append(history, providers.Message{Role: m.Role, Content: m.Content})
			}
		}
	}

	// Build handoff message
	handoffMsg := "You are taking over this conversation."
	if instruction != "" {
		handoffMsg += " " + instruction
	}

	// Run the target Soul with full history
	provider := t.desk.resolveProvider(soul)
	if provider == nil {
		return tools.ErrorResult("no provider for soul " + soulKey)
	}

	systemPrompt := soul.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = fmt.Sprintf("You are %s.", soul.DisplayName)
	}

	messages := []providers.Message{{Role: "system", Content: systemPrompt}}
	messages = append(messages, history...)
	messages = append(messages, providers.Message{Role: "user", Content: handoffMsg})

	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Model:    soul.Model,
		Messages: messages,
		Options:  map[string]any{"temperature": soul.Temperature},
	})
	if err != nil {
		return tools.ErrorResult("handoff failed: " + err.Error())
	}

	return tools.TextResult(fmt.Sprintf("[Handoff to @%s]\n\n%s", soulKey, resp.Content))
}
