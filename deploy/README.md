# Deploying Beacon on Railway

Production hostname: `beacon.magiconair.net`  
Suggested staging hostname: `beacon-staging.magiconair.net` (or Railway’s `*.up.railway.app` until DNS is ready)

Repo: [RainbowHaven/beacon](https://github.com/RainbowHaven/beacon)

## Layout

Use **one Railway project** with **two environments**:

| Environment | Deploy | Domain | Database |
|-------------|--------|--------|----------|
| **staging** | `make publish-staging SUFFIX=…` | staging host | Staging Postgres only |
| **production** | `make publish-patch` / `publish-minor` / `publish-major` | `beacon.magiconair.net` | Production Postgres only |

Do not share a Postgres instance between staging and production.

**Turn off Railway auto-deploy from GitHub** on both app services. Releases are annotated git tags created on a laptop, then uploaded with `railway up`. There is no GitHub Actions deploy workflow.

Service settings live in [`.railway/railway.ts`](../.railway/railway.ts). `railway up` does not apply that file. After pulling this change, review once per environment with `railway config plan`, then `railway config apply`. Do not apply a plan that deletes Postgres, variables, or the staging wipe. Restart policy stays on the service in the dashboard (`ON_FAILURE`, 5 retries); Infrastructure as Code has no field for it.

## Create staging (one-time)

1. Open the existing Beacon Railway project (the one that already serves production).
2. Create a **new environment** named `staging` (project → Environments → New).
3. In `staging`:
   - Add **PostgreSQL**.
   - Add / duplicate the **app** service from the same GitHub repo (`RainbowHaven/beacon`), Dockerfile root.
   - Set variables from [`.env.staging.example`](./.env.staging.example), including `DATABASE_URL` → staging Postgres reference (private `*.railway.internal` is fine).
   - Set **`BEACON_ALLOW_SCHEMA_WIPE=true`**.
   - **Pre-deploy command**: `/app/beacon wipe-schema` (runs once per deploy on the private network). Never set this on production.
   - Set a **Pre-deploy timeout** (e.g. 60s) so a stuck wipe fails the deploy instead of hanging forever.
   - **Start command**: `/app/beacon server` (migrates, then serves). Same as production.
   - For pre-pilot, keep `BOOTSTRAP_ADMIN_EMAIL` set and `BOOTSTRAP_REISSUE=true` so each wipe yields a fresh invite in the logs.
4. **Networking**
   - Generate a Railway domain, *or* attach `beacon-staging.magiconair.net` (CNAME as Railway instructs).
   - Target port: whatever the logs show (`PORT` from Railway, often `8080`).
5. **Source / Triggers** (staging and production app services)
   - **Disable** automatic deploys from GitHub.
6. Publish once with `make publish-staging` (below). Check `/healthz`, then bootstrap invite + enroll a passkey on the **staging** hostname (passkeys are host-bound).

Schema wipe stays on the private network: Railway’s pre-deploy runs `/app/beacon wipe-schema` inside the staging service; then `/app/beacon server` migrates and listens. `make publish-*` only uploads a new image; it does not connect to Postgres.

Railway project id: `c8091dad-9100-4147-a749-f5baea7a3372`. Service name: `beacon`.

### Keychain tokens

`make publish-*` reads the Railway token from the macOS login keychain (the item’s service name):

| Keychain service | Environment |
|------------------|-------------|
| `RAILWAY_BEACON_STAGING_TOKEN` | `staging` |
| `RAILWAY_BEACON_PRODUCTION_TOKEN` | `production` |

```bash
security add-generic-password -s RAILWAY_BEACON_STAGING_TOKEN -a "$USER" -w
security add-generic-password -s RAILWAY_BEACON_PRODUCTION_TOKEN -a "$USER" -w
```

Install the CLI once: `brew install railway`.

The working tree must be clean. The command creates an annotated tag, pushes it to `origin`, stamps `VERSION` / `GIT_COMMIT` / `GIT_DATE` on the service, then runs `railway up`. A tag records what was published, not that the deploy succeeded: if the deploy fails, the tag stays on `origin` and the next publish uses the next tag.

## Day-to-day workflow

Staging requires a suffix. The annotated tag is `<latest version>-staging-<suffix>`, where the latest version is the semver from `git describe --tags --abbrev=0` (`v1.2.3` and `v1.2.3-staging-menu.1` both use base `v1.2.3`). With no tags yet, the base is `v0.0.0`.

```bash
make publish-staging SUFFIX=menu.1
# tag: v1.2.3-staging-menu.1  →  Railway environment staging
```

Smoke-test staging (passkeys are host-bound to the staging hostname). Staging is **shared**: the latest publish wins. Staging wipes on **each deploy** (Railway pre-deploy), not on crash restarts of a running container.

Production bumps the highest exact `vMAJOR.MINOR.PATCH` tag and deploys that commit to environment `production`:

```bash
make publish-patch   # v1.2.3 → v1.2.4
make publish-minor   # v1.2.3 → v1.3.0
make publish-major   # v1.2.3 → v2.0.0
```

HEAD must already contain that previous release tag. There is no wipe pre-deploy on production.

## Production service

On the **production** environment app service:

- **Disable** automatic deploys from GitHub.
- Start command: `/app/beacon server` (default in [`.railway/railway.ts`](../.railway/railway.ts)).
- **Do not** set a wipe pre-deploy command or `BEACON_ALLOW_SCHEMA_WIPE` on production.

## Wipe a database (allowed while pre-pilot)

Staging: Railway pre-deploy `/app/beacon wipe-schema` with `BEACON_ALLOW_SCHEMA_WIPE=true` (see above).

Local:

```bash
BEACON_ALLOW_SCHEMA_WIPE=true go run ./cmd/beacon wipe-schema
```

Or the optional psql helper (needs a reachable `DATABASE_URL`):

```bash
DATABASE_URL='postgres://…' ./scripts/wipe-postgres-schema.sh
```

Never point wipe tooling at production unless you set `FORCE_WIPE=1` deliberately.

Never run local `verify-phase{1,2,3}` scripts against Railway databases.

## Checklist after any env change

- [ ] `DATABASE_URL` points at **this** environment’s Postgres  
- [ ] `BASE_URL` / `WEBAUTHN_RP_ID` / `WEBAUTHN_RP_ORIGINS` match the public HTTPS host  
- [ ] `SECURE_COOKIES=true`  
- [ ] No leftover `IDENTITY_*` variables  
- [ ] `/healthz` returns OK  
- [ ] Passkey enrolled on that host  
- [ ] Staging: `BEACON_ALLOW_SCHEMA_WIPE=true` + pre-deploy `/app/beacon wipe-schema` + start `/app/beacon server`; Railway auto-deploy **off**  
- [ ] Production: start `/app/beacon server` only; **no** wipe pre-deploy / allow env  
