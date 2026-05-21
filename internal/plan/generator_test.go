package plan

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/ataliaferro46/go-workout-api/internal/exercise"
)

func fullGymRequest() domain.GenerateRequest {
	return domain.GenerateRequest{
		Goal:        domain.GoalMuscleGain,
		Experience:  domain.Intermediate,
		DaysPerWeek: 4,
		AvailableEquipment: []domain.Equipment{
			domain.Barbell, domain.Dumbbell, domain.Cable, domain.Machine,
			domain.PullupBar, domain.Bench,
		},
	}
}

func newGen() *Generator { return NewGenerator(exercise.Library(), 42) }

func TestGenerate_Validation(t *testing.T) {
	cases := map[string]domain.GenerateRequest{
		"bad goal":       {Goal: "swimming", Experience: domain.Beginner, DaysPerWeek: 3},
		"bad experience": {Goal: domain.GoalStrength, Experience: "pro", DaysPerWeek: 3},
		"too few days":   {Goal: domain.GoalStrength, Experience: domain.Beginner, DaysPerWeek: 1},
		"too many days":  {Goal: domain.GoalStrength, Experience: domain.Beginner, DaysPerWeek: 7},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := newGen().Generate(req)
			var ve *domain.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *domain.ValidationError, got %v", err)
			}
		})
	}
}

func TestGenerate_SplitByDays(t *testing.T) {
	cases := []struct {
		days      int
		wantSplit string
	}{
		{2, "Full Body"},
		{3, "Full Body"},
		{4, "Upper / Lower"},
		{6, "Push / Pull / Legs (x2)"},
	}
	for _, tc := range cases {
		req := fullGymRequest()
		req.DaysPerWeek = tc.days
		p, err := newGen().Generate(req)
		if err != nil {
			t.Fatalf("days=%d: %v", tc.days, err)
		}
		if len(p.Days) != tc.days {
			t.Errorf("days=%d: got %d days, want %d", tc.days, len(p.Days), tc.days)
		}
		if p.Split != tc.wantSplit {
			t.Errorf("days=%d: split = %q, want %q", tc.days, p.Split, tc.wantSplit)
		}
	}
}

func TestGenerate_RespectsEquipment(t *testing.T) {
	req := domain.GenerateRequest{
		Goal:               domain.GoalMuscleGain,
		Experience:         domain.Intermediate,
		DaysPerWeek:        3,
		AvailableEquipment: []domain.Equipment{domain.Dumbbell, domain.Bench},
	}
	available := req.EquipmentSet()
	p, err := newGen().Generate(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, day := range p.Days {
		for _, pe := range day.Exercises {
			if !pe.Exercise.RequiresOnly(available) {
				t.Errorf("%q requires unavailable equipment %v", pe.Exercise.Name, pe.Exercise.RequiredEquipment)
			}
		}
	}
}

func TestGenerate_ExcludesInjuries(t *testing.T) {
	req := fullGymRequest()
	req.Injuries = []domain.BodyPart{domain.Knee, domain.LowerBack}
	injuries := req.InjurySet()
	p, err := newGen().Generate(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, day := range p.Days {
		for _, pe := range day.Exercises {
			if pe.Exercise.ConflictsWith(injuries) {
				t.Errorf("%q is contraindicated for the user's injuries", pe.Exercise.Name)
			}
		}
	}
}

func TestGenerate_BeginnerExcludesAdvancedMovements(t *testing.T) {
	req := fullGymRequest()
	req.Experience = domain.Beginner
	p, err := newGen().Generate(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, day := range p.Days {
		for _, pe := range day.Exercises {
			if !domain.Beginner.CanPerform(pe.Exercise.MinLevel) {
				t.Errorf("beginner plan includes %q (min level %q)", pe.Exercise.Name, pe.Exercise.MinLevel)
			}
		}
	}
}

func TestGenerate_DeterministicForSameSeed(t *testing.T) {
	req := fullGymRequest()
	a, err := NewGenerator(exercise.Library(), 7).Generate(req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewGenerator(exercise.Library(), 7).Generate(req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Error("same seed and request produced different plans (generation is not deterministic)")
	}
}

func TestGenerate_DaysArePopulatedAndOrdered(t *testing.T) {
	p, err := newGen().Generate(fullGymRequest())
	if err != nil {
		t.Fatal(err)
	}
	for _, day := range p.Days {
		if len(day.Exercises) == 0 {
			t.Errorf("%s day has no exercises", day.Name)
		}
		for i, pe := range day.Exercises {
			if pe.Order != i+1 {
				t.Errorf("%s day: exercise index %d has order %d", day.Name, i, pe.Order)
			}
			if pe.Sets < 2 {
				t.Errorf("%s day: %q prescribed %d sets", day.Name, pe.Exercise.Name, pe.Sets)
			}
			if pe.RepsLow <= 0 || pe.RepsHigh < pe.RepsLow {
				t.Errorf("%s day: %q has invalid rep range %d-%d", day.Name, pe.Exercise.Name, pe.RepsLow, pe.RepsHigh)
			}
		}
	}
}

func TestGenerate_BodyweightOnlyStillProducesPlan(t *testing.T) {
	req := domain.GenerateRequest{
		Goal:        domain.GoalGeneralFitness,
		Experience:  domain.Beginner,
		DaysPerWeek: 3,
		// no equipment: bodyweight only
	}
	p, err := newGen().Generate(req)
	if err != nil {
		t.Fatalf("bodyweight-only should still generate a plan: %v", err)
	}
	if len(p.Days) != 3 {
		t.Fatalf("got %d days, want 3", len(p.Days))
	}
	total := 0
	for _, d := range p.Days {
		total += len(d.Exercises)
	}
	if total == 0 {
		t.Error("bodyweight-only plan has no exercises at all")
	}
}

func TestPrescribe_ByGoal(t *testing.T) {
	compound := domain.Exercise{Compound: true}
	cases := []struct {
		goal        domain.Goal
		wantRepsLow int
		minRest     int
	}{
		{domain.GoalStrength, 3, 150},
		{domain.GoalEndurance, 15, 0},
	}
	for _, tc := range cases {
		req := domain.GenerateRequest{Goal: tc.goal, Experience: domain.Intermediate}
		p := prescribe(req, compound)
		if p.repsLow != tc.wantRepsLow {
			t.Errorf("goal=%s: repsLow = %d, want %d", tc.goal, p.repsLow, tc.wantRepsLow)
		}
		if p.restSeconds < tc.minRest {
			t.Errorf("goal=%s: rest = %d, want >= %d", tc.goal, p.restSeconds, tc.minRest)
		}
		if p.sets < 2 {
			t.Errorf("goal=%s: sets = %d, want >= 2", tc.goal, p.sets)
		}
	}
}
