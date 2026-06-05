package exercise

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// testRepositoryContract is the single behavioral specification for an
// exercise Repository. Both InMemoryRepository and (under build tag
// integration) PostgresRepository run against the same scenarios — when both
// suites pass, behavior cannot have silently drifted between
// implementations.
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

	t.Run("update unknown id returns ErrNotFound", func(t *testing.T) {
		repo := newRepo(t)
		err := repo.Update(context.Background(), sampleExercise("missing", "Missing"))
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("create then get round-trips all fields", func(t *testing.T) {
		repo := newRepo(t)
		want := sampleExercise("squat-1", "Sample Back Squat")
		mustCreate(t, repo, want)

		got, err := repo.Get(context.Background(), "squat-1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		assertExerciseEqual(t, got, want)
	})

	t.Run("create duplicate name returns ErrDuplicateName (and ErrConflict)", func(t *testing.T) {
		repo := newRepo(t)
		mustCreate(t, repo, sampleExercise("a", "Back Squat"))
		err := repo.Create(context.Background(), sampleExercise("b", "Back Squat"))
		if !errors.Is(err, ErrDuplicateName) {
			t.Fatalf("expected ErrDuplicateName, got %v", err)
		}
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected ErrDuplicateName to wrap domain.ErrConflict, got %v", err)
		}
	})

	t.Run("list returns deterministic id-ascending order", func(t *testing.T) {
		repo := newRepo(t)
		mustCreate(t, repo, sampleExercise("c", "C movement"))
		mustCreate(t, repo, sampleExercise("a", "A movement"))
		mustCreate(t, repo, sampleExercise("b", "B movement"))

		got, err := repo.List(context.Background())
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("expected 3, got %d", len(got))
		}
		if got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "c" {
			t.Fatalf("expected [a b c], got [%s %s %s]", got[0].ID, got[1].ID, got[2].ID)
		}
	})

	t.Run("list empty returns empty slice not nil", func(t *testing.T) {
		repo := newRepo(t)
		got, err := repo.List(context.Background())
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil empty slice so JSON encodes as []")
		}
	})

	t.Run("list by pattern filters", func(t *testing.T) {
		repo := newRepo(t)
		squat := sampleExercise("s1", "Squat 1")
		squat.Pattern = domain.SquatPattern
		hinge := sampleExercise("h1", "Hinge 1")
		hinge.Pattern = domain.HingePattern
		mustCreate(t, repo, squat)
		mustCreate(t, repo, hinge)

		got, err := repo.ListByPattern(context.Background(), domain.SquatPattern)
		if err != nil {
			t.Fatalf("list by pattern: %v", err)
		}
		if len(got) != 1 || got[0].ID != "s1" {
			t.Fatalf("expected only [s1], got %d entries", len(got))
		}
	})

	t.Run("update changes the named fields", func(t *testing.T) {
		repo := newRepo(t)
		orig := sampleExercise("e1", "Original Name")
		mustCreate(t, repo, orig)

		updated := orig
		updated.Name = "Updated Name"
		updated.Compound = !orig.Compound
		if err := repo.Update(context.Background(), updated); err != nil {
			t.Fatalf("update: %v", err)
		}

		got, err := repo.Get(context.Background(), "e1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Name != "Updated Name" {
			t.Errorf("Name = %q, want Updated Name", got.Name)
		}
		if got.Compound != updated.Compound {
			t.Errorf("Compound = %v, want %v", got.Compound, updated.Compound)
		}
	})

	t.Run("update to colliding name returns ErrDuplicateName", func(t *testing.T) {
		repo := newRepo(t)
		mustCreate(t, repo, sampleExercise("a", "Original A"))
		mustCreate(t, repo, sampleExercise("b", "Original B"))

		updated := sampleExercise("b", "Original A") // rename b to a's name
		err := repo.Update(context.Background(), updated)
		if !errors.Is(err, ErrDuplicateName) {
			t.Fatalf("expected ErrDuplicateName, got %v", err)
		}
	})

	t.Run("delete removes only the target", func(t *testing.T) {
		repo := newRepo(t)
		mustCreate(t, repo, sampleExercise("a", "Keep A"))
		mustCreate(t, repo, sampleExercise("b", "Delete B"))

		if err := repo.Delete(context.Background(), "b"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := repo.Get(context.Background(), "a"); err != nil {
			t.Fatalf("a should still exist: %v", err)
		}
		if _, err := repo.Get(context.Background(), "b"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected b ErrNotFound, got %v", err)
		}
	})

	t.Run("create with empty optional slices round-trips", func(t *testing.T) {
		repo := newRepo(t)
		minimal := domain.Exercise{
			ID:                "min",
			Name:              "Minimal",
			PrimaryMuscle:     domain.Chest,
			Pattern:           domain.Isolation,
			RequiredEquipment: []domain.Equipment{domain.Bodyweight},
			Compound:          false,
			MinLevel:          domain.Beginner,
			// SecondaryMuscles and Contraindications intentionally nil.
		}
		mustCreate(t, repo, minimal)

		got, err := repo.Get(context.Background(), "min")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		// Both nil-or-empty are acceptable representations of "no
		// secondary muscles" / "no contraindications" — Postgres normalizes
		// to nil on the read path, InMemory preserves what was set. Treat
		// both as equivalent.
		if lenOrZero(got.SecondaryMuscles) != 0 {
			t.Errorf("SecondaryMuscles = %v, want empty", got.SecondaryMuscles)
		}
		if lenOrZero(got.Contraindications) != 0 {
			t.Errorf("Contraindications = %v, want empty", got.Contraindications)
		}
	})
}

// TestInMemoryRepository_Contract runs the contract against the in-memory
// implementation.
func TestInMemoryRepository_Contract(t *testing.T) {
	testRepositoryContract(t, func(t *testing.T) Repository {
		return NewInMemoryRepository(nil) // empty repo per scenario
	})
}

// --- helpers ---------------------------------------------------------------

func sampleExercise(id, name string) domain.Exercise {
	return domain.Exercise{
		ID:               id,
		Name:             name,
		PrimaryMuscle:    domain.Quads,
		SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Hamstrings},
		Pattern:          domain.SquatPattern,
		RequiredEquipment: []domain.Equipment{
			domain.Barbell,
		},
		Compound: true,
		MinLevel: domain.Intermediate,
		Contraindications: []domain.BodyPart{
			domain.Knee, domain.LowerBack,
		},
	}
}

func mustCreate(t *testing.T, repo Repository, e domain.Exercise) {
	t.Helper()
	if err := repo.Create(context.Background(), e); err != nil {
		t.Fatalf("create %s: %v", e.ID, err)
	}
}

// assertExerciseEqual compares two exercises, treating nil and empty slices
// as equivalent (because Postgres round-trips a written empty slice as nil
// and we'd rather make the contract permissive than the postgres helper
// asymmetric).
func assertExerciseEqual(t *testing.T, got, want domain.Exercise) {
	t.Helper()
	got = normalize(got)
	want = normalize(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("exercise mismatch:\n got:  %+v\n want: %+v", got, want)
	}
}

func normalize(e domain.Exercise) domain.Exercise {
	if len(e.SecondaryMuscles) == 0 {
		e.SecondaryMuscles = nil
	}
	if len(e.Contraindications) == 0 {
		e.Contraindications = nil
	}
	return e
}

func lenOrZero[T any](s []T) int { return len(s) }
