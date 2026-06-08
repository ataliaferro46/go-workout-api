package biometrics

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// WhoopConfig captures the WhoopProvider's OAuth + webhook configuration,
// typically loaded from environment variables by main.go.
type WhoopConfig struct {
	ClientID      string // WHOOP_CLIENT_ID
	ClientSecret  string // WHOOP_CLIENT_SECRET
	RedirectURI   string // WHOOP_REDIRECT_URI
	WebhookSecret string // WHOOP_WEBHOOK_SECRET
	// HTTP overrides default to the production endpoints; tests inject a
	// httptest.Server URL and the test transport.
	OAuthBaseURL string // default: https://api.prod.whoop.com
	APIBaseURL   string // default: https://api.prod.whoop.com/developer
	HTTPClient   *http.Client
}

// WhoopProvider implements Provider against the Whoop API v1. Endpoints and
// field names are taken from Whoop's public developer documentation. The
// webhook signature scheme is HMAC-SHA256 of the raw body under
// WebhookSecret, transmitted in the X-WHOOP-Signature header (hex-encoded).
type WhoopProvider struct {
	cfg WhoopConfig
}

// NewWhoopProvider returns a WhoopProvider with sensible defaults applied
// to cfg. If OAuthBaseURL / APIBaseURL / HTTPClient are zero, production
// values are used.
func NewWhoopProvider(cfg WhoopConfig) *WhoopProvider {
	if cfg.OAuthBaseURL == "" {
		cfg.OAuthBaseURL = "https://api.prod.whoop.com"
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://api.prod.whoop.com/developer"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &WhoopProvider{cfg: cfg}
}

func (*WhoopProvider) Name() string { return "whoop" }

func (p *WhoopProvider) AuthURL(state string) string {
	q := url.Values{
		"response_type": {"code"},
		"client_id":     {p.cfg.ClientID},
		"redirect_uri":  {p.cfg.RedirectURI},
		"state":         {state},
		"scope":         {"read:recovery read:sleep read:workout"},
	}
	return p.cfg.OAuthBaseURL + "/oauth/oauth2/auth?" + q.Encode()
}

// ExchangeCode and Refresh hit the same /oauth/oauth2/token endpoint with
// different grant_type values, so they share an internal helper.
func (p *WhoopProvider) ExchangeCode(ctx context.Context, code string) (domain.Token, error) {
	return p.exchange(ctx, url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {p.cfg.RedirectURI},
	})
}

func (p *WhoopProvider) Refresh(ctx context.Context, refreshToken string) (domain.Token, error) {
	return p.exchange(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
}

func (p *WhoopProvider) exchange(ctx context.Context, form url.Values) (domain.Token, error) {
	form.Set("client_id", p.cfg.ClientID)
	form.Set("client_secret", p.cfg.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.OAuthBaseURL+"/oauth/oauth2/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return domain.Token{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.cfg.HTTPClient.Do(req)
	if err != nil {
		return domain.Token{}, fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return domain.Token{}, fmt.Errorf("whoop oauth: %d %s", resp.StatusCode, string(body))
	}

	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return domain.Token{}, fmt.Errorf("decode token: %w", err)
	}
	return domain.Token{
		AccessToken:  out.AccessToken,
		RefreshToken: out.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(out.ExpiresIn) * time.Second),
		Scopes:       strings.Fields(out.Scope),
	}, nil
}

// LatestSince fetches recovery records from the Whoop /v1/recovery endpoint.
// Whoop paginates; we follow the next-token until either the cursor empties
// or the loop guard kicks in (defensive against pagination bugs).
func (p *WhoopProvider) LatestSince(ctx context.Context, accessToken string, since time.Time) ([]domain.Reading, error) {
	out := []domain.Reading{}
	nextToken := ""

	const maxPages = 50 // belt-and-suspenders cap; ~10k records at default 200/page
	for page := 0; page < maxPages; page++ {
		q := url.Values{
			"start": {since.UTC().Format(time.RFC3339)},
			"limit": {"50"},
		}
		if nextToken != "" {
			q.Set("nextToken", nextToken)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			p.cfg.APIBaseURL+"/v1/recovery?"+q.Encode(), nil)
		if err != nil {
			return out, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")

		resp, err := p.cfg.HTTPClient.Do(req)
		if err != nil {
			return out, fmt.Errorf("get recovery: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return out, fmt.Errorf("whoop recovery: %d %s", resp.StatusCode, string(body))
		}

		var page struct {
			Records []struct {
				CycleID   int64     `json:"cycle_id"`
				CreatedAt time.Time `json:"created_at"`
				Score     struct {
					RecoveryScore float64 `json:"recovery_score"`
				} `json:"score"`
			} `json:"records"`
			NextToken string `json:"next_token"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return out, fmt.Errorf("decode page: %w", err)
		}

		for _, rec := range page.Records {
			out = append(out, domain.Reading{
				Kind:           domain.KindRecovery,
				Value:          rec.Score.RecoveryScore / 100.0, // Whoop ships 0–100; we normalize to 0–1.
				RecordedAt:     rec.CreatedAt,
				IdempotencyKey: fmt.Sprintf("whoop:recovery:%d", rec.CycleID),
				RawPayload:     mustJSON(rec),
				Provider:       "whoop",
			})
		}
		if page.NextToken == "" {
			return out, nil
		}
		nextToken = page.NextToken
	}
	return out, nil
}

// VerifyWebhook checks the X-WHOOP-Signature header against an HMAC-SHA256
// of the body under WebhookSecret.
func (p *WhoopProvider) VerifyWebhook(headers http.Header, body []byte) error {
	got := headers.Get("X-WHOOP-Signature")
	if got == "" {
		return ErrInvalidSignature
	}
	want := HMACSign(p.cfg.WebhookSecret, body)
	if !hmac.Equal([]byte(got), []byte(want)) {
		return ErrInvalidSignature
	}
	return nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
