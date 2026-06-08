# Deploying go-workout-api

The default deployment target is **Fly.io** for the Go binary plus **Neon** for
managed Postgres. Both have generous free tiers — a portfolio-traffic deployment
runs at $0/month indefinitely.

Why this stack:

- **Fly.io** runs the existing `Dockerfile` as-is (multi-stage, distroless,
  static binary). No build-pack magic; `fly deploy` builds the same image you'd
  build locally.
- **Neon** is serverless Postgres that scales to zero between connections, so
  it doesn't burn free-tier hours while idle. It hands you a standard
  `postgres://` connection string the app can use directly.

The `fly.toml` in this repo configures: auto-stop machines on idle, auto-start
on first request, `/healthz` probes, 256 MB / 1 shared CPU per machine.

## One-time setup

```bash
# 1. Install flyctl
brew install flyctl                    # macOS
# or: curl -L https://fly.io/install.sh | sh
fly auth signup                        # creates account; requires a card for verification

# 2. Create a Neon project
#    Sign up at https://neon.tech
#    Create a project named "go-workout-api"
#    Copy the connection string (the one labeled "Pooled" works best for Fly).
#    It looks like:
#      postgres://USER:PASS@ep-xxxxx.us-east-2.aws.neon.tech/neondb?sslmode=require

# 3. Provision the Fly app (does NOT deploy yet)
fly launch --no-deploy --copy-config --name=go-workout-api
# If "go-workout-api" is taken, pick another name and edit fly.toml's `app` line to match.
```

## Set the secrets

The app reads three secrets at startup. Generate and set them all in one call so
Fly doesn't roll the machine three times.

```bash
DB_URL='postgres://USER:PASS@ep-xxxxx.us-east-2.aws.neon.tech/neondb?sslmode=require'

fly secrets set \
  DATABASE_URL="$DB_URL" \
  ADMIN_API_KEY="$(openssl rand -hex 32)" \
  BIOMETRICS_MASTER_KEY="$(openssl rand -hex 32)" \
  MOCK_WEBHOOK_SECRET="$(openssl rand -hex 32)"
```

Take a note of `ADMIN_API_KEY` locally — you'll need it to hit the admin
endpoints. (`fly secrets` is write-only; you can't read them back.)

## Deploy

```bash
fly deploy
```

The first deploy takes ~2–3 minutes (image build + push + machine start).
Subsequent deploys are 30–60 seconds because the dependency layer caches.

When it finishes:

```bash
fly status
fly logs                               # tail the running app logs
fly open                               # opens https://go-workout-api.fly.dev in browser
```

## Verify

```bash
APP="https://go-workout-api.fly.dev"   # or whatever name fly assigned

curl -s $APP/healthz
# {"status":"ok"}

curl -s $APP/v1/exercises | jq '.exercises | length'
# 94

curl -s -X POST $APP/v1/plans/generate \
  -H 'X-User-ID: demo-user' -H 'Content-Type: application/json' \
  -d '{
    "goal":"muscle_gain","experience":"intermediate","days_per_week":4,
    "available_equipment":["barbell","dumbbell","cable","bench","pullup_bar"]
  }' | jq '.split, .days[0].exercises[0].exercise.name'
```

## Hit it from your phone

- **GET** endpoints render as JSON in mobile Safari. Try
  `https://your-app.fly.dev/v1/exercises` and use a JSON viewer extension
  ("JSON Peep" works) for nice formatting.
- **POST / DELETE** endpoints need a client that sets headers + body. Two
  zero-install options:
  - [hoppscotch.io](https://hoppscotch.io) — opens in mobile browser, no app
    needed. Save your requests as a collection for one-tap reuse.
  - The official Postman mobile app — heavier but supports collections + env
    variables (handy for `X-Admin-API-Key`).

## Operations

```bash
fly logs                  # real-time logs
fly status                # current machine state
fly scale show            # current CPU / memory
fly secrets list          # names of configured secrets (not values)
fly machine restart       # force a fresh boot
fly destroy go-workout-api   # wipes the app entirely
```

When you redeploy: `git push` to GitHub if you want, but `fly deploy` does NOT
require GitHub — it builds from the local working directory.

## Cost notes

- Fly: machines auto-stop after ~5 min idle. A typical portfolio app runs <1
  CPU-hour/day awake, well under the free monthly allowance.
- Neon: free tier is 0.5 vCPU + 3 GB storage with scale-to-zero compute. The
  app is connection-light (one pgxpool, ~10 max conns) — easy fit.
- The only ongoing cost risk is if a polling daemon ran constantly hitting
  Whoop/Oura — but with no connected users it's a no-op.

## Production-shaped upgrades (deferred)

- Custom domain + HTTPS cert: `fly certs create api.yourdomain.com`
- Replicas in additional regions: `fly machine clone --region lax`
- Metrics + dashboards: Fly bundles Grafana; turn on with `fly dashboard metrics`
- Real auth: replace the `ADMIN_API_KEY` stopgap with the JWT roadmap item
  (ADR-roadmap)
