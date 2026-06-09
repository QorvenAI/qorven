-- 032_cfo_proposals: CFO budget-allocation proposals (multi-line, user-approved)
-- and the per-tenant CFO authority setting. Approval applies each line through
-- the validated budgets store. Mirrors budgets.BudgetScope field-for-field.

CREATE TABLE IF NOT EXISTS budget_allocation_proposals (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL,
    proposed_by   UUID,
    reason        TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'pending', -- pending|approved|rejected|partially_approved|auto_applied
    decided_by    TEXT,
    decision_note TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_alloc_proposals_tenant_status
    ON budget_allocation_proposals (tenant_id, status, created_at);

CREATE TABLE IF NOT EXISTS budget_allocation_lines (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    proposal_id           UUID NOT NULL,
    scope                 TEXT NOT NULL,            -- tenant|department|project|agent
    scope_id              UUID,
    proposed_monthly_usd  NUMERIC(10,2) NOT NULL DEFAULT 0,
    proposed_lifetime_usd NUMERIC(10,2) NOT NULL DEFAULT 0,
    proposed_pct          NUMERIC,                 -- optional "% of parent" before $ resolution
    allocation_mode       TEXT NOT NULL DEFAULT 'carved', -- carved|fresh
    parent_scope          TEXT,
    parent_scope_id       UUID,
    funding_mode          TEXT,
    status                TEXT NOT NULL DEFAULT 'pending', -- pending|approved|rejected|applied
    decision_note         TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_alloc_lines_proposal ON budget_allocation_lines (proposal_id);

CREATE TABLE IF NOT EXISTS tenant_finance_settings (
    tenant_id         UUID PRIMARY KEY,
    cfo_authority     TEXT NOT NULL DEFAULT 'threshold', -- ask|threshold|full
    cfo_threshold_usd NUMERIC(10,2) NOT NULL DEFAULT 25,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
