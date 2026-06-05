# Spec 02 — Diet plans and grocery lists

## Goal

A new `internal/nutrition` feature slice that mirrors the workout-plan engine for the
nutrition domain. The same constraint-and-coverage shape produces a week of meals:
takes a goal (cut/maintain/gain), activity level, dietary preferences, and allergens;
computes TDEE and macro targets; selects meals from a seeded library to hit targets
while respecting constraints; and prescribes per-meal portion sizes. A grocery-list
endpoint then aggregates ingredients across the plan into a deduplicated shopping list.

Together with workout plans, the product becomes "full-stack fitness" — many users want
both surfaces and apps that only do one are stickier than apps that do neither but less
sticky than apps that do both.

## Architectural shape

```
internal/nutrition/
├── domain.go                       # NutritionGoal, ActivityLevel, DietPref, Allergen types
├── food.go                         # Food + FoodCategory
├── meal.go                         # Meal, MealSlot, Component types
├── food_library.go                 # seeded ~120 foods
├── meal_library.go                 # seeded ~80 meals composed from foods
├── targets.go                      # TDEE, BMR (Mifflin-St Jeor), macro splits per goal
├── selection.go                    # per-meal-slot meal picker w/ scoring
├── generator.go                    # orchestrator, parallels plan/generator.go
├── grocery.go                      # ingredient aggregation across plan
├── repository.go                   # plan persistence interface + InMemory
├── postgres_repository.go          # Postgres plan repository
├── repository_contract_test.go     # shared contract
├── postgres_repository_test.go     # integration build tag
├── service.go                      # wraps generator + repo + IDGenerator/Clock
├── service_test.go
├── handler.go                      # /v1/diet-plans/* HTTP endpoints
├── handler_test.go
├── uuid.go                         # package-local NewUUID (mirrors plan/workout)
├── generator_test.go
└── selection_test.go
```

Files outside the new package that change:

- `internal/domain/nutrition.go` — `NutritionGoal`, `ActivityLevel`, `DietPreference`,
  `Allergen`, `MealSlot`, plus `NutritionRequest`, `DietPlan`, `DietDay`, `DietMeal`
  value types and a `Validate()` method (mirroring `domain/request.go`).
- `internal/db/migrations/0003_diet_plans.sql` — three tables: `diet_plans`,
  `diet_days`, `diet_meals`. (If executed after Spec 01, becomes `0004`.)
- `cmd/api/main.go` — wires the nutrition service, registers routes.

## External surface (HTTP endpoints)

| Method | Path                              | Auth        | Body                | Success |
|--------|-----------------------------------|-------------|---------------------|---------|
| POST   | `/v1/diet-plans/generate`         | `X-User-ID` | `NutritionRequest`  | 201     |
| GET    | `/v1/diet-plans`                  | `X-User-ID` | —                   | 200     |
| GET    | `/v1/diet-plans/{id}`             | —           | —                   | 200     |
| DELETE | `/v1/diet-plans/{id}`             | —           | —                   | 204     |
| GET    | `/v1/diet-plans/{id}/grocery-list`| —           | —                   | 200     |

`NutritionRequest` fields:

- `goal` — `cut`, `maintain`, `gain`
- `age` — integer years
- `sex` — `male`, `female` (used for BMR formula; documented as a biological-sex input
  with no judgment about identity; the formula is the formula)
- `height_cm` — number
- `weight_kg` — number
- `activity_level` — `sedentary`, `light`, `moderate`, `very_active`
- `dietary_preferences` — set: `omnivore`, `vegetarian`, `vegan`, `pescatarian`, `keto`
- `allergens` — set: `dairy`, `gluten`, `nuts`, `eggs`, `soy`, `shellfish`, `fish`
- `days_per_week` — integer 1–7 (typically 7)
- `meals_per_day` — integer 3–5 (default 3: breakfast, lunch, dinner; 4 adds snack;
  5 adds second snack)

## Domain types

