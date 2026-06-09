package workout

import (
	"context"
	"fmt"
	"strings"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// MET (metabolic equivalent of task) values per activity & intensity. The
// numbers come from the Compendium of Physical Activities (Ainsworth et
// al. 2011, the canonical reference used by sports-medicine textbooks).
// We bucket intensity into three categories rather than asking the user
// for speed/cadence because honestly, even an athlete can't accurately
// peg their cycling cadence after the fact.
var metTable = map[string]map[string]float64{
	"walking":  {"light": 2.8, "moderate": 4.3, "vigorous": 5.5},
	"running":  {"light": 7.0, "moderate": 9.8, "vigorous": 11.8},
	"cycling":  {"light": 4.0, "moderate": 8.0, "vigorous": 12.0},
	"rowing":   {"light": 4.8, "moderate": 7.0, "vigorous": 9.5},
	"swimming": {"light": 5.8, "moderate": 7.0, "vigorous": 9.8},
	"hiit":     {"light": 6.0, "moderate": 8.0, "vigorous": 10.0},
	"elliptical": {"light": 5.0, "moderate": 7.0, "vigorous": 9.0},
	"other":    {"light": 4.0, "moderate": 6.0, "vigorous": 8.0},
}

// EstimateCaloriesMET computes calorie burn via the standard formula:
//
//	calories = MET × weight_kg × duration_hours
//
// Returns 0 if we can't estimate (missing weight, unknown activity).
// The caller decides what to display when the value is 0.
func EstimateCaloriesMET(activity, intensity string, durationMinutes int, weightKG float64) int {
	if weightKG <= 0 || durationMinutes <= 0 {
		return 0
	}
	activity = strings.ToLower(activity)
	intensity = strings.ToLower(intensity)
	tbl, ok := metTable[activity]
	if !ok {
		tbl = metTable["other"]
	}
	met, ok := tbl[intensity]
	if !ok {
		met = tbl["moderate"]
	}
	hours := float64(durationMinutes) / 60.0
	return int(met*weightKG*hours + 0.5)
}

// CardioInput is the wire shape for creating a cardio workout.
type CardioInput struct {
	UserID          string
	Name            string
	Notes           string
	Activity        string
	Intensity       string
	DurationMinutes int
	DistanceKM      float64
	UserWeightKG    float64 // injected by caller from auth user profile
}

// CreateCardio persists a cardio workout. Calories are computed via the
// MET formula when we know the user's weight; otherwise the row stores 0
// and the source as "" (the caller may overwrite with an Oura value later).
func (s *Service) CreateCardio(ctx context.Context, in CardioInput) (domain.Workout, error) {
	in.Activity = strings.TrimSpace(strings.ToLower(in.Activity))
	in.Intensity = strings.TrimSpace(strings.ToLower(in.Intensity))
	if in.Activity == "" {
		return domain.Workout{}, &domain.ValidationError{Message: "activity is required"}
	}
	if in.DurationMinutes <= 0 {
		return domain.Workout{}, &domain.ValidationError{Message: "duration_minutes must be > 0"}
	}
	if in.Intensity == "" {
		in.Intensity = "moderate"
	}
	calories := 0
	source := ""
	if in.UserWeightKG > 0 {
		calories = EstimateCaloriesMET(in.Activity, in.Intensity, in.DurationMinutes, in.UserWeightKG)
		source = "met_formula"
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = fmt.Sprintf("%s · %d min", titleCase(in.Activity), in.DurationMinutes)
	}
	w := domain.Workout{
		ID:        s.newID(),
		UserID:    in.UserID,
		Name:      name,
		Notes:     in.Notes,
		Type:      "cardio",
		CreatedAt: s.now().UTC(),
		Exercises: []domain.LoggedExercise{},
		CardioSession: &domain.CardioSession{
			Activity:        in.Activity,
			Intensity:       in.Intensity,
			DurationMinutes: in.DurationMinutes,
			DistanceKM:      in.DistanceKM,
			Calories:        calories,
			CaloriesSource:  source,
			Notes:           in.Notes,
		},
	}
	if cr, ok := s.repo.(CardioWriter); ok {
		if err := cr.CreateCardio(ctx, w); err != nil {
			return domain.Workout{}, err
		}
	} else {
		// In-memory fallback — just store on the workout map.
		if err := s.repo.Create(ctx, w); err != nil {
			return domain.Workout{}, err
		}
	}
	return w, nil
}

// CardioWriter is the optional repo capability the Postgres impl provides
// for atomic two-table cardio inserts.
type CardioWriter interface {
	CreateCardio(ctx context.Context, w domain.Workout) error
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
