# Spec 03 — Exercise library to Postgres + admin endpoints

## Goal

Move the in-code exercise library to a Postgres table, expand it from ~40 to ~100
movements, and add admin-only endpoints for managing it. The architectural payoff is
threefold: (1) the library becomes an editable data surface, not a hardcoded fixture;
(2) future personalization can write to it (per-user custom movements); (3) the
codebase introduces its first **auth boundary** via a minimal `ADMIN_API_KEY`
middleware, paving the way for real JWT auth later.

This spec is smallest of the three but foundational — if you intend to do all three,
do this one first because both Spec 01 and Spec 02 benefit from a database-backed
exercise library (e.g., for joining biometric history against logged exercises by
canonical exercise ID).

## Architectural shape

```
internal/exercise/
├── library.go                  # MOVES: now a thin Service.Load() wrapper, cached at boot
├── seed.go                     # the ORIGINAL in-code list, now feeding the migration
├── repository.go               # interface + InMemoryRepository
├── postgres_repository.go      # Postgres impl
├── repository_contract_test.go # shared contract
├── postgres_repository_test.go # integration build tag
├── service.go                  # admin operations + read access
├── service_test.go
├── handler.go                  # /v1/admin/exercises/* (auth-gated)
├── handler_test.go
└── library_test.go             # existing tests, adapted

internal/httpx/
└── admin.go                    # NEW: AdminAuth middleware reading ADMIN_API_KEY
```

Files outside that change:

- `internal/db/migrations/0003_exercises.sql` — `exercises` table + seed inserts
  generated from the existing seed list. (Becomes `0004` if executed after Spec 01 or
  Spec 02.)
- `cmd/api/main.go` — replaces `exercise.Library()` with `exercise.NewService(...)` +
  one boot-time snapshot load; wires admin auth middleware.
- `internal/plan/handler.go`, `internal/plan/service.go` — no signature changes but now
  receive `[]domain.Exercise` loaded from Postgres rather than from in-code seed.

## External surface (HTTP endpoints)

| Method | Path                            | Auth              | Body            | Success |
|--------|---------------------------------|-------------------|-----------------|---------|
| GET    | `/v1/exercises`                 | —                 | —               | 200     |
| GET    | `/v1/exercises/{id}`            | —                 | —               | 200     |
| POST   | `/v1/admin/exercises`           | `X-Admin-API-Key` | `domain.Exercise` | 201    |
| PUT    | `/v1/admin/exercises/{id}`      | `X-Admin-API-Key` | `domain.Exercise` | 200    |
| DELETE | `/v1/admin/exercises/{id}`      | `X-Admin-API-Key` | —               | 204     |

Public read endpoints under `/v1/exercises/*` so clients can browse the library; admin
write endpoints under `/v1/admin/exercises/*` so the auth surface is one path prefix.

## Schema (`0003_exercises.sql`)

```sql
-- The exercise library, previously in-code. Each row is one canonical movement.
-- Plans and (future) logged sessions reference this table by id.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE exercises (
    id                 TEXT     PRIMARY KEY,
    name               TEXT     NOT NULL UNIQUE,
    primary_muscle     TEXT     NOT NULL,
    secondary_muscles  TEXT[]   NOT NULL DEFAULT '{}',
    pattern            TEXT     NOT NULL,
    required_equipment TEXT[]   NOT NULL DEFAULT '{}',
    compound           BOOLEAN  NOT NULL,
    min_level          TEXT     NOT NULL,
    contraindications  TEXT[]   NOT NULL DEFAULT '{}'
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX exercises_pattern_idx ON exercises (pattern);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX exercises_primary_muscle_idx ON exercises (primary_muscle);
-- +goose StatementEnd

-- Seed the library from the existing in-code list. The seed.go file in
-- internal/exercise is the source of truth for these inserts — when expanding
-- the library, add to seed.go and write a follow-up migration that does the
-- additional inserts.
-- +goose StatementBegin
INSERT INTO exercises (id, name, primary_muscle, secondary_muscles, pattern,
                       required_equipment, compound, min_level, contraindications)
VALUES
    ('barbell-bench-press', 'Barbell Bench Press', 'chest',
        ARRAY['triceps','shoulders'], 'horizontal_push',
        ARRAY['barbell','bench'], TRUE, 'beginner',
        ARRAY['shoulder']),
    -- ... 40 existing entries, generated from seed.go ...
    -- ... PLUS ~60 new entries (see below) ...
;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS exercises;
-- +goose StatementEnd
```

