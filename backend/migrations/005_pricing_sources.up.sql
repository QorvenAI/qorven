-- Per-source pricing storage for comparison and audit
CREATE TABLE IF NOT EXISTS model_pricing_sources (
    model_id     TEXT        NOT NULL,
    source       TEXT        NOT NULL,  -- 'litellm' | 'openrouter' | 'artificial_analysis' | 'builtin' | 'manual'
    provider     TEXT,
    input_per_1m  NUMERIC(14,6) NOT NULL DEFAULT 0,
    output_per_1m NUMERIC(14,6) NOT NULL DEFAULT 0,
    cache_write_per_1m NUMERIC(14,6) NOT NULL DEFAULT 0,
    cache_read_per_1m  NUMERIC(14,6) NOT NULL DEFAULT 0,
    intelligence_index NUMERIC(6,2)  NOT NULL DEFAULT 0,
    coding_index       NUMERIC(6,2)  NOT NULL DEFAULT 0,
    speed_tps          NUMERIC(10,2) NOT NULL DEFAULT 0,
    context_window     INT           NOT NULL DEFAULT 0,
    fetched_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (model_id, source)
);

CREATE INDEX IF NOT EXISTS idx_pricing_sources_model ON model_pricing_sources (model_id);
