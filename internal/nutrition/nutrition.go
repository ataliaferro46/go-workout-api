// Package nutrition manages the food library, per-user food logs, and
// macro/calorie targets driven by the user's profile. Calorie burn from
// today's cardio sessions is folded in so the daily net-calorie figure
// reflects actual energy balance.
package nutrition

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/auth"
	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/ataliaferro46/go-workout-api/internal/httpx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns persistence for foods + logs and a couple of derived
// queries we need for the daily summary (sum macros, sum cardio
// calories). One package == one repo keeps this focused.
type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// SearchFoods returns up to 25 foods matching the query (case-insensitive,
// substring match on name). The user's own custom foods get prefix-sorted
// above the seed library so personal entries surface first.
func (r *Repository) SearchFoods(ctx context.Context, userID, q string) ([]domain.Food, error) {
	q = strings.ToLower(strings.TrimSpace(q))
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, brand, serving_size, serving_unit,
		       calories, protein_g, carbs_g, fat_g, fiber_g, sugar_g, source
		FROM foods
		WHERE (user_id IS NULL OR user_id = $2)
		  AND ($1 = '' OR lower(name) LIKE '%' || $1 || '%')
		ORDER BY (user_id = $2) DESC, position($1 IN lower(name)), name
		LIMIT 25
	`, q, userID)
	if err != nil {
		return nil, fmt.Errorf("search foods: %w", err)
	}
	defer rows.Close()
	out := make([]domain.Food, 0)
	for rows.Next() {
		var f domain.Food
		if err := rows.Scan(&f.ID, &f.Name, &f.Brand, &f.ServingSize, &f.ServingUnit,
			&f.Calories, &f.ProteinG, &f.CarbsG, &f.FatG, &f.FiberG, &f.SugarG, &f.Source); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// LogFood inserts a food_log row.
func (r *Repository) LogFood(ctx context.Context, log domain.FoodLog) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO food_logs (id, user_id, food_id, servings, meal_type, logged_at, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, log.ID, log.UserID, log.FoodID, log.Servings, log.MealType, log.LoggedAt, log.Notes)
	if err != nil {
		return fmt.Errorf("insert food log: %w", err)
	}
	return nil
}

// DailyLogs returns the user's food logs for the given date (in their
// local interpretation — we accept a [start, end] range from the caller).
func (r *Repository) DailyLogs(ctx context.Context, userID string, start, end time.Time) ([]domain.FoodLog, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT l.id, l.user_id, l.food_id, l.servings, l.meal_type, l.logged_at, l.notes,
		       f.id, f.name, f.brand, f.serving_size, f.serving_unit,
		       f.calories, f.protein_g, f.carbs_g, f.fat_g, f.fiber_g, f.sugar_g
		FROM food_logs l
		JOIN foods f ON f.id = l.food_id
		WHERE l.user_id = $1 AND l.logged_at >= $2 AND l.logged_at < $3
		ORDER BY l.logged_at
	`, userID, start, end)
	if err != nil {
		return nil, fmt.Errorf("daily logs: %w", err)
	}
	defer rows.Close()
	out := make([]domain.FoodLog, 0)
	for rows.Next() {
		var l domain.FoodLog
		var f domain.Food
		if err := rows.Scan(&l.ID, &l.UserID, &l.FoodID, &l.Servings, &l.MealType, &l.LoggedAt, &l.Notes,
			&f.ID, &f.Name, &f.Brand, &f.ServingSize, &f.ServingUnit,
			&f.Calories, &f.ProteinG, &f.CarbsG, &f.FatG, &f.FiberG, &f.SugarG); err != nil {
			return nil, err
		}
		l.Food = &f
		out = append(out, l)
	}
	return out, rows.Err()
}

