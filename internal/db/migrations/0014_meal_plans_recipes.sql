-- Persisted meal plans + recipes + user dietary preferences.
--
-- Meal plans persist a single-day or 7-day generated plan so the user
-- can re-open it (e.g., "this is the weekly plan I'm running for the
-- next two weeks"). Plan rows reference foods by id; if a food row gets
-- deleted later we leave the reference dangling rather than cascade —
-- the plan still shows the macros that were snapshotted on save.
--
-- Recipes are user-created composite foods ("chicken-and-rice bowl"
-- made of 4 ingredients). On save we synthesize a row in the foods
-- table with id 'recipe-{recipe_id}' so recipe entries show up in
-- search results and logging treats them just like foods. The
-- recipe_ingredients table is the editable backing store; updates
-- recompute the synthetic food's macros.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE meal_plans (
    id          TEXT             PRIMARY KEY,
    user_id     TEXT             NOT NULL,
    name        TEXT             NOT NULL,
    days        INT              NOT NULL DEFAULT 1,
    targets     JSONB            NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX meal_plans_user_idx ON meal_plans (user_id, created_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE meal_plan_items (
    plan_id    TEXT             NOT NULL REFERENCES meal_plans(id) ON DELETE CASCADE,
    day_idx    INT              NOT NULL,
    meal_type  TEXT             NOT NULL,
    sort_order INT              NOT NULL DEFAULT 0,
    food_id    TEXT             NOT NULL,
    servings   DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (plan_id, day_idx, meal_type, sort_order)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE recipes (
    id          TEXT             PRIMARY KEY,
    user_id     TEXT             NOT NULL,
    name        TEXT             NOT NULL,
    notes       TEXT             NOT NULL DEFAULT '',
    servings_per_recipe DOUBLE PRECISION NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX recipes_user_idx ON recipes (user_id, name);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE recipe_ingredients (
    recipe_id  TEXT             NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    sort_order INT              NOT NULL DEFAULT 0,
    food_id    TEXT             NOT NULL,
    servings   DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (recipe_id, sort_order)
);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE users
    ADD COLUMN allergies      TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN disliked_foods TEXT[] NOT NULL DEFAULT '{}';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users
    DROP COLUMN IF EXISTS disliked_foods,
    DROP COLUMN IF EXISTS allergies;
DROP TABLE IF EXISTS recipe_ingredients;
DROP TABLE IF EXISTS recipes;
DROP TABLE IF EXISTS meal_plan_items;
DROP TABLE IF EXISTS meal_plans;
-- +goose StatementEnd
