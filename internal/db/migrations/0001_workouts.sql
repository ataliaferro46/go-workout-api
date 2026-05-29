-- Workouts and their ordered exercises. Two tables instead of a single jsonb
-- column because we will eventually want to query exercises across sessions
-- (e.g. "how often has this user logged a back squat?") and indexes on a
-- relational layout are dramatically cheaper than on jsonb paths.
--
-- IDs are TEXT rather than UUID so the application layer keeps full control
-- over ID generation (currently UUIDv4 via crypto/rand; ULIDs or external IDs
-- could be swapped in without a migration). user_id is TEXT for the same
-- reason and is intentionally not yet a foreign key — the users table is a
-- later milestone in the roadmap.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE workouts (
    id         TEXT        PRIMARY KEY,
    user_id    TEXT        NOT NULL,
    name       TEXT        NOT NULL,
    notes      TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
-- The composite index supports ListByUser's "newest first, tiebreak by id"
-- ordering directly from the index, so the query never sorts on the heap.
CREATE INDEX workouts_user_created_idx
    ON workouts (user_id, created_at DESC, id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE workout_exercises (
    workout_id TEXT             NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
    position   INT              NOT NULL,
    name       TEXT             NOT NULL,
    sets       INT              NOT NULL,
    reps       INT              NOT NULL,
    weight_kg  DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (workout_id, position),
    CHECK (sets    >= 0),
    CHECK (reps    >= 0),
    CHECK (weight_kg >= 0)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS workout_exercises;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS workouts;
-- +goose StatementEnd
