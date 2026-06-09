-- Per-set logging. The original workout_exercises row carries the
-- prescription (sets/reps/weight) as flat fields; this table records what
-- the user *actually* did, set by set, with a real timestamp so a
-- progress-charts feature can later plot estimated 1RM and volume per
-- session.
--
-- The workouts table gains optional plan_id / plan_day_idx so a workout
-- session that originated from a generated plan can be traced back; an
-- ad-hoc gym session can still leave them NULL.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE workouts
    ADD COLUMN plan_id      TEXT,
    ADD COLUMN plan_day_idx INT;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE workout_sets (
    workout_id        TEXT             NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
    exercise_position INT              NOT NULL,
    set_number        INT              NOT NULL,
    reps              INT              NOT NULL,
    weight_kg         DOUBLE PRECISION NOT NULL,
    completed_at      TIMESTAMPTZ      NOT NULL,
    PRIMARY KEY (workout_id, exercise_position, set_number),
    FOREIGN KEY (workout_id, exercise_position)
        REFERENCES workout_exercises(workout_id, position) ON DELETE CASCADE,
    CHECK (reps      >= 0),
    CHECK (weight_kg >= 0),
    CHECK (set_number >= 1)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX workout_sets_user_lookup_idx
    ON workout_sets (workout_id, exercise_position, set_number);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS workout_sets;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE workouts DROP COLUMN IF EXISTS plan_id, DROP COLUMN IF EXISTS plan_day_idx;
-- +goose StatementEnd