```go
// internal/domain/nutrition.go (sketch)

type NutritionGoal string

const (
    NutritionCut      NutritionGoal = "cut"
    NutritionMaintain NutritionGoal = "maintain"
    NutritionGain     NutritionGoal = "gain"
)

type ActivityLevel string

const (
    Sedentary   ActivityLevel = "sedentary"     // 1.2  multiplier
    Light       ActivityLevel = "light"         // 1.375
    Moderate    ActivityLevel = "moderate"      // 1.55
    VeryActive  ActivityLevel = "very_active"   // 1.725
)

type DietPreference string
const (
    Omnivore     DietPreference = "omnivore"
    Vegetarian   DietPreference = "vegetarian"
    Vegan        DietPreference = "vegan"
    Pescatarian  DietPreference = "pescatarian"
    Keto         DietPreference = "keto"
)

type Allergen string
const (
    AllergenDairy     Allergen = "dairy"
    AllergenGluten    Allergen = "gluten"
    AllergenNuts      Allergen = "nuts"
    AllergenEggs      Allergen = "eggs"
    AllergenSoy       Allergen = "soy"
    AllergenShellfish Allergen = "shellfish"
    AllergenFish      Allergen = "fish"
)

type MealSlot string
const (
    Breakfast MealSlot = "breakfast"
    Lunch     MealSlot = "lunch"
    Dinner    MealSlot = "dinner"
    Snack1    MealSlot = "snack_1"
    Snack2    MealSlot = "snack_2"
)

type MacroTarget struct {
    Kcal     int `json:"kcal"`
    ProteinG int `json:"protein_g"`
    CarbG    int `json:"carb_g"`
    FatG     int `json:"fat_g"`
}

type DietPlan struct {
    ID            string         `json:"id,omitempty"`
    UserID        string         `json:"user_id,omitempty"`
    Goal          NutritionGoal  `json:"goal"`
    Targets       MacroTarget    `json:"targets"`
    DaysPerWeek   int            `json:"days_per_week"`
    MealsPerDay   int            `json:"meals_per_day"`
    Days          []DietDay      `json:"days"`
    Warnings      []string       `json:"warnings,omitempty"`
    CreatedAt     time.Time      `json:"created_at,omitempty"`
}

type DietDay struct {
    Index int        `json:"index"`
    Meals []DietMeal `json:"meals"`
}

type DietMeal struct {
    Slot     MealSlot `json:"slot"`
    Meal     Meal     `json:"meal"`
    Servings float64  `json:"servings"` // scales the meal up or down
    Kcal     int      `json:"kcal"`
    ProteinG int      `json:"protein_g"`
    CarbG    int      `json:"carb_g"`
    FatG     int      `json:"fat_g"`
}

// Foods/Meals live in the nutrition package and are referenced as snapshots inside DietMeal
// (same pattern as domain.Exercise snapshotting into PlanExercise).
```

## TDEE and macro math (`targets.go`)

```go
// BMR via Mifflin-St Jeor (validated for general population).
func BMR(sex string, ageY int, heightCm, weightKg float64) float64 {
    base := 10*weightKg + 6.25*heightCm - 5*float64(ageY)
    if sex == "female" {
        return base - 161
    }
    return base + 5
}

func TDEE(bmr float64, lvl ActivityLevel) float64 {
    return bmr * activityMultiplier[lvl]
}

// Target deltas per goal (kcal/day).
//   cut       → TDEE - 500 (≈ 0.5 kg/wk loss)
//   maintain  → TDEE
//   gain      → TDEE + 300 (lean bulk; avoids fat-gain overshoot)
func KcalTarget(tdee float64, goal NutritionGoal) int {
    switch goal {
    case NutritionCut:  return int(tdee) - 500
    case NutritionGain: return int(tdee) + 300
    default:            return int(tdee)
    }
}

// Macro split: protein anchored to body weight, fat to a percentage of kcal,
// carbs are the residual. Each goal nudges the split.
//   cut       → 2.2 g/kg protein, 25% fat, rest carbs
//   maintain  → 1.8 g/kg protein, 30% fat, rest carbs
//   gain      → 1.8 g/kg protein, 25% fat, rest carbs
func MacroSplit(weightKg float64, kcal int, goal NutritionGoal) MacroTarget {
    var proteinPerKg, fatPct float64
    switch goal {
    case NutritionCut:  proteinPerKg, fatPct = 2.2, 0.25
    case NutritionGain: proteinPerKg, fatPct = 1.8, 0.25
    default:            proteinPerKg, fatPct = 1.8, 0.30
    }
    proteinG := int(weightKg * proteinPerKg)
    fatG     := int(float64(kcal) * fatPct / 9.0)
    carbG    := (kcal - proteinG*4 - fatG*9) / 4
    if carbG < 0 { carbG = 0 }
    return MacroTarget{Kcal: kcal, ProteinG: proteinG, CarbG: carbG, FatG: fatG}
}
```

These are the standard published formulas; the values are tunable like
`scoreExercise`'s weights are. Promote to a `Weights` struct if A/B testing or per-user
customization becomes a need.

## Food and meal library

Foods are atoms:

```go
type Food struct {
    ID         string
    Name       string
    KcalPer100 int
    ProteinG   float64  // per 100g
    CarbG      float64
    FatG       float64
    Category   FoodCategory  // protein, grain, vegetable, fruit, dairy, fat, condiment
    Allergens  []Allergen
    DietTags   []DietPreference  // diets this food is compatible with
}
```

