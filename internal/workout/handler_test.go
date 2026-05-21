package workout

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// newTestServer builds a mux wired to a fresh in-memory service, exercising the
// same routing the real server uses.
func newTestServer() *http.ServeMux {
	svc := NewService(NewInMemoryRepository(), nil, nil)
	mux := http.NewServeMux()
	NewHandler(svc).Routes(mux)
	return mux
}

func TestHandler_CreateAndGet(t *testing.T) {
	mux := newTestServer()

	body, _ := json.Marshal(map[string]any{
		"name": "Push Day",
		"exercises": []map[string]any{
			{"name": "Bench Press", "sets": 3, "reps": 8, "weight_kg": 80},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/workouts", bytes.NewReader(body))
	req.Header.Set("X-User-ID", "user-1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d (body: %s)", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created domain.Workout
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/workouts/"+created.ID, nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", rec.Code, http.StatusOK)
	}
	var fetched domain.Workout
	if err := json.Unmarshal(rec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if fetched.Name != "Push Day" {
		t.Errorf("Name = %q, want %q", fetched.Name, "Push Day")
	}
	if len(fetched.Exercises) != 1 || fetched.Exercises[0].WeightKG != 80 {
		t.Errorf("unexpected exercises: %+v", fetched.Exercises)
	}
}

func TestHandler_Create_MissingUserID(t *testing.T) {
	mux := newTestServer()

	req := httptest.NewRequest(http.MethodPost, "/v1/workouts", bytes.NewReader([]byte(`{"name":"x"}`)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandler_Create_UnknownField(t *testing.T) {
	mux := newTestServer()

	req := httptest.NewRequest(http.MethodPost, "/v1/workouts", bytes.NewReader([]byte(`{"name":"x","bogus":true}`)))
	req.Header.Set("X-User-ID", "user-1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (unknown fields must be rejected)", rec.Code, http.StatusBadRequest)
	}
}

func TestHandler_Get_NotFound(t *testing.T) {
	mux := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/v1/workouts/missing", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandler_DeleteAndList(t *testing.T) {
	mux := newTestServer()

	body, _ := json.Marshal(map[string]any{"name": "Pull Day"})
	req := httptest.NewRequest(http.MethodPost, "/v1/workouts", bytes.NewReader(body))
	req.Header.Set("X-User-ID", "user-9")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var created domain.Workout
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	req = httptest.NewRequest(http.MethodDelete, "/v1/workouts/"+created.ID, nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/workouts", nil)
	req.Header.Set("X-User-ID", "user-9")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", rec.Code, http.StatusOK)
	}
	var listResp struct {
		Workouts []domain.Workout `json:"workouts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Workouts) != 0 {
		t.Errorf("len = %d, want 0 after delete", len(listResp.Workouts))
	}
}
