package workout

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// newTestService wires a Service with deterministic IDs and a fixed clock so
// assertions are stable across runs.
func newTestService() *Service {
	fixedTime := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	counter := 0
	seqID := func() string {
		counter++
		return "id-" + strconv.Itoa(counter)
	}
	return NewService(NewInMemoryRepository(), seqID, fixedTime)
}

func TestService_Create(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	got, err := svc.Create(ctx, CreateInput{
		UserID: "user-1",
		Name:   "  Leg Day  ", // leading/trailing space should be trimmed
		Exercises: []domain.Exercise{
			{Name: "Squat", Sets: 5, Reps: 5, WeightKG: 100},
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if got.ID != "id-1" {
		t.Errorf("ID = %q, want %q", got.ID, "id-1")
	}
	if got.Name != "Leg Day" {
		t.Errorf("Name = %q, want %q (should be trimmed)", got.Name, "Leg Day")
	}
	if len(got.Exercises) != 1 {
		t.Fatalf("len(Exercises) = %d, want 1", len(got.Exercises))
	}
	want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !got.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, want)
	}
}

func TestService_Create_DefaultsExercisesToEmptySlice(t *testing.T) {
	svc := newTestService()
	got, err := svc.Create(context.Background(), CreateInput{UserID: "u", Name: "n"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if got.Exercises == nil {
		t.Error("Exercises = nil, want empty non-nil slice")
	}
}

func TestService_Create_Validation(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	cases := []struct {
		name string
		in   CreateInput
	}{
		{"missing user", CreateInput{Name: "x"}},
		{"missing name", CreateInput{UserID: "user-1"}},
		{"blank exercise name", CreateInput{UserID: "u", Name: "n", Exercises: []domain.Exercise{{Name: "   "}}}},
		{"negative reps", CreateInput{UserID: "u", Name: "n", Exercises: []domain.Exercise{{Name: "Squat", Reps: -1}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(ctx, tc.in)
			var ve *domain.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *domain.ValidationError, got %v", err)
			}
		})
	}
}

func TestService_Get_NotFound(t *testing.T) {
	svc := newTestService()
	_, err := svc.Get(context.Background(), "does-not-exist")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound, got %v", err)
	}
}

func TestService_ListByUser(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := svc.Create(ctx, CreateInput{UserID: "user-1", Name: "W"}); err != nil {
			t.Fatalf("seed Create failed: %v", err)
		}
	}
	if _, err := svc.Create(ctx, CreateInput{UserID: "user-2", Name: "W"}); err != nil {
		t.Fatalf("seed Create failed: %v", err)
	}

	got, err := svc.ListByUser(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListByUser returned error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("len = %d, want 3 (other users must be excluded)", len(got))
	}
}

func TestService_ListByUser_RequiresUserID(t *testing.T) {
	svc := newTestService()
	_, err := svc.ListByUser(context.Background(), "")
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *domain.ValidationError, got %v", err)
	}
}

func TestService_Delete(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	created, err := svc.Create(ctx, CreateInput{UserID: "u", Name: "n"})
	if err != nil {
		t.Fatalf("seed Create failed: %v", err)
	}
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := svc.Get(ctx, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	if err := svc.Delete(ctx, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound on double delete, got %v", err)
	}
}
