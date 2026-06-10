-- Nutrition tracking: foods library + per-user food logs + calorie/macro
-- targets stored on the user. The foods table is seeded with ~40 staples
-- at the bottom — enough to demonstrate the feature without needing the
-- USDA API on day one. Users can add their own foods later.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE foods (
    id            TEXT             PRIMARY KEY,
    name          TEXT             NOT NULL,
    brand         TEXT             NOT NULL DEFAULT '',
    serving_size  DOUBLE PRECISION NOT NULL,
    serving_unit  TEXT             NOT NULL,
    calories      INT              NOT NULL,
    protein_g     DOUBLE PRECISION NOT NULL,
    carbs_g       DOUBLE PRECISION NOT NULL,
    fat_g         DOUBLE PRECISION NOT NULL,
    fiber_g       DOUBLE PRECISION NOT NULL DEFAULT 0,
    sugar_g       DOUBLE PRECISION NOT NULL DEFAULT 0,
    source        TEXT             NOT NULL DEFAULT 'seed',
    user_id       TEXT,
    created_at    TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX foods_name_lower_idx ON foods (lower(name));
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE food_logs (
    id          TEXT             PRIMARY KEY,
    user_id     TEXT             NOT NULL,
    food_id     TEXT             NOT NULL REFERENCES foods(id),
    servings    DOUBLE PRECISION NOT NULL,
    meal_type   TEXT             NOT NULL,
    logged_at   TIMESTAMPTZ      NOT NULL,
    notes       TEXT             NOT NULL DEFAULT '',
    CHECK (servings > 0),
    CHECK (meal_type IN ('breakfast', 'lunch', 'dinner', 'snack'))
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX food_logs_user_date_idx ON food_logs (user_id, logged_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE users
    ADD COLUMN target_calories  INT,
    ADD COLUMN target_protein_g INT,
    ADD COLUMN target_carbs_g   INT,
    ADD COLUMN target_fat_g     INT,
    ADD COLUMN nutrition_goal   TEXT,
    ADD COLUMN activity_level   TEXT;
-- +goose StatementEnd

-- +goose StatementBegin
-- Seed foods: per-serving macros. Values from USDA FoodData Central
-- standard reference. Names kept short and search-friendly.
INSERT INTO foods (id, name, serving_size, serving_unit, calories, protein_g, carbs_g, fat_g, fiber_g, sugar_g) VALUES
  ('egg-large',          'Egg, large',                    1,   'piece', 70,   6,    0.6,  5,    0,    0.6),
  ('chicken-breast',     'Chicken breast, cooked',        100, 'g',     165,  31,   0,    3.6,  0,    0),
  ('chicken-thigh',      'Chicken thigh, cooked',         100, 'g',     209,  26,   0,    11,   0,    0),
  ('beef-85',            'Beef, ground 85% lean, cooked', 100, 'g',     230,  22,   0,    15,   0,    0),
  ('salmon',             'Salmon, cooked',                100, 'g',     208,  22,   0,    13,   0,    0),
  ('tuna-can',           'Tuna, canned in water',         100, 'g',     116,  26,   0,    1,    0,    0),
  ('shrimp',             'Shrimp, cooked',                100, 'g',     99,   24,   0,    0.3,  0,    0),
  ('pork-chop',          'Pork chop, cooked',             100, 'g',     231,  25,   0,    13.9, 0,    0),
  ('turkey-ground',      'Turkey, ground 93% lean, cooked', 100, 'g',   170,  22,   0,    9,    0,    0),
  ('oats-dry',           'Oats, dry',                     100, 'g',     389,  17,   66,   7,    11,   1),
  ('rice-white',         'Rice, white cooked',            100, 'g',     130,  2.7,  28,   0.3,  0.4,  0),
  ('rice-brown',         'Rice, brown cooked',            100, 'g',     112,  2.6,  23,   0.9,  1.8,  0.4),
  ('quinoa-cooked',      'Quinoa, cooked',                100, 'g',     120,  4.4,  21,   1.9,  2.8,  0.9),
  ('pasta-cooked',       'Pasta, cooked',                 100, 'g',     158,  5.8,  31,   0.9,  1.8,  0.6),
  ('sweet-potato',       'Sweet potato, baked',           100, 'g',     90,   2,    21,   0.1,  3,    7),
  ('bread-whole-wheat',  'Whole-wheat bread, slice',      1,   'slice', 80,   4,    14,   1.5,  2,    1.5),
  ('bagel-plain',        'Bagel, plain',                  1,   'piece', 245,  10,   48,   1.5,  2,    6),
  ('banana',             'Banana, medium',                1,   'piece', 105,  1.3,  27,   0.4,  3,    14),
  ('apple',              'Apple, medium',                 1,   'piece', 95,   0.5,  25,   0.3,  4,    19),
  ('orange',             'Orange, medium',                1,   'piece', 62,   1.2,  15.4, 0.2,  3,    12),
  ('blueberries',        'Blueberries',                   100, 'g',     57,   0.7,  14,   0.3,  2.4,  10),
  ('strawberries',       'Strawberries',                  100, 'g',     32,   0.7,  7.7,  0.3,  2,    4.9),
  ('avocado',            'Avocado',                       100, 'g',     160,  2,    9,    15,   7,    0.7),
  ('broccoli',           'Broccoli, cooked',              100, 'g',     35,   2.4,  7,    0.4,  3.3,  1.7),
  ('spinach-raw',        'Spinach, raw',                  100, 'g',     23,   2.9,  3.6,  0.4,  2.2,  0.4),
  ('carrot',             'Carrot, raw',                   100, 'g',     41,   0.9,  9.6,  0.2,  2.8,  4.7),
  ('mixed-greens',       'Mixed greens, raw',             100, 'g',     17,   1.4,  2.9,  0.2,  1.8,  0.4),
  ('greek-yogurt-plain', 'Greek yogurt, plain nonfat',    100, 'g',     59,   10,   3.6,  0.4,  0,    3.2),
  ('cottage-cheese',     'Cottage cheese, 1% fat',        100, 'g',     72,   12,   2.7,  1,    0,    2.7),
  ('cheddar',            'Cheddar cheese',                100, 'g',     403,  25,   1.3,  33,   0,    0.5),
  ('milk-whole',         'Milk, whole',                   240, 'ml',    149,  8,    12,   8,    0,    12),
  ('milk-skim',          'Milk, skim',                    240, 'ml',    83,   8,    12,   0.2,  0,    12),
  ('almonds',            'Almonds',                       28,  'g',     164,  6,    6,    14,   3.5,  1.2),
  ('peanut-butter',      'Peanut butter',                 32,  'g',     188,  8,    6.4,  16,   2,    3),
  ('olive-oil',          'Olive oil',                     1,   'tbsp',  119,  0,    0,    13.5, 0,    0),
  ('whey-scoop',         'Whey protein, scoop',           30,  'g',     120,  24,   3,    1.5,  0,    1.5),
  ('black-beans',        'Black beans, cooked',           100, 'g',     132,  8.9,  24,   0.5,  8.7,  0.3),
  ('lentils',            'Lentils, cooked',               100, 'g',     116,  9,    20,   0.4,  7.9,  1.8),
  ('protein-bar',        'Protein bar (typical)',         1,   'piece', 200,  20,   25,   7,    5,    8),
  ('granola-bar',        'Granola bar (typical)',         1,   'piece', 130,  3,    22,   5,    2,    10),
  ('coffee-black',       'Coffee, black',                 240, 'ml',    2,    0.3,  0,    0,    0,    0),
  ('beer-can',           'Beer, 12oz can',                1,   'can',   153,  1.6,  13,   0,    0,    0),
  ('wine-glass',         'Wine, 5oz glass',               1,   'glass', 125,  0,    4,    0,    0,    1),
  ('pizza-slice',        'Pizza, cheese slice',           1,   'slice', 285,  12,   36,   10,   2,    4)
ON CONFLICT (id) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users
    DROP COLUMN IF EXISTS activity_level,
    DROP COLUMN IF EXISTS nutrition_goal,
    DROP COLUMN IF EXISTS target_fat_g,
    DROP COLUMN IF EXISTS target_carbs_g,
    DROP COLUMN IF EXISTS target_protein_g,
    DROP COLUMN IF EXISTS target_calories;
DROP TABLE IF EXISTS food_logs;
DROP TABLE IF EXISTS foods;
-- +goose StatementEnd
