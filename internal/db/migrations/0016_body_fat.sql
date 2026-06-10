-- Optional body-fat-percentage on the user profile. When set, the
-- nutrition target calculator uses Katch-McArdle (BMR based on lean
-- body mass) instead of Mifflin-St Jeor (BMR based on total body
-- weight + age). Katch-McArdle is meaningfully more accurate for
-- muscular athletes because Mifflin underestimates BMR when a user
-- has more LBM than the population average for their weight.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN body_fat_percentage DOUBLE PRECISION;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN IF EXISTS body_fat_percentage;
-- +goose StatementEnd
