# Specs

Engineering specs for the next three planned extensions to this codebase. Each spec is
written so that when execution begins, every meaningful decision is already made — the
work is mechanical from there.

A spec covers:

- **Goal** — what shipping this changes for users and for the codebase.
- **Architectural shape** — file layout, package structure, integration points.
- **Schema** — migration SQL, indexes, constraint reasoning.
- **Tests** — what each layer asserts, what the contract test covers, what's gated.
- **Interface boundaries** — what's exposed publicly, what stays unexported.
- **External dependencies** — which third parties (if any), credentials needed, mock paths.
- **ADRs** — the architecture decisions this introduces.
- **Definition of done** — the acceptance bar that ends the spec.
- **Out of scope** — work that's adjacent but explicitly *not* in this spec.

The format is consistent across specs so once you've read one, you know how to read the
others.

---

## The three specs

### [01 — Biometric integrations (Whoop / Oura)](./01-biometrics.md)

Plug Whoop and Oura into the plan engine so generated workouts adjust for the user's
actual recovery state. Adds a new `internal/biometrics` feature slice with OAuth-aware
providers, encrypted token storage, polling + webhook ingestion, and a recovery
parameter that influences scoring in `plan.scoreExercise`.

**Effort estimate:** ~6–10 days of focused work. The OAuth and webhook
verification are where the surface lives.

**Interview signal:** highest of the three. OAuth flow, third-party API integration,
encrypted secrets at rest, webhook idempotency, polling-vs-push trade-offs, network
failure handling. Every backend interview wants to hear those stories.

**What I can build without your involvement:** everything except live OAuth against the
real providers. The provider implementations are written; they connect to
`mock_provider.go` for tests and a real Whoop/Oura sandbox only after you supply
credentials.

---

### [02 — Diet plans + grocery lists](./02-nutrition.md)

A new `internal/nutrition` feature slice that mirrors the workout-plan engine but for
meals. Takes a goal (cut/maintain/gain), activity level, dietary preferences, and
allergens; computes TDEE and macro targets; selects meals from a seeded library to
hit targets while respecting constraints. Includes a grocery list endpoint that
aggregates ingredients across the plan.

**Effort estimate:** ~5–7 days for v1 generation plus 2–3 days for grocery list.

**Interview signal:** moderate. The architecture is the same as plan generation
(constraint-and-coverage), so it's reinforcement rather than new signal. The
*product* value is high — fitness apps with both surfaces are stickier.

**What I can build without your involvement:** everything. The food/meal libraries
are seeded in code (~150 items). No external API. No credentials.

---

### [03 — Exercise library moves to Postgres + admin endpoints](./03-exercise-library-postgres.md)

Move the in-code exercise library to a Postgres table, expand it from ~40 to ~100
movements, and add admin-only endpoints for managing it. Introduces a minimal
`ADMIN_API_KEY` middleware as an interim auth gate until real JWT auth lands.

**Effort estimate:** ~3–5 days. Smallest of the three.

**Interview signal:** moderate. Less novel than biometrics, but it touches admin auth,
seeding strategies for migrations, and the boot-time-cache pattern, all of which come
up in interviews.

**What I can build without your involvement:** everything. The library expansion is
data entry; the architecture is the same Repository pattern used elsewhere.

---

## Execution order if you do all three

If you decide to ship all three, the dependency-aware order is:

1. **Spec 3 first (exercise library to Postgres).** It's foundational — the other two
   specs *can* be done independently but both touch the exercise library indirectly,
   and having it in Postgres makes future joins natural.
2. **Spec 1 second (biometrics).** Standalone; no dependency on Spec 2.
3. **Spec 2 third (nutrition).** Standalone but biggest scope; saves the most
   product-shaped work for last.

If you decide to do *only one* given a 60-day job clock: **Spec 1**. The interview
signal per hour invested is highest, and OAuth/webhook stories are exactly what backend
loops probe.

## How to execute when you're ready

Tell me "execute spec N" and I will:

1. Re-read the spec.
2. Set up tasks mirroring the spec's "implementation order" section.
3. Write the files in dependency order, with the same review-as-I-go discipline used in
   the existing codebase.
4. Update `ARCHITECTURE.md` with the new ADRs.
5. Update the README's API table with the new endpoints.
6. Hand back a verification checklist (`go test -race`, `make test-integration`, sample
   curl commands).

Each spec is sized so that "execute" is one focused work session, not a multi-week
project. The spec's "out of scope" section names what gets *deferred* to a follow-up
spec rather than crammed in.

## How to extend the specs themselves

If you want to push back on a decision in a spec, edit the spec — it's the contract.
Don't change it during execution; that's how scope creep happens. The right discipline:

- Spec frozen before execution → execution matches spec → review + ship → next spec.
- New idea mid-execution → captured in the spec's "follow-ups" section → handled later.

Same model as ADRs: decisions are made on paper first, then implemented.
