package domain

// GenerateRequest is the validated input to the workout generation engine.
type GenerateRequest struct {
	Goal               Goal            `json:"goal"`
	Experience         ExperienceLevel `json:"experience"`
	DaysPerWeek        int             `json:"days_per_week"`
	SessionMinutes     int             `json:"session_minutes"`
	AvailableEquipment []Equipment     `json:"available_equipment"`
	Injuries           []BodyPart      `json:"injuries,omitempty"`
}

// Bounds on request fields, exported so callers and tests can reference them.
const (
	MinDaysPerWeek    = 2
	MaxDaysPerWeek    = 6
	MinSessionMinutes = 20
	MaxSessionMinutes = 120
	DefaultSession    = 60
)

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
	return nil
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
