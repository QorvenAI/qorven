// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/providers"
)

type Agent struct {
	ID                string          `json:"id"`
	TenantID          string          `json:"tenant_id"`
	AgentKey          string          `json:"agent_key"`
	DisplayName       string          `json:"display_name"`
	Avatar            string          `json:"avatar,omitempty"`

	// Hierarchy
	Role              *string         `json:"role,omitempty"`
	Title             *string         `json:"title,omitempty"`
	ManagerID         *string         `json:"manager_id,omitempty"`

	// LLM
	ProviderID        *string         `json:"provider_id,omitempty"`
	Model             string          `json:"model"`
	FallbackModel     string          `json:"fallback_model,omitempty"`
	SystemPrompt      string          `json:"system_prompt"`
	Temperature       float64         `json:"temperature"`
	ContextWindow     int             `json:"context_window"`
	MaxToolIterations int             `json:"max_tool_iterations"`
	ThinkingLevel     string          `json:"thinking_level,omitempty"` // off | medium | high

	// Tools
	ToolProfile       string          `json:"tool_profile"`
	ToolsAllowed      json.RawMessage `json:"tools_allowed,omitempty"`
	ToolsDenied       json.RawMessage `json:"tools_denied,omitempty"`

	// Skills
	Skills            []string        `json:"skills,omitempty"`

	// Memory
	MemoryEnabled     bool            `json:"memory_enabled"`
	MemorySharing     string          `json:"memory_sharing"`
	AutoCompact       bool            `json:"auto_compact"`

	// Web Intelligence
	WebSearchEnabled  bool            `json:"web_search_enabled"`
	OutboundApproval  string          `json:"outbound_approval"` // none, supervisor, user, both

	// Budget
	CreditBudgetCents int64           `json:"credit_budget_cents"`
	CreditUsedCents   int64           `json:"credit_used_cents"`

	// Heartbeat
	HeartbeatEnabled  bool            `json:"heartbeat_enabled,omitempty"`
	HeartbeatInterval int             `json:"heartbeat_interval,omitempty"`

	// Mail
	MailPolicy   string `json:"mail_policy,omitempty"`

	// Autonomous runtime (071)
	RuntimeMode  string `json:"runtime_mode,omitempty"`
	CanDelegate  bool   `json:"can_delegate,omitempty"`

	// Inception linkage — non-nil for agents created via the inception flow
	ProjectBriefID string `json:"project_brief_id,omitempty"`

	Status            string          `json:"status"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type CreateAgentInput struct {
	AgentKey          string   `json:"agent_key"`
	DisplayName       string   `json:"display_name"`
	Avatar            string   `json:"avatar"`
	Role              string   `json:"role"`
	Title             string   `json:"title"`
	ManagerID         string   `json:"manager_id"`
	ProviderID        string   `json:"provider_id"`
	Model             string   `json:"model"`
	SystemPrompt      string   `json:"system_prompt"`
	Temperature       float64  `json:"temperature"`
	ContextWindow     int      `json:"context_window"`
	MaxToolIterations int      `json:"max_tool_iterations"`
	ToolProfile       string   `json:"tool_profile"`
	ToolsAllowed      []string `json:"tools_allowed"`
	ToolsDenied       []string `json:"tools_denied"`
	Skills            []string `json:"skills"`
	MemoryEnabled     *bool    `json:"memory_enabled"`
	MemorySharing     string   `json:"memory_sharing"`
	AutoCompact       *bool    `json:"auto_compact"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Create(ctx context.Context, tenantID string, in CreateAgentInput) (*Agent, error) {
	id := uuid.New().String()
	if in.AgentKey == "" {
		in.AgentKey = id[:8]
	}

	// Validate agent key — alphanumeric, hyphens, underscores only
	if !isValidAgentKey(in.AgentKey) {
		return nil, fmt.Errorf("invalid agent_key: must be alphanumeric with hyphens/underscores only")
	}

	// Sanitize display name — strip HTML tags to prevent stored XSS
	in.DisplayName = stripHTMLTags(in.DisplayName)

	if in.Model == "" {
		in.Model = "default"
	}
	if in.ToolProfile == "" {
		in.ToolProfile = "full"
	}
	if in.Temperature == 0 {
		in.Temperature = 0.7
	}
	if in.ContextWindow == 0 {
		in.ContextWindow = providers.GetContextWindow(in.Model)
	}
	if in.MaxToolIterations == 0 {
		in.MaxToolIterations = 20
	}
	if in.MemorySharing == "" {
		in.MemorySharing = "private"
	}
	memEnabled := true
	if in.MemoryEnabled != nil {
		memEnabled = *in.MemoryEnabled
	}
	autoCompact := true
	if in.AutoCompact != nil {
		autoCompact = *in.AutoCompact
	}

	var toolsAllowed, toolsDenied json.RawMessage
	if len(in.ToolsAllowed) > 0 {
		toolsAllowed, _ = json.Marshal(in.ToolsAllowed)
	}
	if len(in.ToolsDenied) > 0 {
		toolsDenied, _ = json.Marshal(in.ToolsDenied)
	}

	now := time.Now()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agents (id, tenant_id, agent_key, display_name, avatar,
		 role, title, manager_id, provider_id, model, system_prompt, temperature,
		 context_window, max_tool_iterations, tool_profile, tools_allowed, tools_denied,
		 skills, memory_enabled, memory_sharing, auto_compact, status, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`,
		id, tenantID, in.AgentKey, in.DisplayName, in.Avatar,
		nilStr(in.Role), nilStr(in.Title), nilStr(in.ManagerID), nilStr(in.ProviderID),
		in.Model, in.SystemPrompt, in.Temperature,
		in.ContextWindow, in.MaxToolIterations, in.ToolProfile, toolsAllowed, toolsDenied,
		in.Skills, memEnabled, in.MemorySharing, autoCompact, "active", now, now)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	slog.Info("agent created", "id", id, "key", in.AgentKey, "model", in.Model)
	return s.Get(ctx, id)
}

func (s *Store) Get(ctx context.Context, id string) (*Agent, error) {
	return s.getBy(ctx, "id", id)
}

func (s *Store) GetByKey(ctx context.Context, key string) (*Agent, error) {
	return s.getBy(ctx, "agent_key", key)
}

const agentSelectCols = `SELECT id, tenant_id, agent_key, display_name, COALESCE(avatar,''),
        role, title, manager_id, provider_id, model, COALESCE(system_prompt,''),
        COALESCE(temperature,0.7), COALESCE(context_window,128000),
        COALESCE(max_tool_iterations,20), COALESCE(tool_profile,'full'),
        tools_allowed, tools_denied,
        COALESCE(skills, ARRAY[]::TEXT[]), COALESCE(memory_enabled,true),
        COALESCE(memory_sharing,'private'), COALESCE(auto_compact,true),
        COALESCE(web_search_enabled,true), COALESCE(outbound_approval,'supervisor'),
        credit_budget_cents, credit_used_cents, status, created_at, updated_at,
        COALESCE(thinking_level,'off'),
        COALESCE(runtime_mode,''), COALESCE(can_delegate,false),
        COALESCE(project_brief_id::text,''),
        COALESCE(mail_policy,'')
 FROM agents`

func (s *Store) scanAgent(row interface{ Scan(...any) error }) (*Agent, error) {
	a := &Agent{}
	err := row.Scan(&a.ID, &a.TenantID, &a.AgentKey, &a.DisplayName, &a.Avatar,
		&a.Role, &a.Title, &a.ManagerID, &a.ProviderID, &a.Model, &a.SystemPrompt, &a.Temperature,
		&a.ContextWindow, &a.MaxToolIterations, &a.ToolProfile, &a.ToolsAllowed, &a.ToolsDenied,
		&a.Skills, &a.MemoryEnabled, &a.MemorySharing, &a.AutoCompact,
		&a.WebSearchEnabled, &a.OutboundApproval,
		&a.CreditBudgetCents, &a.CreditUsedCents, &a.Status, &a.CreatedAt, &a.UpdatedAt,
		&a.ThinkingLevel, &a.RuntimeMode, &a.CanDelegate, &a.ProjectBriefID, &a.MailPolicy)
	return a, err
}

func (s *Store) getBy(ctx context.Context, col, val string) (*Agent, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`%s WHERE %s = $1 AND deleted_at IS NULL`, agentSelectCols, col), val)
	a, err := s.scanAgent(row)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %s=%s (%v)", col, val, err)
	}
	return a, nil
}

// GetForTenant fetches an agent by ID with an explicit tenant_id filter so
// the query works regardless of whether RLS bypass is active on the connection.
func (s *Store) GetForTenant(ctx context.Context, id, tenantID string) (*Agent, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`%s WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`, agentSelectCols),
		id, tenantID)
	a, err := s.scanAgent(row)
	if err != nil {
		return nil, fmt.Errorf("agent not found: id=%s (%v)", id, err)
	}
	return a, nil
}

func (s *Store) Update(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	// Whitelist allowed columns to prevent SQL injection
	allowed := map[string]bool{
		"display_name": true, "model": true, "fallback_model": true,
		"system_prompt": true, "temperature": true, "context_window": true,
		"max_tool_iterations": true, "tool_profile": true, "memory_enabled": true,
		"memory_sharing": true, "auto_compact": true, "web_search_enabled": true,
		"avatar": true, "role": true, "title": true, "manager_id": true,
		"provider_id": true, "status": true, "credit_budget_cents": true,
		"thinking_level": true, "runtime_mode": true, "can_delegate": true,
		"mail_policy": true,
	}
	setClauses := ""
	args := []any{id}
	i := 2
	for key, val := range updates {
		if !allowed[key] {
			continue // skip non-whitelisted columns
		}
		if setClauses != "" {
			setClauses += ", "
		}
		setClauses += fmt.Sprintf("%s = $%d", key, i)
		args = append(args, val)
		i++
	}
	if setClauses == "" {
		// All keys were filtered out — only update the timestamp.
		_, err := s.pool.Exec(ctx,
			`UPDATE agents SET updated_at = $2 WHERE id = $1 AND deleted_at IS NULL`,
			id, time.Now())
		return err
	}
	setClauses += fmt.Sprintf(", updated_at = $%d", i)
	args = append(args, time.Now())

	_, err := s.pool.Exec(ctx,
		fmt.Sprintf("UPDATE agents SET %s WHERE id = $1 AND deleted_at IS NULL", setClauses),
		args...)
	return err
}

func (s *Store) List(ctx context.Context, tenantID string) ([]*Agent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, agent_key, display_name, COALESCE(avatar,''),
		        role, title, manager_id, COALESCE(model,'default'),
		        COALESCE(tool_profile,'full'), status,
		        COALESCE(credit_budget_cents,0), COALESCE(credit_used_cents,0),
		        COALESCE(memory_enabled,true), COALESCE(memory_sharing,'private'), COALESCE(outbound_approval,'supervisor'),
		        created_at, updated_at, COALESCE(thinking_level,'off'),
		        COALESCE(runtime_mode,''), COALESCE(can_delegate,false)
		 FROM agents WHERE tenant_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := []*Agent{}
	for rows.Next() {
		a := &Agent{}
		rows.Scan(&a.ID, &a.TenantID, &a.AgentKey, &a.DisplayName, &a.Avatar, &a.Role, &a.Title, &a.ManagerID,
			&a.Model, &a.ToolProfile, &a.Status, &a.CreditBudgetCents, &a.CreditUsedCents,
			&a.MemoryEnabled, &a.MemorySharing, &a.OutboundApproval, &a.CreatedAt, &a.UpdatedAt, &a.ThinkingLevel,
			&a.RuntimeMode, &a.CanDelegate)
		agents = append(agents, a)
	}
	return agents, nil
}

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// TrackUsage increments an agent's token usage (for budget tracking).
// Estimates cost at ~$0.001 per 1K tokens (rough average across models).
func (s *Store) TrackUsage(ctx context.Context, agentID string, inputTokens, outputTokens int) {
	totalTokens := inputTokens + outputTokens
	if totalTokens <= 0 {
		return
	}
	// Rough cost: $0.001/1K input, $0.003/1K output → cents
	costCents := int64(float64(inputTokens)/1000*0.1 + float64(outputTokens)/1000*0.3)
	if costCents < 1 {
		costCents = 1
	}
	s.pool.Exec(ctx,
		`UPDATE agents SET credit_used_cents = credit_used_cents + $1, updated_at = NOW() WHERE id = $2`,
		costCents, agentID)
}

// CheckBudget returns true if the agent is within budget (or has no budget set).
func (s *Store) CheckBudget(ctx context.Context, agentID string) (withinBudget bool, usedCents, budgetCents int64) {
	s.pool.QueryRow(ctx,
		`SELECT COALESCE(credit_budget_cents, 0), COALESCE(credit_used_cents, 0) FROM agents WHERE id = $1`,
		agentID).Scan(&budgetCents, &usedCents)
	if budgetCents <= 0 {
		return true, usedCents, budgetCents // no budget = unlimited
	}
	return usedCents < budgetCents, usedCents, budgetCents
}

// GetBudgetSummary returns budget info for all agents in a tenant.
func (s *Store) GetBudgetSummary(ctx context.Context, tenantID string) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_key, display_name, COALESCE(credit_budget_cents,0), COALESCE(credit_used_cents,0)
		 FROM agents WHERE tenant_id = $1 AND deleted_at IS NULL ORDER BY credit_used_cents DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var id, key, name string
		var budget, used int64
		rows.Scan(&id, &key, &name, &budget, &used)
		result = append(result, map[string]any{
			"id": id, "agent_key": key, "display_name": name,
			"budget_cents": budget, "used_cents": used,
			"remaining_cents": max(budget-used, 0),
		})
	}
	return result, nil
}

func max(a, b int64) int64 {
	if a > b { return a }
	return b
}

func (s *Store) Delete(ctx context.Context, id string) error {
	// Cascade: clean up related data
	s.pool.Exec(ctx, `DELETE FROM agent_channel_bindings WHERE agent_id = $1`, id)
	s.pool.Exec(ctx, `DELETE FROM learned_skills WHERE agent_id = $1`, id)
	s.pool.Exec(ctx, `DELETE FROM sessions WHERE agent_id = $1`, id)
	// Soft-delete the agent
	_, err := s.pool.Exec(ctx, `UPDATE agents SET deleted_at = NOW(), status = 'deleted' WHERE id = $1`, id)
	return err
}

func nilStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// --- Heartbeat (unchanged) ---

type Heartbeat struct {
	mu     sync.RWMutex
	agents map[string]*HeartbeatEntry
}

type HeartbeatEntry struct {
	AgentID    string    `json:"agent_id"`
	Status     string    `json:"status"`
	LastPing   time.Time `json:"last_ping"`
	ErrorCount int       `json:"error_count"`
	LastError  string    `json:"last_error,omitempty"`
}

func NewHeartbeat() *Heartbeat { return &Heartbeat{agents: make(map[string]*HeartbeatEntry)} }

func (h *Heartbeat) Ping(agentID string) {
	h.mu.Lock(); defer h.mu.Unlock()
	e, ok := h.agents[agentID]
	if !ok { h.agents[agentID] = &HeartbeatEntry{AgentID: agentID, Status: "healthy", LastPing: time.Now()}; return }
	e.LastPing = time.Now(); e.Status = "healthy"
}

func (h *Heartbeat) CheckStale() []string {
	h.mu.Lock(); defer h.mu.Unlock()
	dead := []string{}
	for id, e := range h.agents {
		if time.Since(e.LastPing) > 30*time.Second && e.Status != "dead" {
			e.Status = "dead"; dead = append(dead, id)
		}
	}
	return dead
}

func (h *Heartbeat) StartMonitor(ctx context.Context, onDead func(string)) {
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for {
			select {
			case <-ctx.Done(): ticker.Stop(); return
			case <-ticker.C:
				for _, id := range h.CheckStale() { if onDead != nil { onDead(id) } }
			}
		}
	}()
	slog.Info("heartbeat monitor started")
}


// GetDefault returns the default agent (first by created_at).
func (s *Store) GetDefault(ctx context.Context, tenantID string) (*Agent, error) {
	return s.getBy(ctx, "tenant_id", tenantID)
}

// GetByIDs returns agents matching the given IDs.
func (s *Store) GetByIDs(ctx context.Context, ids []string) ([]*Agent, error) {
	if len(ids) == 0 { return nil, nil }
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, agent_key, display_name, COALESCE(avatar,''), role, title, manager_id,
		        provider_id, model, COALESCE(fallback_model,''), system_prompt, temperature,
		        context_window, max_tool_iterations, tool_profile, tools_allowed, tools_denied,
		        memory_enabled, COALESCE(memory_sharing,'private'), auto_compact,
		        credit_budget_cents, credit_used_cents, status, created_at, updated_at,
		        COALESCE(thinking_level,'off')
		 FROM agents WHERE id = ANY($1)`, ids)
	if err != nil { return nil, err }
	defer rows.Close()
	agents := []*Agent{}
	for rows.Next() {
		a := &Agent{}
		rows.Scan(&a.ID, &a.TenantID, &a.AgentKey, &a.DisplayName, &a.Avatar,
			&a.Role, &a.Title, &a.ManagerID, &a.ProviderID, &a.Model, &a.FallbackModel,
			&a.SystemPrompt, &a.Temperature, &a.ContextWindow, &a.MaxToolIterations,
			&a.ToolProfile, &a.ToolsAllowed, &a.ToolsDenied,
			&a.MemoryEnabled, &a.MemorySharing, &a.AutoCompact,
			&a.CreditBudgetCents, &a.CreditUsedCents, &a.Status, &a.CreatedAt, &a.UpdatedAt,
			&a.ThinkingLevel)
		agents = append(agents, a)
	}
	return agents, nil
}

// ListAll returns all agents for a tenant (including inactive).
func (s *Store) ListAll(ctx context.Context, tenantID string) ([]*Agent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, agent_key, display_name, COALESCE(avatar,''), role, title, manager_id,
		        provider_id, model, COALESCE(fallback_model,''), system_prompt, temperature,
		        context_window, max_tool_iterations, tool_profile, tools_allowed, tools_denied,
		        memory_enabled, COALESCE(memory_sharing,'private'), auto_compact,
		        credit_budget_cents, credit_used_cents, status, created_at, updated_at,
		        COALESCE(thinking_level,'off')
		 FROM agents WHERE tenant_id = $1 ORDER BY created_at`, tenantID)
	if err != nil { return nil, err }
	defer rows.Close()
	agents := []*Agent{}
	for rows.Next() {
		a := &Agent{}
		rows.Scan(&a.ID, &a.TenantID, &a.AgentKey, &a.DisplayName, &a.Avatar,
			&a.Role, &a.Title, &a.ManagerID, &a.ProviderID, &a.Model, &a.FallbackModel,
			&a.SystemPrompt, &a.Temperature, &a.ContextWindow, &a.MaxToolIterations,
			&a.ToolProfile, &a.ToolsAllowed, &a.ToolsDenied,
			&a.MemoryEnabled, &a.MemorySharing, &a.AutoCompact,
			&a.CreditBudgetCents, &a.CreditUsedCents, &a.Status, &a.CreatedAt, &a.UpdatedAt,
			&a.ThinkingLevel)
		agents = append(agents, a)
	}
	return agents, nil
}

func (s *Store) GetByIDs2(ctx context.Context, ids []string) ([]*Agent, error) {
	// Already have GetByIDs — this is the interface-compatible version
	return s.GetByIDs(ctx, ids)
}

func (s *Store) ShareAgent(ctx context.Context, agentID, targetUserID, permission string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_shares (agent_id, user_id, permission, created_at)
		 VALUES ($1, $2, $3, NOW()) ON CONFLICT (agent_id, user_id) DO UPDATE SET permission = $3`,
		agentID, targetUserID, permission)
	return err
}

