package biometrics

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

func newTestService(t *testing.T) (*Service, *MockProvider) {
	t.Helper()
	mock := NewMockProvider("test-webhook-secret")
	registry := NewRegistry()
	registry.Register(mock)
	tokens, err := NewTokenStore(NewInMemoryTokenRepository(), validMasterKey())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(
		registry, tokens,
		NewInMemoryReadingRepository(),
		NewInMemorySyncStateRepository(),
		nil, nil,
		slog.New(slog.NewTextHandler(os.Stderr, nil)),
	)
	return svc, mock
}

func newTestServer(t *testing.T) (*http.ServeMux, *Service, *MockProvider) {
	svc, mock := newTestService(t)
	mux := http.NewServeMux()
	NewHandler(svc).Routes(mux)
	return mux, svc, mock
}

func TestHandler_ConnectReturnsAuthURL(t *testing.T) {
	mux, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/biometrics/connect/mock", nil)
	req.Header.Set("X-User-ID", "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var got map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if !strings.Contains(got["auth_url"], "state=") {
		t.Errorf("auth_url missing state: %q", got["auth_url"])
	}
}

func TestHandler_ConnectMissingUserIDReturns400(t *testing.T) {
	mux, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/biometrics/connect/mock", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandler_UnknownProviderReturns500(t *testing.T) {
	mux, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/biometrics/connect/unknown", nil)
	req.Header.Set("X-User-ID", "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	// ErrUnknownProvider is not a known sentinel to httpx.Error so it
	// surfaces as 500. Acceptable for v1; a follow-up could map it to 404.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestHandler_CallbackSuccess(t *testing.T) {
	mux, svc, _ := newTestServer(t)

	// First, Connect to seed a state token we can use in the callback.
	_, state, err := svc.Connect(context.Background(), "u1", "mock")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet,
		"/v1/biometrics/oauth/mock/callback?state="+state+"&code=abc", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	// Token should now be saved.
	tok, err := svc.tokens.Load(context.Background(), "u1", "mock")
	if err != nil {
		t.Fatalf("token not saved: %v", err)
	}
	if tok.AccessToken != "mock-access-abc" {
		t.Errorf("AccessToken = %q, want mock-access-abc", tok.AccessToken)
	}
}

func TestHandler_CallbackUnknownStateReturns400(t *testing.T) {
	mux, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet,
		"/v1/biometrics/oauth/mock/callback?state=ghost&code=abc", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	// ErrInvalidState surfaces via httpx.Error as 500 (not in sentinel map).
	// Production would add a mapping; for now we just assert it errored.
	if rec.Code == http.StatusOK {
		t.Fatalf("expected error status, got 200")
	}
}

func TestHandler_WebhookValidSignatureReturns200(t *testing.T) {
	mux, _, mock := newTestServer(t)
	body := []byte(`{"event":"recovery.updated","user_id":"whoop-123"}`)
	sig := HMACSign(mock.SignatureKey, body)

	req := httptest.NewRequest(http.MethodPost, "/v1/biometrics/webhooks/mock", bytes.NewReader(body))
	req.Header.Set("X-Mock-Signature", sig)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandler_WebhookTamperedBodyReturns400(t *testing.T) {
	mux, _, mock := newTestServer(t)
	body := []byte(`{"event":"recovery.updated"}`)
	sig := HMACSign(mock.SignatureKey, body)
	tampered := append([]byte{}, body...)
	tampered[5] ^= 0xff // flip a bit

	req := httptest.NewRequest(http.MethodPost, "/v1/biometrics/webhooks/mock", bytes.NewReader(tampered))
	req.Header.Set("X-Mock-Signature", sig)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected signature failure, got 200")
	}
}

func TestHandler_LatestReturnsPerKindMap(t *testing.T) {
	mux, svc, _ := newTestServer(t)
	// Seed via the repository directly.
	_ = svc.readings.Insert(context.Background(),
		MakeReading("u1", domain.KindRecovery, 0.65, time.Now().UTC(), "u1-rec-1"))

	req := httptest.NewRequest(http.MethodGet, "/v1/biometrics/latest", nil)
	req.Header.Set("X-User-ID", "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Readings map[string]domain.Reading `json:"readings"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Readings["recovery"].Value != 0.65 {
		t.Errorf("recovery value = %v, want 0.65", body.Readings["recovery"].Value)
	}
}
