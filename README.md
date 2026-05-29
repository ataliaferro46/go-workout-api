# go-workout-api

A fitness backend in Go that does two things: **generates** structured training programs from a user's goal, equipment, experience, and injuries — and **logs** the workouts they actually do. Built on Go's **net/http and standard library** with two narrow runtime dependencies — `pgx` for Postgres and `goose` for embedded migrations — chosen because they are a driver and a migration runner, not a framework. There is no web framework and no ORM, so the architecture is visible rather than hidden behind tooling.

Storage is **interface-shaped**: a `workout.Repository` is either in-memory (default; perfect for `go run` and unit tests) or Postgres-backed (when `DATABASE_URL` is set). The rest of the program — handlers, service, validation, tests — doesn't change between modes.

The headline feature is the generation engine: a constraint-and-coverage algorithm, not CRUD. It filters a movement library by equipment and injuries, picks a training split for the user's frequency, selects exercises for balanced muscle and movement-pattern coverage, and prescribes sets/reps/rest tuned to the goal.

## Quick look

```
$ go run ./cmd/demo

Goal: muscle_gain    Experience: intermediate    Split: Upper / Lower    Days/week: 4
Working around injuries: [lower_back]

Day 1 — Upper
  1. Dumbbell Bench Press         4 x 8-12   rest 90s
  2. Lat Pulldown                 4 x 8-12   rest 90s
  3. Machine Shoulder Press       4 x 8-12   rest 90s
  4. Seated Cable Row             4 x 8-12   rest 90s
  5. Cable Biceps Curl            2 x 8-12   rest 90s
  6. Cable Triceps Pushdown       2 x 8-12   rest 90s

Day 2 — Lower
  ...
```

(Selections vary by seed; the barbell back squat and deadlift are absent here because the request declared a lower-back injury.)

## What it demonstrates

- **A real domain algorithm**, not request plumbing: split selection, candidate filtering, coverage-based exercise selection, and goal-based prescription.
- **Constraint handling that matters**: equipment availability and injury contraindications are hard filters; experience gates movement difficulty.
- **Two feature slices, one clean architecture**: plan generation (`internal/plan`) and workout logging (`internal/workout`) are independent vertical slices over a shared `domain` and `httpx`.
- **Deterministic, testable design**: the engine's RNG and the logging service's clock/ID generator are injected, so tests are reproducible. The generate endpoint even accepts `?seed=` for repeatable output.
- **Concurrency-aware transport**: the generate handler builds a fresh generator per request, so there's no shared mutable RNG state and no locking under concurrent load.
- **Operational basics**: structured logging (`log/slog`), request IDs, panic-recovery middleware, graceful shutdown, health check, multi-stage distroless Docker build, and CI that runs `gofmt`, `vet`, and race-enabled tests.
- **Real persistence behind an interface**: a Postgres-backed `workout.Repository` with embedded migrations runs alongside the in-memory implementation, and a single contract-test suite is run against both so the implementations cannot drift.

## How the engine works

Generation is a four-stage pipeline (`internal/plan`):

1. **Validate** the request — goal, experience, day count, and any equipment/injury values must be recognized.
2. **Build the candidate pool** — filter the library to movements the user can do: equipment they have (bodyweight is always available), nothing contraindicated by an active injury, within their experience level.
3. **Choose a split and select exercises** — pick a split for the training frequency (2–3 days → full body, 4 → upper/lower, 5–6 → push/pull/legs variants). For each day, satisfy its priority movement patterns first with the best-scoring candidates, then fill remaining slots for muscle coverage. Selection penalizes movements already used elsewhere in the week, so the program has variety.
4. **Prescribe dosage** — set sets/reps/rest from the goal (strength → low reps, long rest; endurance → high reps, short rest; hypertrophy in between), then adjust for compound vs. isolation and the user's experience.

Hard constraints (equipment, injuries) are enforced by *filtering*; soft preferences (compound-first, variety) by *scoring*. That separation is deliberate: an injured user must never see a contraindicated lift, but two equally-suitable accessory movements can be chosen flexibly.

## Project structure

```
.
├── cmd/
│   ├── api/main.go             # HTTP server: storage selection + graceful shutdown
│   └── demo/main.go            # prints a sample generated plan to stdout
├── internal/
│   ├── domain/                 # pure types: enums, library Exercise, GenerateRequest,
│   │                           #   WorkoutPlan, logged Workout/LoggedExercise, errors
│   ├── exercise/               # seed exercise library (~40 movements) + tests
│   ├── plan/                   # the engine + its HTTP handler
│   ├── workout/                # logged-workout CRUD: handler, service,
│   │                           #   in-memory + Postgres repositories, contract test
│   ├── db/                     # pgxpool factory + goose-driven migrations
│   │   └── migrations/         # embedded *.sql, applied at boot
│   └── httpx/                  # JSON + error mapping, middleware
├── docker-compose.yml          # local Postgres for dev + integration tests
├── .github/workflows/ci.yml
├── Dockerfile                  # multi-stage, static binary, distroless
├── Makefile
└── go.mod
```

Two domain types intentionally coexist: **`Exercise`** is a library movement the engine can *prescribe*; **`LoggedExercise`** is a movement a user *performed* in a logged session. Keeping them distinct avoids overloading one type with two meanings.

