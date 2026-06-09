-- Adds a Weekday column to plan_days for calendar-bound plans. Existing rows
-- get NULL (which the application maps to "" / "unbound") so older plans keep
-- rendering unchanged.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE plan_days
    ADD COLUMN weekday TEXT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE plan_days
    ADD CONSTRAINT plan_days_weekday_check
    CHECK (weekday IS NULL OR weekday IN ('mon','tue','wed','thu','fri','sat','sun'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE plan_days DROP CONSTRAINT IF EXISTS plan_days_weekday_check;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE plan_days DROP COLUMN IF EXISTS weekday;
-- +goose StatementEnd
