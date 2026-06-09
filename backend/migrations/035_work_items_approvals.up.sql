-- 035_work_items_approvals: Operations Fabric Phase B.
-- work_items = durable delegation chains (owner, status, blocked_on, parent).
-- work_item_events = append-only audit of every transition/event.
-- approvals (unified) = the one approval object new modules use; opening one
-- reaches the user via the Phase A escalation ladder, deciding it acks the climb.

CREATE TABLE IF NOT EXISTS work_items (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    title           TEXT NOT NULL,
    origin          TEXT NOT NULL DEFAULT '',
    room_id         UUID,
    owner_agent_id  TEXT NOT NULL DEFAULT '',
    requested_by    TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'open',
    blocked_on_kind TEXT NOT NULL DEFAULT '',
    blocked_on_id   TEXT NOT NULL DEFAULT '',
    parent_id       UUID,
    budget_plan_id  UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at       TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_work_items_owner  ON work_items (tenant_id, owner_agent_id, status);
CREATE INDEX IF NOT EXISTS idx_work_items_parent ON work_items (parent_id);

CREATE TABLE IF NOT EXISTS work_item_events (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    work_item_id UUID NOT NULL,
    event_type   TEXT NOT NULL,
    actor_id     TEXT NOT NULL DEFAULT '',
    from_status  TEXT NOT NULL DEFAULT '',
    to_status    TEXT NOT NULL DEFAULT '',
    detail       TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_work_item_events_wi ON work_item_events (work_item_id, created_at);

CREATE TABLE IF NOT EXISTS approvals_unified (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID NOT NULL,
    kind               TEXT NOT NULL,
    requester_agent_id TEXT NOT NULL DEFAULT '',
    work_item_id       UUID,
    summary            TEXT NOT NULL DEFAULT '',
    amount_uusd        BIGINT,
    risk               TEXT NOT NULL DEFAULT 'normal',
    context            JSONB NOT NULL DEFAULT '{}',
    status             TEXT NOT NULL DEFAULT 'pending',
    decided_by         TEXT,
    decision_note      TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at         TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_approvals_unified_pending ON approvals_unified (tenant_id, status, created_at);
