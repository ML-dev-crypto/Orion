CREATE TABLE IF NOT EXISTS audit_log (
    id           BIGSERIAL PRIMARY KEY,
    flag_key     TEXT NOT NULL,
    action       TEXT NOT NULL,
    before_state JSONB,
    after_state  JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_flag_key ON audit_log (flag_key);