func (s *Store) RevokeShare(ctx context.Context, agentID, targetUserID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM agent_shares WHERE agent_id = $1 AND user_id = $2`, agentID, targetUserID)
	return err
}

func (s *Store) CanAccess(ctx context.Context, agentID, userID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM agents WHERE id = $1 AND (tenant_id = $2 OR id IN (SELECT agent_id FROM agent_shares WHERE user_id = $2))
		)`, agentID, userID).Scan(&exists)
	return exists, err
}

func (s *Store) GetAgentContextFiles(ctx context.Context, agentID string) (map[string]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT file_name, content FROM agent_context_files WHERE agent_id = $1`, agentID)
	if err != nil { return nil, err }
	defer rows.Close()
	files := map[string]string{}
	for rows.Next() {
		var name, content string
		rows.Scan(&name, &content)
		files[name] = content
	}
	return files, nil
}

func (s *Store) SetAgentContextFile(ctx context.Context, agentID, fileName, content string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_context_files (agent_id, file_name, content, updated_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT (agent_id, file_name) DO UPDATE SET content = $3, updated_at = NOW()`,
		agentID, fileName, content)
	return err
}

// isValidAgentKey checks that agent keys contain only safe characters.
func isValidAgentKey(key string) bool {
	if key == "" || len(key) > 100 { return false }
	for _, r := range key {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

// stripHTMLTags removes HTML tags from a string to prevent stored XSS.
func stripHTMLTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' { inTag = true; continue }
		if r == '>' { inTag = false; continue }
		if !inTag { b.WriteRune(r) }
	}
	return strings.TrimSpace(b.String())
}
