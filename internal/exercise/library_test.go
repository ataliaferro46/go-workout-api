package exercise

import (
	"testing"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

func TestLibrary_ReturnsCopy(t *testing.T) {
	a := Library()
	if len(a) == 0 {
		t.Fatal("library is empty")
	}
	a[0].Name = "mutated"
	b := Library()
	if b[0].Name == "mutated" {
		t.Error("Library() returned a shared slice; callers can mutate seed data")
	}
}

func TestLibrary_UniqueIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range Library() {
		if e.ID == "" {
			t.Errorf("exercise %q has empty ID", e.Name)
		}
		if seen[e.ID] {
			t.Errorf("duplicate exercise ID: %q", e.ID)
		}
		seen[e.ID] = true
	}
}

func TestLibrary_EveryExerciseIsWellFormed(t *testing.T) {
	for _, e := range Library() {
		if e.Name == "" {
			t.Errorf("%s: empty name", e.ID)
		}
		if e.PrimaryMuscle == "" {
			t.Errorf("%s: empty primary muscle", e.ID)
		}
		if e.Pattern == "" {
			t.Errorf("%s: empty movement pattern", e.ID)
		}
		if len(e.RequiredEquipment) == 0 {
			t.Errorf("%s: no required equipment (use bodyweight if none)", e.ID)
		}
		if !e.MinLevel.Valid() {
			t.Errorf("%s: invalid min level %q", e.ID, e.MinLevel)
		}
	}
}

// A beginner with only bodyweight must still be able to train every major
// movement pattern except pulling (which realistically needs a bar or bands).
func TestLibrary_BodyweightBeginnerCoverage(t *testing.T) {
	available := map[domain.Equipment]bool{domain.Bodyweight: true}
	patterns := map[domain.MovementPattern]bool{}
	for _, e := range Library() {
		if e.RequiresOnly(available) && domain.Beginner.CanPerform(e.MinLevel) {
			patterns[e.Pattern] = true
		}
	}
	for _, want := range []domain.MovementPattern{
		domain.HorizontalPush, domain.SquatPattern, domain.HingePattern,
		domain.LungePattern, domain.CorePattern,
	} {
		if !patterns[want] {
			t.Errorf("no bodyweight beginner exercise for pattern %q", want)
		}
	}
}

// A fully equipped intermediate should have at least one option for every
// movement pattern.
func TestLibrary_FullGymCoversAllPatterns(t *testing.T) {
	available := map[domain.Equipment]bool{
		domain.Bodyweight: true, domain.Barbell: true, domain.Dumbbell: true,
		domain.Cable: true, domain.Machine: true, domain.PullupBar: true, domain.Bench: true,
	}
	patterns := map[domain.MovementPattern]bool{}
	for _, e := range Library() {
		if e.RequiresOnly(available) && domain.Intermediate.CanPerform(e.MinLevel) {
			patterns[e.Pattern] = true
		}
	}
	for _, want := range []domain.MovementPattern{
		domain.HorizontalPush, domain.VerticalPush, domain.HorizontalPull,
		domain.VerticalPull, domain.SquatPattern, domain.HingePattern,
		domain.LungePattern, domain.CorePattern, domain.Isolation,
	} {
		if !patterns[want] {
			t.Errorf("full gym missing pattern %q", want)
		}
	}
}
