-- Social post analytics — stores per-platform engagement metrics
CREATE TABLE IF NOT EXISTS social_post_metrics (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id      UUID NOT NULL,
    platform     TEXT NOT NULL,
    platform_post_id TEXT NOT NULL DEFAULT '',
    impressions  BIGINT NOT NULL DEFAULT 0,
    likes        BIGINT NOT NULL DEFAULT 0,
    shares       BIGINT NOT NULL DEFAULT 0,
    comments     BIGINT NOT NULL DEFAULT 0,
    clicks       BIGINT NOT NULL DEFAULT 0,
    reach        BIGINT NOT NULL DEFAULT 0,
    fetched_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (post_id, platform, fetched_at)
);

CREATE INDEX IF NOT EXISTS idx_social_post_metrics_post ON social_post_metrics (post_id, fetched_at DESC);

-- Track platform post IDs returned after publishing so analytics can poll engagement
-- Note: requires superuser/owner to ALTER social_posts. Run as postgres if needed.
ALTER TABLE social_posts ADD COLUMN IF NOT EXISTS platform_post_ids JSONB NOT NULL DEFAULT '{}';
