-- 038_sidebar_pins: per-user pinned hubs/chats for the sidebar's pinned group.
-- A user pins a hub (room) or a chat (agent/soul) so it stays one tap away at
-- the top of the sidebar. Ordered by order_index then creation.
CREATE TABLE IF NOT EXISTS sidebar_pins (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    user_id     UUID NOT NULL,
    item_type   TEXT NOT NULL,        -- 'hub' | 'chat'
    item_id     UUID NOT NULL,
    order_index INT  NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, item_type, item_id)
);
CREATE INDEX IF NOT EXISTS idx_sidebar_pins_user ON sidebar_pins (user_id, order_index, created_at);
