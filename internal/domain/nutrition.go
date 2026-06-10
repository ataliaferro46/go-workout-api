package domain

import "time"

// Food is a row in the food library with per-serving macros.
type Food struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Brand       string  `json:"brand,omitempty"`
	ServingSize float64 `json:"serving_size"`
	ServingUnit string  `json:"serving_unit"`
	Calories    int     `json:"calories"`
	ProteinG    float64 `json:"protein_g"`
	CarbsG      float64 `json:"carbs_g"`
	FatG        float64 `json:"fat_g"`
	FiberG      float64 `json:"fiber_g,omitempty"`
	SugarG      float64 `json:"sugar_g,omitempty"`
	Source      string  `json:"source,omitempty"`
}

// FoodLog records one consumption event. Food is populated on read by
// joining against the foods table so callers get macros without a
// second round-trip.
type FoodLog struct {
	ID       string    `json:"id"`
	UserID   string    `json:"user_id"`
	FoodID   string    `json:"food_id"`
	Food     *Food     `json:"food,omitempty"`
	Servings float64   `json:"servings"`
	MealType string    `json:"meal_type"` // "breakfast" | "lunch" | "dinner" | "snack"
	LoggedAt time.Time `json:"logged_at"`
	Notes    string    `json:"notes,omitempty"`
}

// NutritionTargets is the daily calorie + macro target derived from
// the user's profile + goal (or set manually).
type NutritionTargets struct {
	Calories      int     `json:"calories"`
	ProteinG      int     `json:"protein_g"`
	CarbsG        int     `json:"carbs_g"`
	FatG          int     `json:"fat_g"`
	Goal          string  `json:"goal,omitempty"`           // "maintain" | "lose" | "gain" | "recomp"
	ActivityLevel string  `json:"activity_level,omitempty"` // "sedentary" | "light" | "moderate" | "very" | "extreme"
	BMR           int     `json:"bmr,omitempty"`            // computed; informational
	TDEE          int     `json:"tdee,omitempty"`           // computed; informational
}

// NutritionTotals aggregates a day's consumption.
type NutritionTotals struct {
	Calories int     `json:"calories"`
	ProteinG float64 `json:"protein_g"`
	CarbsG   float64 `json:"carbs_g"`
	FatG     float64 `json:"fat_g"`
}

// NutritionDaily is the full daily view: targets, what was eaten, what
// was burned via workouts (cardio sessions today), net calorie balance.
type NutritionDaily struct {
	Date          time.Time          `json:"date"`
	Logs          []FoodLog          `json:"logs"`
	Totals        NutritionTotals    `json:"totals"`
	Targets       NutritionTargets   `json:"targets"`
	CaloriesBurned int               `json:"calories_burned"`
	NetCalories   int                `json:"net_calories"` // consumed - burned
}

// MealPlan is a persisted meal plan (single-day or multi-day) the user
// can re-open. Items are stored separately and joined on Get.
type MealPlan struct {
	ID        string         `json:"id"`
	UserID    string         `json:"user_id"`
	Name      string         `json:"name"`
	Days      int            `json:"days"`
	Targets   NutritionTargets `json:"targets,omitempty"`
	Items     []MealPlanItem `json:"items,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// MealPlanItem is one food in one meal slot on one day of a plan.
type MealPlanItem struct {
	DayIdx    int     `json:"day_idx"`    // 1-based
	MealType  string  `json:"meal_type"`
	SortOrder int     `json:"sort_order"`
	FoodID    string  `json:"food_id"`
	Food      *Food   `json:"food,omitempty"`
	Servings  float64 `json:"servings"`
}

// Recipe is a user-created composite food. The recipe row tracks
// ingredients and notes; on save we synthesize a foods-table row so the
// recipe shows up in search and can be logged like any other food.
type Recipe struct {
	ID                string             `json:"id"`
	UserID            string             `json:"user_id"`
	Name              string             `json:"name"`
	Notes             string             `json:"notes,omitempty"`
	ServingsPerRecipe float64            `json:"servings_per_recipe"`
	Ingredients       []RecipeIngredient `json:"ingredients"`
	Totals            NutritionTotals    `json:"totals,omitempty"` // per single recipe serving
	CreatedAt         time.Time          `json:"created_at"`
}

// RecipeIngredient is one food in a recipe at a particular serving count.
type RecipeIngredient struct {
	SortOrder int     `json:"sort_order"`
	FoodID    string  `json:"food_id"`
	Food      *Food   `json:"food,omitempty"`
	Servings  float64 `json:"servings"`
}
