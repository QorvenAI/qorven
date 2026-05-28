-- 025: ERP Governance — Designation catalog, approval matrix, policy engine,
-- task state machine, variance tracking, segregation of duties.

-- ─── 1. Designation Catalog ─────────────────────────────────────────────────
-- Master record for every position in the agent org. CHRO defines positions,
-- CKO assigns knowledge packs, CFO assigns budget tiers.

CREATE TABLE IF NOT EXISTS designations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL,
    position_name TEXT NOT NULL,
    department    TEXT NOT NULL,       -- finance, hr, knowledge, engineering, marketing, sales, ops, support, project
    org_layer     INT NOT NULL DEFAULT 3,  -- 2=C-Suite, 3=Worker, 4=Subagent
    nature_of_work TEXT,
    reports_to_designation UUID REFERENCES designations(id),
    skill_family  TEXT,               -- orchestration, governance, knowledge, research, coding, content, sales, support, planning, analytics
    model_tier    TEXT DEFAULT 'balanced',  -- fast, balanced, powerful, reasoning
    tools_allowed TEXT[] DEFAULT '{}',
    tools_denied  TEXT[] DEFAULT '{}',
    max_budget_usd NUMERIC(10,2) DEFAULT 0,
    can_create_subagents BOOLEAN DEFAULT false,
    can_approve_actions  BOOLEAN DEFAULT false,
    requires_approval    BOOLEAN DEFAULT false,  -- does creating this role need CHRO approval?
    user_creatable       BOOLEAN DEFAULT true,   -- can users create agents with this designation?
    knowledge_packs      TEXT[] DEFAULT '{}',    -- SOPs, templates, memory domains
    approval_scope       TEXT[] DEFAULT '{}',    -- what action types this role can approve
    created_at    TIMESTAMPTZ DEFAULT now(),
    updated_at    TIMESTAMPTZ DEFAULT now(),
    UNIQUE (tenant_id, position_name)
);

CREATE INDEX idx_designations_tenant ON designations (tenant_id);
CREATE INDEX idx_designations_dept ON designations (tenant_id, department);

-- ─── 2. Skill Family / Capability Matrix ────────────────────────────────────

CREATE TABLE IF NOT EXISTS skill_families (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    name         TEXT NOT NULL,       -- orchestration, governance, knowledge, etc.
    description  TEXT,
    capabilities TEXT[] DEFAULT '{}', -- reasoning, coding, research, content, routing, analysis
    model_suggestions TEXT[] DEFAULT '{}', -- suggested model IDs for this family
    tool_permissions  TEXT[] DEFAULT '{}', -- default tools for this family
    created_at   TIMESTAMPTZ DEFAULT now(),
    UNIQUE (tenant_id, name)
);

-- ─── 3. Approval Matrix ─────────────────────────────────────────────────────
-- Defines WHO can approve WHAT at WHICH threshold.

CREATE TABLE IF NOT EXISTS approval_matrix (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID NOT NULL,
    action_type    TEXT NOT NULL,     -- spawn_agent, model_upgrade, external_publish, budget_exceed, delete_memory, tool_install, production_deploy
    threshold_usd  NUMERIC(10,2) DEFAULT 0,  -- 0 = always requires approval regardless of cost
    threshold_field TEXT,             -- optional: field name for non-USD thresholds (e.g., "token_count")
    threshold_value NUMERIC DEFAULT 0,
    approver_role  TEXT NOT NULL,     -- org_role of who approves: cfo, chro, coo, user
    approver_level INT DEFAULT 2,    -- minimum org_level that can approve
    requires_human BOOLEAN DEFAULT false, -- must a human (L1) approve, not just C-suite agent?
    auto_approve_below NUMERIC(10,2) DEFAULT 0, -- auto-approve if cost below this
    escalation_timeout_min INT DEFAULT 60, -- escalate after N minutes without response
    escalation_to TEXT,              -- escalate to this role if timeout
    enabled       BOOLEAN DEFAULT true,
    priority      INT DEFAULT 0,     -- lower = checked first
    created_at    TIMESTAMPTZ DEFAULT now(),
    UNIQUE (tenant_id, action_type, approver_role, priority)
);

CREATE INDEX idx_approval_matrix_tenant ON approval_matrix (tenant_id, enabled);

