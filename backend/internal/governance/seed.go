// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package governance

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedDefaults populates the designation catalog, skill families, approval matrix,
// and default policies if none exist. Safe to call on every boot.
func SeedDefaults(ctx context.Context, db *pgxpool.Pool, tenantID string) {
	if db == nil {
		return
	}

	// Only seed if empty
	var count int
	db.QueryRow(ctx, `SELECT COUNT(*) FROM designations WHERE tenant_id = $1`, tenantID).Scan(&count)
	if count > 0 {
		return
	}

	slog.Info("governance.seed: populating default designations and policies", "tenant", tenantID)

	// ─── Skill Families ─────────────────────────────────────────────────────
	families := []struct {
		Name, Desc string
		Caps, Tools []string
	}{
		{"orchestration", "Workflow coordination, routing, and delegation", []string{"reasoning", "planning", "routing"}, []string{"delegate", "team_tasks", "workflow"}},
		{"governance", "Budget, compliance, audit, and policy enforcement", []string{"analysis", "compliance"}, []string{"billing", "audit", "budget"}},
		{"knowledge", "Memory, SOPs, rules, and knowledge curation", []string{"research", "synthesis", "memory"}, []string{"memory_search", "knowledge_graph_search", "memory_write"}},
		{"research", "Deep web research, social listening, data gathering", []string{"research", "analysis", "synthesis"}, []string{"web_search", "research", "web_fetch", "scrape"}},
		{"coding", "Software engineering, architecture, DevOps", []string{"coding", "debugging", "architecture"}, []string{"exec", "write_file", "read_file", "git"}},
		{"content", "Writing, marketing copy, social media", []string{"writing", "creativity", "brand"}, []string{"write_file", "social_post", "cms_publish"}},
		{"sales", "Outreach, pipeline, proposals, CRM", []string{"persuasion", "research", "writing"}, []string{"email_send", "crm_update", "web_search"}},
		{"support", "Customer support, onboarding, escalation", []string{"empathy", "problem_solving"}, []string{"ticket_update", "knowledge_search", "email_send"}},
		{"planning", "Project management, milestones, dependencies", []string{"planning", "coordination", "analysis"}, []string{"team_tasks", "calendar", "project_update"}},
		{"analytics", "KPIs, reporting, forecasting, data analysis", []string{"analysis", "statistics", "visualization"}, []string{"query_data", "create_chart", "report"}},
	}
	for _, f := range families {
		db.Exec(ctx, `INSERT INTO skill_families (tenant_id, name, description, capabilities, tool_permissions)
			VALUES ($1,$2,$3,$4,$5) ON CONFLICT (tenant_id, name) DO NOTHING`,
			tenantID, f.Name, f.Desc, f.Caps, f.Tools)
	}

	// ─── L2 C-Suite Designations ────────────────────────────────────────────
	csuite := []Designation{
		{TenantID: tenantID, PositionName: "Chief Operating Officer Agent", Department: "ops", OrgLayer: 2, NatureOfWork: "Receives user goals, converts into plans, routes work, coordinates handoffs, resolves conflicts", SkillFamily: "orchestration", ModelTier: "powerful", CanCreateSubagents: true, CanApproveActions: true, ApprovalScope: []string{"workflow_override", "routing_exception", "escalation"}},
		{TenantID: tenantID, PositionName: "Chief Finance Officer Agent", Department: "finance", OrgLayer: 2, NatureOfWork: "Tracks token usage, assigns budgets, monitors cost efficiency, approves expensive model use", SkillFamily: "governance", ModelTier: "balanced", CanApproveActions: true, ApprovalScope: []string{"budget_exceed", "model_upgrade", "spend_exception"}},
		{TenantID: tenantID, PositionName: "Chief Human Resources Officer Agent", Department: "hr", OrgLayer: 2, NatureOfWork: "Creates, assigns, promotes, demotes, and terminates agents; maps job roles to skills", SkillFamily: "governance", ModelTier: "balanced", CanCreateSubagents: true, CanApproveActions: true, ApprovalScope: []string{"spawn_agent", "agent_terminate", "role_change"}},
		{TenantID: tenantID, PositionName: "Chief Knowledge Officer Agent", Department: "knowledge", OrgLayer: 2, NatureOfWork: "Maintains KB, memory, rules, SOPs; distributes context to agents", SkillFamily: "knowledge", ModelTier: "balanced", CanApproveActions: true, ApprovalScope: []string{"knowledge_write", "memory_delete", "classification_change"}},
		{TenantID: tenantID, PositionName: "Chief Technology Officer Agent", Department: "engineering", OrgLayer: 2, NatureOfWork: "Owns engineering architecture, code quality, technical planning, system design", SkillFamily: "coding", ModelTier: "powerful", CanCreateSubagents: true, CanApproveActions: true, ApprovalScope: []string{"production_deploy", "architecture_change"}},
		{TenantID: tenantID, PositionName: "Chief Marketing Officer Agent", Department: "marketing", OrgLayer: 2, NatureOfWork: "Owns campaigns, messaging, brand, content strategy, social workflows", SkillFamily: "content", ModelTier: "balanced", CanCreateSubagents: true, CanApproveActions: true, ApprovalScope: []string{"external_publish", "campaign_launch"}},
		{TenantID: tenantID, PositionName: "Chief Sales Officer Agent", Department: "sales", OrgLayer: 2, NatureOfWork: "Owns lead strategy, pipeline, outreach design, deal prioritization", SkillFamily: "sales", ModelTier: "balanced", CanApproveActions: true, ApprovalScope: []string{"outreach_send", "deal_exception"}},
		{TenantID: tenantID, PositionName: "Chief Risk & Compliance Officer Agent", Department: "compliance", OrgLayer: 2, NatureOfWork: "Reviews policy violations, risky actions, approvals, audit events, governance exceptions", SkillFamily: "governance", ModelTier: "balanced", CanApproveActions: true, ApprovalScope: []string{"policy_override", "security_exception"}},
	}

	// ─── L3 Worker Designations ─────────────────────────────────────────────
	workers := []Designation{
		// Operations
		{TenantID: tenantID, PositionName: "Workflow Coordinator Agent", Department: "ops", OrgLayer: 3, NatureOfWork: "Executes assigned workflows step by step, updates status, hands work to next role", SkillFamily: "orchestration", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Task Routing Agent", Department: "ops", OrgLayer: 3, NatureOfWork: "Routes tasks to correct agent based on rules, urgency, cost, and complexity", SkillFamily: "orchestration", ModelTier: "fast"},
		{TenantID: tenantID, PositionName: "Escalation Desk Agent", Department: "ops", OrgLayer: 3, NatureOfWork: "Detects blocked workflows, failed outputs, or policy exceptions and escalates", SkillFamily: "orchestration", ModelTier: "fast"},
		{TenantID: tenantID, PositionName: "Audit Trail Agent", Department: "ops", OrgLayer: 3, NatureOfWork: "Records actions, tool calls, handoffs, approvals, and evidence into system ledger", SkillFamily: "governance", ModelTier: "fast"},
		// HR
		{TenantID: tenantID, PositionName: "Agent Recruiter", Department: "hr", OrgLayer: 3, NatureOfWork: "Matches tasks with right agent role, recommends hiring or creation of new agents", SkillFamily: "governance", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Agent Onboarding Specialist", Department: "hr", OrgLayer: 3, NatureOfWork: "Sets up new agents with prompts, tools, policies, KB access, reporting relationships", SkillFamily: "governance", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Agent Performance Reviewer", Department: "hr", OrgLayer: 3, NatureOfWork: "Measures output quality, cost efficiency, compliance, and role effectiveness", SkillFamily: "analytics", ModelTier: "balanced"},
		// Knowledge
		{TenantID: tenantID, PositionName: "Knowledge Librarian Agent", Department: "knowledge", OrgLayer: 3, NatureOfWork: "Organizes documents, SOPs, rules, and reusable knowledge assets", SkillFamily: "knowledge", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Memory Curator Agent", Department: "knowledge", OrgLayer: 3, NatureOfWork: "Decides what to store, update, merge, archive, or remove from memory", SkillFamily: "knowledge", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Research Analyst Agent", Department: "knowledge", OrgLayer: 3, NatureOfWork: "Collects external/internal information and turns it into structured insight", SkillFamily: "research", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Context Packaging Agent", Department: "knowledge", OrgLayer: 3, NatureOfWork: "Prepares minimum relevant context bundle needed for a task before handoff", SkillFamily: "knowledge", ModelTier: "fast"},
		// Engineering
		{TenantID: tenantID, PositionName: "Software Engineer Agent", Department: "engineering", OrgLayer: 3, NatureOfWork: "Writes, edits, and tests code within a defined scope", SkillFamily: "coding", ModelTier: "powerful", CanCreateSubagents: true},
		{TenantID: tenantID, PositionName: "Code Review Agent", Department: "engineering", OrgLayer: 3, NatureOfWork: "Reviews code quality, architecture alignment, bugs, and best-practice violations", SkillFamily: "coding", ModelTier: "powerful"},
		{TenantID: tenantID, PositionName: "QA Test Engineer Agent", Department: "engineering", OrgLayer: 3, NatureOfWork: "Runs test cases, validates functionality, finds regressions, reports defects", SkillFamily: "coding", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Integration Engineer Agent", Department: "engineering", OrgLayer: 3, NatureOfWork: "Connects APIs, tools, databases, connectors, and automation services", SkillFamily: "coding", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "DevOps Agent", Department: "engineering", OrgLayer: 3, NatureOfWork: "Handles deployments, environments, CI/CD, monitoring, rollback", SkillFamily: "coding", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Security Review Agent", Department: "engineering", OrgLayer: 3, NatureOfWork: "Checks permissions, secrets, unsafe code paths, external integrations", SkillFamily: "governance", ModelTier: "powerful"},
		// Finance
		{TenantID: tenantID, PositionName: "Budget Analyst Agent", Department: "finance", OrgLayer: 3, NatureOfWork: "Tracks spend by workflow, project, and agent; flags variances", SkillFamily: "analytics", ModelTier: "fast"},
		{TenantID: tenantID, PositionName: "Cost Optimisation Analyst Agent", Department: "finance", OrgLayer: 3, NatureOfWork: "Suggests cheaper models, better routing, batching to reduce cost", SkillFamily: "analytics", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Forecasting Analyst Agent", Department: "finance", OrgLayer: 3, NatureOfWork: "Predicts cost trends, resource demand, and future workload", SkillFamily: "analytics", ModelTier: "balanced"},
		// Marketing
		{TenantID: tenantID, PositionName: "Social Media Manager Agent", Department: "marketing", OrgLayer: 3, NatureOfWork: "Plans and executes posting schedules, social content, platform-specific outputs", SkillFamily: "content", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Content Writer Agent", Department: "marketing", OrgLayer: 3, NatureOfWork: "Produces blogs, landing-page copy, captions, scripts, campaign assets", SkillFamily: "content", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "SEO Analyst Agent", Department: "marketing", OrgLayer: 3, NatureOfWork: "Keyword research, search optimization, content improvements, ranking audits", SkillFamily: "research", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Brand Guardian Agent", Department: "marketing", OrgLayer: 3, NatureOfWork: "Ensures outputs match tone, brand rules, messaging standards", SkillFamily: "content", ModelTier: "balanced"},
		// Sales
		{TenantID: tenantID, PositionName: "Lead Research Agent", Department: "sales", OrgLayer: 3, NatureOfWork: "Finds prospects, enriches lead data, prepares prospect context", SkillFamily: "research", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Outreach Agent", Department: "sales", OrgLayer: 3, NatureOfWork: "Drafts personalized outreach messages and sequences for leads", SkillFamily: "sales", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Pipeline Coordinator Agent", Department: "sales", OrgLayer: 3, NatureOfWork: "Moves opportunities through stages, updates CRM, tracks blockers", SkillFamily: "sales", ModelTier: "fast"},
		{TenantID: tenantID, PositionName: "Proposal Writer Agent", Department: "sales", OrgLayer: 3, NatureOfWork: "Creates proposals, sales docs, summaries, account-facing materials", SkillFamily: "content", ModelTier: "balanced"},
		// Support
		{TenantID: tenantID, PositionName: "Customer Support Agent", Department: "support", OrgLayer: 3, NatureOfWork: "Handles customer queries, tickets, FAQs, guided issue resolution", SkillFamily: "support", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Onboarding Specialist Agent", Department: "support", OrgLayer: 3, NatureOfWork: "Helps new users adopt platform, set up workflows, activate use cases", SkillFamily: "support", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Success Manager Agent", Department: "support", OrgLayer: 3, NatureOfWork: "Monitors adoption, identifies churn signals, recommends actions", SkillFamily: "analytics", ModelTier: "balanced"},
		// Project
		{TenantID: tenantID, PositionName: "Project Coordinator Agent", Department: "project", OrgLayer: 3, NatureOfWork: "Tracks milestones, dependencies, owners, and completion state", SkillFamily: "planning", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Business Analyst Agent", Department: "project", OrgLayer: 3, NatureOfWork: "Gathers requirements, clarifies needs, turns requests into specs", SkillFamily: "planning", ModelTier: "balanced"},
		{TenantID: tenantID, PositionName: "Documentation Agent", Department: "project", OrgLayer: 3, NatureOfWork: "Produces internal docs, release notes, SOPs, summaries", SkillFamily: "content", ModelTier: "fast"},
	}

	ds := NewDesignationStore(db)
	for _, d := range csuite {
		if err := ds.Upsert(ctx, d); err != nil {
			slog.Warn("governance.seed.csuite_failed", "position", d.PositionName, "error", err)
		}
	}
	for _, d := range workers {
		if err := ds.Upsert(ctx, d); err != nil {
			slog.Warn("governance.seed.worker_failed", "position", d.PositionName, "error", err)
		}
	}

	// ─── Default Approval Matrix ────────────────────────────────────────────
	approvals := []struct {
		Action, Approver string
		Threshold, Auto  float64
		Human            bool
	}{
		{"spawn_agent", "chro", 0, 0, false},
		{"model_upgrade", "cfo", 0.50, 0.10, false},
		{"external_publish", "cmo", 0, 0, false},
		{"budget_exceed", "cfo", 0, 0, true},
		{"production_deploy", "cto", 0, 0, true},
		{"delete_memory", "cko", 0, 0, false},
		{"tool_install", "cto", 0, 0, false},
		{"policy_override", "user", 0, 0, true},
	}
	for i, a := range approvals {
		db.Exec(ctx, `INSERT INTO approval_matrix (tenant_id, action_type, threshold_usd, approver_role, approver_level, requires_human, auto_approve_below, priority, enabled)
			VALUES ($1,$2,$3,$4,2,$5,$6,$7,true)
			ON CONFLICT (tenant_id, action_type, approver_role, priority) DO NOTHING`,
			tenantID, a.Action, a.Threshold, a.Approver, a.Human, a.Auto, i)
	}

	// ─── Default Policies ───────────────────────────────────────────────────
	type seedPolicy struct {
		Name, Cat, Trigger, Action string
		Conds                      []PolicyCond
	}
	policies := []seedPolicy{
		{"Block PII in outputs", "output", "output_deliver", "deny", []PolicyCond{{Field: "has_pii", Operator: "equals", Value: "true"}}},
		{"Warn on expensive model", "budget", "model_switch", "warn", []PolicyCond{{Field: "cost_per_1m_input", Operator: "gt", Value: "15"}}},
		{"Require approval for external API", "tool", "tool_call", "require_approval", []PolicyCond{{Field: "tool_category", Operator: "equals", Value: "external_api"}}},
		{"Log all memory writes", "memory", "memory_write", "log", []PolicyCond{}},
		{"Throttle rapid agent spawns", "lifecycle", "agent_spawn", "throttle", []PolicyCond{{Field: "spawns_last_5min", Operator: "gt", Value: "10"}}},
	}
	for i, p := range policies {
		condJSON := "[]"
		if len(p.Conds) > 0 {
			b, _ := json.Marshal(p.Conds)
			condJSON = string(b)
		}
		db.Exec(ctx, `INSERT INTO policies (tenant_id, name, category, trigger_event, conditions, action, priority, enabled)
			VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,true) ON CONFLICT DO NOTHING`,
			tenantID, p.Name, p.Cat, p.Trigger, condJSON, p.Action, i)
	}

	// ─── Default SoD Rules ──────────────────────────────────────────────────
	sods := [][2]string{
		{"request_budget", "approve_budget"},
		{"write_code", "approve_deploy"},
		{"create_agent", "approve_agent"},
		{"write_policy", "evaluate_policy"},
	}
	for _, s := range sods {
		db.Exec(ctx, `INSERT INTO sod_rules (tenant_id, name, action_a, action_b, scope, enabled)
			VALUES ($1, $2, $3, $4, 'same_task', true) ON CONFLICT (tenant_id, action_a, action_b) DO NOTHING`,
			tenantID, "SoD: "+s[0]+" / "+s[1], s[0], s[1])
	}

	slog.Info("governance.seed: complete", "designations", len(csuite)+len(workers), "families", len(families))
}