// CaloriesBurnedOn sums calories from cardio sessions whose workouts
// were created on the given date.
func (r *Repository) CaloriesBurnedOn(ctx context.Context, userID string, start, end time.Time) (int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(c.calories), 0)
		FROM cardio_sessions c
		JOIN workouts w ON w.id = c.workout_id
		WHERE w.user_id = $1 AND w.created_at >= $2 AND w.created_at < $3
	`, userID, start, end).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

// DeleteLog removes a single log entry. The handler does ownership check first.
func (r *Repository) DeleteLog(ctx context.Context, logID, userID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM food_logs WHERE id = $1 AND user_id = $2`, logID, userID)
	if err != nil {
		return fmt.Errorf("delete log: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// --- diet preferences ---------------------------------------------------

// GetDietPreferences returns the user's allergies + disliked-food
// keywords. Lower-cased so the filter is case-insensitive.
func (r *Repository) GetDietPreferences(ctx context.Context, userID string) ([]string, []string, error) {
	var a, d []string
	err := r.pool.QueryRow(ctx, `SELECT allergies, disliked_foods FROM users WHERE id = $1`, userID).
		Scan(&a, &d)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	return a, d, nil
}

// SetDietPreferences overwrites the user's allergies + disliked foods.
func (r *Repository) SetDietPreferences(ctx context.Context, userID string, allergies, dislikes []string) error {
	// Lower-case for case-insensitive matching at filter time.
	lower := func(in []string) []string {
		out := make([]string, 0, len(in))
		for _, s := range in {
			s = strings.TrimSpace(strings.ToLower(s))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	_, err := r.pool.Exec(ctx, `UPDATE users SET allergies = $2, disliked_foods = $3, updated_at = NOW() WHERE id = $1`,
		userID, lower(allergies), lower(dislikes))
	return err
}

// FilterByDietPreferences removes foods whose name contains any allergy
// or disliked-food substring (case-insensitive). Used by the meal-plan
// generator before running the macro-fit algorithm so the user never
// sees a peanut-butter suggestion when peanuts are on their allergy
// list.
func FilterByDietPreferences(foods []domain.Food, allergies, dislikes []string) []domain.Food {
	avoid := make([]string, 0, len(allergies)+len(dislikes))
	avoid = append(avoid, allergies...)
	avoid = append(avoid, dislikes...)
	if len(avoid) == 0 {
		return foods
	}
	out := make([]domain.Food, 0, len(foods))
	for _, f := range foods {
		name := strings.ToLower(f.Name + " " + f.Brand)
		bad := false
		for _, term := range avoid {
			if term != "" && strings.Contains(name, term) {
				bad = true
				break
			}
		}
		if !bad {
			out = append(out, f)
		}
	}
	return out
}

// --- meal plan persistence ----------------------------------------------

func (r *Repository) SaveMealPlan(ctx context.Context, p domain.MealPlan) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	targetsJSON, _ := json.Marshal(p.Targets)
	if _, err := tx.Exec(ctx, `
		INSERT INTO meal_plans (id, user_id, name, days, targets, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, p.ID, p.UserID, p.Name, p.Days, targetsJSON, p.CreatedAt); err != nil {
		return fmt.Errorf("insert plan: %w", err)
	}
	for _, it := range p.Items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO meal_plan_items (plan_id, day_idx, meal_type, sort_order, food_id, servings)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, p.ID, it.DayIdx, it.MealType, it.SortOrder, it.FoodID, it.Servings); err != nil {
			return fmt.Errorf("insert item: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) ListMealPlans(ctx context.Context, userID string) ([]domain.MealPlan, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, name, days, created_at
		FROM meal_plans WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.MealPlan, 0)
	for rows.Next() {
		var p domain.MealPlan
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Days, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) GetMealPlan(ctx context.Context, id string) (domain.MealPlan, error) {
	var p domain.MealPlan
	var targetsJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, name, days, targets, created_at
		FROM meal_plans WHERE id = $1
	`, id).Scan(&p.ID, &p.UserID, &p.Name, &p.Days, &targetsJSON, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return p, domain.ErrNotFound
		}
		return p, err
	}
	_ = json.Unmarshal(targetsJSON, &p.Targets)
	rows, err := r.pool.Query(ctx, `
		SELECT i.day_idx, i.meal_type, i.sort_order, i.food_id, i.servings,
		       f.id, f.name, f.brand, f.serving_size, f.serving_unit,
		       f.calories, f.protein_g, f.carbs_g, f.fat_g
		FROM meal_plan_items i
		LEFT JOIN foods f ON f.id = i.food_id
		WHERE i.plan_id = $1
		ORDER BY i.day_idx, i.meal_type, i.sort_order
	`, id)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var it domain.MealPlanItem
		var f domain.Food
		var fid, fname, fbrand, funit *string
		var fss *float64
		var fcal *int32
		var fp, fc, ff *float64
		if err := rows.Scan(&it.DayIdx, &it.MealType, &it.SortOrder, &it.FoodID, &it.Servings,
			&fid, &fname, &fbrand, &fss, &funit, &fcal, &fp, &fc, &ff); err != nil {
			return p, err
		}
		if fid != nil {
			f.ID = *fid
			if fname != nil { f.Name = *fname }
			if fbrand != nil { f.Brand = *fbrand }
			if fss != nil { f.ServingSize = *fss }
			if funit != nil { f.ServingUnit = *funit }
			if fcal != nil { f.Calories = int(*fcal) }
			if fp != nil { f.ProteinG = *fp }
			if fc != nil { f.CarbsG = *fc }
			if ff != nil { f.FatG = *ff }
			it.Food = &f
		}
		p.Items = append(p.Items, it)
	}
	return p, rows.Err()
}

func (r *Repository) DeleteMealPlan(ctx context.Context, id, userID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM meal_plans WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// --- recipes ------------------------------------------------------------

// SaveRecipe inserts a recipe, its ingredients, and a synthetic row in
// the foods table so the recipe shows up in food search and can be
// logged like any other food. Per-recipe macros are computed by summing
// ingredient macros, then divided by servings_per_recipe to get per-
// serving values stored on the foods row.
func (r *Repository) SaveRecipe(ctx context.Context, rec domain.Recipe) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO recipes (id, user_id, name, notes, servings_per_recipe, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, rec.ID, rec.UserID, rec.Name, rec.Notes, rec.ServingsPerRecipe, rec.CreatedAt); err != nil {
		return err
	}
	// Sum ingredient macros so we can synthesize a foods row.
	var cal float64
	var pG, cG, fG float64
	for _, ing := range rec.Ingredients {
		if _, err := tx.Exec(ctx, `
			INSERT INTO recipe_ingredients (recipe_id, sort_order, food_id, servings)
			VALUES ($1, $2, $3, $4)
		`, rec.ID, ing.SortOrder, ing.FoodID, ing.Servings); err != nil {
			return err
		}
		// Look up ingredient macros from foods.
		var fcal int
		var fp, fc, ffat float64
		if err := tx.QueryRow(ctx,
			`SELECT calories, protein_g, carbs_g, fat_g FROM foods WHERE id = $1`, ing.FoodID).
			Scan(&fcal, &fp, &fc, &ffat); err != nil {
			continue // ingredient missing from foods table; skip in totals
		}
		cal += float64(fcal) * ing.Servings
		pG += fp * ing.Servings
		cG += fc * ing.Servings
		fG += ffat * ing.Servings
	}
	sps := rec.ServingsPerRecipe
	if sps <= 0 {
		sps = 1
	}
	syntheticID := "recipe-" + rec.ID
	if _, err := tx.Exec(ctx, `
		INSERT INTO foods (id, name, serving_size, serving_unit, calories,
		                   protein_g, carbs_g, fat_g, source, user_id)
		VALUES ($1, $2, 1, 'serving', $3, $4, $5, $6, 'recipe', $7)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			calories = EXCLUDED.calories,
			protein_g = EXCLUDED.protein_g,
			carbs_g = EXCLUDED.carbs_g,
			fat_g = EXCLUDED.fat_g
	`, syntheticID, rec.Name+" (recipe)",
		int(cal/sps+0.5), pG/sps, cG/sps, fG/sps, rec.UserID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) ListRecipes(ctx context.Context, userID string) ([]domain.Recipe, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, notes, servings_per_recipe, created_at
		FROM recipes WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Recipe, 0)
	for rows.Next() {
		var rec domain.Recipe
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.Notes, &rec.ServingsPerRecipe, &rec.CreatedAt); err != nil {
			return nil, err
		}
		rec.UserID = userID
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteRecipe(ctx context.Context, id, userID string) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `DELETE FROM recipes WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	// Best-effort delete the synthetic food row too.
	_, _ = tx.Exec(ctx, `DELETE FROM foods WHERE id = $1`, "recipe-"+id)
	return tx.Commit(ctx)
}

// GetFood loads one food by id. Returns nil + nil when not found
// (callers treat that as "go fetch from upstream").
func (r *Repository) GetFood(ctx context.Context, id string) (*domain.Food, error) {
	var f domain.Food
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, brand, serving_size, serving_unit,
		       calories, protein_g, carbs_g, fat_g, fiber_g, sugar_g, source
		FROM foods WHERE id = $1
	`, id).Scan(&f.ID, &f.Name, &f.Brand, &f.ServingSize, &f.ServingUnit,
		&f.Calories, &f.ProteinG, &f.CarbsG, &f.FatG, &f.FiberG, &f.SugarG, &f.Source)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get food: %w", err)
	}
	return &f, nil
}

