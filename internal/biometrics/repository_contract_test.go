package biometrics

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// testReadingRepositoryContract is the single behavioral spec for a
// ReadingRepository. Both InMemory and Postgres run it.
func testReadingRepositoryContract(t *testing.T, newRepo func(t *testing.T) ReadingRepository) {
	t.Helper()

	t.Run("insert + LatestByUserAndKind round-trips", func(t *testing.T) {
		repo := newRepo(t)
		r := makeReading("r1", "u1", "whoop", domain.KindRecovery, 0.72, time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC), "whoop:rec:1")
		if err := repo.Insert(context.Background(), r); err != nil {
			t.Fatalf("insert: %v", err)
		}
		got, err := repo.LatestByUserAndKind(context.Background(), "u1", domain.KindRecovery)
		if err != nil {
			t.Fatalf("latest: %v", err)
		}
		if got.Value != 0.72 {
			t.Errorf("Value = %v, want 0.72", got.Value)
		}
	})

	t.Run("latest unknown kind returns ErrNotFound", func(t *testing.T) {
		repo := newRepo(t)
		_, err := repo.LatestByUserAndKind(context.Background(), "ghost", domain.KindRecovery)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("duplicate insert is idempotent (not an error)", func(t *testing.T) {
		repo := newRepo(t)
		r := makeReading("r1", "u1", "whoop", domain.KindRecovery, 0.5, time.Now().UTC(), "dup-key")
		if err := repo.Insert(context.Background(), r); err != nil {
			t.Fatalf("first insert: %v", err)
		}
		r2 := r
		r2.ID = "r2"   // different id, same (provider, idempotency_key)
		r2.Value = 0.9 // try to overwrite
		if err := repo.Insert(context.Background(), r2); err != nil {
			t.Fatalf("dup insert: %v", err)
		}
		// First write wins because ON CONFLICT DO NOTHING.
		got, err := repo.LatestByUserAndKind(context.Background(), "u1", domain.KindRecovery)
		if err != nil {
			t.Fatalf("latest: %v", err)
		}
		if got.Value != 0.5 {
			t.Errorf("Value = %v, want 0.5 (first write wins)", got.Value)
		}
	})

	t.Run("multiple kinds for one user don't interfere", func(t *testing.T) {
		repo := newRepo(t)
		_ = repo.Insert(context.Background(), makeReading("r1", "u1", "whoop", domain.KindRecovery, 0.7, time.Now().UTC(), "rec-1"))
		_ = repo.Insert(context.Background(), makeReading("r2", "u1", "whoop", domain.KindStrain, 12.3, time.Now().UTC(), "str-1"))

		rec, _ := repo.LatestByUserAndKind(context.Background(), "u1", domain.KindRecovery)
		str, _ := repo.LatestByUserAndKind(context.Background(), "u1", domain.KindStrain)
		if rec.Value != 0.7 || str.Value != 12.3 {
			t.Fatalf("kinds interfered: rec=%v str=%v", rec.Value, str.Value)
		}
	})

	t.Run("latest returns the most recent", func(t *testing.T) {
		repo := newRepo(t)
		base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		_ = repo.Insert(context.Background(), makeReading("r1", "u1", "whoop", domain.KindRecovery, 0.3, base, "rec-1"))
		_ = repo.Insert(context.Background(), makeReading("r2", "u1", "whoop", domain.KindRecovery, 0.6, base.Add(24*time.Hour), "rec-2"))
		_ = repo.Insert(context.Background(), makeReading("r3", "u1", "whoop", domain.KindRecovery, 0.9, base.Add(48*time.Hour), "rec-3"))

		got, _ := repo.LatestByUserAndKind(context.Background(), "u1", domain.KindRecovery)
		if got.Value != 0.9 {
			t.Errorf("Value = %v, want 0.9 (most recent)", got.Value)
		}
	})

	t.Run("cross-user isolation", func(t *testing.T) {
		repo := newRepo(t)
		_ = repo.Insert(context.Background(), makeReading("r1", "u1", "whoop", domain.KindRecovery, 0.7, time.Now().UTC(), "u1-rec"))
		_ = repo.Insert(context.Background(), makeReading("r2", "u2", "whoop", domain.KindRecovery, 0.4, time.Now().UTC(), "u2-rec"))

		u1, _ := repo.LatestByUserAndKind(context.Background(), "u1", domain.KindRecovery)
		u2, _ := repo.LatestByUserAndKind(context.Background(), "u2", domain.KindRecovery)
		if u1.Value != 0.7 || u2.Value != 0.4 {
			t.Fatalf("cross-user leak: u1=%v u2=%v", u1.Value, u2.Value)
		}
	})

	t.Run("list returns newest first and non-nil empty", func(t *testing.T) {
		repo := newRepo(t)
		got, err := repo.ListByUser(context.Background(), "ghost")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil empty slice so JSON encodes as []")
		}

		base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		_ = repo.Insert(context.Background(), makeReading("a", "u1", "whoop", domain.KindRecovery, 0.5, base, "a"))
		_ = repo.Insert(context.Background(), makeReading("b", "u1", "whoop", domain.KindRecovery, 0.6, base.Add(time.Hour), "b"))

		got, _ = repo.ListByUser(context.Background(), "u1")
		if len(got) != 2 || got[0].ID != "b" || got[1].ID != "a" {
			t.Fatalf("expected [b a], got %+v", got)
		}
	})
}

// TestInMemoryReadingRepository_Contract runs the contract against InMemory.
func TestInMemoryReadingRepository_Contract(t *testing.T) {
	testReadingRepositoryContract(t, func(t *testing.T) ReadingRepository {
		return NewInMemoryReadingRepository()
	})
}

// makeReading is a constructor used by the contract test.
func makeReading(id, userID, provider string, kind domain.Kind, value float64, recordedAt time.Time, idem string) domain.Reading {
	raw, _ := json.Marshal(map[string]any{"id": id, "value": value})
	return domain.Reading{
		ID:             id,
		UserID:         userID,
		Provider:       provider,
		Kind:           kind,
		Value:          value,
		RecordedAt:     recordedAt,
		IngestedAt:     time.Now().UTC(),
		IdempotencyKey: idem,
		RawPayload:     raw,
	}
}
