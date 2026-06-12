-- 051_drive_mirrors: mirror a Drive scope/folder OUT to an external cloud drive.
CREATE TABLE IF NOT EXISTS drive_mirrors (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL,
    scope           text NOT NULL,
    scope_id        uuid,
    owner_agent_id  uuid,
    provider        text NOT NULL,
    account_id      text NOT NULL DEFAULT '',
    remote_folder_id text NOT NULL DEFAULT '',
    direction       text NOT NULL DEFAULT 'out',
    enabled         boolean NOT NULL DEFAULT true,
    last_synced_at  timestamptz,
    error           text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_drive_mirrors_tenant ON drive_mirrors (tenant_id, enabled);
CREATE INDEX IF NOT EXISTS idx_drive_mirrors_scope ON drive_mirrors (tenant_id, scope, scope_id);

CREATE TABLE IF NOT EXISTS drive_file_remote (
    file_id        uuid NOT NULL,
    mirror_id      uuid NOT NULL,
    remote_file_id text NOT NULL DEFAULT '',
    last_pushed_at timestamptz NOT NULL DEFAULT now(),
    status         text NOT NULL DEFAULT 'ok',
    error          text NOT NULL DEFAULT '',
    PRIMARY KEY (file_id, mirror_id)
);
