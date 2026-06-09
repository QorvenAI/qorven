-- 037_room_run_budgets: per-room agent-turn ledger for the Operations Fabric hub.
-- One row per agent response triggered in a room. A rolling-window count of these
-- rows caps how many automated agent turns a room may consume, so agent activity
-- in a room can never loop or burn money unbounded.

CREATE TABLE IF NOT EXISTS room_run_budgets (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID NOT NULL,
    room_id    UUID NOT NULL,
    agent_id   UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_room_run_budgets_window
    ON room_run_budgets (room_id, created_at);
