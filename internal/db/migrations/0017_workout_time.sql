-- Workout time on the user profile lets the nutrition layer suggest
-- pre/post-workout macro distribution. Stored as a string ("HH:MM") so
-- it's timezone-naive — we interpret it as the user's local clock, the
-- same convention browsers use for date inputs.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN workout_time TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN IF EXISTS workout_time;
-- +goose StatementEnd