-- Approval requests (pending decisions)
CREATE TABLE IF NOT EXISTS approval_requests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    action_type     TEXT NOT NULL,
    requestor_id    UUID NOT NULL,     -- agent requesting approval
    requestor_key   TEXT,
    approver_role   TEXT NOT NULL,
    approver_id     UUID,              -- assigned approver (NULL = any with role)
    matrix_rule_id  UUID REFERENCES approval_matrix(id),
    context         JSONB DEFAULT '{}', -- action details, cost, scope
    status          TEXT DEFAULT 'pending', -- pending, approved, denied, expired, escalated
    decision_by     UUID,              -- who approved/denied
    decision_at     TIMESTAMPTZ,
    decision_reason TEXT,
    expires_at      TIMESTAMPTZ,
    escalated_to    TEXT,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_approval_requests_pending ON approval_requests (tenant_id, status) WHERE status = 'pending';

-- ─── 4. Policy Engine ────────────────────────────────────────────────────────
-- Executable business rules: conditions → actions. Evaluated on every sensitive operation.

CREATE TABLE IF NOT EXISTS policies (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL,
    name          TEXT NOT NULL,
    description   TEXT,
    category      TEXT NOT NULL,      -- budget, access, tool, model, memory, output, lifecycle
    trigger_event TEXT NOT NULL,      -- tool_call, model_switch, budget_spend, memory_write, agent_spawn, output_deliver, external_action
    conditions    JSONB NOT NULL DEFAULT '[]',  -- [{field, operator, value}] — AND logic
    action        TEXT NOT NULL,      -- allow, deny, require_approval, warn, log, throttle, escalate
    action_params JSONB DEFAULT '{}', -- { approver_role, message, limit, cooldown_sec }
    applies_to_roles TEXT[] DEFAULT '{}', -- empty = all roles
    applies_to_levels INT[] DEFAULT '{}', -- empty = all levels
    priority      INT DEFAULT 0,     -- lower = evaluated first
    enabled       BOOLEAN DEFAULT true,
    created_at    TIMESTAMPTZ DEFAULT now(),
    updated_at    TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_policies_trigger ON policies (tenant_id, trigger_event, enabled);

-- Policy evaluation log (every policy hit is recorded)
CREATE TABLE IF NOT EXISTS policy_events (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    policy_id    UUID REFERENCES policies(id),
    policy_name  TEXT,
    agent_id     UUID,
    agent_key    TEXT,
    trigger_event TEXT,
    action_taken TEXT,                -- allow, deny, require_approval, warn
    context      JSONB DEFAULT '{}',
    created_at   TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_policy_events_recent ON policy_events (tenant_id, created_at DESC);

-- ─── 5. Task State Machine ──────────────────────────────────────────────────
-- Every work unit moves through standard states. Append-only transitions.

DO $$ BEGIN
    ALTER TABLE tasks ADD COLUMN IF NOT EXISTS workflow_state TEXT DEFAULT 'draft';
    ALTER TABLE tasks ADD COLUMN IF NOT EXISTS state_changed_at TIMESTAMPTZ DEFAULT now();
    ALTER TABLE tasks ADD COLUMN IF NOT EXISTS state_changed_by UUID;
    ALTER TABLE tasks ADD COLUMN IF NOT EXISTS blocked_reason TEXT;
    ALTER TABLE tasks ADD COLUMN IF NOT EXISTS requires_approval BOOLEAN DEFAULT false;
    ALTER TABLE tasks ADD COLUMN IF NOT EXISTS approval_request_id UUID;
EXCEPTION WHEN insufficient_privilege THEN NULL;
END $$;

-- Valid states: draft, submitted, routed, in_progress, waiting_approval, blocked, completed, rejected, archived
-- Transitions are enforced in code (TaskStateMachine)

CREATE TABLE IF NOT EXISTS task_state_transitions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    task_id      UUID NOT NULL,
    from_state   TEXT NOT NULL,
    to_state     TEXT NOT NULL,
    changed_by   UUID,               -- agent or user who caused transition
    reason       TEXT,
    metadata     JSONB DEFAULT '{}',
    created_at   TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_task_transitions_task ON task_state_transitions (task_id, created_at DESC);

-- ─── 6. Variance / Exception Tracking ──────────────────────────────────────
-- Unified exception ledger for policy violations, cost overruns, quality failures.

CREATE TABLE IF NOT EXISTS exceptions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL,
    category      TEXT NOT NULL,      -- budget_exceeded, quality_failure, policy_violation, sla_breach, approval_timeout, tool_scope_violation
    severity      TEXT NOT NULL DEFAULT 'warning', -- info, warning, critical
    agent_id      UUID,
    agent_key     TEXT,
    description   TEXT NOT NULL,
    context       JSONB DEFAULT '{}', -- details: cost, threshold, tool, rule violated
    resolution    TEXT,               -- NULL = unresolved
    resolved_by   UUID,
    resolved_at   TIMESTAMPTZ,
    acknowledged  BOOLEAN DEFAULT false,
    created_at    TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_exceptions_unresolved ON exceptions (tenant_id, resolved_at) WHERE resolved_at IS NULL;
CREATE INDEX idx_exceptions_recent ON exceptions (tenant_id, created_at DESC);

-- ─── 7. Segregation of Duties ───────────────────────────────────────────────
-- Rules preventing same agent from performing conflicting actions.

CREATE TABLE IF NOT EXISTS sod_rules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    name         TEXT NOT NULL,
    description  TEXT,
    action_a     TEXT NOT NULL,       -- e.g., "request_budget"
    action_b     TEXT NOT NULL,       -- e.g., "approve_budget"
    scope        TEXT DEFAULT 'same_task', -- same_task, same_session, always
    enabled      BOOLEAN DEFAULT true,
    created_at   TIMESTAMPTZ DEFAULT now(),
    UNIQUE (tenant_id, action_a, action_b)
);

-- ─── 8. Asset Inventory (Reusable Intelligence Assets) ──────────────────────

CREATE TABLE IF NOT EXISTS asset_library (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    asset_type   TEXT NOT NULL,       -- prompt_template, workflow_template, tool_recipe, knowledge_pack, memory_bundle, integration_adapter
    name         TEXT NOT NULL,
    description  TEXT,
    content      JSONB NOT NULL,      -- the actual asset data
    version      INT DEFAULT 1,
    tags         TEXT[] DEFAULT '{}',
    created_by   UUID,
    approved_by  UUID,
    usage_count  INT DEFAULT 0,
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ DEFAULT now(),
    updated_at   TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_asset_library_type ON asset_library (tenant_id, asset_type);
CREATE INDEX idx_asset_library_tags ON asset_library USING gin (tags);

-- ─── 9. Forecasting / Planning ──────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS capacity_forecasts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL,
    forecast_date DATE NOT NULL,
    metric        TEXT NOT NULL,      -- daily_cost_usd, daily_tokens, active_agents, queue_depth, avg_latency_ms
    predicted     NUMERIC NOT NULL,
    actual        NUMERIC,            -- filled in once the day passes
    confidence    NUMERIC DEFAULT 0.8,
    model_used    TEXT,               -- what forecast model generated this
    created_at    TIMESTAMPTZ DEFAULT now(),
    UNIQUE (tenant_id, forecast_date, metric)
);

-- ─── 10. SLA Tracking ───────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS sla_definitions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    name         TEXT NOT NULL,
    target_type  TEXT NOT NULL,       -- task_completion, response_time, quality_score, uptime
    target_value NUMERIC NOT NULL,    -- e.g., 300000 (ms for response time), 7.0 (quality score)
    measurement  TEXT NOT NULL,       -- p50, p95, p99, avg, min
    time_window  TEXT DEFAULT '1d',   -- 1h, 1d, 7d, 30d
    applies_to   TEXT[] DEFAULT '{}', -- agent keys or departments (empty = all)
    enabled      BOOLEAN DEFAULT true,
    created_at   TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sla_measurements (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    sla_id       UUID REFERENCES sla_definitions(id),
    period_start TIMESTAMPTZ NOT NULL,
    period_end   TIMESTAMPTZ NOT NULL,
    measured     NUMERIC NOT NULL,
    met          BOOLEAN NOT NULL,
    breach_count INT DEFAULT 0,
    created_at   TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_sla_measurements_recent ON sla_measurements (tenant_id, period_end DESC);
