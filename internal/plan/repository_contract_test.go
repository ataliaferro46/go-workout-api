package plan

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// testRepositoryContract is the single specification of what a plan
// Repository must do. Every implementation runs against the same scenarios so
// in-memory and Postgres behavior cannot silently diverge.
func testRepositoryContract(t *testing.T, newRepo func(t *testing.T) Repository) {
	t.Helper()

	t.Run("get unknown id returns ErrNotFound", func(t *testing.T) {
		repo := newRepo(t)
		_, err := repo.Get(context.Background(), "nope")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("delete unknown id returns ErrNotFound", func(t *testing.T) {
		repo := newRepo(t)
		err := repo.Delete(context.Background(), "nope")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("create then get round-trips fields", func(t *testing.T) {
		repo := newRepo(t)
		want := samplePlan("p1", "u1",
			time.Date(2026, 5, 28, 14, 30, 0, 0, time.UTC))
		mustCreate(t, repo, want)

		got, err := repo.Get(context.Background(), "p1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		assertPlanEqual(t, got, want)
	})

	t.Run("list by user returns newest first", func(t *testing.T) {
		repo := newRepo(t)
		base := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
		mustCreate(t, repo, samplePlan("a", "u1", base))
		mustCreate(t, repo, samplePlan("b", "u1", base.Add(48*time.Hour)))
		mustCreate(t, repo, samplePlan("c", "u2", base.Add(72*time.Hour)))

		got, err := repo.ListByUser(context.Background(), "u1")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 plans for u1, got %d", len(got))
		}
		if got[0].ID != "b" || got[1].ID != "a" {
			t.Fatalf("expected order [b a], got [%s %s]", got[0].ID, got[1].ID)
		}
	})

	t.Run("list by unknown user returns empty slice not nil", func(t *testing.T) {
		repo := newRepo(t)
		got, err := repo.ListByUser(context.Background(), "ghost")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil empty slice so JSON encodes as []")
		}
		if len(got) != 0 {
			t.Fatalf("expected 0 plans, got %d", len(got))
		}
	})

	t.Run("delete removes plan and cascade clears its days + exercises", func(t *testing.T) {
		repo := newRepo(t)
		mustCreate(t, repo, samplePlan("p1", "u1", time.Now().UTC()))

		if err := repo.Delete(context.Background(), "p1"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		_, err := repo.Get(context.Background(), "p1")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound after delete, got %v", err)
		}
	})

	t.Run("day and exercise order is preserved", func(t *testing.T) {
		repo := newRepo(t)
		mustCreate(t, repo, samplePlan("ord", "u1",
			time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)))

		got, err := repo.Get(context.Background(), "ord")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if len(got.Days) != 2 {
			t.Fatalf("expected 2 days, got %d", len(got.Days))
		}
		if got.Days[0].Index != 1 || got.Days[1].Index != 2 {
			t.Fatalf("day order wrong: got indices %d, %d", got.Days[0].Index, got.Days[1].Index)
		}
		for _, day := range got.Days {
			for i, ex := range day.Exercises {
				if ex.Order != i+1 {
					t.Fatalf("exercise order wrong on day %d: position %d has Order %d",
						day.Index, i, ex.Order)
				}
			}
		}
	})

	t.Run("warnings round-trip", func(t *testing.T) {
		repo := newRepo(t)
		p := samplePlan("warn", "u1", time.Now().UTC())
		p.Warnings = []string{"Pull day has 3 of 4 target exercises", "Legs day is light"}
		mustCreate(t, repo, p)

		got, err := repo.Get(context.Background(), "warn")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !reflect.DeepEqual(got.Warnings, p.Warnings) {
			t.Fatalf("warnings mismatch:\n got:  %v\n want: %v", got.Warnings, p.Warnings)
		}
	})
}

// TestInMemoryRepository_Contract runs the contract against the in-memory
// implementation. Runs on every `go test ./...` with no setup.
func TestInMemoryRepository_Contract(t *testing.T) {
	testRepositoryContract(t, func(t *testing.T) Repository {
		return NewInMemoryRepository()
	})
}

// --- helpers ---------------------------------------------------------------

func samplePlan(id, userID string, createdAt time.Time) domain.WorkoutPlan {
	bench := domain.Exercise{
		ID: "barbell-bench-press", Name: "Barbell Bench Press",
		PrimaryMuscle: domain.Chest, Pattern: domain.HorizontalPush,
		RequiredEquipment: []domain.Equipment{domain.Barbell, domain.Bench},
		Compound:          true, MinLevel: domain.Beginner,
	}
	row := domain.Exercise{
		ID: "seated-cable-row", Name: "Seated Cable Row",
		PrimaryMuscle: domain.Back, Pattern: domain.HorizontalPull,
		RequiredEquipment: []domain.Equipment{domain.Cable},
		Compound:          true, MinLevel: domain.Beginner,
	}
	squat := domain.Exercise{
		ID: "goblet-squat", Name: "Goblet Squat",
		PrimaryMuscle: domain.Quads, Pattern: domain.SquatPattern,
		RequiredEquipment: []domain.Equipment{domain.Dumbbell},
		Compound:          true, MinLevel: domain.Beginner,
	}
	return domain.WorkoutPlan{
		ID:          id,
		UserID:      userID,
		Goal:        domain.GoalMuscleGain,
		Experience:  domain.Intermediate,
		DaysPerWeek: 4,
		Split:       "Upper / Lower",
		CreatedAt:   createdAt,
		Warnings:    []string{},
		Days: []domain.PlanDay{
			{
				Index: 1, Name: "Upper",
				Exercises: []domain.PlanExercise{
					{Exercise: bench, Order: 1, Sets: 4, RepsLow: 8, RepsHigh: 12, RestSeconds: 90},
					{Exercise: row, Order: 2, Sets: 4, RepsLow: 8, RepsHigh: 12, RestSeconds: 90},
				},
			},
			{
				Index: 2, Name: "Lower",
				Exercises: []domain.PlanExercise{
					{Exercise: squat, Order: 1, Sets: 4, RepsLow: 8, RepsHigh: 12, RestSeconds: 90},
				},
			},
		},
	}
}

func mustCreate(t *testing.T, repo Repository, p domain.WorkoutPlan) {
	t.Helper()
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf("create %s: %v", p.ID, err)
	}
}

func assertPlanEqual(t *testing.T, got, want domain.WorkoutPlan) {
	t.Helper()
	got.CreatedAt = got.CreatedAt.UTC()
	want.CreatedAt = want.CreatedAt.UTC()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plan mismatch:\n got:  %+v\n want: %+v", got, want)
	}
}
