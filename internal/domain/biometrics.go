package domain

import (
	"encoding/json"
	"time"
)

// Kind classifies a biometric reading. Each provider may surface a subset of
// these; the engine reads them through a uniform interface. New kinds are
// added by extending this enum and validating the new value via
// validBiometricKind below.
type Kind string

const (
	// KindRecovery is a normalized 0.0–1.0 score representing how prepared
	// the user is for hard training. Whoop and Oura both surface this,
	// scaled internally to 0–100 — we store the normalized value.
	KindRecovery Kind = "recovery"
	// KindStrain is a Whoop-native cardiovascular load metric, 0.0–21.0.
	KindStrain Kind = "strain"
	// KindSleepScore is a 0.0–1.0 sleep quality score.
	KindSleepScore Kind = "sleep_score"
	// KindSleepMinutes is total minutes of sleep recorded for the period.
	KindSleepMinutes Kind = "sleep_minutes"
	// KindReadiness is Oura's analog of recovery — kept as a separate
	// concept so consumers can decide whether to treat them as
	// interchangeable.
	KindReadiness Kind = "readiness"
)

// Valid reports whether k is a recognized biometric kind.
func (k Kind) Valid() bool {
	switch k {
	case KindRecovery, KindStrain, KindSleepScore, KindSleepMinutes, KindReadiness:
		return true
	default:
		return false
	}
}

// Reading is one biometric data point ingested from a provider. The
// idempotency key is per-provider-event and is the basis for the unique
// constraint on the biometric_readings table — webhook retries and double-
// polling are safe by construction.
type Reading struct {
	ID             string          `json:"id"`
	UserID         string          `json:"user_id"`
	Provider       string          `json:"provider"`
	Kind           Kind            `json:"kind"`
	Value          float64         `json:"value"`
	RecordedAt     time.Time       `json:"recorded_at"`
	IngestedAt     time.Time       `json:"ingested_at"`
	IdempotencyKey string          `json:"-"`
	RawPayload     json.RawMessage `json:"-"`
}

// Token is an OAuth access/refresh pair plus its expiry and scopes. Always
// passed by value; the encrypted-at-rest representation lives in
// biometrics.TokenStore and never leaks past it.
type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Scopes       []string
}
