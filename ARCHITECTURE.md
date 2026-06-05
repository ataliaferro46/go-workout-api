# Architecture

A reference document for the architectural decisions in this codebase. Each entry is an
**Architecture Decision Record (ADR)** with a stable shape:

- **Context** — what made this a decision worth making.
- **Decision** — what we picked.
- **Why** — the reasoning that justifies the pick over the alternatives.
- **Trade-offs** — what we give up, so a future reader doesn't have to rediscover the cost.

New decisions are appended at the bottom with the next ADR number. A superseded decision
stays in the document but is marked `Status: Superseded by ADR-NN` so the history of the
codebase's thinking is recoverable.

---

## Overview

The system is a Go HTTP service with two features: a workout-plan **generation** engine and
logged-workout **CRUD**. Built on the standard library plus `pgx` (Postgres driver) and
`goose` (migration runner). No web framework, no ORM, no third-party logging or routing
library. The dependency tree is intentionally narrow so the architecture reads off the
source rather than the framework.

**Elevator pitch.** A Go HTTP service with two features — a constraint-based workout-plan
generator and logged-workout CRUD. Standard library plus `pgx` and `goose`; no framework.
Architecture is feature-sliced — `internal/plan` and `internal/workout` each own their
handler/service/repository, sharing a pure-type `domain` package and a transport-layer
`httpx`. Persistence is interface-shaped: in-memory for tests, Postgres for production, a
single contract test runs against both. Operational basics — structured logging, request
IDs, panic recovery, graceful shutdown, multi-stage distroless Docker, race-enabled CI —
are in place.

**The shape rule.** Dependencies point one way: `cmd → internal/{plan,workout} →
internal/domain`. Cross-cutting transport-layer helpers live in `internal/httpx`. Feature
packages do not import each other. The `domain` package imports no other internal package.

---

## Code organization

### ADR-001: Feature-sliced (vertical) layering over horizontal layering

**Context.** A small service can be organized either by horizontal layer (`handlers/`,
`services/`, `repositories/`) or by vertical feature (`internal/plan/`,
`internal/workout/`, each owning their own handler/service/repository).

**Decision.** Vertical. Each feature package is a self-contained slice.

**Why.** With horizontal layering, every new feature touches every layer directory, and
unrelated features accrete in the same files. With vertical slicing, a feature is one
package you can delete with `rm -r` without affecting the others. The cost of touching a
feature scales with that feature's own surface, not the codebase's.

**Trade-offs.** Code that would be shared across features (e.g., HTTP helpers, domain
types) needs a deliberate home — we give it `internal/httpx` and `internal/domain`. Without
that discipline, vertical slicing degenerates into duplicated code per feature.

### ADR-002: `internal/domain` holds pure types with strict downward imports

**Context.** The shared vocabulary of the system (`Exercise`, `Workout`, `Goal`, error
sentinels) needs a stable home that every other package can depend on without creating
cycles.

**Decision.** `internal/domain` contains only types, enums, validation methods, and error
definitions. It imports no other `internal/*` package.

**Why.** The downward-only rule is what makes the layering hold. The moment `domain`
imports `workout`, an import cycle becomes possible the next time `workout` references a
domain type. Strict directionality means changes propagate predictably — adding a domain
type doesn't ripple, removing one is a compile-error pointer to every consumer.

**Trade-offs.** Methods that would naturally live on a domain type but require I/O (e.g.,
`Workout.Save(ctx, db)`) cannot live in `domain`. We push those to the repository, which is
correct but means a reader following "what does a workout do" must look in two packages.

### ADR-003: `cmd/api/main.go` is composition root, not business logic

**Context.** Every Go service has a `main`. The question is how much code lives there.

**Decision.** `main` does only wiring: read config, construct dependencies, register
routes, start the server, shut down gracefully. No business logic, no inline handlers
beyond `/healthz`.

**Why.** Composition root is the only place that knows the concrete types of every
dependency (`PostgresRepository` vs `InMemoryRepository`, `crypto/rand` vs a test stub).
Keeping that knowledge centralized means the rest of the code talks to interfaces. Swapping
an implementation is a single-line `main.go` change.

**Trade-offs.** `main.go` is the busiest file in the repo and any new dependency lands as
new lines here. Mitigated by extracting helpers like `buildWorkoutRepo` when `main` would
otherwise sprawl.

### ADR-004: `internal/httpx` is the transport-layer uniformity package

**Context.** Multiple feature handlers need to encode JSON, map errors to HTTP statuses,
parse request bodies strictly, and run identical middleware. Inlining that in each handler
would let two endpoints subtly disagree about response shape.

**Decision.** All transport-layer helpers (JSON, Error, DecodeJSON) and middleware
(RequestID, Logger, Recover, Chain) live in `internal/httpx`. Every handler imports it;
nothing else does.

**Why.** The error envelope, the JSON content-type, the `DisallowUnknownFields` policy, and
the log format are API-wide contracts. Centralizing them is how you keep the API feeling
like one product instead of two endpoints that drifted apart.

**Trade-offs.** `httpx` accumulates everything transport-related and can become a grab-bag
if not curated. Counter-policy: each addition must be used by ≥1 feature package; one-off
helpers stay in the feature package that needs them.

