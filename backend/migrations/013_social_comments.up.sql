-- Team collaboration comments on social posts
CREATE TABLE IF NOT EXISTS social_post_comments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id    UUID NOT NULL,
    author_id  TEXT NOT NULL,
    author_name TEXT NOT NULL DEFAULT '',
    body       TEXT NOT NULL,
    parent_id  UUID REFERENCES social_post_comments(id) ON DELETE CASCADE,
    resolved   BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_social_post_comments_post ON social_post_comments (post_id, created_at);
