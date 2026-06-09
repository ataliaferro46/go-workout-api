package domain

// GenerateRequest is the validated input to the workout generation engine.
type GenerateRequest struct {
	Goal               Goal            `json:"goal"`
	Experience         ExperienceLevel `json:"experience"`
	DaysPerWeek        int             `json:"days_per_week"`
	SessionMinutes     int             `json:"session_minutes"`
	AvailableEquipment []Equipment     `json:"available_equipment"`
	Injuries           []BodyPart      `json:"injuries,omitempty"`

	// Split, if set, overrides the auto-pick driven by DaysPerWeek. Allowed
	// values: "auto" (default), "full_body", "upper_lower", "ppl",
	// "ppl_upper_lower". An auto value lets the engine choose based on
	// DaysPerWeek as it always has.
	Split string `json:"split,omitempty"`

	// Weekdays, if set, binds the generated plan days to specific calendar
	// days. Length must equal the chosen split's day count. Allowed values:
	// "mon" "tue" "wed" "thu" "fri" "sat" "sun" — order matters and defines
	// the cadence. If omitted, the plan days remain calendar-agnostic.
	Weekdays []string `json:"weekdays,omitempty"`

	// MinRecoveryDays is the minimum number of days the user wants between
	// sessions hitting the same muscle group. Default 2. The engine surfaces
	// a warning (does not refuse to generate) when the chosen
	// split + weekdays combination doesn't meet this.
	MinRecoveryDays *int `json:"min_recovery_days,omitempty"`

	// SetsOverride, when set, replaces the engine's default working-set
	// count for every prescribed exercise. Allows "I just want 2 hard sets
	// per movement" or "I'm running 1×AMRAP today" workflows. Must be 1..5.
	// When nil, prescription falls back to the goal/experience defaults.
	SetsOverride *int `json:"sets_override,omitempty"`

	// RecoveryHint, when set, biases generation toward less-taxing variants.
	// 1.0 = fully recovered (no adjustment); 0.0 = no recovery (largest
	// adjustment). Populated by plan.Service when a recovery-aware request
	// has a fresh-enough biometric reading; never set directly by clients.
	RecoveryHint *float64 `json:"recovery_hint,omitempty"`
}

// Bounds on request fields, exported so callers and tests can reference them.
const (
	MinDaysPerWeek      = 2
	MaxDaysPerWeek      = 6
	MinSessionMinutes   = 20
	MaxSessionMinutes   = 120
	DefaultSession      = 60
	DefaultMinRecovery  = 2
	MaxMinRecovery      = 4
)

// ValidWeekdays is the canonical short-name set used in GenerateRequest.Weekdays.
// Exported so the handler can echo the same vocabulary to validation errors.
var ValidWeekdays = map[string]int{
	"mon": 0, "tue": 1, "wed": 2, "thu": 3, "fri": 4, "sat": 5, "sun": 6,
}

// ValidSplits enumerates the allowed values of GenerateRequest.Split.
var ValidSplits = map[string]bool{
	"":                true, // omitted == auto
	"auto":            true,
	"full_body":       true,
	"upper_lower":     true,
	"ppl":             true,
	"ppl_upper_lower": true,
}

// Validate checks the request and returns a *ValidationError describing the
// first problem found, or nil if the request is well-formed.
func (r GenerateRequest) Validate() error {
	if !r.Goal.Valid() {
		return &ValidationError{Message: "goal is missing or invalid"}
	}
	if !r.Experience.Valid() {
		return &ValidationError{Message: "experience is missing or invalid"}
	}
	if r.DaysPerWeek < MinDaysPerWeek || r.DaysPerWeek > MaxDaysPerWeek {
		return &ValidationError{Message: "days_per_week must be between 2 and 6"}
	}
	if r.SessionMinutes != 0 && (r.SessionMinutes < MinSessionMinutes || r.SessionMinutes > MaxSessionMinutes) {
		return &ValidationError{Message: "session_minutes, if set, must be between 20 and 120"}
	}
	for _, eq := range r.AvailableEquipment {
		if !validEquipment[eq] {
			return &ValidationError{Message: "unknown equipment: " + string(eq)}
		}
	}
	for _, inj := range r.Injuries {
		if !validBodyPart[inj] {
			return &ValidationError{Message: "unknown injury body part: " + string(inj)}
		}
	}
	if !ValidSplits[r.Split] {
		return &ValidationError{Message: "unknown split: " + r.Split}
	}
	seenDay := make(map[string]bool, len(r.Weekdays))
	for _, d := range r.Weekdays {
		if _, ok := ValidWeekdays[d]; !ok {
			return &ValidationError{Message: "unknown weekday: " + d + " (use mon|tue|wed|thu|fri|sat|sun)"}
		}
		if seenDay[d] {
			return &ValidationError{Message: "duplicate weekday: " + d}
		}
		seenDay[d] = true
	}
	if len(r.Weekdays) > 0 && len(r.Weekdays) != r.DaysPerWeek {
		return &ValidationError{Message: "weekdays length must equal days_per_week"}
	}
	if r.MinRecoveryDays != nil && (*r.MinRecoveryDays < 0 || *r.MinRecoveryDays > MaxMinRecovery) {
		return &ValidationError{Message: "min_recovery_days, if set, must be between 0 and 4"}
	}
	if r.SetsOverride != nil && (*r.SetsOverride < 1 || *r.SetsOverride > 5) {
		return &ValidationError{Message: "sets_override, if set, must be between 1 and 5"}
	}
	return nil
}

// MinRecoveryDaysOrDefault returns the requested minimum recovery window or
// the package default (2 days).
func (r GenerateRequest) MinRecoveryDaysOrDefault() int {
	if r.MinRecoveryDays == nil {
		return DefaultMinRecovery
	}
	return *r.MinRecoveryDays
}

// EquipmentSet returns the available equipment as a set, always including
// Bodyweight (which needs no gear).
func (r GenerateRequest) EquipmentSet() map[Equipment]bool {
	set := map[Equipment]bool{Bodyweight: true}
	for _, eq := range r.AvailableEquipment {
		set[eq] = true
	}
	return set
}

// InjurySet returns the user's active injuries as a set for fast lookup.
func (r GenerateRequest) InjurySet() map[BodyPart]bool {
	set := make(map[BodyPart]bool, len(r.Injuries))
	for _, inj := range r.Injuries {
		set[inj] = true
	}
	return set
}

// SessionMinutesOrDefault returns the requested session length or the default.
func (r GenerateRequest) SessionMinutesOrDefault() int {
	if r.SessionMinutes == 0 {
		return DefaultSession
	}
	return r.SessionMinutes
}

var validEquipment = map[Equipment]bool{
	Barbell: true, Dumbbell: true, Cable: true, Machine: true,
	Kettlebell: true, Bands: true, PullupBar: true, Bench: true, Bodyweight: true,
}

var validBodyPart = map[BodyPart]bool{
	LowerBack: true, Knee: true, Shoulder: true, Elbow: true,
	Wrist: true, Hip: true, Ankle: true, Neck: true,
}
