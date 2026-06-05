package exercise

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/ataliaferro46/go-workout-api/internal/httpx"
)

const testAdminKey = "test-admin-key"

// newExerciseServer wires the exercise handler over an in-memory repo +
// service, with the test admin key gating admin routes.
func newExerciseServer(t *testing.T, seed []domain.Exercise) *http.ServeMux {
	t.Helper()
	repo := NewInMemoryRepository(seed)
	svc := NewService(repo)
	if err := svc.LoadSnapshot(context.Background()); err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	mux := http.NewServeMux()
	NewHandler(svc).Routes(mux, httpx.AdminAuth(testAdminKey))
	return mux
}

func TestPublicGet(t *testing.T) {
	mux := newExerciseServer(t, []domain.Exercise{sampleExercise("a", "Movement A")})

	req := httptest.NewRequest(http.MethodGet, "/v1/exercises/a", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var got domain.Exercise
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != "a" {
		t.Errorf("ID = %q, want a", got.ID)
	}
}

func TestPublicGetUnknownReturns404(t *testing.T) {
	mux := newExerciseServer(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/exercises/ghost", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestPublicList(t *testing.T) {
	mux := newExerciseServer(t, []domain.Exercise{
		sampleExercise("a", "A"),
		sampleExercise("b", "B"),
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/exercises", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Exercises []domain.Exercise `json:"exercises"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Exercises) != 2 {
		t.Fatalf("expected 2 exercises, got %d", len(body.Exercises))
	}
}

func TestAdminCreateRequiresApiKey(t *testing.T) {
	mux := newExerciseServer(t, nil)
	body, _ := json.Marshal(sampleExercise("new", "New Movement"))

	cases := []struct {
		name string
		key  string
	}{
		{"missing header", ""},
		{"wrong key", "wrong-key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/admin/exercises", bytes.NewReader(body))
			if tc.key != "" {
				req.Header.Set("X-Admin-API-Key", tc.key)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestAdminCreateWithValidKeySucceeds(t *testing.T) {
	mux := newExerciseServer(t, nil)
	body, _ := json.Marshal(sampleExercise("new", "New Movement"))

	req := httptest.NewRequest(http.MethodPost, "/v1/admin/exercises", bytes.NewReader(body))
	req.Header.Set("X-Admin-API-Key", testAdminKey)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestAdminCreateDuplicateNameReturns409(t *testing.T) {
	mux := newExerciseServer(t, []domain.Exercise{sampleExercise("a", "Squat")})

	body, _ := json.Marshal(sampleExercise("b", "Squat")) // collides
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/exercises", bytes.NewReader(body))
	req.Header.Set("X-Admin-API-Key", testAdminKey)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestAdminUpdate(t *testing.T) {
	mux := newExerciseServer(t, []domain.Exercise{sampleExercise("e", "Old Name")})

	updated := sampleExercise("e", "New Name")
	body, _ := json.Marshal(updated)
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/exercises/e", bytes.NewReader(body))
	req.Header.Set("X-Admin-API-Key", testAdminKey)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var got domain.Exercise
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Name != "New Name" {
		t.Errorf("Name = %q, want New Name", got.Name)
	}
}

func TestAdminDelete(t *testing.T) {
	mux := newExerciseServer(t, []domain.Exercise{sampleExercise("doomed", "Doomed")})

	req := httptest.NewRequest(http.MethodDelete, "/v1/admin/exercises/doomed", nil)
	req.Header.Set("X-Admin-API-Key", testAdminKey)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	// Confirm it's gone.
	getReq := httptest.NewRequest(http.MethodGet, "/v1/exercises/doomed", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("post-delete GET status = %d, want 404", getRec.Code)
	}
}

func TestAdminAuthEmptyKeyFailsClosed(t *testing.T) {
	// Same setup but with the AdminAuth middleware constructed with an
	// empty expected key: every admin request should be refused, no matter
	// what the client sends.
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	_ = svc.LoadSnapshot(context.Background())
	mux := http.NewServeMux()
	NewHandler(svc).Routes(mux, httpx.AdminAuth("")) // empty config

	body, _ := json.Marshal(sampleExercise("nope", "Should Not Land"))
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/exercises", bytes.NewReader(body))
	req.Header.Set("X-Admin-API-Key", "anything")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 fail-closed, got %d", rec.Code)
	}
}
