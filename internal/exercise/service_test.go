package exercise

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

func TestService_SnapshotBeforeLoadIsNil(t *testing.T) {
	svc := NewService(NewInMemoryRepository(nil))
	if snap := svc.Snapshot(); snap != nil {
		t.Errorf("expected nil snapshot before LoadSnapshot, got %v", snap)
	}
}

func TestService_LoadSnapshotCachesList(t *testing.T) {
	repo := NewInMemoryRepository([]domain.Exercise{
		sampleExercise("a", "A"),
		sampleExercise("b", "B"),
	})
	svc := NewService(repo)
	if err := svc.LoadSnapshot(context.Background()); err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	snap := svc.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 in snapshot, got %d", len(snap))
	}
}

func TestService_CreateRefreshesSnapshot(t *testing.T) {
	svc := NewService(NewInMemoryRepository(nil))
	if err := svc.LoadSnapshot(context.Background()); err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if len(svc.Snapshot()) != 0 {
		t.Fatalf("expected empty snapshot, got %d", len(svc.Snapshot()))
	}

	if _, err := svc.Create(context.Background(), sampleExercise("new", "Brand New")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	snap := svc.Snapshot()
	if len(snap) != 1 || snap[0].ID != "new" {
		t.Fatalf("expected snapshot to contain newly created exercise, got %v", snap)
	}
}

func TestService_UpdateRefreshesSnapshot(t *testing.T) {
	repo := NewInMemoryRepository([]domain.Exercise{sampleExercise("e", "Original")})
	svc := NewService(repo)
	if err := svc.LoadSnapshot(context.Background()); err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}

	updated := sampleExercise("e", "Updated Name")
	if _, err := svc.Update(context.Background(), updated); err != nil {
		t.Fatalf("Update: %v", err)
	}
	snap := svc.Snapshot()
	if snap[0].Name != "Updated Name" {
		t.Errorf("snapshot stale after update, got %q", snap[0].Name)
	}
}

func TestService_DeleteRefreshesSnapshot(t *testing.T) {
	repo := NewInMemoryRepository([]domain.Exercise{sampleExercise("doomed", "Gone Soon")})
	svc := NewService(repo)
	if err := svc.LoadSnapshot(context.Background()); err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if err := svc.Delete(context.Background(), "doomed"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(svc.Snapshot()) != 0 {
		t.Errorf("expected empty snapshot after delete, got %d", len(svc.Snapshot()))
	}
}

// TestService_ConcurrentSnapshotReadsAreSafe exercises the atomic.Pointer
// cache under simultaneous read pressure and snapshot writes. Run under
// `go test -race` — the test relies on the race detector to surface any
// concurrent map access that the atomic swap protects against.
func TestService_ConcurrentSnapshotReadsAreSafe(t *testing.T) {
	repo := NewInMemoryRepository([]domain.Exercise{sampleExercise("a", "A")})
	svc := NewService(repo)
	if err := svc.LoadSnapshot(context.Background()); err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Many readers continuously walking the snapshot.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					for _, e := range svc.Snapshot() {
						_ = e.ID
					}
				}
			}
		}()
	}

	// One writer doing periodic snapshot refreshes.
	for i := 0; i < 200; i++ {
		if err := svc.LoadSnapshot(context.Background()); err != nil {
			t.Fatalf("LoadSnapshot: %v", err)
		}
	}
	close(stop)
	wg.Wait()
}

func TestService_ValidationRejectsBadInput(t *testing.T) {
	svc := NewService(NewInMemoryRepository(nil))
	bad := domain.Exercise{ID: "", Name: ""}
	_, err := svc.Create(context.Background(), bad)
	if err == nil {
		t.Fatal("expected validation error for empty fields")
	}
	var vErr *domain.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("expected *domain.ValidationError, got %T (%v)", err, err)
	}
}
