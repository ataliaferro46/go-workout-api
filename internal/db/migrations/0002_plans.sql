-- Persisted generated workout plans. A plan is a header row, a row per day,
-- and a row per prescribed exercise. The structure mirrors the in-code
-- domain.WorkoutPlan shape (WorkoutPlan → []PlanDay → []PlanExercise) so the
-- repository can round-trip plans without lossy translation.
--
-- The exercise itself is stored as JSONB because the domain.Exercise type has
-- eight fields including two slices, and the persisted plan should be a
-- self-contained snapshot — if the in-code exercise library is later edited or
-- pruned, plans generated before the change should still render exactly as
-- they were generated. JSONB stores the snapshot cleanly, round-trips through
-- encoding/json, and avoids schema churn when domain.Exercise gains fields.
-- The trade-off: we can't easily query into the JSONB shape from SQL. We don't
-- need to today; if we ever do, we add an expression index or denormalize.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE workout_plans (
    id            TEXT        PRIMARY KEY,
    user_id       TEXT        NOT NULL,
    goal          TEXT        NOT NULL,
    experience    TEXT        NOT NULL,
    days_per_week INT         NOT NULL,
    split         TEXT        NOT NULL,
    warnings      TEXT[]      NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Mirrors the workouts index design: equality filter on user_id, ordered scan
-- on (created_at DESC, id) satisfied from the index alone.
CREATE INDEX workout_plans_user_created_idx
    ON workout_plans (user_id, created_at DESC, id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE plan_days (
    plan_id TEXT NOT NULL REFERENCES workout_plans(id) ON DELETE CASCADE,
    day_idx INT  NOT NULL,
    name    TEXT NOT NULL,
    PRIMARY KEY (plan_id, day_idx)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE plan_exercises (
    plan_id      TEXT  NOT NULL,
    day_idx      INT   NOT NULL,
    order_idx    INT   NOT NULL,
    exercise     JSONB NOT NULL,
    sets         INT   NOT NULL,
    reps_low     INT   NOT NULL,
    reps_high    INT   NOT NULL,
    rest_seconds INT   NOT NULL,
    PRIMARY KEY (plan_id, day_idx, order_idx),
    FOREIGN KEY (plan_id, day_idx)
        REFERENCES plan_days(plan_id, day_idx) ON DELETE CASCADE,
    CHECK (sets         >= 0),
    CHECK (reps_low     >= 0),
    CHECK (reps_high    >= 0),
    CHECK (rest_seconds >= 0)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS plan_exercises;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS plan_days;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS workout_plans;
-- +goose StatementEnd
