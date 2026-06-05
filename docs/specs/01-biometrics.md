# Spec 01 — Biometric integrations (Whoop / Oura)

## Goal

Plug Whoop and Oura wearables into the plan engine so generated workouts adapt to the
user's actual recovery state. When a user's Whoop recovery is at 35% and they ask for a
hard squat session, the engine should respond with a lower-volume, lower-intensity
program and tell them why. The architectural payoff is that the codebase grows its first
**third-party API integration**, with everything that comes with it: OAuth 2.0, encrypted
token storage, idempotent webhook ingestion, polling fallback, and graceful behavior when
the third party is down.

## Architectural shape

A new feature slice `internal/biometrics` mirroring the existing `plan` and `workout`
packages, plus a small surface area inside `internal/plan` where recovery flows into
scoring.

```
internal/biometrics/
├── domain.go                       # Provider, Kind, Reading, Token types
├── provider.go                     # Provider interface + registry
├── whoop.go                        # WhoopProvider — Whoop API v1 client
├── oura.go                         # OuraProvider — Oura API v2 client
├── mock_provider.go                # MockProvider for tests + dev
├── token_store.go                  # OAuth token persistence + encryption
├── repository.go                   # Reading repository interface + InMemory
├── postgres_repository.go          # Postgres reading repository
├── repository_contract_test.go     # shared contract across implementations
├── postgres_repository_test.go     # integration build tag
├── service.go                      # orchestrates fetch + persist + lookup
├── service_test.go                 # uses MockProvider
├── oauth.go                        # OAuth callback handler
├── webhook.go                      # webhook receiver with HMAC verify
├── sync.go                         # background polling daemon
├── handler.go                      # /v1/biometrics/* HTTP endpoints
└── handler_test.go
```

Files outside the new package that change:

- `internal/domain/biometrics.go` — `Provider`, `Kind`, `Reading` value types (mirroring
  the pattern of `domain/workout.go`).
- `internal/db/migrations/0003_biometrics.sql` — three tables: `oauth_tokens`,
  `biometric_readings`, `biometric_sync_state`.
- `internal/plan/service.go` — accepts an optional `recovery` parameter on `Create`;
  threads it into the Generator.
- `internal/plan/selection.go` — `scoreExercise` takes a `recoveryAdjustment` term;
  documented in ADR.
- `cmd/api/main.go` — boots the biometrics service, optionally starts the polling
  daemon as a goroutine, registers webhook + OAuth routes.

## External surface (HTTP endpoints)

| Method | Path                                       | Auth        | Purpose                                  |
|--------|--------------------------------------------|-------------|------------------------------------------|
| GET    | `/v1/biometrics/connect/{provider}`        | `X-User-ID` | Begin OAuth: returns provider auth URL    |
| GET    | `/v1/biometrics/oauth/{provider}/callback` | —           | OAuth redirect target; exchanges code     |
| GET    | `/v1/biometrics/latest`                    | `X-User-ID` | Latest reading per kind for current user  |
| POST   | `/v1/biometrics/webhooks/{provider}`       | HMAC sig    | Push ingestion endpoint per provider      |
| DELETE | `/v1/biometrics/connect/{provider}`        | `X-User-ID` | Revoke tokens, disconnect provider        |

The `plan.NewService` Create path also gains an optional `recovery_aware` query param
that — when true — fetches the latest readiness reading and biases generation. We do not
silently inject recovery; the client opts in so behavior is predictable.

## Schema (migration `0003_biometrics.sql`)

```sql
-- OAuth tokens per (user, provider). Tokens are encrypted at rest using AES-GCM
-- with a key derived from BIOMETRICS_MASTER_KEY env var via HKDF. Plaintext
-- tokens never touch disk.
CREATE TABLE oauth_tokens (
    user_id            TEXT        NOT NULL,
    provider           TEXT        NOT NULL,
    access_token_enc   BYTEA       NOT NULL,
    refresh_token_enc  BYTEA       NOT NULL,
    nonce              BYTEA       NOT NULL,
    expires_at         TIMESTAMPTZ NOT NULL,
    scopes             TEXT[]      NOT NULL DEFAULT '{}',
    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, provider)
);

-- Ingested readings. idempotency_key is set per provider-event to defend
-- against webhook retries (and to allow re-running a poll without
-- double-counting). UNIQUE constraint on (provider, idempotency_key) makes
-- ingestion safe with ON CONFLICT DO NOTHING.
CREATE TABLE biometric_readings (
    id              TEXT             PRIMARY KEY,
    user_id         TEXT             NOT NULL,
    provider        TEXT             NOT NULL,
    kind            TEXT             NOT NULL,
    value           DOUBLE PRECISION NOT NULL,
    recorded_at     TIMESTAMPTZ      NOT NULL,
    ingested_at     TIMESTAMPTZ      NOT NULL,
    idempotency_key TEXT             NOT NULL,
    raw_payload     JSONB            NOT NULL,
    UNIQUE (provider, idempotency_key),
    CHECK (value >= 0),
    CHECK (kind IN ('recovery', 'strain', 'sleep_score', 'sleep_minutes', 'readiness'))
);

CREATE INDEX biometric_readings_user_kind_idx
    ON biometric_readings (user_id, kind, recorded_at DESC);

-- Per-user, per-provider sync watermarks for polling.
CREATE TABLE biometric_sync_state (
    user_id           TEXT        NOT NULL,
    provider          TEXT        NOT NULL,
    last_synced_at    TIMESTAMPTZ NOT NULL,
    last_event_cursor TEXT,
    PRIMARY KEY (user_id, provider)
);
```

