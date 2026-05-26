-- Migration 018: Budget raise requests — agents request increases, CFO approves/denies.
CREATE TABLE IF NOT EXISTS budget_requests (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID NOT NULL,
    agent_id       UUID NOT NULL,
    current_usd    NUMERIC(10,2) NOT NULL DEFAULT 0,
    requested_usd  NUMERIC(10,2) NOT NULL,
    reason         TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'pending',  -- pending | approved | denied
    decided_by     TEXT,          -- agent_id or 'user' who decided
    decision_note  TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_budget_requests_tenant ON budget_requests (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_budget_requests_agent  ON budget_requests (agent_id, created_at);