---

## Domain modeling

### ADR-005: Typed-string enums (`type Goal string`) over `iota` ints

**Context.** Go enums are commonly `iota`-based ints. They could also be typed strings
backed by named constants.

**Decision.** Typed strings throughout (`Goal`, `Equipment`, `ExperienceLevel`, etc.).

**Why.** Wire serialization: a JSON `goal: "muscle_gain"` is what downstream services,
logs, and analytics actually want. With ints you'd ship `goal: 2` and force every consumer
to keep a lookup table in sync. Source stability: inserting a value mid-list of `iota`
constants silently renumbers everything below it; string constants don't care about order.

**Trade-offs.** Validation requires an explicit `Valid()` method per enum (you can't rely
on "value out of range"). Slightly higher memory cost (a string vs a small int) — negligible
in practice.

### ADR-006: Two `Exercise` types — library template vs logged event

**Context.** A "back squat" appears both as a movement the engine can *prescribe* and as
something a user has *performed*. The two have different lifecycles, different fields, and
different consumers.

**Decision.** `domain.Exercise` is the immutable library template. `domain.LoggedExercise`
is the user-performed record (sets, reps, weight). Both live in `internal/domain`.

**Why.** Conflating them would force `Exercise` to carry optional `Sets *int`, `Reps *int`
fields, and every consumer would have to disambiguate "is this an exercise-the-template or
an exercise-the-event?" Separation makes each type honest about what it represents.

**Trade-offs.** The names are similar and reviewers occasionally confuse them. Mitigated by
the doc comment on each type pointing to the other.

### ADR-007: Anemic domain — invariant-preserving methods only, no I/O

**Context.** The "rich domain model" pattern would put methods like `Workout.Save(ctx, db)`
or `Exercise.IsTopForToday()` on the domain types.

**Decision.** Domain types carry only pure methods that answer questions intrinsic to the
type (`Exercise.RequiresOnly(equip)`, `GenerateRequest.Validate()`). I/O and orchestration
live in services and repositories.

**Why.** Anemic types stay testable without mocks, can flow freely through any layer, and
have a single import boundary. The moment a domain type calls a database, the package
becomes infrastructure and the layering collapses.