// FoodCount returns the total number of foods in the library. Used by
// the meal-plan generator to decide whether the variety pool needs
// enrichment from USDA.
func (r *Repository) FoodCount(ctx context.Context) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM foods`).Scan(&n)
	return n, err
}

// SaveTargets stores user-overridden targets, or values computed by
// ComputeTargets when the user picks "auto from profile".
func (r *Repository) SaveTargets(ctx context.Context, userID string, t domain.NutritionTargets) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET
			target_calories  = $2,
			target_protein_g = $3,
			target_carbs_g   = $4,
			target_fat_g     = $5,
			nutrition_goal   = $6,
			activity_level   = $7,
			updated_at       = NOW()
		WHERE id = $1
	`, userID, t.Calories, t.ProteinG, t.CarbsG, t.FatG, t.Goal, t.ActivityLevel)
	if err != nil {
		return fmt.Errorf("save targets: %w", err)
	}
	return nil
}

// LoadTargets fetches stored targets. Returns zero-valued struct when
// the user hasn't set any (the handler treats that as "compute defaults").
func (r *Repository) LoadTargets(ctx context.Context, userID string) (domain.NutritionTargets, error) {
	var t domain.NutritionTargets
	var cal, p, c, f *int32
	var goal, lvl *string
	err := r.pool.QueryRow(ctx, `
		SELECT target_calories, target_protein_g, target_carbs_g, target_fat_g,
		       nutrition_goal, activity_level
		FROM users WHERE id = $1
	`, userID).Scan(&cal, &p, &c, &f, &goal, &lvl)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return t, nil
		}
		return t, err
	}
	if cal != nil {
		t.Calories = int(*cal)
	}
	if p != nil {
		t.ProteinG = int(*p)
	}
	if c != nil {
		t.CarbsG = int(*c)
	}
	if f != nil {
		t.FatG = int(*f)
	}
	if goal != nil {
		t.Goal = *goal
	}
	if lvl != nil {
		t.ActivityLevel = *lvl
	}
	return t, nil
}

// --- Computation ---------------------------------------------------------

// activityMultipliers is the standard Harris-Benedict multiplier table.
var activityMultipliers = map[string]float64{
	"sedentary": 1.2,
	"light":     1.375,
	"moderate": 1.55,
	"very":     1.725,
	"extreme":  1.9,
}

// goalAdjustments shifts calories by goal: lose = -500/day (~1 lb/wk
// deficit), gain = +300 (lean bulk), maintain = 0, recomp = -200 (slight
// deficit with protein bias).
var goalAdjustments = map[string]int{
	"maintain": 0,
	"lose":     -500,
	"gain":     300,
	"recomp":   -200,
}

// ComputeTargets uses Mifflin-St Jeor for BMR, then applies activity
// multiplier + goal adjustment. Returns zero-valued targets if the
// inputs are insufficient (no weight, no height, no birth date).
func ComputeTargets(u auth.User, goal, activity string) domain.NutritionTargets {
	if u.WeightKG == nil || u.HeightCM == nil || u.BirthDate == nil {
		return domain.NutritionTargets{Goal: goal, ActivityLevel: activity}
	}
	age := yearsBetween(*u.BirthDate, time.Now().UTC())
	weight := *u.WeightKG
	height := float64(*u.HeightCM)
	// Mifflin-St Jeor (sex-aware).
	bmrBase := 10*weight + 6.25*height - 5*float64(age)
	var bmr float64
	switch strings.ToLower(u.Sex) {
	case "male":
		bmr = bmrBase + 5
	case "female":
		bmr = bmrBase - 161
	default:
		// Average of the two formulas — best honest default for
		// unspecified / other.
		bmr = bmrBase - 78
	}
	mult, ok := activityMultipliers[activity]
	if !ok {
		mult = 1.375 // light activity baseline
	}
	tdee := bmr * mult
	adj, ok := goalAdjustments[goal]
	if !ok {
		adj = 0
	}
	calories := int(tdee) + adj

	// Macro split: protein 1g/lb body weight (recomp/gain), 1.2g/lb
	// (lose). Then fat at ~25% of calories. Carbs fill the rest.
	proteinG := int(weight * 2.2 * 1.0)
	if goal == "lose" {
		proteinG = int(weight * 2.2 * 1.2)
	}
	if goal == "gain" {
		proteinG = int(weight * 2.2 * 0.9)
	}
	fatG := int(float64(calories) * 0.25 / 9.0)
	carbCalories := calories - proteinG*4 - fatG*9
	carbsG := carbCalories / 4
	if carbsG < 0 {
		carbsG = 0
	}

	return domain.NutritionTargets{
		Calories: calories, ProteinG: proteinG, CarbsG: carbsG, FatG: fatG,
		Goal: goal, ActivityLevel: activity,
		BMR: int(bmr), TDEE: int(tdee),
	}
}

