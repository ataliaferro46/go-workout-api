-- Biometric integration: OAuth tokens per user/provider, ingested readings,
-- and per-(user, provider) sync watermarks. See ADR-047 through ADR-052
-- for the surrounding architectural decisions.

-- +goose Up
-- +goose StatementBegin
-- OAuth tokens per (user, provider). Tokens are encrypted at rest with
-- AES-GCM; a per-purpose key is derived from BIOMETRICS_MASTER_KEY via HKDF
-- (see biometrics.TokenStore). Plaintext tokens never touch disk.
CREATE TABLE oauth_tokens (
    user_id            TEXT        NOT NULL,
    provider           TEXT        NOT NULL,
    access_token_enc   BYTEA       NOT NULL,
    refresh_token_enc  BYTEA       NOT NULL,
    nonce              BYTEA       NOT NULL,
    expires_at         TIMESTAMPTZ NOT NULL,
    scopes             TEXT[]      NOT NULL DEFAULT '{}',
    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, provider)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Ingested readings. The UNIQUE constraint on (provider, idempotency_key) is
-- the foundation of safe ingestion: webhook retries and double-polling both
-- insert the same key and the second attempt becomes a no-op via
-- ON CONFLICT DO NOTHING.
CREATE TABLE biometric_readings (
    id              TEXT             PRIMARY KEY,
    user_id         TEXT             NOT NULL,
    provider        TEXT             NOT NULL,
    kind            TEXT             NOT NULL,
    value           DOUBLE PRECISION NOT NULL,
    recorded_at     TIMESTAMPTZ      NOT NULL,
    ingested_at     TIMESTAMPTZ      NOT NULL,
    idempotency_key TEXT             NOT NULL,
    raw_payload     JSONB            NOT NULL,
    UNIQUE (provider, idempotency_key),
    CHECK (value >= 0),
    CHECK (kind IN ('recovery', 'strain', 'sleep_score', 'sleep_minutes', 'readiness'))
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Supports the "latest reading of kind X for user U" query that drives the
-- recovery-aware plan generation and the GET /v1/biometrics/latest endpoint.
-- The DESC ordering means the planner can satisfy "most recent" directly
-- from the index without a separate sort step.
CREATE INDEX biometric_readings_user_kind_recorded_idx
    ON biometric_readings (user_id, kind, recorded_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
-- Per-user, per-provider polling watermarks. Updated after each successful
-- sync so the next poll fetches only events since last_synced_at.
CREATE TABLE biometric_sync_state (
    user_id           TEXT        NOT NULL,
    provider          TEXT        NOT NULL,
    last_synced_at    TIMESTAMPTZ NOT NULL,
    last_event_cursor TEXT,
    PRIMARY KEY (user_id, provider)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS biometric_sync_state;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS biometric_readings;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS oauth_tokens;
-- +goose StatementEnd