Index reasoning: `biometric_readings_user_kind_idx` makes "latest readiness for this user"
a single B-tree lookup. The UNIQUE constraint serves as both the dedup mechanism and an
index supporting webhook retries.

## Provider interface

```go
// internal/biometrics/provider.go

type Kind string

const (
    KindRecovery     Kind = "recovery"      // 0.0–1.0
    KindStrain       Kind = "strain"        // 0.0–21.0 (Whoop scale)
    KindSleepScore   Kind = "sleep_score"   // 0.0–1.0
    KindSleepMinutes Kind = "sleep_minutes" // actual minutes
    KindReadiness    Kind = "readiness"     // 0.0–1.0 (Oura)
)

type Reading struct {
    UserID          string
    Provider        string
    Kind            Kind
    Value           float64
    RecordedAt      time.Time
    IdempotencyKey  string
    RawPayload      json.RawMessage
}

type Provider interface {
    Name() string
    AuthURL(state string) string
    ExchangeCode(ctx context.Context, code string) (Token, error)
    Refresh(ctx context.Context, refreshToken string) (Token, error)
    LatestSince(ctx context.Context, accessToken string, since time.Time) ([]Reading, error)
    VerifyWebhook(headers http.Header, body []byte) error
}

type Token struct {
    AccessToken  string
    RefreshToken string
    ExpiresAt    time.Time
    Scopes       []string
}

// Registry maps provider name → Provider; main wires concrete impls in.
type Registry map[string]Provider
```

The same interface implemented by `WhoopProvider`, `OuraProvider`, and `MockProvider`.
Tests use the mock; production registers both real providers; dev with no credentials
uses `MockProvider` returning canned recovery values.

## Token store and encryption

Tokens are sensitive — leaking a refresh token gives an attacker durable access to a
user's health data. We encrypt at rest with AES-GCM, derived from a master key in
`BIOMETRICS_MASTER_KEY` env var.

```go
// internal/biometrics/token_store.go (sketch)

type TokenStore struct {
    pool *pgxpool.Pool
    aead cipher.AEAD
}

func NewTokenStore(pool *pgxpool.Pool, masterKey []byte) (*TokenStore, error) {
    // HKDF-SHA256 derives a 32-byte AES key from the master key.
    key := hkdfDerive(masterKey, []byte("biometrics-token-v1"), 32)
    block, _ := aes.NewCipher(key)
    aead, _ := cipher.NewGCM(block)
    return &TokenStore{pool: pool, aead: aead}, nil
}

func (s *TokenStore) Save(ctx context.Context, userID, provider string, tok Token) error {
    nonce := randBytes(s.aead.NonceSize())
    accCT := s.aead.Seal(nil, nonce, []byte(tok.AccessToken),  []byte(provider))
    refCT := s.aead.Seal(nil, nonce, []byte(tok.RefreshToken), []byte(provider))
    // UPSERT into oauth_tokens with (user_id, provider) PK
    ...
}

func (s *TokenStore) Load(ctx context.Context, userID, provider string) (Token, error) {
    // SELECT, then Open(nonce, ct, []byte(provider)) for each
    ...
}
```

Notes:

- Per-row nonce; both ciphertexts in a row use the same nonce because the AAD differs
  per field (the field name is part of the AAD).
- Master key rotation: bump the HKDF info string to `biometrics-token-v2`, lazy-rewrap on
  next Save. Not implemented in v1 of the spec; documented as a follow-up.
- `BIOMETRICS_MASTER_KEY` must be at least 32 bytes; fail at boot if shorter.

## OAuth flow

```
1. Client: GET /v1/biometrics/connect/whoop
   Server: generates state token (random 32-byte hex), stores
           (state → user_id) in short-lived map with 10-min TTL,
           returns 200 { auth_url: "https://api.prod.whoop.com/oauth/..." }

2. Client: redirects user to auth_url. User authorizes on Whoop.
   Whoop:   redirects to /v1/biometrics/oauth/whoop/callback?code=...&state=...

3. Server: looks up state → user_id (rejects if expired/missing),
           calls provider.ExchangeCode(ctx, code) → Token,
           encrypts and persists via TokenStore.Save,
           triggers an initial sync for this user/provider,
           returns 200 { status: "connected" }.
```