## Running it

Requires Go 1.22+ (uses the standard library's method-aware routing) and Docker (only if you want Postgres locally; in-memory mode needs neither).

```bash
make demo            # print a sample plan
make run             # start the API on :8080 (in-memory storage)
make test            # go test -race ./...   (unit tests only)
make cover           # coverage summary
```

### With Postgres

```bash
make db-up                 # docker compose up -d postgres
make run-pg                # API with DATABASE_URL pointed at compose
make test-integration      # run the repository contract against Postgres
```

`DATABASE_URL` is the on/off switch for Postgres mode. With it unset the server uses the in-memory repository and starts in milliseconds — no Docker, no `go get` for runtime; the Postgres path is exercised only when you opt in.

Migrations are embedded in the binary (`internal/db/migrations/*.sql` via `embed.FS`) and run at boot, so a deployed container brings an empty database to schema by itself — no sidecar, no separate migration image.

Generate a plan:

```bash
curl -s -X POST 'localhost:8080/v1/plans/generate?seed=42' \
  -H 'Content-Type: application/json' \
  -d '{
        "goal": "muscle_gain",
        "experience": "intermediate",
        "days_per_week": 4,
        "available_equipment": ["barbell","dumbbell","cable","bench","pullup_bar"],
        "injuries": ["lower_back"]
      }'
```

Log a workout you did:

```bash
curl -s -X POST localhost:8080/v1/workouts \
  -H 'X-User-ID: user-1' -H 'Content-Type: application/json' \
  -d '{"name":"Leg Day","exercises":[{"name":"Back Squat","sets":5,"reps":5,"weight_kg":120}]}'
```

## API

| Method | Path                  | Auth        | Body              | Success |
|--------|-----------------------|-------------|-------------------|---------|
| GET    | `/healthz`            | —           | —                 | 200     |
| POST   | `/v1/plans/generate`  | —           | `GenerateRequest` | 200     |
| POST   | `/v1/workouts`        | `X-User-ID` | logged workout    | 201     |
| GET    | `/v1/workouts`        | `X-User-ID` | —                 | 200     |
| GET    | `/v1/workouts/{id}`   | —           | —                 | 200     |
| DELETE | `/v1/workouts/{id}`   | —           | —                 | 204     |

`POST /v1/plans/generate` accepts an optional `?seed=<int>` for reproducible output. Errors use a stable envelope so clients branch on `code`, not the message:

```json
{ "error": { "code": "validation_failed", "message": "days_per_week must be between 2 and 6" } }
```

**GenerateRequest fields**: `goal` (`fat_loss`, `muscle_gain`, `strength`, `endurance`, `general_fitness`), `experience` (`beginner`, `intermediate`, `advanced`), `days_per_week` (2–6), `session_minutes` (optional, 20–120), `available_equipment` (`barbell`, `dumbbell`, `cable`, `machine`, `kettlebell`, `bands`, `pullup_bar`, `bench`), `injuries` (`lower_back`, `knee`, `shoulder`, `elbow`, `wrist`, `hip`, `ankle`, `neck`).

## Design notes

**Why no framework.** Go 1.22's `ServeMux` handles method/path routing natively and the standard library makes the request flow legible — what middleware runs, in what order, with what state. A framework would hide all of that.

**Why pgx + goose, not an ORM or sqlc.** `pgx` is the Postgres driver, not a framework around it — the SQL in `internal/workout/postgres_repository.go` is the SQL that runs. `goose` is small, embeddable via `embed.FS`, and removes the need for a separate migration sidecar. `sqlc` was considered: it generates a beautiful type-safe query layer at scale, but it adds a separate code-generation step and a non-Go binary dependency. For the current query surface, hand-written SQL is short enough to read in one sitting and avoids the toolchain.

**Why two repositories instead of one.** In-memory storage exists for fast, hermetic unit tests and one-line `go run` demos; Postgres exists for the real product. A shared **contract test** (`repository_contract_test.go`) runs the same scenarios against both, so the implementations cannot silently diverge.

**Why inject the RNG / clock / ID generator.** Variety needs randomness and timestamps need a clock, but tests need determinism. Injecting them gives both: reproducible tests, varied production behavior, and a `?seed=` escape hatch.

**Tests describe behavior, not implementation.** They assert the things a user cares about: injured movements never appear, unavailable equipment is never required, beginners aren't handed advanced lifts, splits match the requested frequency, prescriptions match the goal, and a bodyweight-only request still produces a usable plan.

## Roadmap

Natural next steps, roughly in order:

- ~~**Persistence** — Postgres behind a repository interface with embedded migrations.~~ ✓ done.
- **Persist generated plans** — promote plan generation from a stateless endpoint to a `plan.Repository` so users can revisit programs.
- **Users & auth** — JWT-authenticated users who own profiles, generated plans, and logged sessions (replacing the `X-User-ID` stand-in).
- **Close the loop** — link logged sessions back to the plan that produced them, to measure adherence.
- **Personalization** — bias future generation toward what the user actually logs (e.g., away from repeatedly-skipped movements).
- **Nutrition + grocery** — macro-target meal planning and grocery list aggregation.

## License

MIT — see [LICENSE](LICENSE).