Generation note: the spec executor generates the `INSERT` statements **from
`internal/exercise/seed.go`** so the seed list and the migration are guaranteed in sync
the first time. Subsequent library additions follow the standard migration cadence —
add to `seed.go` (for documentation), write `0004_exercises_add_X.sql` for the actual
DB change.

## Library expansion (~60 new movements)

Categorized so the breadth is structured rather than scattered:

**Olympic + power (8):** power clean, power snatch, hang clean, push press, push jerk,
clean pull, snatch pull, jump shrug.

**Lower-body variants (10):** front squat, zercher squat, hack squat, safety-bar squat,
trap-bar deadlift, Romanian deadlift (barbell), single-leg RDL, step-up, box squat,
sissy squat.

**Upper-body push variants (8):** incline barbell bench, decline barbell bench, close-
grip bench, paused bench, dumbbell incline press, landmine press, Z-press, dip.

**Upper-body pull variants (8):** weighted pull-up, chin-up, neutral-grip pull-up, T-bar
row, Pendlay row, chest-supported row, single-arm cable row, kettlebell swing.

**Plyometric + conditioning (10):** box jump, broad jump, depth jump, kettlebell snatch,
sled push, sled pull, prowler push, sandbag carry, farmer's walk, jump rope.

**Mobility + accessory (8):** Cuban rotation, band pull-apart, scapular pull-up, dead
hang, Cossack squat, world's greatest stretch, deep squat hold, hanging windshield wiper.

**Isolation additions (8):** Romanian deadlift (dumbbell), cable lateral raise, cable
reverse fly, hammer curl, concentration curl, lying triceps extension (skullcrusher),
calf raise on leg press, weighted plank.

Total: ~100 movements (40 existing + 60 new). Reasonable variety for intermediate /
advanced users without bloat.

## `seed.go` and the boot-time cache

The plan engine consumes the library synchronously — it iterates a `[]domain.Exercise`
slice many times per `Generate` call. Hitting Postgres for each generation would be
wasteful and slow. The pattern:

```go
// internal/exercise/service.go (sketch)

type Service struct {
    repo  Repository
    cache atomic.Pointer[[]domain.Exercise]
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// LoadSnapshot reads the library from the repo and caches it. Called once at
// boot from main, then again on admin writes that invalidate the cache.
func (s *Service) LoadSnapshot(ctx context.Context) error {
    list, err := s.repo.List(ctx)
    if err != nil { return err }
    s.cache.Store(&list)
    return nil
}

// Snapshot returns the cached library. Callers (plan engine) must not mutate
// the returned slice; defensive copying on each call is too costly for the
// hot path.
func (s *Service) Snapshot() []domain.Exercise {
    p := s.cache.Load()
    if p == nil { return nil }
    return *p
}
```

`atomic.Pointer[[]domain.Exercise]` swaps the cache pointer in a single instruction.
Readers (`Snapshot`) get a consistent view without locking. Writers (admin endpoints)
call `LoadSnapshot` after each successful mutation to refresh the cache.

If multi-replica writes ever become a concern, the cache becomes a TTL'd version or
a Redis pub/sub invalidation. Not in v1.

## Admin auth middleware (`internal/httpx/admin.go`)

Minimal but real:

```go
// AdminAuth gates a handler behind a static API key supplied via env. It is a
// stopgap until ADR-roadmap real JWT auth lands. Constant-time comparison
// defends against timing-attack key enumeration.
func AdminAuth(expectedKey string) func(http.Handler) http.Handler {
    if expectedKey == "" {
        return func(next http.Handler) http.Handler {
            return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                Error(w, &domain.ValidationError{Message: "admin api not configured"})
            })
        }
    }
    expected := []byte(expectedKey)
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            got := []byte(r.Header.Get("X-Admin-API-Key"))
            if len(got) == 0 || subtle.ConstantTimeCompare(got, expected) != 1 {
                w.Header().Set("WWW-Authenticate", `Key realm="admin"`)
                Error(w, &domain.ValidationError{Message: "invalid admin api key"})
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

Applied **per-route subset** by building a small inner mux for admin endpoints and
chaining the middleware over it. Pattern documented in the spec executor's section.

`ADMIN_API_KEY` is generated by `openssl rand -hex 32` and stored as an env var. If
empty/unset, the middleware returns 400 with "admin api not configured" — so a
forgotten config doesn't accidentally open the admin surface.

## Tests

### Repository contract test

Eight scenarios:

- Insert + lookup round-trips all fields including TEXT[] slices
- List returns deterministic order
- Delete then Get → ErrNotFound
- Update changes fields, leaves others intact
- Duplicate name on Insert → distinct error type (mapped to 409 by handler)
- Empty TEXT[] preserved as empty, not null
- Filter-by-pattern returns only matching rows
- Cross-record isolation: deleting one doesn't affect another

Runs against InMemory unconditionally; Postgres under `-tags=integration`.

### Service tests

- LoadSnapshot caches; Snapshot returns the cached slice
- After Create, calling LoadSnapshot returns the new entry
- Concurrent Snapshot reads while LoadSnapshot writes are race-free (run under `-race`)

### Handler tests

- `GET /v1/exercises` returns all ~100 entries
- `GET /v1/exercises/{id}` returns the matching entry
- `POST /v1/admin/exercises` without `X-Admin-API-Key` → 400
- `POST /v1/admin/exercises` with wrong key → 400
- `POST /v1/admin/exercises` with right key + valid body → 201, snapshot reloaded
- `PUT /v1/admin/exercises/{id}` with right key → 200, snapshot reloaded
- `DELETE /v1/admin/exercises/{id}` with right key → 204, snapshot reloaded

### Plan engine integration

- Existing plan tests continue to pass — the engine sees the same library shape,
  just sourced from Postgres at boot instead of from `seed.go` at compile time.
- Add one new test: plan generation references an exercise that was added at runtime
  via admin endpoint (proves the cache-reload path actually works).

## ADRs introduced

- **ADR-059: Exercise library moves from in-code to Postgres.** Compile-time fixture
  becomes runtime data; library can be edited without redeploy.
- **ADR-060: Boot-time snapshot cached via `atomic.Pointer`.** The plan engine reads
  the library on every Generate; hitting Postgres each time would be wasteful. Cache
  invalidates on admin writes.
- **ADR-061: Admin endpoints gated behind `ADMIN_API_KEY` middleware.** Interim auth
  until JWT lands. Constant-time comparison defends against timing attacks; empty key
  fails closed.
- **ADR-062: Library expansion via migration, not seed.go edits.** `seed.go` becomes
  documentation; the database is the truth. New movements ship as
  `0004_exercises_add_X.sql`.
- **ADR-063: Per-route middleware via sub-mux chaining.** Standard library `ServeMux`
  doesn't natively support per-route middleware; we compose a small admin mux and chain
  AdminAuth around it. The pattern documents the migration trigger for `chi`/`gorilla`
  if per-route middleware grows.

## Definition of done

- [ ] All new files written, package builds cleanly.
- [ ] `gofmt -w . && go vet ./... && go test -race ./...` green.
- [ ] `make test-integration` covers `./internal/exercise/...` against Postgres.
- [ ] Migration `0003_exercises.sql` (or `0004_` if after Spec 01/02) seeds all ~100
      entries successfully.
- [ ] Existing plan generator and handler tests continue to pass without change.
- [ ] Five new ADRs in `ARCHITECTURE.md`.
- [ ] README "Running it" section documents `ADMIN_API_KEY` env var with a generation
      command.
- [ ] README API table includes the public read and admin write endpoints.
- [ ] One end-to-end curl walkthrough: list library → POST a new exercise via admin →
      list again and see it → use it in a generated plan.

## Out of scope (deferred)

- **Real JWT auth.** `ADMIN_API_KEY` is interim. The migration to JWT-validated admin
  users with proper role claims is a separate spec.
- **Per-user custom movements.** Today every row in `exercises` is global. A future
  `owner_user_id` column with `NULL` for canonical movements and a user ID for
  per-user customs is a follow-up.
- **Image / video URLs on movements.** Add `media_url TEXT` later; the JSONB
  alternative is overkill for a single optional field.
- **Versioning of canonical movements.** If "Barbell Bench Press" gets renamed or
  reclassified, today the change is destructive. A history table is future work.
- **Search / autocomplete on the library.** Easy to add via a `pg_trgm` GIN index;
  out of scope here.

## What I cannot do without your involvement

Nothing external — this spec is fully self-contained. Generating the
`ADMIN_API_KEY` is one shell command (`openssl rand -hex 32`) and goes in your `.env`.