State storage: in-memory map guarded by `sync.RWMutex` with a janitor goroutine that
prunes entries older than 10 minutes. Adequate for single-process; if we scale, this
moves to Redis or to a `oauth_state` Postgres table. Documented as a follow-up.

## Webhook receiver

Provider POSTs to `/v1/biometrics/webhooks/{provider}` with a signature header. We:

1. Read body into a buffer (bounded by `http.MaxBytesReader` at 64 KB).
2. Call `provider.VerifyWebhook(r.Header, body)` — HMAC-SHA256 against the
   provider-specific secret stored in env (`WHOOP_WEBHOOK_SECRET`, `OURA_WEBHOOK_SECRET`).
3. Parse the event payload; extract `userID` (mapped from provider's user ID via a
   `provider_user_links` table — added in this spec).
4. Insert a `biometric_readings` row with `ON CONFLICT (provider, idempotency_key) DO
   NOTHING`, so retried webhooks land exactly once.
5. Return 200 with empty body so the provider stops retrying.

If any step fails (bad signature, unknown user, persistence error), return 400 / 401 /
500 as appropriate. Providers retry on non-2xx with exponential backoff; idempotency
keys make that safe.

## Polling daemon

Webhooks are the primary ingestion path but not all providers expose them reliably.
Polling is the safety net.

```go
// internal/biometrics/sync.go (sketch)

type Daemon struct {
    svc      *Service
    interval time.Duration   // default 15 min
    logger   *slog.Logger
}

func (d *Daemon) Run(ctx context.Context) {
    ticker := time.NewTicker(d.interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            d.syncAll(ctx)
        }
    }
}

func (d *Daemon) syncAll(ctx context.Context) {
    // List all (user, provider) pairs with a token.
    // For each, fetch since last_synced_at, persist new readings, advance watermark.
    // Errors per-user are logged but do not stop the loop.
}
```

Started from `main.go` as `go d.Run(ctx)` with the request context. Bounded concurrency
(`semaphore.NewWeighted(N)`) so a thousand users don't all hit Whoop at the same instant.

## How recovery flows into plan generation

The simplest integration that lands signal without rewriting the engine:

1. `plan.Service.Create` gets an optional `recoveryAware bool` parameter from the
   handler (driven by `?recovery_aware=true`).
2. If true, the service calls `biometrics.Service.LatestForUser(ctx, userID, "recovery")`
   to get the most recent recovery value (a `Reading` with `Kind=KindRecovery`).
3. If a value exists and is fresher than 24 hours, the service includes it in the
   `domain.GenerateRequest` as a new `RecoveryHint *float64` field.
4. `plan.scoreExercise` reads `req.RecoveryHint`:
   - High recovery (≥ 0.66): no change.
   - Medium (0.33–0.66): `-0.5` per compound score, slight nudge toward isolation.
   - Low (< 0.33): `-1.5` per compound, prefer machine variants when available.
5. `plan.Generator` adds a warning when recovery is medium/low: `"recovery 28% — picked a
   lower-intensity variant"`. The user sees what the engine did and why.

This is intentionally a **soft preference** (a score adjustment), not a hard filter.
Hard filtering on recovery would make the engine refuse to generate a heavy day; we
prefer to let the user override by setting `?recovery_aware=false`.

## Tests

### Repository contract test

Same pattern as `plan` and `workout`. `testReadingRepositoryContract(t, newRepo)` runs
seven scenarios:

- Insert + lookup round-trips
- Idempotent insert (same `(provider, idempotency_key)`) is a no-op, not an error
- `LatestByUserAndKind` returns the most recent
- `LatestByUserAndKind` returns `ErrNotFound` when no readings exist
- Multiple kinds for one user don't interfere
- Ordering is `recorded_at DESC, id ASC`
- Cross-user isolation: u1's readings invisible to u2

Run against InMemory unconditionally; Postgres under `-tags=integration`.

### Token store tests (unit, no integration tag)

- Save → Load round-trips both tokens
- Wrong master key → Open fails with auth error
- Nonce reuse with different AAD → Open with right AAD succeeds; wrong AAD fails
- Master key shorter than 32 bytes → `NewTokenStore` returns error

### Provider tests (unit, using `httptest`)

- `WhoopProvider.ExchangeCode` against a mock OAuth server returns expected Token
- `WhoopProvider.LatestSince` parses Whoop's actual JSON response shape (use a fixture
  copied from Whoop's API docs)
- `WhoopProvider.VerifyWebhook` accepts a known-good signature, rejects a tampered body
- Same suite for `OuraProvider`

### Service tests (unit, using MockProvider)

