-- 024: ERP-grade orchestration tables
-- Pillars: Org Hierarchy, Audit Trails, Routing, Output Quality, Workflow Orchestration

-- Pillar 1: Org Hierarchy enforcement
CREATE TABLE IF NOT EXISTS org_hierarchy (
    tenant_id       UUID NOT NULL,
    agent_id        UUID NOT NULL,
    reports_to      UUID,
    org_level       INT NOT NULL,
    org_role        TEXT,
    can_delegate_to UUID[],
    max_budget_usd  NUMERIC(10,2),
    PRIMARY KEY (tenant_id, agent_id)
);

-- Pillar 1: L4 Subagent run persistence
CREATE TABLE IF NOT EXISTS subagent_runs (
    id              TEXT PRIMARY KEY,
    tenant_id       UUID NOT NULL,
    parent_id       TEXT NOT NULL,
    agent_key       TEXT,
    task            TEXT NOT NULL,
    status          TEXT NOT NULL,
    result          TEXT,
    depth           INT NOT NULL,
    iterations      INT DEFAULT 0,
    tools_used      TEXT[],
    tokens_in       BIGINT DEFAULT 0,
    tokens_out      BIGINT DEFAULT 0,
    cost_uusd       BIGINT DEFAULT 0,
    session_id      TEXT,
    trace_id        UUID,
    created_at      TIMESTAMPTZ DEFAULT now(),
    completed_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_subagent_runs_parent ON subagent_runs (parent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_subagent_runs_tenant ON subagent_runs (tenant_id, created_at DESC);

-- Pillar 2: Budget enhancements
ALTER TABLE gateway_budgets ADD COLUMN IF NOT EXISTS project_id UUID;
ALTER TABLE gateway_budgets ADD COLUMN IF NOT EXISTS lifetime_usd NUMERIC(10,2);
ALTER TABLE gateway_budgets ADD COLUMN IF NOT EXISTS warn_percent INT DEFAULT 80;

-- Pillar 3: Output audit
CREATE TABLE IF NOT EXISTS output_audit (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    agent_id        UUID NOT NULL,
    session_id      TEXT,
    channel         TEXT,
    content_type    TEXT,
    content_hash    TEXT NOT NULL,
    content_preview TEXT,
    full_content    TEXT,
    metadata        JSONB,
    quality_score   NUMERIC(3,1),
    validation_result JSONB,
    delivered_at    TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_output_audit_agent ON output_audit (agent_id, delivered_at DESC);
CREATE INDEX IF NOT EXISTS idx_output_audit_hash ON output_audit (content_hash);

-- Pillar 3: Trace linking
ALTER TABLE traces ADD COLUMN IF NOT EXISTS parent_trace_id UUID;
ALTER TABLE traces ADD COLUMN IF NOT EXISTS delegation_id TEXT;

-- Pillar 4: Routing rules
CREATE TABLE IF NOT EXISTS routing_rules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    name        TEXT NOT NULL,
    priority    INT DEFAULT 100,
    conditions  JSONB NOT NULL,
    action      JSONB NOT NULL,
    enabled     BOOLEAN DEFAULT true,
    created_at  TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_routing_rules_priority ON routing_rules (tenant_id, priority ASC);

-- Pillar 6: Workflow run tracking
CREATE TABLE IF NOT EXISTS workflow_runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    workflow_id     UUID NOT NULL,
    status          TEXT NOT NULL DEFAULT 'running',
    current_step_id TEXT,
    context         JSONB DEFAULT '{}',
    trigger_type    TEXT,
    triggered_by    TEXT,
    deadline        TIMESTAMPTZ,
    total_cost_uusd BIGINT DEFAULT 0,
    started_at      TIMESTAMPTZ DEFAULT now(),
    completed_at    TIMESTAMPTZ,
    error           TEXT
);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_wf ON workflow_runs (workflow_id, started_at DESC);

CREATE TABLE IF NOT EXISTS workflow_step_runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id          UUID NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
    step_id         TEXT NOT NULL,
    agent_id        UUID,
    status          TEXT NOT NULL DEFAULT 'pending',
    input           JSONB,
    output          JSONB,
    cost_uusd       BIGINT DEFAULT 0,
    quality_score   NUMERIC(3,1),
    duration_ms     INT,
    retry_count     INT DEFAULT 0,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    error           TEXT
);
CREATE INDEX IF NOT EXISTS idx_wf_step_runs ON workflow_step_runs (run_id, started_at);
