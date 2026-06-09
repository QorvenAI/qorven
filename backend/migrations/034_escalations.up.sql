-- 034_escalations: the reach-the-user engine. One row per "reach the human"
-- request; a ticker advances pending rows up the ladder (in-app → IM → email)
-- until acknowledged or exhausted. escalation_steps is the per-delivery audit log.

CREATE TABLE IF NOT EXISTS escalations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    user_id         TEXT NOT NULL,
    kind            TEXT NOT NULL,
    ref_id          TEXT NOT NULL DEFAULT '',
    title           TEXT NOT NULL DEFAULT '',
    body            TEXT NOT NULL DEFAULT '',
    urgency         TEXT NOT NULL DEFAULT 'normal',
    current_rung    INT  NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'pending',
    next_advance_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    acked_at        TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_escalations_due
    ON escalations (status, next_advance_at);
CREATE INDEX IF NOT EXISTS idx_escalations_ref
    ON escalations (kind, ref_id);

CREATE TABLE IF NOT EXISTS escalation_steps (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    escalation_id UUID NOT NULL,
    rung          INT  NOT NULL,
    channel       TEXT NOT NULL DEFAULT '',
    outcome       TEXT NOT NULL DEFAULT '',
    detail        TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_escalation_steps_esc
    ON escalation_steps (escalation_id, created_at);