func yearsBetween(birth, now time.Time) int {
	years := now.Year() - birth.Year()
	if now.YearDay() < birth.YearDay() {
		years--
	}
	if years < 0 {
		years = 0
	}
	return years
}

// SumTotals adds up the macros across logs. Servings are already
// embedded — a serving of 2.0 doubles the macros.
func SumTotals(logs []domain.FoodLog) domain.NutritionTotals {
	var t domain.NutritionTotals
	for _, l := range logs {
		if l.Food == nil {
			continue
		}
		s := l.Servings
		t.Calories += int(float64(l.Food.Calories) * s)
		t.ProteinG += l.Food.ProteinG * s
		t.CarbsG += l.Food.CarbsG * s
		t.FatG += l.Food.FatG * s
	}
	return t
}

// --- HTTP layer ----------------------------------------------------------

// UserFetcher returns the auth User for the given id — injected so this
// package doesn't take a hard dependency on auth.Service.
type UserFetcher func(ctx context.Context, userID string) (auth.User, error)

type Handler struct {
	repo    *Repository
	getUser UserFetcher
}

func NewHandler(repo *Repository, getUser UserFetcher) *Handler {
	return &Handler{repo: repo, getUser: getUser}
}

func (h *Handler) Routes(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/foods/search", requireAuth(http.HandlerFunc(h.searchFoods)))
	mux.Handle("POST /v1/nutrition/log", requireAuth(http.HandlerFunc(h.logFood)))
	mux.Handle("DELETE /v1/nutrition/log/{id}", requireAuth(http.HandlerFunc(h.deleteLog)))
	mux.Handle("GET /v1/nutrition/daily", requireAuth(http.HandlerFunc(h.daily)))
	mux.Handle("GET /v1/nutrition/targets", requireAuth(http.HandlerFunc(h.getTargets)))
	mux.Handle("PUT /v1/nutrition/targets", requireAuth(http.HandlerFunc(h.putTargets)))
	mux.Handle("POST /v1/nutrition/targets/auto", requireAuth(http.HandlerFunc(h.autoTargets)))
	mux.Handle("POST /v1/nutrition/meal-plan", requireAuth(http.HandlerFunc(h.generateMealPlan)))
	mux.Handle("POST /v1/nutrition/meal-plan/multi-day", requireAuth(http.HandlerFunc(h.generateMultiDayPlan)))
	mux.Handle("POST /v1/nutrition/log/batch", requireAuth(http.HandlerFunc(h.batchLog)))
	mux.Handle("POST /v1/foods/barcode", requireAuth(http.HandlerFunc(h.barcodeLookup)))

	// Persisted meal plans
	mux.Handle("POST /v1/nutrition/meal-plans", requireAuth(http.HandlerFunc(h.savePlan)))
	mux.Handle("GET /v1/nutrition/meal-plans", requireAuth(http.HandlerFunc(h.listPlans)))
	mux.Handle("GET /v1/nutrition/meal-plans/{id}", requireAuth(http.HandlerFunc(h.getPlan)))
	mux.Handle("DELETE /v1/nutrition/meal-plans/{id}", requireAuth(http.HandlerFunc(h.deletePlan)))

	// Recipes
	mux.Handle("POST /v1/nutrition/recipes", requireAuth(http.HandlerFunc(h.createRecipe)))
	mux.Handle("GET /v1/nutrition/recipes", requireAuth(http.HandlerFunc(h.listRecipes)))
	mux.Handle("DELETE /v1/nutrition/recipes/{id}", requireAuth(http.HandlerFunc(h.deleteRecipe)))

	// Diet preferences (allergies / dislikes)
	mux.Handle("PATCH /v1/nutrition/preferences", requireAuth(http.HandlerFunc(h.updatePreferences)))
	mux.Handle("GET /v1/nutrition/preferences", requireAuth(http.HandlerFunc(h.getPreferences)))
}

// --- preferences (allergies + dislikes) ---------------------------------

func (h *Handler) getPreferences(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	a, d, err := h.repo.GetDietPreferences(r.Context(), u.ID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"allergies": a, "disliked_foods": d})
}

