# go-workout-api

A small, production-shaped REST API in Go for tracking workouts. Built on the **standard library only** — no web framework, no ORM, no third-party dependencies — to show clean backend architecture without hiding it behind tooling.

The point of this repo is not the domain (workouts). It's the structure: how handlers, services, and storage are separated; how errors flow; how the thing is tested, containerized, and shut down cleanly.

## What it demonstrates

- **Layered architecture** — HTTP transport → service (business logic) → repository (storage), each depending only on the layer beneath it through interfaces.
- **Dependency injection without a framework** — everything is wired explicitly in `cmd/api/main.go`.
- **Interface-based storage** — the service depends on a `Repository` interface; an in-memory implementation ships here, and a Postgres implementation drops in without touching business logic (see [Extending to Postgres](#extending-to-postgres)).
- **Testability by design** — the clock and ID generator are injected, so tests are deterministic. Unit tests cover the service; `httptest` tests cover the full routing + handler path.
- **Idiomatic error handling** — typed domain errors mapped to HTTP status codes at the edge, with a stable error envelope for clients.
- **Standard-library routing** — Go 1.22's method-aware `http.ServeMux` (`GET /v1/workouts/{id}`), no router dependency.
- **Operational basics** — structured logging (`log/slog`), request IDs, panic recovery middleware, graceful shutdown, health check, multi-stage distroless Docker build, and CI.

## Project structure

```
.
├── cmd/
│   └── api/
│       └── main.go            # entrypoint: wiring + graceful shutdown only
├── internal/
│   ├── domain/                # core types + errors; imports nothing internal
│   │   ├── workout.go
│   │   └── errors.go
│   ├── workout/               # the workout feature, as a vertical slice
│   │   ├── handler.go         # HTTP adapters (no business logic)
│   │   ├── handler_test.go    # httptest coverage of routing + handlers
│   │   ├── service.go         # business logic + validation
│   │   ├── service_test.go    # deterministic unit tests
│   │   ├── repository.go      # Repository interface + in-memory impl
│   │   └── uuid.go            # crypto/rand UUIDv4 generator
│   └── httpx/                 # cross-cutting transport helpers
│       ├── respond.go         # JSON + error mapping
│       └── middleware.go      # request ID, logging, recovery, chain
├── .github/workflows/ci.yml   # gofmt + vet + race tests + build
├── Dockerfile                 # multi-stage, static binary, distroless
├── Makefile
└── go.mod
```

The feature-package layout (`internal/workout/` holds its own handler, service, and repository) keeps a vertical slice together. Adding a second feature — say `exercise` or `user` — means adding a sibling package, not threading a change through `handlers/`, `services/`, and `repositories/` directories.

## Running locally

Requires Go 1.22+.

```bash
make run            # starts on :8080
# or: ADDR=:9000 go run ./cmd/api
```

Then:

```bash
# Health check
curl -s localhost:8080/healthz

# Create a workout (X-User-ID stands in for an authenticated user)
curl -s -X POST localhost:8080/v1/workouts \
  -H 'X-User-ID: user-1' \
  -H 'Content-Type: application/json' \
  -d '{
        "name": "Leg Day",
        "notes": "felt strong",
        "exercises": [
          {"name": "Back Squat", "sets": 5, "reps": 5, "weight_kg": 120},
          {"name": "Romanian Deadlift", "sets": 3, "reps": 8, "weight_kg": 100}
        ]
      }'

# List your workouts
curl -s localhost:8080/v1/workouts -H 'X-User-ID: user-1'

# Get one (use an id returned above)
curl -s localhost:8080/v1/workouts/<id>

# Delete one
curl -s -X DELETE localhost:8080/v1/workouts/<id> -i
```

## Testing

```bash
make test     # go test -race ./...
make cover    # coverage summary
make vet      # go vet ./...
```

Tests run with the race detector and have no external dependencies, so CI is fast and hermetic.

## API

| Method | Path                 | Auth header  | Body          | Success |
|--------|----------------------|--------------|---------------|---------|
| GET    | `/healthz`           | —            | —             | 200     |
| POST   | `/v1/workouts`       | `X-User-ID`  | workout JSON  | 201     |
| GET    | `/v1/workouts`       | `X-User-ID`  | —             | 200     |
| GET    | `/v1/workouts/{id}`  | —            | —             | 200     |
| DELETE | `/v1/workouts/{id}`  | —            | —             | 204     |

Errors use a stable envelope so clients branch on `code`, not on the message:

```json
{ "error": { "code": "not_found", "message": "resource not found" } }
```

| Condition          | Status | Code                |
|--------------------|--------|---------------------|
| Invalid input      | 400    | `validation_failed` |
| Unknown resource   | 404    | `not_found`         |
| Unexpected failure | 500    | `internal_error`    |

## Design notes and trade-offs

**Why no framework.** chi or gin would be reasonable in production, but for a reference implementation the standard library makes the architecture legible — there's no router magic between the request and the handler. Go 1.22's `ServeMux` covers method and path-parameter routing, which is most of what a framework provides.

**Why an in-memory store.** It keeps the repo runnable and the test suite hermetic. The important part is the `Repository` interface boundary: the service neither knows nor cares what's behind it. Swapping in Postgres is a localized change.

**Why inject the clock and ID generator.** Time and randomness are the two things that make tests flaky. Injecting them makes `CreatedAt` and IDs deterministic in tests while defaulting to `time.Now` and random UUIDs in production.

**Authentication is stubbed.** `X-User-ID` stands in for an authenticated principal. In production this would be JWT-verifying middleware that injects the user ID into the request context; the handlers would read it from the context instead of a header, and nothing else would change.

**What's intentionally omitted.** Pagination, rate limiting, real persistence, and metrics/tracing are out of scope for a focused sample but are noted where they'd attach (the middleware chain, the repository interface, the list endpoint).

## Extending to Postgres

The service depends only on `workout.Repository`. A Postgres implementation satisfies the same four methods and is selected in `main.go` — no other file changes. Sketch:

```go
type PostgresRepository struct {
    db *sql.DB // or *pgxpool.Pool
}

func (r *PostgresRepository) Get(ctx context.Context, id string) (domain.Workout, error) {
    const q = `SELECT id, user_id, name, notes, created_at FROM workouts WHERE id = $1`
    var w domain.Workout
    err := r.db.QueryRowContext(ctx, q, id).
        Scan(&w.ID, &w.UserID, &w.Name, &w.Notes, &w.CreatedAt)
    if errors.Is(err, sql.ErrNoRows) {
        return domain.Workout{}, domain.ErrNotFound
    }
    return w, err
}
```

```go
// main.go
repo := workout.NewPostgresRepository(pool) // instead of NewInMemoryRepository()
svc := workout.NewService(repo, nil, nil)
```

That interface seam is the whole point of the layering: storage is a detail, not a dependency the business logic is coupled to.

## License

MIT — see [LICENSE](LICENSE).
