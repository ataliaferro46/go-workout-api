package domain

import "time"

// WorkoutPlan is the engine's output: a full training week, plus persistence
// metadata. ID / UserID / CreatedAt are populated by the plan.Service when a
// plan is persisted; the demo binary leaves them as zero values, which is why
// they are tagged omitempty so the wire format stays clean in both cases.
type WorkoutPlan struct {
	ID          string          `json:"id,omitempty"`
	UserID      string          `json:"user_id,omitempty"`
	Goal        Goal            `json:"goal"`
	Experience  ExperienceLevel `json:"experience"`
	DaysPerWeek int             `json:"days_per_week"`
	Split       string          `json:"split"`
	Days        []PlanDay       `json:"days"`
	Warnings    []string        `json:"warnings,omitempty"`
	CreatedAt   time.Time       `json:"created_at,omitempty"`
}

// PlanDay is a single training day within a plan.
type PlanDay struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	// Weekday is the canonical short name ("mon", "tue", ...) the user
	// asked this day to fall on. Empty when the request did not bind days
	// to a calendar.
	Weekday   string         `json:"weekday,omitempty"`
	Exercises []PlanExercise `json:"exercises"`
}

// SetType describes how the user should execute the exercise's working
// sets. The default is SetStandard (straight sets at the prescribed
// reps/rest). The other types are programming variations the engine
// applies opportunistically to add stimulus variety and address
// muscle-specific best practices:
//
//   - SetAMRAP: last working set is "as many reps as possible" with good
//     form, used for hypertrophy and general fitness goals to push
//     proximity-to-failure on the final set.
//   - SetDropSet: at failure on the last set, drop the weight ~30% and
//     continue without rest. Classic small-muscle isolation finisher
//     (lateral raise, pushdown, calf raise, fly).
//   - SetTwentyOnes: 7 lower-half reps + 7 upper-half + 7 full ROM in one
//     extended set. Almost exclusively a biceps tool (the original).
//   - SetSuperset: do this exercise back-to-back with another (see
//     SupersetWith) with no rest between them. Time-efficient and great
//     for antagonist pairing (biceps + triceps, chest + back).
type SetType string

const (
	SetStandard   SetType = "standard"
	SetAMRAP      SetType = "amrap"
	SetDropSet    SetType = "drop_set"
	SetTwentyOnes SetType = "twenty_ones"
	SetSuperset   SetType = "superset"
)

// PlanExercise is a prescribed exercise: the movement plus its dosage.
type PlanExercise struct {
	Exercise    Exercise    `json:"exercise"`
	Order       int         `json:"order"`
	Warmups     []WarmupSet `json:"warmups,omitempty"`
	Sets        int         `json:"sets"`
	RepsLow     int         `json:"reps_low"`
	RepsHigh    int         `json:"reps_high"`
	RestSeconds int         `json:"rest_seconds"`

	// SetType modulates how the working sets are executed. Empty == standard.
	SetType SetType `json:"set_type,omitempty"`

	// SupersetWith, when SetType == SetSuperset, holds the Order of the
	// paired exercise within the same PlanDay. Both exercises in a
	// superset pair carry SetType=SetSuperset and point at each other,
	// so the renderer can draw the connection from either side.
	SupersetWith *int `json:"superset_with,omitempty"`

	// SetTypeNote is a short human description of how to execute the set
	// type ("21s: 7 bottom-half + 7 top-half + 7 full"). Populated when
	// SetType != "" / SetStandard so the front-end doesn't have to know
	// the meaning of every variant.
	SetTypeNote string `json:"set_type_note,omitempty"`
}

// WarmupSet describes a single warmup set as a percentage of the user's
// working weight. We don't know the actual working weight at plan-generation
// time, so the prescription is relative — the front-end can compute the
// concrete weight once the user enters their working load (and the future
// progress-tracking feature will plug actual numbers in here).
type WarmupSet struct {
	Reps             int     `json:"reps"`
	PercentOfWorking float64 `json:"percent_of_working"` // 0.0–1.0
}