func (h *Handler) updatePreferences(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	var req struct {
		Allergies     []string `json:"allergies"`
		DislikedFoods []string `json:"disliked_foods"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if err := h.repo.SetDietPreferences(r.Context(), u.ID, req.Allergies, req.DislikedFoods); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"allergies": req.Allergies, "disliked_foods": req.DislikedFoods})
}

// --- meal plan persistence ----------------------------------------------

func (h *Handler) savePlan(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	var req domain.MealPlan
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if req.Name == "" {
		req.Name = "Meal plan " + time.Now().Format("Jan 2")
	}
	if req.Days <= 0 {
		req.Days = 1
	}
	req.ID = newID()
	req.UserID = u.ID
	req.CreatedAt = time.Now().UTC()
	if err := h.repo.SaveMealPlan(r.Context(), req); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, req)
}

func (h *Handler) listPlans(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	plans, err := h.repo.ListMealPlans(r.Context(), u.ID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"plans": plans})
}

func (h *Handler) getPlan(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	plan, err := h.repo.GetMealPlan(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if plan.UserID != u.ID {
		httpx.Error(w, domain.ErrNotFound)
		return
	}
	httpx.JSON(w, http.StatusOK, plan)
}

func (h *Handler) deletePlan(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	if err := h.repo.DeleteMealPlan(r.Context(), r.PathValue("id"), u.ID); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}

// --- recipes ------------------------------------------------------------

func (h *Handler) createRecipe(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	var req domain.Recipe
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if req.Name == "" || len(req.Ingredients) == 0 {
		httpx.Error(w, &domain.ValidationError{Message: "name + at least one ingredient required"})
		return
	}
	if req.ServingsPerRecipe <= 0 {
		req.ServingsPerRecipe = 1
	}
	req.ID = newID()
	req.UserID = u.ID
	req.CreatedAt = time.Now().UTC()
	if err := h.repo.SaveRecipe(r.Context(), req); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, req)
}

func (h *Handler) listRecipes(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	rs, err := h.repo.ListRecipes(r.Context(), u.ID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"recipes": rs})
}

func (h *Handler) deleteRecipe(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	if err := h.repo.DeleteRecipe(r.Context(), r.PathValue("id"), u.ID); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}

// barcodeLookup hits Open Food Facts for the barcode. Cache hit returns
// from local DB; miss fetches from OFF, caches, returns. Endpoint shape:
//
//	POST /v1/foods/barcode { "barcode": "0049000028928" }
//	→ { "food": { ... } }
func (h *Handler) barcodeLookup(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	var req struct {
		Barcode string `json:"barcode"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if req.Barcode == "" {
		httpx.Error(w, &domain.ValidationError{Message: "barcode required"})
		return
	}
	id := "off-" + req.Barcode
	if existing, _ := h.repo.GetFood(r.Context(), id); existing != nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"food": existing})
		return
	}
	food, err := lookupOpenFoodFacts(r.Context(), req.Barcode)
	if err != nil {
		httpx.Error(w, &domain.ValidationError{Message: err.Error()})
		return
	}
	h.repo.CacheUSDAResults(r.Context(), []domain.Food{*food}) // same upsert path
	_ = u
	httpx.JSON(w, http.StatusOK, map[string]any{"food": food})
}

// lookupOpenFoodFacts queries the public OFF product endpoint. No auth
// required, but we identify ourselves via User-Agent per their terms.
// Returns nil + error if the product isn't found or has no macro data.
func lookupOpenFoodFacts(ctx context.Context, barcode string) (*domain.Food, error) {
	endpoint := "https://world.openfoodfacts.org/api/v2/product/" + url.PathEscape(barcode) + ".json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "workout.api (alexandertaliaferro1@gmail.com)")
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("open food facts returned %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var page struct {
		Status  int    `json:"status"`
		Product struct {
			ProductName string `json:"product_name"`
			Brands      string `json:"brands"`
			ServingSize string `json:"serving_size"`
			Nutriments  map[string]any `json:"nutriments"`
		} `json:"product"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	if page.Status != 1 {
		return nil, fmt.Errorf("product not in Open Food Facts")
	}
	name := strings.TrimSpace(page.Product.ProductName)
	if name == "" {
		return nil, fmt.Errorf("product has no name")
	}
	// OFF nutriments use "_100g" suffix for per-100g values. Pull them as
	// float64 with safe casts (the API returns floats but some fields are
	// missing for some products).
	num := func(k string) float64 {
		v, ok := page.Product.Nutriments[k]
		if !ok {
			return 0
		}
		f, _ := v.(float64)
		return f
	}
	cal := num("energy-kcal_100g")
	if cal == 0 {
		// Fallback: kJ → kcal (4.184 kJ per kcal)
		if kj := num("energy_100g"); kj > 0 {
			cal = kj / 4.184
		}
	}
	if cal == 0 {
		return nil, fmt.Errorf("product has no calorie data")
	}
	food := &domain.Food{
		ID:          "off-" + barcode,
		Name:        name,
		Brand:       strings.TrimSpace(page.Product.Brands),
		ServingSize: 100,
		ServingUnit: "g",
		Calories:    int(cal + 0.5),
		ProteinG:    num("proteins_100g"),
		CarbsG:      num("carbohydrates_100g"),
		FatG:        num("fat_100g"),
		FiberG:      num("fiber_100g"),
		SugarG:      num("sugars_100g"),
		Source:      "off",
	}
	return food, nil
}

// batchLog inserts multiple food log entries in one request. Useful for
// "log entire suggested meal plan in one tap." Each entry validates
// independently; the response includes the count inserted.
func (h *Handler) batchLog(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	var req struct {
		Logs []struct {
			FoodID   string  `json:"food_id"`
			Servings float64 `json:"servings"`
			MealType string  `json:"meal_type"`
			Notes    string  `json:"notes,omitempty"`
		} `json:"logs"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	now := time.Now().UTC()
	inserted := 0
	for _, l := range req.Logs {
		if l.FoodID == "" || l.Servings <= 0 {
			continue
		}
		mealType := l.MealType
		if mealType == "" {
			mealType = "snack"
		}
		if err := h.repo.LogFood(r.Context(), domain.FoodLog{
			ID: newID(), UserID: u.ID, FoodID: l.FoodID,
			Servings: l.Servings, MealType: mealType,
			LoggedAt: now, Notes: l.Notes,
		}); err != nil {
			httpx.Error(w, err)
			return
		}
		inserted++
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"inserted": inserted})
}