Meals are compositions:

```go
type Meal struct {
    ID         string
    Name       string
    Slot       MealSlot           // default slot; can be served in others
    Components []Component        // {food, grams}
    PrepMin    int                // minutes to prepare
    CuisineTag string             // optional tagging for variety scoring
}

type Component struct {
    Food  Food
    Grams float64
}
```

A meal's kcal/macros are computed from its components — never stored on the Meal type
directly, because that would mean updating two places when a Food's macros change. The
`MacrosOf(meal Meal) MacroTarget` helper is the single source of truth.

Library size at v1:

- **~120 foods** across 7 categories. Examples: chicken breast, oats, lentils, almonds,
  spinach, salmon, brown rice, tofu, eggs, greek yogurt, broccoli, sweet potato,
  bananas, peanut butter, olive oil, etc.
- **~80 meals** composed from the foods. Examples: "Greek yogurt + berries + granola",
  "Chicken + rice + broccoli", "Lentil soup + sourdough", "Tofu stir-fry + rice",
  "Salmon + sweet potato + asparagus".

Library variety needs to be enough that an 80%-omnivore can hit 7 distinct days. With
80 meals across 5 slots × 7 days = 35 selections, variety scoring keeps each meal from
repeating more than ~2× in a week.

## Selection algorithm

The shape mirrors `plan/selection.go`:

```
1. Build candidate pool: filter library
   - drop meals whose ingredients hit any allergen
   - drop meals whose ingredients violate dietary preference
   - (no "experience level" equivalent for food)

2. Compute per-slot kcal target by splitting daily kcal across slots:
   Default 3-meal: 25% breakfast, 35% lunch, 35% dinner, 5% snack-buffer
   Default 4-meal: 22%, 30%, 33%, 15%
   Default 5-meal: 20%, 28%, 30%, 12%, 10%

3. For each day, for each slot:
   - Filter pool to meals tagged for that slot (or untagged → eligible for any slot)
   - Score each candidate:
       + closer to slot kcal target = higher score
       + closer to slot macro targets (proportional) = higher score
       + lower repetition penalty (used map across all days)
       + prep-time score: lower prep is better, but capped (variety wins)
       + small seeded jitter for variety run-to-run
   - Pick top candidate
   - Compute servings to hit slot kcal target exactly
   - Insert as DietMeal

4. Tally day totals vs day target; emit a warning if off by more than 10%.
```

This is the same constraint-and-coverage shape as `plan/selection.go`. The architectural
copy is intentional — reviewers see the pattern reused, and future personalization
weights apply identically.

## Grocery list (`grocery.go`)

Given a `DietPlan`, walk every meal in every day, multiply each component's grams by the
meal's servings, sum across all meals per Food.ID. Group results by `FoodCategory` for
human-readable output.

```go
type GroceryItem struct {
    Food     Food            `json:"food"`
    TotalG   float64         `json:"total_grams"`
    Category FoodCategory    `json:"category"`
}

type GroceryList struct {
    PlanID string                            `json:"plan_id"`
    Items  map[FoodCategory][]GroceryItem    `json:"items"`
}

func AggregateGroceries(p DietPlan) GroceryList { ... }
```

Returned by `GET /v1/diet-plans/{id}/grocery-list`. The aggregation is **derived** —
not stored. If a meal swap happens, the grocery list updates by being re-derived; we
don't have to invalidate a cache.

## Schema (`0003_diet_plans.sql` or `0004_…` if after Spec 01)

```sql
-- +goose Up
CREATE TABLE diet_plans (
    id            TEXT        PRIMARY KEY,
    user_id       TEXT        NOT NULL,
    goal          TEXT        NOT NULL,
    kcal_target   INT         NOT NULL,
    protein_g     INT         NOT NULL,
    carb_g        INT         NOT NULL,
    fat_g         INT         NOT NULL,
    days_per_week INT         NOT NULL,
    meals_per_day INT         NOT NULL,
    warnings      TEXT[]      NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX diet_plans_user_created_idx
    ON diet_plans (user_id, created_at DESC, id);

CREATE TABLE diet_days (
    plan_id TEXT NOT NULL REFERENCES diet_plans(id) ON DELETE CASCADE,
    day_idx INT  NOT NULL,
    PRIMARY KEY (plan_id, day_idx)
);

CREATE TABLE diet_meals (
    plan_id   TEXT NOT NULL,
    day_idx   INT  NOT NULL,
    slot      TEXT NOT NULL,
    meal      JSONB NOT NULL,   -- snapshot of domain.Meal w/ components
    servings  DOUBLE PRECISION NOT NULL,
    kcal      INT NOT NULL,
    protein_g INT NOT NULL,
    carb_g    INT NOT NULL,
    fat_g     INT NOT NULL,
    PRIMARY KEY (plan_id, day_idx, slot),
    FOREIGN KEY (plan_id, day_idx)
        REFERENCES diet_days(plan_id, day_idx) ON DELETE CASCADE,
    CHECK (servings > 0)
);
```