- Sync persists new readings, advances watermark
- Sync is idempotent (re-running with the same window inserts no duplicates)
- Sync handles provider errors per-user (one user's failure doesn't block others)
- LatestForUser returns the freshest reading of the requested kind

### Handler tests (httptest, in-memory repo + MockProvider)

- `GET /connect/whoop` returns a valid auth URL and stores state
- `GET /oauth/whoop/callback` with a valid code → 200, token saved
- `GET /oauth/whoop/callback` with an unknown state → 400
- `POST /webhooks/whoop` with a valid signature → 200, reading persisted
- `POST /webhooks/whoop` with a tampered body → 401
- `GET /latest` returns the latest reading per kind for the user

### Plan integration test

- Generate with `recovery_aware=true` and a low-recovery reading → result has more
  machine variants and a warning string mentioning recovery
- Generate with `recovery_aware=false` → result is unchanged from baseline
- Generate with no biometric data → behaves as if `recovery_aware=false`

## ADRs introduced

- **ADR-047: Provider-shaped interface for biometric integrations.** Same Repository
  pattern shape applied to external APIs — one interface, multiple implementations
  (Whoop, Oura, Mock), contract-tested.
- **ADR-048: OAuth tokens encrypted at rest with AES-GCM.** HKDF-derived per-purpose
  keys from a master key in env. Per-row nonce; field-name AAD prevents cross-field
  copy attacks.
- **ADR-049: Webhook-first, polling-as-fallback ingestion.** Webhooks are lower-latency
  and lower-cost; polling exists to catch missed events and to bootstrap new
  connections. `biometric_sync_state` tracks the watermark.
- **ADR-050: Idempotency on every reading via `(provider, idempotency_key)`.** Webhook
  retries and double-polling are both safe by construction. ON CONFLICT DO NOTHING is
  the SQL idiom.
- **ADR-051: Recovery influences plan generation as a *score adjustment*, not a hard
  filter.** Users opt in via `?recovery_aware=true`. Engine surfaces a warning so the
  user sees what changed. Hard filtering would make the engine over-paternalistic.
- **ADR-052: Polling daemon is a single goroutine started from main, bounded by a
  weighted semaphore.** Adequate for single-process; documented upgrade path to a real
  job system (NATS JetStream or River) when concurrency demands it.

## Definition of done

- [ ] All new files written, package builds cleanly.
- [ ] `gofmt -w . && go vet ./... && go test -race ./...` green, including the seven
      contract scenarios for `biometric_readings`.
- [ ] `make test-integration` covers `./internal/biometrics/...` against real Postgres.
- [ ] `internal/plan` accepts and uses `RecoveryHint`; an integration test asserts the
      warning is emitted when recovery is low.
- [ ] `cmd/api/main.go` boots the biometrics service, starts the polling daemon, and
      registers the OAuth + webhook + read routes.
- [ ] `BIOMETRICS_MASTER_KEY` documented in README's "Running it" section with a
      generation command (`openssl rand -hex 32`).
- [ ] `WHOOP_WEBHOOK_SECRET` and `OURA_WEBHOOK_SECRET` documented similarly.
- [ ] Six new ADRs added to `ARCHITECTURE.md`.
- [ ] README's API table updated with the five new endpoints.
- [ ] One end-to-end curl walkthrough in the README showing OAuth init → callback →
      sync → recovery-aware plan generation, using the MockProvider so it works without
      real credentials.

## Out of scope (deferred to follow-ups)

- **Multi-process state for OAuth state tokens** (Redis-backed). Single-process map is
  fine until horizontal scale demands it.
- **Master key rotation.** Pattern is documented above but not implemented.
- **Apple Health / Garmin / Fitbit.** Same provider interface absorbs them; out of scope
  here.
- **Recovery-aware nutrition.** Diet plan spec is independent; cross-feature integration
  is its own work.
- **Sleep-based scheduling.** "Skip workout tomorrow because user slept 4h" is a real
  product feature but a different scope of behavioral change.
- **Real job queue.** River, asynq, NATS JetStream would replace the in-process polling
  daemon; the trigger is multi-replica deployment.

## What I cannot do without your involvement

- **Get real Whoop / Oura developer accounts and OAuth client credentials.** You'd need
  to register at developer.whoop.com and cloud.ouraring.com, accept their dev terms,
  and supply the client ID / client secret / redirect URI as env vars.
- **Run end-to-end live tests against the real APIs.** Without credentials, all live
  tests stay gated and the codebase ships with `MockProvider` as the default in dev.
- **Get production webhook signing secrets.** Same gating; mocks cover the verification
  logic comprehensively.

If you supply credentials in a `.env.local` (gitignored), I can run the integration end
to end in your local environment. Otherwise the codebase is fully shaped and tested with
the mock; switching to real providers is a one-line registry change in `main.go`.
