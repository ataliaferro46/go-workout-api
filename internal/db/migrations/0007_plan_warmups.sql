-- Adds a per-exercise warmup-set prescription to plan_exercises. Stored as
-- JSONB because warmups are a small list of (reps, percent) tuples we
-- render but don't query — the same trade-off rationale as the exercise
-- column. Defaults to '[]' so old rows round-trip unchanged.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE plan_exercises
    ADD COLUMN warmups JSONB NOT NULL DEFAULT '[]'::jsonb;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE plan_exercises DROP COLUMN IF EXISTS warmups;
-- +goose StatementEnd
