-- Enrich workout_exercises so the live workout view matches the planned
-- view: pass through set type (AMRAP / drop set / 21s / superset), the
-- warmup ramp, the rest period, and per-set target reps for variable
-- schemes (3 × 10, 8, 6).
--
-- prescription holds the planning metadata as JSON (set_type, set_type_note,
-- warmups, reps_low, reps_high, rest_seconds). Old rows render fine —
-- '{}' just means "no extra metadata, use the flat reps column."
--
-- target_reps is a per-set integer array. NULL/empty means "every set has
-- the same target reps" (the existing reps column). When populated, the
-- live workout page uses it to show different rep targets per set.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE workout_exercises
    ADD COLUMN prescription JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN target_reps  INT[] NOT NULL DEFAULT '{}';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE workout_exercises
    DROP COLUMN IF EXISTS target_reps,
    DROP COLUMN IF EXISTS prescription;
-- +goose StatementEnd
