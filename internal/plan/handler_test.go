package plan

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/ataliaferro46/go-workout-api/internal/exercise"
)

func newPlanServer() *http.ServeMux {
	mux := http.NewServeMux()
	NewHandler(exercise.Library()).Routes(mux)
	return mux
}

func TestGenerateEndpoint_OK(t *testing.T) {
	mux := newPlanServer()
	body, _ := json.Marshal(domain.GenerateRequest{
		Goal:               domain.GoalMuscleGain,
		Experience:         domain.Intermediate,
		DaysPerWeek:        3,
		AvailableEquipment: []domain.Equipment{domain.Dumbbell, domain.Bench},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/plans/generate?seed=1", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var p domain.WorkoutPlan
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(p.Days) != 3 {
		t.Errorf("got %d days, want 3", len(p.Days))
	}
}

func TestGenerateEndpoint_ValidationError(t *testing.T) {
	mux := newPlanServer()
	body := []byte(`{"goal":"swimming","experience":"beginner","days_per_week":3}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/plans/generate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGenerateEndpoint_UnknownFieldRejected(t *testing.T) {
	mux := newPlanServer()
	body := []byte(`{"goal":"strength","experience":"beginner","days_per_week":3,"bogus":1}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/plans/generate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (unknown fields must be rejected)", rec.Code)
	}
}

func TestGenerateEndpoint_SeedReproducible(t *testing.T) {
	mux := newPlanServer()
	body, _ := json.Marshal(domain.GenerateRequest{
		Goal:               domain.GoalStrength,
		Experience:         domain.Advanced,
		DaysPerWeek:        4,
		AvailableEquipment: []domain.Equipment{domain.Barbell, domain.Dumbbell, domain.Bench, domain.PullupBar},
	})
	do := func() string {
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/generate?seed=99", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Body.String()
	}
	if do() != do() {
		t.Error("same seed produced different responses")
	}
}
