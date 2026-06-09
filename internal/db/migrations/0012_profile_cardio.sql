-- User profile fields (needed for calorie estimation without a wearable)
-- and a new cardio_sessions table that records cardio-specific data
-- (activity type, duration, distance, calories). The existing workouts
-- table gains a `type` discriminator so the front-end and queries can
-- separate strength sessions from cardio sessions.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE users
    ADD COLUMN height_cm INT,
    ADD COLUMN weight_kg DOUBLE PRECISION,
    ADD COLUMN birth_date DATE,
    ADD COLUMN sex TEXT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE workouts ADD COLUMN type TEXT NOT NULL DEFAULT 'strength';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE workouts
    ADD CONSTRAINT workouts_type_check CHECK (type IN ('strength', 'cardio'));
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE cardio_sessions (
    workout_id        TEXT             PRIMARY KEY REFERENCES workouts(id) ON DELETE CASCADE,
    activity          TEXT             NOT NULL,   -- "running", "cycling", "walking", "rowing", "swimming", "hiit", "other"
    intensity         TEXT             NOT NULL DEFAULT 'moderate', -- "light", "moderate", "vigorous"
    duration_minutes  INT              NOT NULL,
    distance_km       DOUBLE PRECISION,
    avg_hr            INT,
    calories          INT              NOT NULL DEFAULT 0,
    calories_source   TEXT             NOT NULL DEFAULT '', -- 'oura', 'met_formula', ''
    notes             TEXT             NOT NULL DEFAULT ''
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX cardio_sessions_workout_idx ON cardio_sessions (workout_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS cardio_sessions;
ALTER TABLE workouts DROP CONSTRAINT IF EXISTS workouts_type_check;
ALTER TABLE workouts DROP COLUMN IF EXISTS type;
ALTER TABLE users
    DROP COLUMN IF EXISTS sex,
    DROP COLUMN IF EXISTS birth_date,
    DROP COLUMN IF EXISTS weight_kg,
    DROP COLUMN IF EXISTS height_cm;
-- +goose StatementEnd
