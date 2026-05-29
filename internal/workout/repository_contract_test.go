package workout

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// testRepositoryContract is the single specification of what a Repository
// implementation must do. Every implementation (InMemoryRepository,
// PostgresRepository, any future one) runs against the same scenarios — when
// the InMemory tests pass and the Postgres tests pass, behavior is consistent.
//
// This is the "contract test" pattern: catching divergence between
// implementations is impossible if each has its own bespoke tests.
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
		want := sampleWorkout("w1", "u1", "Leg Day",
			time.Date(2026, 5, 28, 14, 30, 0, 0, time.UTC))
		mustCreate(t, repo, want)

		got, err := repo.Get(context.Background(), "w1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		assertWorkoutEqual(t, got, want)
	})

	t.Run("list by user returns newest first", func(t *testing.T) {
		repo := newRepo(t)
		base := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
		mustCreate(t, repo, sampleWorkout("a", "u1", "old", base))
		mustCreate(t, repo, sampleWorkout("b", "u1", "new", base.Add(48*time.Hour)))
		mustCreate(t, repo, sampleWorkout("c", "u2", "other user", base.Add(72*time.Hour)))

		got, err := repo.ListByUser(context.Background(), "u1")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 workouts for u1, got %d", len(got))
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
			t.Fatalf("expected 0 workouts, got %d", len(got))
		}
	})

	t.Run("workout with no exercises round-trips", func(t *testing.T) {
		repo := newRepo(t)
		w := domain.Workout{
			ID:        "empty",
			UserID:    "u1",
			Name:      "Rest day check-in",
			CreatedAt: time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC),
			Exercises: []domain.LoggedExercise{},
		}
		mustCreate(t, repo, w)

		got, err := repo.Get(context.Background(), "empty")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Exercises == nil {
			t.Fatal("expected non-nil empty exercises slice")
		}
		if len(got.Exercises) != 0 {
			t.Fatalf("expected 0 exercises, got %d", len(got.Exercises))
		}
	})

	t.Run("delete removes workout and its exercises", func(t *testing.T) {
		repo := newRepo(t)
		mustCreate(t, repo, sampleWorkout("w1", "u1", "Leg Day", time.Now().UTC()))

		if err := repo.Delete(context.Background(), "w1"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		_, err := repo.Get(context.Background(), "w1")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound after delete, got %v", err)
		}
	})

	t.Run("exercise order is preserved", func(t *testing.T) {
		repo := newRepo(t)
		w := domain.Workout{
			ID:        "order",
			UserID:    "u1",
			Name:      "ordered",
			CreatedAt: time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC),
			Exercises: []domain.LoggedExercise{
				{Name: "first", Sets: 1, Reps: 1, WeightKG: 1},
				{Name: "second", Sets: 2, Reps: 2, WeightKG: 2},
				{Name: "third", Sets: 3, Reps: 3, WeightKG: 3},
			},
		}
		mustCreate(t, repo, w)

		got, err := repo.Get(context.Background(), "order")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		for i, ex := range got.Exercises {
			if ex.Name != w.Exercises[i].Name {
				t.Fatalf("exercise %d: expected %q, got %q",
					i, w.Exercises[i].Name, ex.Name)
			}
		}
	})
}

// TestInMemoryRepository_Contract runs the shared contract against the
// in-memory implementation. It runs on every `go test ./...` invocation with
// no setup required.
func TestInMemoryRepository_Contract(t *testing.T) {
	testRepositoryContract(t, func(t *testing.T) Repository {
		return NewInMemoryRepository()
	})
}

// --- helpers ---------------------------------------------------------------

func sampleWorkout(id, userID, name string, createdAt time.Time) domain.Workout {
	return domain.Workout{
		ID:        id,
		UserID:    userID,
		Name:      name,
		Notes:     "felt good",
		CreatedAt: createdAt,
		Exercises: []domain.LoggedExercise{
			{Name: "Back Squat", Sets: 5, Reps: 5, WeightKG: 120},
			{Name: "Romanian Deadlift", Sets: 3, Reps: 10, WeightKG: 90},
		},
	}
}

func mustCreate(t *testing.T, repo Repository, w domain.Workout) {
	t.Helper()
	if err := repo.Create(context.Background(), w); err != nil {
		t.Fatalf("create %s: %v", w.ID, err)
	}
}

func assertWorkoutEqual(t *testing.T, got, want domain.Workout) {
	t.Helper()
	// Postgres returns TIMESTAMPTZ as UTC; the in-memory repo preserves the
	// original location. Normalize both before comparing.
	got.CreatedAt = got.CreatedAt.UTC()
	want.CreatedAt = want.CreatedAt.UTC()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("workout mismatch:\n got:  %+v\n want: %+v", got, want)
	}
}
