-- Caches the intensity readout for a workout so we don't re-hit Oura's
-- HR endpoint on every page load. One row per (workout_id) — the
-- computation is deterministic for a given (start, end, set of sets) so
-- we recompute only when new sets are logged after caching.
--
-- The fields capture what most users actually want to see post-session:
-- average and peak heart rate, total session minutes, and zone-4 time
-- (the "this was hard" indicator for hypertrophy/strength work).

-- +goose Up
-- +goose StatementBegin
CREATE TABLE workout_intensity (
    workout_id        TEXT             PRIMARY KEY REFERENCES workouts(id) ON DELETE CASCADE,
    avg_bpm           INT              NOT NULL DEFAULT 0,
    max_bpm           INT              NOT NULL DEFAULT 0,
    samples_count     INT              NOT NULL DEFAULT 0,
    duration_minutes  INT              NOT NULL DEFAULT 0,
    minutes_above_140 INT              NOT NULL DEFAULT 0,
    source            TEXT             NOT NULL DEFAULT '',
    computed_at       TIMESTAMPTZ      NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS workout_intensity;
-- +goose StatementEnd