JSONB snapshot for meals — same reasoning as ADR-044 for plan exercises. Plans are
historical records; if the library is edited later, an existing plan still renders the
way it was generated.

## Tests

### Targets math (`targets_test.go`)

- BMR for a known reference person (the published validations from Mifflin-St Jeor's
  original paper)
- TDEE multiplies BMR correctly for each activity level
- KcalTarget subtracts 500 for cut, adds 300 for gain, matches TDEE for maintain
- MacroSplit produces kcal that sum (within rounding) to the input

### Library tests (`food_library_test.go`, `meal_library_test.go`)

- Every food has positive macros
- Every meal's components reference existing food IDs
- Vegan meals' components are all vegan-tagged foods
- Vegetarian meals' components contain no fish/shellfish/meat
- No meal exceeds 1500 kcal at default servings (sanity)

### Generator tests (`generator_test.go`)

- Vegan request → no animal-product foods appear
- Allergen filter: nut-allergy request → no nut-containing meals
- Cut goal hits kcal within 10% of target
- 7-day plan has reasonable variety (no meal repeats more than 3×)
- Seed reproducibility: same seed → same plan
- Beginner-impossible constraint (vegan + every allergen ticked) → emits warning, fills
  what's possible, doesn't crash

### Repository contract test

Same shape as plan: 7 scenarios across InMemory + Postgres.

### Grocery list tests

- Aggregating a known plan produces expected total grams per food
- Two meals using the same food sum, not duplicate
- Category grouping is correct

### Handler tests

- Create → 201 with plan ID in body
- Get → 200 with the same plan
- List by user → newest first
- Grocery list endpoint returns ingredients aggregated by category
- Missing X-User-ID → 400
- Validation errors → 400

## ADRs introduced

- **ADR-053: Nutrition mirrors plan as a feature slice.** Same architectural shape;
  reviewers see the pattern reused rather than learning a new layout.
- **ADR-054: TDEE/macro targets computed from request, not stored on a user profile.**
  No user-profile model exists yet. When it does, body weight / activity become profile
  defaults; this stays request-driven.
- **ADR-055: Food and meal libraries seeded in code, like exercise library.** Migration
  path to Postgres is the same as Spec 03 if/when adopted.
- **ADR-056: Macro splits are fixed per goal, not user-tunable.** Tunability becomes a
  product feature later; v1 uses published evidence-based defaults.
- **ADR-057: Grocery list is derived, not stored.** Re-deriving from the plan is cheap;
  storing it would require cache invalidation on every plan edit.
- **ADR-058: Meal stored as JSONB snapshot inside diet_meals.** Same reasoning as
  ADR-044 for plan exercises.

## Definition of done

- [ ] All new files written, package builds cleanly.
- [ ] `gofmt -w . && go vet ./... && go test -race ./...` green.
- [ ] `make test-integration` covers `./internal/nutrition/...` against Postgres.
- [ ] Six new ADRs in `ARCHITECTURE.md`.
- [ ] README's API table includes the five new endpoints.
- [ ] One end-to-end curl walkthrough in the README: generate plan → list → fetch →
      grocery list.
- [ ] Food/meal library counts at least 100 / 60 respectively (variety floor).

## Out of scope (deferred)

- **User profile.** Today the request carries every body metric. A `users` table with
  weight/height/age would let the client send a leaner request, but that's a separate
  feature.
- **Custom meals / user-uploaded recipes.** Admin-style endpoints for managing the
  meal library are out of scope here; could pair with Spec 03's exercise-library admin
  endpoints if both ship together.
- **Real food database (USDA / commercial).** ~120 in-code foods cover most home
  cooking; expanding to thousands is a library job, not an architectural one.
- **Meal substitution UI.** "Swap this dinner for another" would be a small additional
  endpoint, not in v1.
- **Smart shopping.** Grouping the grocery list by store aisle, deduping near-identical
  items, suggesting prep order — all future product work.
- **Nutrient tracking against logged meals.** Pairs with adherence loop (deferred in
  main architecture doc).

## What I cannot do without your involvement

Nothing external — this spec is fully self-contained. No third-party APIs, no
credentials, no encryption keys. Everything ships when you say "execute spec 02".
