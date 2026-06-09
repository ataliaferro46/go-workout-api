-- Adds a `region` column to exercises so the plan engine can rotate across
-- anatomical sub-targets within a muscle group (long head vs lateral head
-- of triceps, upper vs mid vs lower chest, lats width vs lats lower, etc.)
-- across the week. Tagging follows the research consensus described in
-- internal/domain/exercise.go above the Region field.
--
-- Existing seed rows are backfilled via id-keyed UPDATEs. The list is
-- intentionally not exhaustive — any untagged exercise keeps Region=''
-- which the planner treats as "no regional preference" and selects on the
-- usual score function. We tag the high-leverage isolations and a handful
-- of compounds where the angle change is the whole point of the variant
-- (incline bench is upper chest, decline is lower chest, etc.).

-- +goose Up
-- +goose StatementBegin
ALTER TABLE exercises ADD COLUMN region TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
-- Triceps
UPDATE exercises SET region = 'long_head'    WHERE id = 'lying-triceps-extension';
UPDATE exercises SET region = 'lateral_head' WHERE id = 'tricep-pushdown';
UPDATE exercises SET region = 'all_heads'    WHERE id = 'bench-dip';
UPDATE exercises SET region = 'all_heads'    WHERE id = 'close-grip-bench-press';

-- Chest
UPDATE exercises SET region = 'upper_chest' WHERE id = 'incline-barbell-bench-press';
UPDATE exercises SET region = 'upper_chest' WHERE id = 'dumbbell-incline-press';
UPDATE exercises SET region = 'mid_chest'   WHERE id = 'barbell-bench-press';
UPDATE exercises SET region = 'mid_chest'   WHERE id = 'dumbbell-bench-press';
UPDATE exercises SET region = 'mid_chest'   WHERE id = 'machine-chest-press';
UPDATE exercises SET region = 'mid_chest'   WHERE id = 'dumbbell-fly';
UPDATE exercises SET region = 'mid_chest'   WHERE id = 'paused-bench-press';
UPDATE exercises SET region = 'mid_chest'   WHERE id = 'push-up';
UPDATE exercises SET region = 'lower_chest' WHERE id = 'decline-barbell-bench-press';
UPDATE exercises SET region = 'lower_chest' WHERE id = 'dip';

-- Biceps
UPDATE exercises SET region = 'short_head'  WHERE id = 'concentration-curl';
UPDATE exercises SET region = 'brachialis'  WHERE id = 'hammer-curl';
UPDATE exercises SET region = 'general'     WHERE id = 'dumbbell-curl';
UPDATE exercises SET region = 'general'     WHERE id = 'cable-curl';

-- Shoulders
UPDATE exercises SET region = 'lateral_delt' WHERE id = 'lateral-raise';
UPDATE exercises SET region = 'lateral_delt' WHERE id = 'cable-lateral-raise';
UPDATE exercises SET region = 'rear_delt'    WHERE id = 'face-pull';
UPDATE exercises SET region = 'rear_delt'    WHERE id = 'cable-reverse-fly';
UPDATE exercises SET region = 'front_delt'   WHERE id = 'barbell-overhead-press';
UPDATE exercises SET region = 'front_delt'   WHERE id = 'dumbbell-shoulder-press';
UPDATE exercises SET region = 'front_delt'   WHERE id = 'machine-shoulder-press';
UPDATE exercises SET region = 'front_delt'   WHERE id = 'pike-push-up';
UPDATE exercises SET region = 'front_delt'   WHERE id = 'push-press';

-- Back
UPDATE exercises SET region = 'lats_width'   WHERE id = 'pull-up';
UPDATE exercises SET region = 'mid_back'     WHERE id = 'barbell-row';
UPDATE exercises SET region = 'mid_back'     WHERE id = 'dumbbell-row';
UPDATE exercises SET region = 'mid_back'     WHERE id = 'seated-cable-row';
UPDATE exercises SET region = 'mid_back'     WHERE id = 'inverted-row';
UPDATE exercises SET region = 'mid_back'     WHERE id = 't-bar-row';
UPDATE exercises SET region = 'mid_back'     WHERE id = 'pendlay-row';
UPDATE exercises SET region = 'mid_back'     WHERE id = 'chest-supported-row';
UPDATE exercises SET region = 'mid_back'     WHERE id = 'single-arm-cable-row';

-- Hamstrings
UPDATE exercises SET region = 'knee_flexion'  WHERE id = 'leg-curl';

-- Calves
UPDATE exercises SET region = 'gastrocnemius' WHERE id = 'standing-calf-raise';
UPDATE exercises SET region = 'gastrocnemius' WHERE id = 'dumbbell-calf-raise';
UPDATE exercises SET region = 'gastrocnemius' WHERE id = 'leg-press-calf-raise';

-- Quads
UPDATE exercises SET region = 'rectus_femoris' WHERE id = 'leg-extension';
UPDATE exercises SET region = 'vasti'          WHERE id = 'sissy-squat';
-- +goose StatementEnd

-- New exercises (overhead triceps extension, preacher curl, rear delt fly,
-- seated calf raise, etc.) are NOT inserted here. They live in seed.go and
-- are applied at boot by the now-idempotent SeedIfEmpty pass, which
-- inserts every row missing from the table. That keeps schema migrations
-- about schema, not content, and avoids juggling INSERTs in two places.

-- +goose Down
-- +goose StatementBegin
ALTER TABLE exercises DROP COLUMN IF EXISTS region;
-- +goose StatementEnd