// generateMultiDayPlan generates suggested meals for N consecutive days.
// We vary the random seed per day so the algorithm picks different
// foods, then dedupe via the existing `used` map across days. Caller
// passes { "days": 7 } for a week.
func (h *Handler) generateMultiDayPlan(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	var req struct {
		Days int `json:"days"`
	}
	_ = httpx.DecodeJSON(r, &req)
	if req.Days <= 0 {
		req.Days = 7
	}
	if req.Days > 14 {
		req.Days = 14
	}
	targets, err := h.repo.LoadTargets(r.Context(), u.ID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if targets.Calories == 0 {
		httpx.Error(w, &domain.ValidationError{
			Message: "set your nutrition targets first",
		})
		return
	}
	h.ensureVarietyPool(r.Context())
	foods, err := h.repo.SearchFoods(r.Context(), u.ID, "")
	if err != nil {
		httpx.Error(w, err)
		return
	}
	allergies, dislikes, _ := h.repo.GetDietPreferences(r.Context(), u.ID)
	foods = FilterByDietPreferences(foods, allergies, dislikes)
	type Day struct {
		Date  string     `json:"date"`
		Meals []MealSlot `json:"meals"`
	}
	days := make([]Day, 0, req.Days)
	used := make(map[string]int) // food id → times used across all days
	start := time.Now().UTC()
	for d := 0; d < req.Days; d++ {
		dayDate := start.AddDate(0, 0, d)
		meals := pickMealsWithUsed(foods, targets, used)
		// Bump used so the next day picks differently.
		for _, m := range meals {
			for _, fs := range m.Foods {
				used[fs.FoodID]++
			}
		}
		days = append(days, Day{
			Date:  dayDate.Format("2006-01-02"),
			Meals: meals,
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"days": days})
}

// pickMealsWithUsed is pickMeals' multi-day-aware sibling: the `used`
// map persists across days so the planner naturally rotates foods over
// a week. pickMeals delegates to this by passing a fresh empty map.
func pickMealsWithUsed(foods []domain.Food, t domain.NutritionTargets, used map[string]int) []MealSlot {
	slots := []struct{ name string; pct float64 }{
		{"breakfast", 0.25}, {"lunch", 0.35}, {"dinner", 0.30}, {"snack", 0.10},
	}
	totalP := float64(t.ProteinG)
	out := make([]MealSlot, 0, len(slots))
	for _, s := range slots {
		mealCal := int(float64(t.Calories) * s.pct)
		mealProt := totalP * s.pct
		picked := pickForMeal(foods, mealCal, mealProt, used)
		var tot domain.NutritionTotals
		for _, fs := range picked {
			tot.Calories += int(float64(fs.Food.Calories) * fs.Servings)
			tot.ProteinG += fs.Food.ProteinG * fs.Servings
			tot.CarbsG += fs.Food.CarbsG * fs.Servings
			tot.FatG += fs.Food.FatG * fs.Servings
			used[fs.FoodID]++
		}
		out = append(out, MealSlot{
			Meal: s.name, TargetCalories: mealCal,
			Foods: picked, Totals: tot,
		})
	}
	return out
}

// generateMealPlan picks foods that fit the user's daily macro budget,
// allocated across four meal slots (breakfast 25%, lunch 35%, dinner 30%,
// snack 10%). Greedy algorithm: for each meal, satisfy protein first,
// then fill carbs + fat to the per-meal budget.
//
// Variety pool: if the local foods library is sparse, we fan out to USDA
// (in parallel) for a list of common-food category queries before
// running the algorithm. The fetched foods land in the cache, so the
// next meal plan benefits from them too.
func (h *Handler) generateMealPlan(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	targets, err := h.repo.LoadTargets(r.Context(), u.ID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if targets.Calories == 0 {
		httpx.Error(w, &domain.ValidationError{
			Message: "set your nutrition targets first (auto-compute or manual)",
		})
		return
	}
	h.ensureVarietyPool(r.Context())
	foods, err := h.repo.SearchFoods(r.Context(), u.ID, "")
	if err != nil {
		httpx.Error(w, err)
		return
	}
	allergies, dislikes, _ := h.repo.GetDietPreferences(r.Context(), u.ID)
	foods = FilterByDietPreferences(foods, allergies, dislikes)
	plan := pickMeals(foods, targets)
	httpx.JSON(w, http.StatusOK, map[string]any{"meal_plan": plan})
}

// varietyQueries are the seed terms we fan out to USDA when local foods
// are sparse. Picked to cover the major macro slots (lean protein, slow
// carbs, fats, vegetables, fruit, dairy) without overlapping each other
// too much.
var varietyQueries = []string{
	"chicken breast", "ground beef", "salmon", "tuna", "eggs",
	"greek yogurt", "cottage cheese", "tofu",
	"oats", "brown rice", "quinoa", "sweet potato", "whole wheat bread",
	"black beans", "lentils",
	"broccoli", "spinach", "kale", "bell pepper",
	"blueberries", "strawberries", "banana",
	"almonds", "peanut butter", "avocado", "olive oil",
}

// ensureVarietyPool fans out to USDA in parallel for the variety queries
// when the local foods library is under ~100 rows. Best-effort: errors
// are silently dropped so a USDA outage doesn't break meal-plan
// generation. Each query's results are cached into the foods table via
// CacheUSDAResults so we don't repeat the call next time.
func (h *Handler) ensureVarietyPool(ctx context.Context) {
	if os.Getenv("FDC_API_KEY") == "" {
		return // USDA disabled; use what we have
	}
	count, err := h.repo.FoodCount(ctx)
	if err != nil || count >= 100 {
		return // already enriched
	}
	type result struct{ foods []domain.Food }
	results := make(chan result, len(varietyQueries))
	for _, q := range varietyQueries {
		go func(q string) {
			// Cap per-query latency so a slow USDA response can't stall
			// the whole pool.
			cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			foods, err := searchUSDA(cctx, q)
			if err != nil {
				results <- result{}
				return
			}
			results <- result{foods: foods}
		}(q)
	}
	all := make([]domain.Food, 0, len(varietyQueries)*8)
	for i := 0; i < len(varietyQueries); i++ {
		r := <-results
		all = append(all, r.foods...)
	}
	if len(all) > 0 {
		h.repo.CacheUSDAResults(ctx, all)
	}
}

// MealSlot is a single meal in the generated plan.
type MealSlot struct {
	Meal     string             `json:"meal"`
	TargetCalories int          `json:"target_calories"`
	Foods    []MealFoodSuggestion `json:"foods"`
	Totals   domain.NutritionTotals `json:"totals"`
}

// MealFoodSuggestion is one food + servings in a meal slot.
type MealFoodSuggestion struct {
	FoodID   string      `json:"food_id"`
	Food     domain.Food `json:"food"`
	Servings float64     `json:"servings"`
}

// pickMeals runs a simple greedy macro fit per meal slot. Per-meal
// budgets: 25/35/30/10 for breakfast/lunch/dinner/snack. Protein gets
// satisfied first via the highest-protein-per-calorie foods, then
// carbs/fat top up.
func pickMeals(foods []domain.Food, t domain.NutritionTargets) []MealSlot {
	slots := []struct{ name string; pct float64 }{
		{"breakfast", 0.25}, {"lunch", 0.35}, {"dinner", 0.30}, {"snack", 0.10},
	}
	totalP := float64(t.ProteinG)
	out := make([]MealSlot, 0, len(slots))
	used := make(map[string]int) // food id → times used (penalize repeats)
	for _, s := range slots {
		mealCal := int(float64(t.Calories) * s.pct)
		mealProt := totalP * s.pct
		picked := pickForMeal(foods, mealCal, mealProt, used)
		var tot domain.NutritionTotals
		for _, fs := range picked {
			tot.Calories += int(float64(fs.Food.Calories) * fs.Servings)
			tot.ProteinG += fs.Food.ProteinG * fs.Servings
			tot.CarbsG += fs.Food.CarbsG * fs.Servings
			tot.FatG += fs.Food.FatG * fs.Servings
			used[fs.FoodID]++
		}
		out = append(out, MealSlot{
			Meal: s.name, TargetCalories: mealCal,
			Foods: picked, Totals: tot,
		})
	}
	return out
}

func pickForMeal(foods []domain.Food, mealCal int, mealProtein float64, used map[string]int) []MealFoodSuggestion {
	// Sort by protein density (g protein per kcal) descending, with a
	// penalty for already-used foods to encourage variety.
	type scored struct {
		ex     domain.Food
		score  float64
	}
	cands := make([]scored, 0, len(foods))
	for _, f := range foods {
		if f.Calories == 0 {
			continue
		}
		pd := f.ProteinG / float64(f.Calories)
		penalty := float64(used[f.ID]) * 0.5
		cands = append(cands, scored{ex: f, score: pd - penalty})
	}
	// Bubble-sort descending — small N.
	for i := 0; i < len(cands); i++ {
		for j := i + 1; j < len(cands); j++ {
			if cands[j].score > cands[i].score {
				cands[i], cands[j] = cands[j], cands[i]
			}
		}
	}

	picked := make([]MealFoodSuggestion, 0, 3)
	usedCal := 0
	usedProt := 0.0
	// First pass: hit protein target with up to 2 protein-dense foods.
	for _, c := range cands {
		if usedProt >= mealProtein*0.7 || len(picked) >= 2 {
			break
		}
		if c.ex.ProteinG < 5 {
			continue
		}
		need := mealProtein*0.5 - usedProt
		if need < 5 {
			need = 5
		}
		servings := need / c.ex.ProteinG
		if servings < 0.5 {
			servings = 0.5
		}
		if servings > 3 {
			servings = 3
		}
		// Cap by calorie budget.
		maxCal := mealCal - usedCal
		if int(float64(c.ex.Calories)*servings) > maxCal {
			if c.ex.Calories == 0 {
				continue
			}
			servings = float64(maxCal) / float64(c.ex.Calories)
			if servings < 0.5 {
				continue
			}
		}
		picked = append(picked, MealFoodSuggestion{
			FoodID: c.ex.ID, Food: c.ex, Servings: round1(servings),
		})
		usedCal += int(float64(c.ex.Calories) * servings)
		usedProt += c.ex.ProteinG * servings
	}
	// Second pass: fill remaining calories with carb/fat-dense foods that
	// haven't been picked yet in this meal.
	for _, c := range cands {
		if usedCal >= int(float64(mealCal)*0.9) || len(picked) >= 3 {
			break
		}
		if containsID(picked, c.ex.ID) {
			continue
		}
		if c.ex.Calories == 0 {
			continue
		}
		need := mealCal - usedCal
		servings := float64(need) / float64(c.ex.Calories)
		if servings < 0.5 {
			servings = 0.5
		}
		if servings > 2 {
			servings = 2
		}
		picked = append(picked, MealFoodSuggestion{
			FoodID: c.ex.ID, Food: c.ex, Servings: round1(servings),
		})
		usedCal += int(float64(c.ex.Calories) * servings)
	}
	return picked
}

func containsID(picks []MealFoodSuggestion, id string) bool {
	for _, p := range picks {
		if p.FoodID == id {
			return true
		}
	}
	return false
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

func (h *Handler) searchFoods(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	q := r.URL.Query().Get("q")
	foods, err := h.repo.SearchFoods(r.Context(), u.ID, q)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	// USDA fallback: if local results are sparse and the user has a real
	// query, hit USDA FoodData Central, cache up to 8 matches, and merge.
	if len(foods) < 4 && len(q) >= 3 {
		if usda, err := searchUSDA(r.Context(), q); err == nil && len(usda) > 0 {
			cached := h.repo.CacheUSDAResults(r.Context(), usda)
			foods = mergeDedup(foods, cached)
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"foods": foods})
}

// USDA FoodData Central minimal search. Requires FDC_API_KEY env var.
// We pull "Foundation" + "SR Legacy" food types (standard reference) for
// reliable macro data. Branded foods are excluded — those macros are
// frequently mis-reported by manufacturers.
func searchUSDA(ctx context.Context, q string) ([]domain.Food, error) {
	apiKey := os.Getenv("FDC_API_KEY")
	if apiKey == "" {
		return nil, errors.New("FDC_API_KEY not set")
	}
	endpoint := "https://api.nal.usda.gov/fdc/v1/foods/search?" + url.Values{
		"query":     {q},
		"dataType":  {"Foundation,SR Legacy"},
		"pageSize":  {"8"},
		"api_key":   {apiKey},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("usda %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var page struct {
		Foods []struct {
			FDCID       int    `json:"fdcId"`
			Description string `json:"description"`
			FoodNutrients []struct {
				NutrientName string  `json:"nutrientName"`
				Value        float64 `json:"value"`
				UnitName     string  `json:"unitName"`
			} `json:"foodNutrients"`
		} `json:"foods"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	out := make([]domain.Food, 0, len(page.Foods))
	for _, f := range page.Foods {
		food := domain.Food{
			ID:          fmt.Sprintf("usda-%d", f.FDCID),
			Name:        f.Description,
			ServingSize: 100,
			ServingUnit: "g",
			Source:      "usda",
		}
		for _, n := range f.FoodNutrients {
			switch n.NutrientName {
			case "Energy":
				if n.UnitName == "KCAL" {
					food.Calories = int(n.Value + 0.5)
				}
			case "Protein":
				food.ProteinG = n.Value
			case "Carbohydrate, by difference":
				food.CarbsG = n.Value
			case "Total lipid (fat)":
				food.FatG = n.Value
			case "Fiber, total dietary":
				food.FiberG = n.Value
			case "Sugars, total including NLEA":
				food.SugarG = n.Value
			}
		}
		if food.Calories > 0 {
			out = append(out, food)
		}
	}
	return out, nil
}

// CacheUSDAResults persists USDA results into the foods table on a
// best-effort basis (ON CONFLICT DO NOTHING) so future searches hit
// the local DB before going out to USDA. Returns the foods (with their
// final IDs) for the caller to merge into search output.
func (r *Repository) CacheUSDAResults(ctx context.Context, foods []domain.Food) []domain.Food {
	for i := range foods {
		_, _ = r.pool.Exec(ctx, `
			INSERT INTO foods
				(id, name, serving_size, serving_unit, calories,
				 protein_g, carbs_g, fat_g, fiber_g, sugar_g, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'usda')
			ON CONFLICT (id) DO NOTHING
		`, foods[i].ID, foods[i].Name, foods[i].ServingSize, foods[i].ServingUnit,
			foods[i].Calories, foods[i].ProteinG, foods[i].CarbsG, foods[i].FatG,
			foods[i].FiberG, foods[i].SugarG)
	}
	return foods
}

func mergeDedup(a, b []domain.Food) []domain.Food {
	seen := make(map[string]bool, len(a))
	out := make([]domain.Food, 0, len(a)+len(b))
	for _, f := range a {
		if !seen[f.ID] {
			seen[f.ID] = true
			out = append(out, f)
		}
	}
	for _, f := range b {
		if !seen[f.ID] {
			seen[f.ID] = true
			out = append(out, f)
		}
	}
	return out
}

func (h *Handler) logFood(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	var req struct {
		FoodID   string  `json:"food_id"`
		Servings float64 `json:"servings"`
		MealType string  `json:"meal_type"`
		LoggedAt string  `json:"logged_at,omitempty"`
		Notes    string  `json:"notes,omitempty"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if req.FoodID == "" || req.Servings <= 0 {
		httpx.Error(w, &domain.ValidationError{Message: "food_id + servings (>0) required"})
		return
	}
	if req.MealType == "" {
		req.MealType = "snack"
	}
	at := time.Now().UTC()
	if req.LoggedAt != "" {
		if t, err := time.Parse(time.RFC3339, req.LoggedAt); err == nil {
			at = t
		}
	}
	log := domain.FoodLog{
		ID: newID(), UserID: u.ID, FoodID: req.FoodID,
		Servings: req.Servings, MealType: req.MealType,
		LoggedAt: at, Notes: req.Notes,
	}
	if err := h.repo.LogFood(r.Context(), log); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, log)
}

func (h *Handler) deleteLog(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	if err := h.repo.DeleteLog(r.Context(), r.PathValue("id"), u.ID); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}

func (h *Handler) daily(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	dateStr := r.URL.Query().Get("date")
	day := time.Now().UTC()
	if dateStr != "" {
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			httpx.Error(w, &domain.ValidationError{Message: "date must be YYYY-MM-DD"})
			return
		}
		day = t
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	logs, err := h.repo.DailyLogs(r.Context(), u.ID, start, end)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	burned, _ := h.repo.CaloriesBurnedOn(r.Context(), u.ID, start, end)
	targets, _ := h.repo.LoadTargets(r.Context(), u.ID)
	totals := SumTotals(logs)
	out := domain.NutritionDaily{
		Date: start, Logs: logs, Totals: totals,
		Targets: targets, CaloriesBurned: burned,
		NetCalories: totals.Calories - burned,
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) getTargets(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	t, err := h.repo.LoadTargets(r.Context(), u.ID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

func (h *Handler) putTargets(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	var t domain.NutritionTargets
	if err := httpx.DecodeJSON(r, &t); err != nil {
		httpx.Error(w, err)
		return
	}
	if err := h.repo.SaveTargets(r.Context(), u.ID, t); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

func (h *Handler) autoTargets(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	var req struct {
		Goal          string `json:"goal"`
		ActivityLevel string `json:"activity_level"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if req.Goal == "" {
		req.Goal = "maintain"
	}
	if req.ActivityLevel == "" {
		req.ActivityLevel = "light"
	}
	authUser, err := h.getUser(r.Context(), u.ID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	t := ComputeTargets(authUser, req.Goal, req.ActivityLevel)
	if t.Calories == 0 {
		httpx.Error(w, &domain.ValidationError{
			Message: "set your height, weight, birth date and sex on Settings first",
		})
		return
	}
	if err := h.repo.SaveTargets(r.Context(), u.ID, t); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("nutrition: rand.Read: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

