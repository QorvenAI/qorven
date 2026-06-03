-- Migration 028: Per-user customisable dashboard layouts
-- Stores react-grid-layout LayoutItem[] + widget configs as JSONB per user.

CREATE TABLE IF NOT EXISTS user_dashboard_layouts (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL,
    tenant_id    UUID        NOT NULL,
    name         TEXT        NOT NULL DEFAULT 'My Dashboard',
    -- layout: react-grid-layout LayoutItem[] — [{i, x, y, w, h, minW, maxW, static?}]
    layout       JSONB       NOT NULL DEFAULT '[]',
    -- widgets: {[id: string]: WidgetConfig} — type, title, dataSource, config
    widgets      JSONB       NOT NULL DEFAULT '{}',
    is_default   BOOLEAN     NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, name)
);

CREATE INDEX IF NOT EXISTS idx_udl_user_default ON user_dashboard_layouts (user_id, is_default);
CREATE INDEX IF NOT EXISTS idx_udl_tenant      ON user_dashboard_layouts (tenant_id);