**Trade-offs.** Some logic that "feels" like it belongs on the type (e.g., "how many sets
should this exercise get given my goal?") lives in `internal/plan/prescription.go` instead.
A reader looking for it has to know the convention.

### ADR-008: Sentinel error `ErrNotFound` and typed error `ValidationError` — both

**Context.** Go errors need to be distinguishable so the transport layer can map them to
HTTP statuses.

**Decision.** Two patterns coexist: `var ErrNotFound = errors.New(...)` (sentinel,
identity-compared with `errors.Is`) and `type ValidationError struct { Message string }`
(typed, data-extracted with `errors.As`).

**Why.** They serve different purposes. `ErrNotFound` is purely a *category* — callers
just need to know "is it this kind of error?" The 404 mapping needs nothing else. A
`ValidationError` is a *category + payload* — the transport layer needs to read the
message field to put it in the response envelope. Using only sentinels for both would
require global state for the message; using only typed errors for both would lose the
clean `errors.Is(err, ErrNotFound)` pattern.

**Trade-offs.** Two patterns mean two places to look for the dispatch logic. Mitigated by
both being handled in one switch in `httpx.Error`.

### ADR-009: Defense-in-depth validation — every public entry point validates

**Context.** Validation can live in one of several places: HTTP handler, service,
repository, database. Should "upstream already validated" be relied on?

**Decision.** Each public entry point (`Handler.create`, `Service.Create`,
`Generator.Generate`) re-validates the inputs it cares about. The database also enforces
invariants via `CHECK` constraints.

**Why.** Trust no upstream. A new code path (a batch importer, a CSV loader, a tool script)
might bypass the handler. The service is the *first* defense; the database is the
*final* one. Both are needed because both protect against different failure modes.

**Trade-offs.** Validation logic is duplicated in spirit (the rules) if not in code (each
layer expresses them in its own terms). Mitigated by keeping the rules' source of truth
in `domain.GenerateRequest.Validate()` and the service's `validateCreate()` — additions are
made in one place at each layer, not scattered.

---

## Plan generation engine

### ADR-010: Constraint-and-coverage algorithm, not LLM

**Context.** Modern alternatives to a hand-written engine include LLM-prompted generation,
solver-based optimization, or ML-recommended workouts.

**Decision.** A four-stage pipeline (validate → candidate pool → split + selection →
prescription) implemented as Go code with a deterministic, scored selection step.

**Why.** Determinism, cost, latency, and explainability. Rules give an auditable program a
trainer could review. LLMs are an excellent *polish* layer (coaching narrative,
periodization across weeks) but a poor *core* — they hallucinate movements, cost per
request, and are slow.

**Trade-offs.** Variety is harder to come by than with a creative model. Mitigated by
seeded jitter in scoring (ADR-011) and the variety penalty in selection.

### ADR-011: Hard constraints filter; soft preferences score

**Context.** Equipment, injuries, experience are non-negotiable. Compounds-first, primary
muscle match, variety, and randomization are preferences.

**Decision.** Hard constraints are filters in `buildCandidatePool` — they discard
candidates outright. Soft preferences are scored in `scoreExercise` and resolved by
sorting.

**Why.** Conflating the two leads to bugs you can't reproduce: a high enough score could
override a constraint that exists for safety. Filtering enforces absolutely; scoring is
where flexibility lives.

**Trade-offs.** You can't tune the strictness of a hard constraint without code change.
That's the point.

### ADR-012: Data-driven splits, not control flow

**Context.** "What is a Push day" can be expressed as imperative code (`if dayName ==
"Push" { add chest, add shoulders, ... }`) or as data (`pushDay = dayTemplate{Patterns:
..., Muscles: ...}`).

**Decision.** Data. Each split is a `splitPlan` composing named `dayTemplate` values.

**Why.** Adding a new split (e.g., a 5-day Bro split) is one new `dayTemplate` value and a
new switch case, no selection-logic changes. The same engine code works against any
template. This is the strategy-pattern shape, just expressed in Go without ceremony.

**Trade-offs.** Templates are package-level vars, which means a test could mutate them and
poison other tests. Mitigated by making them logically immutable (we never reassign them);
adding `// IMMUTABLE` comments would harden this further.

### ADR-013: Inject the RNG; fresh `Generator` per request

**Context.** Variety needs randomness. Tests need determinism. Concurrent requests can't
share mutable RNG state without a mutex.

**Decision.** The `Generator` holds a `*rand.Rand` seeded at construction; the handler
constructs a new `Generator` per request from the immutable library and a seed (either
`?seed=` or `time.Now().UnixNano()`).

**Why.** Per-request RNG instances mean no shared mutable state and no locking under
concurrent load. Tests pass a fixed seed and get reproducible output. Clients pass
`?seed=42` and can regenerate the same plan deterministically.

**Trade-offs.** A small allocation per request (one `*rand.Rand` plus its source). Trivial
at any reasonable load.

### ADR-014: Graceful degradation — partial plans with warnings beats 500

**Context.** A constrained request (e.g., bodyweight-only + lower-back injury) might not
fill every day's target exercise count.

**Decision.** Return a complete plan with a `warnings: [...]` array describing what was
constrained. Fail-fast only when zero candidates pass the filters (`ValidationError → 400`).

**Why.** "Partial results with explicit warnings" is more useful to the client than "your
request is unprocessable." The user gets something to do; the client sees what was
constrained and can prompt for more equipment or fewer injuries.

**Trade-offs.** Clients must learn the `warnings` field exists and surface it. Documented
in the API table; should be mentioned in any client SDK.

### ADR-015: `Library()` returns a fresh copy (defensive immutability)

**Context.** A function returning the seed library could return the shared slice directly
or a copy.

**Decision.** Fresh copy via `copy(out, src)` on every call.

**Why.** Defensive against accidental mutation. A caller that appends or modifies an entry
would silently corrupt every subsequent request's view of the library.

**Trade-offs.** One slice allocation per server boot (the engine handler calls `Library()`
once at construction). Negligible.

---

## Persistence

### ADR-016: Repository pattern with interface + multiple implementations

**Context.** Storage needs to be swappable: in-memory for tests, Postgres for production,
maybe a managed service later.

**Decision.** Define `workout.Repository` as a Go interface; provide `InMemoryRepository`
and `PostgresRepository` implementations. The service depends on the interface only.

**Why.** The service unit-tests against the in-memory implementation in microseconds.
Production uses Postgres without changing service code. A future Postgres-replica router or
DynamoDB implementation is additive — define a new type, register in `main`, ship.

**Trade-offs.** Two implementations to keep in sync, addressed by ADR-017.

### ADR-017: Contract tests — one spec, all implementations

**Context.** With multiple repository implementations, behavior can silently drift between
them (one returns nil where the other returns empty, one sorts differently, etc.).

**Decision.** `testRepositoryContract(t, newRepo)` is the single behavioral spec. It runs
against `InMemoryRepository` unconditionally and against `PostgresRepository` under the
`integration` build tag.

**Why.** When implementations diverge on any of the spec's scenarios, both test runs fail
loudly. Without the contract, you'd find divergence in production six months later.

**Trade-offs.** Tests are slightly less granular ("InMemoryRepository.Create" is now a
subtest of the contract). A small price for the guarantee.

### ADR-018: `pgx` + hand-written SQL, not ORM, not `sqlc`

**Context.** Go has three Postgres styles: ORMs (GORM), code-generation against SQL
(`sqlc`), or hand-written SQL with a driver (`pgx`).

**Decision.** `pgx` with hand-written SQL.

**Why.** The SQL in `postgres_repository.go` is the SQL that runs — no generated layer, no
DSL to read between. For four queries this is more legible than the generated alternative,
and the project stays free of an external code-generation binary. `sqlc` would be the right
upgrade at ~20+ queries, where compile-time type-safety pays for the codegen step.

**Trade-offs.** No compile-time type checking on column names; a schema drift caught at
runtime. Mitigated by the contract test exercising every query path.

### ADR-019: `goose` as a library; migrations embedded via `embed.FS`

**Context.** Migrations can run from a CLI binary in CI/init-container, or from inside the
app at boot.

**Decision.** App-at-boot via `goose` as a library, with migration SQL files embedded into
the binary via `embed.FS`. Goose's advisory-lock makes concurrent startup safe.

**Why.** Single deploy artifact (no separate migration image), zero developer setup (no
`goose` CLI required), and zero possibility of "app deployed but migrator didn't run."
Migrations travel with the binary.

**Trade-offs.** Every replica attempts to run migrations on startup. Safe in practice
because goose takes an advisory lock and skips already-applied versions, but the
architecture remains library-coupled. The clean upgrade path for production scale is a
separate `cmd/migrate` sharing the same `embed.FS`.

### ADR-020: Two relational tables, not `JSONB`

**Context.** Workouts have an embedded list of exercises. They could live in a `JSONB`
column on the workout row, or in a separate `workout_exercises` table.

**Decision.** Two tables (`workouts`, `workout_exercises`) joined by `workout_id`.

**Why.** Relational stays cheap to query into ("how often has this user logged a back
squat?"), index ("per-movement progression queries"), and constrain (`CHECK (sets >= 0)`).
`JSONB` would be cheaper to write and read whole-blob but punish every cross-record query.

**Trade-offs.** A `Create` becomes two inserts in a transaction instead of one row. The
read path is a `LEFT JOIN` and `scanWorkouts` grouping pass. Worth it; both are routine in
`pgx`.

### ADR-021: `TEXT` IDs, no `users` FK yet

**Context.** Postgres has native `UUID`. We could also store user IDs as foreign keys.

**Decision.** `id TEXT`, `user_id TEXT`. No foreign key on `user_id`.

**Why.** TEXT lets the application own ID generation (UUIDv4 now, ULID or external ID
later) without an ALTER. There is no `users` table yet (auth is the next milestone), so
the FK would be unenforceable today and easy to add later with one ALTER.

**Trade-offs.** ~20 extra bytes per row vs native UUID; no FK-level guarantee against
orphan `user_id` values. Both acceptable at current scale and given the planned auth
migration.

### ADR-022: Composite index matched to the query access pattern

**Context.** `ListByUser` filters on `user_id` and sorts by `created_at DESC, id`. The
index could mirror that.

**Decision.** `CREATE INDEX workouts_user_created_idx ON workouts (user_id, created_at
DESC, id)`.

**Why.** Postgres satisfies the entire `WHERE` + `ORDER BY` from the index alone — no heap
sort. The composite order matters: leading column for the equality filter, second column
for the sort, third for the tie-break.

**Trade-offs.** One extra index to maintain on every write. Acceptable; writes are
infrequent compared to reads in this domain.

### ADR-023: `ON DELETE CASCADE` for child-of-workout cleanup

**Context.** Deleting a workout should remove its exercises. This could be enforced in
application code or by the database.

**Decision.** `workout_exercises.workout_id REFERENCES workouts(id) ON DELETE CASCADE`.

**Why.** A single `DELETE FROM workouts WHERE id=$1` cleans up children automatically. The
application cannot accidentally leave orphan rows because the database refuses to.

**Trade-offs.** Cross-table cascade behavior is "magic" in the schema. Mitigated by
documenting it in the migration file and ADR.

### ADR-024: Defense-in-depth `CHECK` constraints

**Context.** Sets, reps, weight should be non-negative. The service already validates this
in `validateCreate`.

**Decision.** Also enforce in the schema: `CHECK (sets >= 0)` etc.

**Why.** The service is the primary gate; the schema is the backstop for any path that
bypasses the service (data import, bulk update, future internal tool). Database invariants
hold against all callers.

**Trade-offs.** A bad insert returns a Postgres error to translate at the repo layer.
Negligible; the validation rejects them long before.

---

## HTTP transport

### ADR-025: Standard library `net/http` + small `httpx`, no web framework

**Context.** Go has many web frameworks (Echo, Gin, chi, Fiber). They offer per-route
middleware, parameter binding, and ecosystem conveniences.

**Decision.** Standard library only, augmented by the small `internal/httpx` package.

**Why.** Go 1.22's method-aware `ServeMux` covers our routing needs. The architecture stays
visible — no framework "magic" between the OS socket and the handler. The repo builds with
nothing but a Go toolchain.

**Trade-offs.** Per-route middleware (e.g., "only this endpoint needs rate limiting") is
awkward — you'd build a second mux and chain. At ~5 routes it's fine; past ~25, `chi` is
the natural migration.

### ADR-026: Three types per write path (wire DTO, service input, domain value)

**Context.** A handler could pass the decoded HTTP body directly into the repository, or
pass intermediate types between layers.

**Decision.** `createRequest` (wire format) → `CreateInput` (service contract) →
`domain.Workout` (final value).

**Why.** Each type expresses the right contract for its boundary. The wire format can
evolve without breaking the service. The service input excludes server-generated fields
(ID, timestamp). The domain value is what the repo persists and the handler returns.

**Trade-offs.** Three small types to maintain instead of one. Worth it for the clean
boundaries; mitigated by keeping the conversions adjacent and trivial.

### ADR-027: Stable error envelope `{error: {code, message}}`

**Context.** Errors could be returned as plain text, as a string in JSON, or as a
structured envelope with multiple fields.

**Decision.** A stable envelope with a machine-readable `code` and a human-readable
`message`.

**Why.** Clients branch on `code` (which is stable across releases and documented).
Messages are for humans (UI display, logs) and can change for clarity without breaking
clients.

**Trade-offs.** A code namespace to manage; each new error category adds a code constant.
Worth it for the API stability.

### ADR-028: Strict JSON decoding (`DisallowUnknownFields`)

**Context.** `json.Decoder` ignores unknown fields by default. Strict mode rejects them.

**Decision.** Strict, always (`httpx.DecodeJSON` sets `DisallowUnknownFields()`).

**Why.** Catches client typos immediately rather than silently dropping fields. Forces a
clean API-versioning conversation when a client sends new fields the server doesn't know.

**Trade-offs.** Breaks forward-compatibility experiments where a client sends both old and
new fields. Acceptable for first-party APIs; for public APIs, consider versioned routes.

### ADR-029: 500 responses don't leak `err.Error()` to clients

**Context.** An unhandled error could be echoed in the response body or hidden.

**Decision.** 500 responses always return `"an internal error occurred"`. The full error
is logged via `slog.Error` for operators.

**Why.** Unhandled errors are by definition unmodeled; their text could reveal internal
table names, connection strings, or stack frames. Operators see the full error in logs;
clients get a generic message and the request ID to correlate.

**Trade-offs.** Debugging requires log access. That's correct — debugging by leaking
internal text to clients is how data exfiltration starts.

### ADR-030: Middleware ordering — RequestID outermost, Recover innermost

**Context.** `RequestID`, `Logger`, and `Recover` could be composed in any order. Each
ordering yields different observability and safety guarantees.

**Decision.** `RequestID(Logger(Recover(handler)))` — RequestID outermost, Recover
innermost (closest to handler).

**Why.** RequestID outermost puts the ID into the request context before any other
middleware reads it, so Logger's `r.Context()` always carries the ID. Recover innermost
catches handler panics synchronously and turns them into a normal-return 500, which Logger
then sees as a completed-request to log. Any other order either loses the request ID in
logs or skips logging panicked requests.

**Trade-offs.** Panics in `RequestID` or `Logger` themselves escape to `net/http`'s
top-level recovery (which logs and drops the connection). Acceptable: those middlewares
are small and well-tested; the handler is where panic risk actually lives.

### ADR-031: Unexported typed context key

**Context.** Context values are keyed by `interface{}`. Using a built-in type like `string`
risks collision with another package using the same key.

**Decision.** `type contextKey string` defined and unexported in `internal/httpx`; the
single key value `requestIDKey` is also unexported.

**Why.** Two packages declaring `"user_id"` as a string key would collide silently. A
package-private type makes collision impossible — no other package can construct or
compare to our key.

**Trade-offs.** Context values defined in `httpx` need accessor functions exported from
`httpx` (`RequestIDFromContext`) so other packages can read them. A trivial cost.

### ADR-032: `statusRecorder` for response-status capture in middleware

**Context.** The standard `http.ResponseWriter` doesn't expose the status code after
`WriteHeader` is called.

**Decision.** A small wrapper struct that embeds `http.ResponseWriter` and overrides
`WriteHeader` to capture the code, used by the Logger middleware.

**Why.** The embedded interface delegates everything we don't override; the overridden
method records the status before delegating. Lets us log "this request returned 404"
without changing handler code.

**Trade-offs.** One additional struct per request. Negligible.

---

## Concurrency

### ADR-033: Per-request goroutines from `net/http`; no fan-out in handlers

**Context.** Request handling could be sequential per connection or per-request.

**Decision.** Use `net/http`'s default — one goroutine per request, no additional goroutines
in handlers.

**Why.** Each handler call is already in its own goroutine; we never need `go` in the
request path. Fan-out patterns (`sync.WaitGroup`, `errgroup.Group`) come into play only
when *we* spawn goroutines to parallelize within a single request, which we don't.

**Trade-offs.** None for current scope. A batch endpoint that needs parallel work would
introduce `errgroup` at that point.

### ADR-034: Stateless handlers and services; sync primitives only at storage seams

**Context.** With many goroutines accessing shared objects, what needs locking?

**Decision.** Handlers and services hold only immutable references (interfaces, function
values, the immutable library). The only sync primitive in business code is
`sync.RWMutex` inside `InMemoryRepository`, protecting the underlying map.

**Why.** Stateless layers are concurrency-safe by default — there is nothing to lock.
`pgxpool` handles connection-level concurrency in its own internals. The only place we
write our own lock is where Go's map type requires it.

**Trade-offs.** Forcing stateless-ness means request-scoped state (like the plan
generator's RNG) is constructed per request. A small per-request allocation; trivial.

### ADR-035: `context.Context` threaded through every blocking call

**Context.** Cancellation, deadlines, and request-scoped values need a propagation
mechanism.

**Decision.** Every method that can block accepts `ctx context.Context` as the first
parameter and passes it down to every blocking call (DB, HTTP client, etc.).

**Why.** A client disconnect at the HTTP layer cancels `r.Context()`, which we thread into
`pgxpool.Pool.Query(ctx, ...)`, which aborts the in-flight query at the driver level. No
ctx, no cancellation propagation, no way to bound the lifetime of work.

**Trade-offs.** Every method signature carries `ctx` even when most won't use it for
anything beyond passing it down. Conventional in Go and worth the extra parameter.

---

## Operations

### ADR-036: Multi-stage distroless Docker image, static binary

**Context.** Container images can be based on `golang`, `alpine`, `scratch`, or
`distroless`.

**Decision.** Two-stage build: `golang:1.25-alpine` for compile, `gcr.io/distroless/static-
debian12:nonroot` for run. Binary built with `CGO_ENABLED=0`, `-trimpath`, and
`-ldflags="-s -w"`.

**Why.** Final image has only the binary, a CA bundle, tzdata, and libc. No shell, no
package manager, no `ls`. Smaller (~2 MB base + ~15 MB binary), faster to pull, dramatically
smaller attack surface. Container escape exploits usually require root and useful tools —
distroless denies both.

**Trade-offs.** No `kubectl exec sh` for debugging in production. The right answer for
production is exec to a sidecar container with the same network/PID namespace; the
distroless tradeoff is intentional.

### ADR-037: Docker layer caching for dependencies

**Context.** The naive Dockerfile copies all source and downloads dependencies in the same
layer. Any source change invalidates the dependency cache.

**Decision.** Copy `go.mod` and `go.sum` first, run `go mod download` to cache deps in
their own layer, then copy source and build.

**Why.** Subsequent builds with unchanged dependencies skip the download entirely.
Build-time drops from ~30s to ~5s on typical iteration. Caching matters for CI throughput
and developer ergonomics.

**Trade-offs.** None. This is just the canonical pattern.

### ADR-038: Graceful shutdown via `srv.Shutdown(ctx)` + signal handling

**Context.** On `SIGTERM`, the server should stop accepting new connections, let in-flight
requests finish, then close resources cleanly.

**Decision.** Listen for `SIGTERM`/`SIGINT` in a select against the server-error channel.
On signal, call `srv.Shutdown(ctx)` with a bounded timeout; on return, deferred
`pool.Close()` drains the database pool.

**Why.** Zero-downtime deploys require draining in-flight work. `Shutdown` is the
standard-library answer. The signal handler gives us a window to drain before the
orchestrator's `SIGKILL`.

**Trade-offs.** The 10-second shutdown timeout is a guess; longer would risk pod-kill
during deploy, shorter would risk truncating slow requests. Worth being aware of and
tuning per environment.

### ADR-039: `/healthz` is liveness only — does not check DB

**Context.** The health endpoint could check just process liveness or also probe
downstream dependencies.

**Decision.** `/healthz` returns `{"status":"ok"}` always (200) as long as the process is
serving.

**Why.** Liveness should fail only on unrecoverable process state. A transient DB outage
should not trigger pod restarts (that just creates a thundering herd). For
dependency-aware checks, the right answer is a separate `/readyz` endpoint that pulls the
pod from the load balancer without killing it. We don't have one yet; it's listed in
deferred work.

**Trade-offs.** A 500 endpoint that depends on Postgres has no built-in detection. Add
`/readyz` before scaling.

### ADR-040: Configuration via environment variables (12-factor)

**Context.** Configuration could live in files, environment variables, command-line flags,
or a config service.

**Decision.** Environment variables, read in `main`. The two we care about today are `ADDR`
(default `:8080`) and `DATABASE_URL` (empty switches to in-memory mode).

**Why.** Same binary across dev/staging/prod with environment-specific config. Secrets stay
out of the codebase. Standard for Kubernetes-style deploys.

**Trade-offs.** Env vars are string-typed and untyped — no compile-time check that you set
the right one. Mitigated by `getenv("KEY", default)` patterns and by failing fast at boot
if a required var is malformed.

### ADR-041: CI runs gofmt + vet + race tests + build; integration tests gated

**Context.** CI can run any subset of checks. The trade-off is signal vs runtime.

**Decision.** Four checks on every push: `gofmt -l . | empty`, `go vet ./...`, `go test
-race ./...`, `go build ./...`. Integration tests (`-tags=integration`) run locally only,
not in CI.

**Why.** The four CI checks cover formatting, static analysis, concurrency safety, and
compile-cleanness across the whole module — caught early, fast to run. Integration tests
require a Postgres service; adding one to CI is a small workflow change and listed as
deferred work.

**Trade-offs.** Integration regressions caught only locally today. Acceptable until the
repo has more contributors.

### ADR-042: Makefile as canonical command documentation

**Context.** Go's `go run` / `go test` are short enough that a Makefile is optional.

**Decision.** Maintain a Makefile with named targets for every developer workflow (`make
demo`, `make run`, `make run-pg`, `make test`, `make test-integration`, `make db-up`, etc.).

**Why.** Documentation by example — a contributor running `make help` (or reading the file)
sees the canonical commands without having to memorize flags. Multi-step recipes
(`test-integration` creates the test database if missing) belong in `make`, not in
contributor instructions.

**Trade-offs.** A second source of truth alongside the README. Mitigated by keeping the
README's command list in sync with the Makefile targets.

---

## Plan persistence

### ADR-043: Generated plans are persisted, not stateless

**Context.** Plan generation was originally a stateless endpoint — call it, get a plan,
nothing stored. Users couldn't revisit or share programs.

**Decision.** Introduce `plan.Repository` (interface + InMemory + Postgres impls), a
`plan.Service` that wraps the engine with persistence and metadata stamping, and four
endpoints: `POST /v1/plans/generate` (creates + persists, returns 201 with the assigned
ID), `GET /v1/plans/{id}`, `GET /v1/plans` (by user), `DELETE /v1/plans/{id}`.

**Why.** Plans become durable artifacts — the user can come back to one, share its ID, or
delete it. The same Repository pattern as `workout.Repository` keeps the architecture
consistent: contract-tested, in-memory for unit tests, Postgres for production.

**Trade-offs.** Generate is now a `POST` with side effects (it was idempotent before in
the absence of state). The `X-User-ID` header becomes mandatory on generate (ADR-045)
because every plan must have an owner.

### ADR-044: Plan exercises stored as JSONB snapshot, not normalized columns

**Context.** Each `PlanExercise` embeds the full `domain.Exercise` (eight fields, two of
which are slices). Persistence could either snapshot the whole `Exercise` as JSONB or
normalize each field into its own column.

**Decision.** JSONB column `exercise` on `plan_exercises` holding the serialized
`domain.Exercise`.

**Why.** Plans should be **self-contained historical records** — if the in-code exercise
library is later edited or pruned, an old plan still renders exactly as it was generated.
Storing the exercise as a snapshot guarantees that. JSONB round-trips cleanly through
`encoding/json`, and adding new fields to `domain.Exercise` later does not require a
schema migration.

**Trade-offs.** Can't easily query *into* the JSONB shape from SQL ("find all plans that
contain a barbell exercise" requires a JSONB path operator). We don't need that today; if
we do later, the fix is an expression index on the JSONB path or denormalization of the
queried fields.

### ADR-045: `X-User-ID` required on plan generation

**Context.** Workout logging already requires `X-User-ID`. The original generate endpoint
did not.

**Decision.** `POST /v1/plans/generate` returns 400 if `X-User-ID` is absent.

**Why.** Every persisted plan must have an owner — there is no "anonymous" plan in the
data model. The auth-replacement work (real JWT) will swap header reads for context-based
user lookups; the header is the placeholder.

**Trade-offs.** Slightly heavier client integration for one-off plan exploration (must
send a user ID). The demo binary doesn't go through the HTTP path so it's unaffected.

### ADR-046: One shared `pgxpool` across all repositories

**Context.** With two feature packages each owning their own Postgres-backed repository,
`main` could either build one pool per repository or share a single pool.

**Decision.** Single pool, constructed once in `buildStorage` and passed to both
`plan.NewPostgresRepository` and `workout.NewPostgresRepository`.

**Why.** Connection pools exist to multiplex concurrent work across a bounded resource.
Two pools against the same database doubles connection usage without doubling throughput —
each pool would be undersaturated and the database would burn slots. One pool sized
correctly serves both features.

**Trade-offs.** A future "per-feature pool tuning" need (e.g., reads on plans should have
a smaller pool than reads on workouts) would require splitting. Today nothing motivates
that; defer until it does.

---

## Exercise library on Postgres + admin endpoints

### ADR-059: Exercise library moves from in-code to Postgres

**Context.** The library was a `var library = []domain.Exercise{...}` slice compiled into
the binary. Editing it required a code change, a PR, and a redeploy.

**Decision.** Persist the library in the `exercises` table. Public read endpoints serve
the canonical list. Admin write endpoints under `/v1/admin/exercises/*` allow CRUD on it.

**Why.** A code-managed library means every iteration on movements, cleanup of typos, or
addition of new variants is a deploy. For a data set that grows continuously over the
life of the product, that bottleneck is the wrong one. Postgres is the right shape for
runtime-editable data.

**Trade-offs.** Adds a database table, an HTTP surface, and an interim auth boundary
(ADR-061). For the demo path, the in-memory mode seeds from `seed.go` at boot so the
zero-setup `go run` story is preserved.

### ADR-060: Boot-time snapshot cached via `atomic.Pointer`

**Context.** The plan engine calls `library()` on every plan generation, iterating ~100
exercises many times per call. A naive implementation would hit Postgres for every
generation, which is wasteful for read-mostly data that changes rarely.

**Decision.** `exercise.Service` holds an `atomic.Pointer[[]domain.Exercise]`. At boot,
`LoadSnapshot` reads the full library into the pointer. The plan service's
`LibrarySource` calls `Snapshot()` on each Generate, which is a single atomic load. Admin
writes (Create/Update/Delete) refresh the snapshot before returning, so subsequent reads
see the new state.

**Why.** Read-mostly data with infrequent writes is the canonical fit for a copy-on-write
cache. `atomic.Pointer` gives us lock-free reads (one CPU instruction) and atomic
single-writer semantics; concurrent reads cannot tear, and `LoadSnapshot` can run
alongside reads without coordination.

**Trade-offs.** The cache adds a small window where two replicas hold different
snapshots — replica A wrote, replica B's snapshot is stale until its next
LoadSnapshot. Acceptable: admin writes are rare, replicas can poll, and the next
generation on replica B uses fresh data once it reloads. A real-time invalidation
mechanism (NOTIFY/LISTEN, Redis pub/sub) would close the window if it ever matters.

### ADR-061: Admin endpoints gated behind `ADMIN_API_KEY` middleware

**Context.** Admin endpoints need auth. The roadmap calls for real JWT-based auth, but
that's a separate effort; meanwhile admin writes need *some* gate.

**Decision.** A small `httpx.AdminAuth` middleware reads `ADMIN_API_KEY` from env and
gates admin routes via `X-Admin-API-Key` header. Constant-time comparison via
`subtle.ConstantTimeCompare` defends against timing-attack key enumeration. Missing or
empty `ADMIN_API_KEY` fails closed (every admin request gets 400 with "admin api not
configured").

**Why.** A real, secure stopgap unblocks shipping admin endpoints without the auth-
system rewrite. Constant-time comparison is the right call even at this scale — it's
two lines of code and removes a real (if exotic) attack vector. Fail-closed on missing
config is the safe default; a forgotten env var should not silently open admin to the
world.

**Trade-offs.** A single shared API key has no per-user audit trail and no rotation
story. Acceptable until real auth lands; the admin endpoint surface is small, the user
count of admins is also small, and the migration to JWT is a localized change in
`main.go` (swap one middleware for another).

### ADR-062: Schema-only migrations + runtime seed-on-empty

**Context.** Migration 0003 introduces the `exercises` table. The seed data could either
be embedded in the migration (94 INSERT statements duplicating `seed.go`) or loaded at
runtime by an `exercise.SeedIfEmpty` helper that bulk-inserts from `seed.go` if the
table is empty.

**Decision.** Schema-only migration. Runtime seed via `SeedIfEmpty` at first Postgres
mode boot.

**Why.** `seed.go` is the source of truth for what the library *is*; duplicating it in
SQL is a maintenance bug waiting to happen. The runtime helper reads the same Go data
that the in-memory mode uses, so the two paths cannot drift. SeedIfEmpty tolerates per-row
ErrDuplicateName so a partial-then-retried seed converges instead of erroring out.

**Trade-offs.** Postgres state is no longer fully reproducible from migration history
alone — the initial seed depends on a Go binary executing once. Acceptable for a
code-managed library where the seed is documentation; would not be acceptable for
customer data. The migration's docstring documents this so reviewers don't expect
classic seed-via-INSERT semantics.

### ADR-063: Per-route AdminAuth via per-handler wrapping (no admin sub-mux)

**Context.** Go 1.22's `ServeMux` doesn't natively support per-route-group middleware
chains. Two patterns work: (a) build a separate admin sub-mux and chain AdminAuth
around the whole sub-mux; (b) wrap each admin HandlerFunc individually with AdminAuth
before registering it on the main mux.

**Decision.** Per-handler wrapping. Each admin route is registered as
`mux.Handle("POST /v1/admin/...", adminAuth(http.HandlerFunc(...)))`.

**Why.** Per-handler wrapping makes the auth requirement visible at the registration
site — a reviewer reading `Routes()` sees the gate next to the route. The sub-mux
alternative hides the gate elsewhere and creates a path-prefix dependency that an
absent-minded contributor could break by adding a route on the wrong mux.

**Trade-offs.** Five admin routes today; if it grew to twenty, the boilerplate would
motivate refactoring to a sub-mux or to a library like `chi` that has native route
groups. Today the cost is small and the safety property of "the gate is at the
declaration site" is worth it.

---

## Deferred / planned

Decisions not yet made because the surface that requires them doesn't exist yet. Listed
here so the next person to need them knows what to look at.

- **Real authentication.** Today: `X-User-ID` header. Planned: JWT bearer token validated
  by auth middleware that injects a typed user ID into `r.Context()`. Handlers stop
  reading the header and read context instead.
- **Adherence loop.** Logged sessions could carry a `plan_day_id` pointer so analytics can
  ask "what percentage of prescribed Squats did the user actually do?"
- **Personalization.** Bias `scoreExercise`'s weights from logged history (e.g., penalize
  exercises the user skips frequently).
- **`/readyz` endpoint** with DB probe. Used for load-balancer membership; failure pulls
  the pod from rotation without killing it.
- **Body-size limits** via `http.MaxBytesReader` wrapping `r.Body` inside `DecodeJSON`.
- **`golangci-lint`** in CI for the linters `go vet` doesn't cover (ineffassign, unused,
  staticcheck, etc.).
- **Integration tests in CI** with a Postgres service container.
- **`cmd/migrate`** as a separate binary for production deploys, sharing the same
  `internal/db/migrations` `embed.FS`.
- **Read replicas.** Add a `readPool` field on `PostgresRepository`; route read-only
  methods to it.
- **Idempotency.** Accept `Idempotency-Key` header on `POST /v1/workouts`; persist
  `(key, workout_id)` and short-circuit retries.
- **Pagination.** `ListByUser` takes `(cursor, limit)`; SQL uses keyset pagination on
  `(created_at, id) < (cursor)`.
