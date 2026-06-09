-- Adds set-type variance to plan_exercises so the engine can prescribe
-- AMRAP last sets, drop sets, 21s, and antagonist supersets — programming
-- variations that add stimulus variety beyond straight sets. Defaults
-- keep older plans rendering unchanged: set_type='' means "standard
-- straight sets", which is what the old engine always produced.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE plan_exercises
    ADD COLUMN set_type     TEXT NOT NULL DEFAULT '',
    ADD COLUMN superset_with INT,
    ADD COLUMN set_type_note TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE plan_exercises
    ADD CONSTRAINT plan_exercises_set_type_check
    CHECK (set_type IN ('', 'standard', 'amrap', 'drop_set', 'twenty_ones', 'superset'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE plan_exercises
    DROP CONSTRAINT IF EXISTS plan_exercises_set_type_check;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE plan_exercises
    DROP COLUMN IF EXISTS set_type,
    DROP COLUMN IF EXISTS superset_with,
    DROP COLUMN IF EXISTS set_type_note;
-- +goose StatementEnd
