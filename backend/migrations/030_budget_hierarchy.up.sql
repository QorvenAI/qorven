-- 030_budget_hierarchy: department + project entities, agent department link,
-- and the hierarchy/allocation columns the budget enforcer rolls up through.
-- Fresh-install schema; backfills `scope` on any existing gateway_budgets rows.

CREATE TABLE IF NOT EXISTS departments (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID NOT NULL,
    name                 TEXT NOT NULL,
    head_agent_id        UUID,
    parent_department_id UUID,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_departments_tenant ON departments (tenant_id);

CREATE TABLE IF NOT EXISTS projects (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL,
    name          TEXT NOT NULL,
    department_id UUID,
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_projects_tenant ON projects (tenant_id);

ALTER TABLE agents ADD COLUMN IF NOT EXISTS department_id UUID;

-- Hierarchy + allocation on the caps table.
ALTER TABLE gateway_budgets ADD COLUMN IF NOT EXISTS scope           TEXT;
ALTER TABLE gateway_budgets ADD COLUMN IF NOT EXISTS department_id   UUID;
ALTER TABLE gateway_budgets ADD COLUMN IF NOT EXISTS allocation_mode TEXT NOT NULL DEFAULT 'carved';
ALTER TABLE gateway_budgets ADD COLUMN IF NOT EXISTS parent_scope    TEXT;
ALTER TABLE gateway_budgets ADD COLUMN IF NOT EXISTS parent_scope_id UUID;

-- Backfill the explicit scope discriminator for any existing rows.
UPDATE gateway_budgets SET scope = 'agent'
    WHERE scope IS NULL AND agent_id IS NOT NULL;
UPDATE gateway_budgets SET scope = 'tenant'
    WHERE scope IS NULL AND agent_id IS NULL AND project_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_gateway_budgets_scope
    ON gateway_budgets (tenant_id, scope, department_id, project_id);

-- Denormalized scope ids on the raw ledger so dept/project/task spend is a
-- plain GROUP BY (no recursive walks over manager_id / org_hierarchy).
ALTER TABLE gateway_spend_raw ADD COLUMN IF NOT EXISTS department_id UUID;
ALTER TABLE gateway_spend_raw ADD COLUMN IF NOT EXISTS project_id    UUID;
ALTER TABLE gateway_spend_raw ADD COLUMN IF NOT EXISTS task_id       UUID;
CREATE INDEX IF NOT EXISTS idx_spend_raw_department ON gateway_spend_raw (tenant_id, department_id, created_at) WHERE department_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_spend_raw_project    ON gateway_spend_raw (tenant_id, project_id, created_at)    WHERE project_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_spend_raw_task       ON gateway_spend_raw (tenant_id, task_id, created_at)       WHERE task_id IS NOT NULL;
