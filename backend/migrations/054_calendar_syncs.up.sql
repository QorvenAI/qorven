-- 054_calendar_syncs: sync a calendar scope OUT to an external calendar (Google/Zoho).
CREATE TABLE IF NOT EXISTS calendar_syncs (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL,
    scope             text NOT NULL,
    scope_id          uuid,
    owner_agent_id    uuid,
    provider          text NOT NULL,
    account_id        text NOT NULL DEFAULT '',
    remote_calendar_id text NOT NULL DEFAULT '',
    direction         text NOT NULL DEFAULT 'out',
    enabled           boolean NOT NULL DEFAULT true,
    last_synced_at    timestamptz,
    error             text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_calendar_syncs_tenant ON calendar_syncs (tenant_id, enabled);

CREATE TABLE IF NOT EXISTS calendar_event_remote (
    item_id        text NOT NULL,
    sync_id        uuid NOT NULL,
    remote_event_id text NOT NULL DEFAULT '',
    last_pushed_at timestamptz NOT NULL DEFAULT now(),
    status         text NOT NULL DEFAULT 'ok',
    error          text NOT NULL DEFAULT '',
    PRIMARY KEY (item_id, sync_id)
);
