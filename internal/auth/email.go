package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Emailer sends transactional emails. Two implementations:
//
//   - ResendEmailer  — real delivery via Resend's API. Used when
//     RESEND_API_KEY is configured.
//   - ConsoleEmailer — logs the verification URL to stdout. Used in dev
//     and as a fallback when Resend is not configured, so the auth flow
//     stays exercisable without external dependencies.
type Emailer interface {
	SendVerification(ctx context.Context, to, verifyURL string) error
}

// ConsoleEmailer prints would-be emails to the logger. The verification URL
// is logged at INFO level so a dev running locally can click it directly
// from the terminal.
type ConsoleEmailer struct {
	Logger *slog.Logger
}

func (c ConsoleEmailer) SendVerification(_ context.Context, to, verifyURL string) error {
	if c.Logger == nil {
		fmt.Printf("[email/dev] verify %s -> %s\n", to, verifyURL)
		return nil
	}
	c.Logger.Info("email verification (dev mode — not sent)",
		"to", to, "verify_url", verifyURL)
	return nil
}

// ResendEmailer sends email via the Resend API (https://resend.com). Free
// tier covers 100 emails/day, plenty for portfolio traffic.
type ResendEmailer struct {
	APIKey     string       // RESEND_API_KEY
	From       string       // verified sender, e.g. "noreply@your-domain.com"
	HTTPClient *http.Client // nil => 10-second default
}

// resendURL is the Resend send endpoint. Hoisted so tests can override.
var resendURL = "https://api.resend.com/emails"

func (r ResendEmailer) SendVerification(ctx context.Context, to, verifyURL string) error {
	client := r.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	subject := "Verify your workout.api account"
	html := verificationEmailHTML(verifyURL)
	text := verificationEmailText(verifyURL)

	body, _ := json.Marshal(map[string]any{
		"from":    r.From,
		"to":      []string{to},
		"subject": subject,
		"html":    html,
		"text":    text,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resendURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("resend request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// verificationEmailHTML is intentionally minimal — most email clients strip
// CSS and we want one button that works everywhere.
func verificationEmailHTML(verifyURL string) string {
	return `<!doctype html>
<html><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#f5f5f5;padding:32px 16px;color:#18181d;">
<table style="max-width:480px;margin:0 auto;background:#fff;border-radius:12px;padding:32px;">
<tr><td>
<h2 style="margin:0 0 16px;color:#18181d;font-weight:700;letter-spacing:-0.01em;">Welcome to workout.api</h2>
<p style="margin:0 0 24px;color:#52525b;line-height:1.5;">Click the button below to verify your email address. The link expires in 24 hours.</p>
<p style="text-align:center;margin:0 0 24px;"><a href="` + verifyURL + `" style="display:inline-block;background:#8b5cf6;color:#fff;text-decoration:none;padding:12px 24px;border-radius:8px;font-weight:600;">Verify email</a></p>
<p style="margin:24px 0 0;color:#71717a;font-size:13px;line-height:1.5;">If the button doesn't work, paste this URL into your browser:</p>
<p style="margin:8px 0 0;color:#52525b;font-size:12px;word-break:break-all;font-family:monospace;">` + verifyURL + `</p>
</td></tr></table>
</body></html>`
}

func verificationEmailText(verifyURL string) string {
	return "Welcome to workout.api\n\nClick the link below to verify your email address. The link expires in 24 hours.\n\n" + verifyURL + "\n\nIf you didn't sign up, you can ignore this message.\n"
}
