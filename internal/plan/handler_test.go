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

// newPlanServer wires a Handler over an in-memory Service for tests. Tests
// don't need persistence semantics — they need the handler to call through
// to the engine — so InMemoryRepository is fine here.
func newPlanServer() *http.ServeMux {
	mux := http.NewServeMux()
	// Static fixture for tests — admin edits don't matter here.
	lib := exercise.Library()
	svc := NewService(func() []domain.Exercise { return lib }, NewInMemoryRepository(), nil, nil)
	NewHandler(svc).Routes(mux)
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
	req.Header.Set("X-User-ID", "user-1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	var p domain.WorkoutPlan
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.ID == "" {
		t.Error("expected plan to have an ID assigned by the service")
	}
	if p.UserID != "user-1" {
		t.Errorf("user_id = %q, want user-1", p.UserID)
	}
	if len(p.Days) != 3 {
		t.Errorf("got %d days, want 3", len(p.Days))
	}
}

func TestGenerateEndpoint_MissingUserID(t *testing.T) {
	mux := newPlanServer()
	body, _ := json.Marshal(domain.GenerateRequest{
		Goal: domain.GoalStrength, Experience: domain.Beginner, DaysPerWeek: 3,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/plans/generate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when X-User-ID is missing", rec.Code)
	}
}

func TestGenerateEndpoint_ValidationError(t *testing.T) {
	mux := newPlanServer()
	body := []byte(`{"goal":"swimming","experience":"beginner","days_per_week":3}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/plans/generate", bytes.NewReader(body))
	req.Header.Set("X-User-ID", "user-1")
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
	req.Header.Set("X-User-ID", "user-1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (unknown fields must be rejected)", rec.Code)
	}
}

func TestGenerateEndpoint_SeedReproducible(t *testing.T) {
	body, _ := json.Marshal(domain.GenerateRequest{
		Goal:               domain.GoalStrength,
		Experience:         domain.Advanced,
		DaysPerWeek:        4,
		AvailableEquipment: []domain.Equipment{domain.Barbell, domain.Dumbbell, domain.Bench, domain.PullupBar},
	})
	// Use a separate server per attempt so the persisted plan IDs (random by
	// design) don't pollute the comparison. Stripping volatile fields from
	// the response body would be the alternative.
	do := func() string {
		mux := newPlanServer()
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/generate?seed=99", bytes.NewReader(body))
		req.Header.Set("X-User-ID", "user-seed")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var p domain.WorkoutPlan
		_ = json.Unmarshal(rec.Body.Bytes(), &p)
		// Zero out the per-call volatile fields so the comparison covers the
		// engine output only.
		p.ID, p.CreatedAt = "", p.CreatedAt.Truncate(0)
		out, _ := json.Marshal(planEngineFingerprint(p))
		return string(out)
	}
	if do() != do() {
		t.Error("same seed produced different engine output")
	}
}

func TestGetAndListEndpoints(t *testing.T) {
	mux := newPlanServer()
	body, _ := json.Marshal(domain.GenerateRequest{
		Goal:               domain.GoalMuscleGain,
		Experience:         domain.Intermediate,
		DaysPerWeek:        3,
		AvailableEquipment: []domain.Equipment{domain.Dumbbell, domain.Bench},
	})

	// Create
	createReq := httptest.NewRequest(http.MethodPost, "/v1/plans/generate?seed=7", bytes.NewReader(body))
	createReq.Header.Set("X-User-ID", "user-42")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", createRec.Code)
	}
	var created domain.WorkoutPlan
	_ = json.Unmarshal(createRec.Body.Bytes(), &created)

	// Get by ID
	getReq := httptest.NewRequest(http.MethodGet, "/v1/plans/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200 (body: %s)", getRec.Code, getRec.Body.String())
	}

	// List by user
	listReq := httptest.NewRequest(http.MethodGet, "/v1/plans", nil)
	listReq.Header.Set("X-User-ID", "user-42")
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listRec.Code)
	}
	var listBody struct {
		Plans []domain.WorkoutPlan `json:"plans"`
	}
	_ = json.Unmarshal(listRec.Body.Bytes(), &listBody)
	if len(listBody.Plans) != 1 {
		t.Fatalf("expected 1 plan in list, got %d", len(listBody.Plans))
	}
}

// planEngineFingerprint extracts the engine-deterministic portion of a plan
// (days + exercises + warnings + split) so seed-reproducibility tests aren't
// fooled by per-call metadata like ID and timestamps.
func planEngineFingerprint(p domain.WorkoutPlan) any {
	return struct {
		Goal        domain.Goal            `json:"goal"`
		Experience  domain.ExperienceLevel `json:"experience"`
		DaysPerWeek int                    `json:"days_per_week"`
		Split       string                 `json:"split"`
		Days        []domain.PlanDay       `json:"days"`
		Warnings    []string               `json:"warnings"`
	}{p.Goal, p.Experience, p.DaysPerWeek, p.Split, p.Days, p.Warnings}
}
