-- The exercise library. Previously lived in code (internal/exercise/seed.go);
-- now persisted so admin endpoints can edit it without a binary rebuild.
--
-- The schema lives in this migration. The initial *content* of the library is
-- bulk-loaded at boot from internal/exercise/seed.go via
-- exercise.SeedIfEmpty — which means an empty `exercises` table will be
-- populated on first Postgres-mode boot, but a non-empty one (post-seed,
-- including any admin edits) is never overwritten. The trade-off is
-- documented in ADR-062: schema-only migrations stay readable, the seed list
-- has one source of truth (Go), but Postgres state is not fully reproducible
-- from migrations alone. Acceptable for a code-managed library; would not be
-- acceptable for customer data.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE exercises (
    id                 TEXT     PRIMARY KEY,
    name               TEXT     NOT NULL UNIQUE,
    primary_muscle     TEXT     NOT NULL,
    secondary_muscles  TEXT[]   NOT NULL DEFAULT '{}',
    pattern            TEXT     NOT NULL,
    required_equipment TEXT[]   NOT NULL DEFAULT '{}',
    compound           BOOLEAN  NOT NULL,
    min_level          TEXT     NOT NULL,
    contraindications  TEXT[]   NOT NULL DEFAULT '{}'
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Supports ListByPattern, which the plan engine could call directly if it
-- ever wanted per-pattern queries instead of an in-memory filter.
CREATE INDEX exercises_pattern_idx ON exercises (pattern);
-- +goose StatementEnd

-- +goose StatementBegin
-- Supports library browsing endpoints that filter by primary muscle target.
CREATE INDEX exercises_primary_muscle_idx ON exercises (primary_muscle);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS exercises;
-- +goose StatementEnd
