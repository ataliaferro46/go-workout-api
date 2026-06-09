-- Authentication: real users with email + password, email verification, and
-- server-side sessions stored as token hashes. See ADRs 064-067.
--
-- Tokens (session cookie values, verification links) are stored as SHA-256
-- digests, not plaintext — a database dump cannot be turned into stolen
-- credentials. The plaintext token only lives in the user's cookie / email
-- inbox.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id                 TEXT        PRIMARY KEY,
    email              TEXT        NOT NULL UNIQUE,
    password_hash      TEXT        NOT NULL,
    email_verified_at  TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX users_email_lower_idx ON users ((lower(email)));
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE email_verifications (
    token_hash   TEXT        PRIMARY KEY,
    user_id      TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at   TIMESTAMPTZ NOT NULL,
    consumed_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX email_verifications_user_id_idx ON email_verifications(user_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE sessions (
    token_hash  TEXT        PRIMARY KEY,
    user_id     TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    user_agent  TEXT        NOT NULL DEFAULT '',
    ip          TEXT        NOT NULL DEFAULT ''
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX sessions_user_id_idx ON sessions(user_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS sessions;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS email_verifications;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
