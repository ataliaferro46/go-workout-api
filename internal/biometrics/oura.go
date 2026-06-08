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

// OuraConfig captures the OuraProvider's OAuth + webhook configuration.
type OuraConfig struct {
	ClientID      string // OURA_CLIENT_ID
	ClientSecret  string // OURA_CLIENT_SECRET
	RedirectURI   string // OURA_REDIRECT_URI
	WebhookSecret string // OURA_WEBHOOK_SECRET

	OAuthBaseURL string // default: https://api.ouraring.com
	APIBaseURL   string // default: https://api.ouraring.com/v2
	HTTPClient   *http.Client
}

// OuraProvider implements Provider against the Oura API v2. Endpoint paths
// and field names are taken from Oura's public API reference. Oura uses
// PKCE-friendly OAuth; we model the same authorization-code grant as
// WhoopProvider (the Service treats both opaquely).
type OuraProvider struct {
	cfg OuraConfig
}

func NewOuraProvider(cfg OuraConfig) *OuraProvider {
	if cfg.OAuthBaseURL == "" {
		cfg.OAuthBaseURL = "https://api.ouraring.com"
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://api.ouraring.com/v2"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &OuraProvider{cfg: cfg}
}

func (*OuraProvider) Name() string { return "oura" }

func (p *OuraProvider) AuthURL(state string) string {
	q := url.Values{
		"response_type": {"code"},
		"client_id":     {p.cfg.ClientID},
		"redirect_uri":  {p.cfg.RedirectURI},
		"state":         {state},
		"scope":         {"daily personal"},
	}
	return p.cfg.OAuthBaseURL + "/oauth/authorize?" + q.Encode()
}

func (p *OuraProvider) ExchangeCode(ctx context.Context, code string) (domain.Token, error) {
	return p.exchange(ctx, url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {p.cfg.RedirectURI},
	})
}

func (p *OuraProvider) Refresh(ctx context.Context, refreshToken string) (domain.Token, error) {
	return p.exchange(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
}

func (p *OuraProvider) exchange(ctx context.Context, form url.Values) (domain.Token, error) {
	form.Set("client_id", p.cfg.ClientID)
	form.Set("client_secret", p.cfg.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.OAuthBaseURL+"/oauth/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return domain.Token{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.cfg.HTTPClient.Do(req)
	if err != nil {
		return domain.Token{}, fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return domain.Token{}, fmt.Errorf("oura oauth: %d %s", resp.StatusCode, string(body))
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

// LatestSince fetches readiness scores from Oura's daily_readiness endpoint
// since the watermark. Oura uses a "next_token" cursor for pagination.
func (p *OuraProvider) LatestSince(ctx context.Context, accessToken string, since time.Time) ([]domain.Reading, error) {
	out := []domain.Reading{}
	nextToken := ""

	const maxPages = 50
	for page := 0; page < maxPages; page++ {
		q := url.Values{
			"start_date": {since.UTC().Format("2006-01-02")},
		}
		if nextToken != "" {
			q.Set("next_token", nextToken)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			p.cfg.APIBaseURL+"/usercollection/daily_readiness?"+q.Encode(), nil)
		if err != nil {
			return out, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := p.cfg.HTTPClient.Do(req)
		if err != nil {
			return out, fmt.Errorf("get readiness: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return out, fmt.Errorf("oura readiness: %d %s", resp.StatusCode, string(body))
		}

		var page struct {
			Data []struct {
				ID        string    `json:"id"`
				Day       string    `json:"day"`
				Score     int       `json:"score"`
				Timestamp time.Time `json:"timestamp"`
			} `json:"data"`
			NextToken string `json:"next_token"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return out, fmt.Errorf("decode page: %w", err)
		}

		for _, r := range page.Data {
			out = append(out, domain.Reading{
				Kind:           domain.KindReadiness,
				Value:          float64(r.Score) / 100.0, // Oura ships 0–100; normalize to 0–1.
				RecordedAt:     r.Timestamp,
				IdempotencyKey: "oura:readiness:" + r.ID,
				RawPayload:     mustJSON(r),
				Provider:       "oura",
			})
		}
		if page.NextToken == "" {
			return out, nil
		}
		nextToken = page.NextToken
	}
	return out, nil
}

// VerifyWebhook checks the X-Oura-Signature header against HMAC-SHA256 of
// the body. (Oura's webhook signature scheme is documented at
// cloud.ouraring.com/v2/docs.)
func (p *OuraProvider) VerifyWebhook(headers http.Header, body []byte) error {
	got := headers.Get("X-Oura-Signature")
	if got == "" {
		return ErrInvalidSignature
	}
	want := HMACSign(p.cfg.WebhookSecret, body)
	if !hmac.Equal([]byte(got), []byte(want)) {
		return ErrInvalidSignature
	}
	return nil
}
