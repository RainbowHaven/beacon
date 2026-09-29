# Deploying Beacon on Railway

Production hostname: `beacon.magiconair.net`  
Suggested staging hostname: `beacon-staging.magiconair.net` (or Railway’s `*.up.railway.app` until DNS is ready)

Repo: [RainbowHaven/beacon](https://github.com/RainbowHaven/beacon)

## Layout

Use **one Railway project** with **two environments**:

| Environment | Deploy trigger | Domain | Database |
|-------------|----------------|--------|----------|
| **staging** | GitHub Actions (`/deploy-staging` on a PR, or restore `main` when a PR closes) | staging host | Staging Postgres only |
| **production** | Manual deploy (for now) | `beacon.magiconair.net` | Production Postgres only |

Do not share a Postgres instance between staging and production.

**Turn off Railway auto-deploy** on the staging app service. Staging deploys are owned by [`.github/workflows/staging-deploy.yml`](../.github/workflows/staging-deploy.yml) so a PR can be tested before merge without fighting Railway’s “watch `main`” trigger.

## Create staging (one-time)

1. Open the existing Beacon Railway project (the one that already serves production).
2. Create a **new environment** named `staging` (project → Environments → New).
3. In `staging`:
   - Add **PostgreSQL**.
   - Add / duplicate the **app** service from the same GitHub repo (`RainbowHaven/beacon`), Dockerfile root.
   - Set variables from [`.env.staging.example`](./.env.staging.example), including `DATABASE_URL` → staging Postgres reference (private `*.railway.internal` is fine).
   - Set **`BEACON_ALLOW_SCHEMA_WIPE=true`**.
   - Custom **start command**: `/app/beacon --wipe-schema` (wipes public schema on every container start, then migrates). Never set this on production.
   - For pre-pilot, keep `BOOTSTRAP_ADMIN_EMAIL` set and `BOOTSTRAP_REISSUE=true` so each wipe yields a fresh invite in the logs.
4. **Networking**
   - Generate a Railway domain, *or* attach `beacon-staging.magiconair.net` (CNAME as Railway instructs).
   - Target port: whatever the logs show (`PORT` from Railway, often `8080`).
5. **Source / Triggers** (staging app service)
   - **Disable** automatic deploys from GitHub (Actions will call `railway up`).
6. Wait for a first manual/Action deploy. Check `/healthz`, then bootstrap invite + enroll a passkey on the **staging** hostname (passkeys are host-bound).

Schema wipe stays on the private network: the running container uses `DATABASE_URL` and `--wipe-schema`. GitHub Actions only uploads a new image; it does not connect to Postgres.

### GitHub Actions secrets & variables

Repository **Secrets**:

| Secret | Purpose |
|--------|---------|
| `RAILWAY_STAGING_TOKEN` | Railway API token with deploy access to the **staging** environment (map to CLI `RAILWAY_TOKEN` in Actions). Use a separate `RAILWAY_PRODUCTION_TOKEN` later if you automate prod. |

Repository **Variables**:

| Variable | Example | Purpose |
|----------|---------|---------|
| `RAILWAY_PROJECT_ID` | (from Railway project settings) | Project for `railway up` |
| `RAILWAY_STAGING_SERVICE` | `beacon` | App service name in staging |
| `RAILWAY_STAGING_ENVIRONMENT` | `staging` | Environment name |
| `STAGING_URL` | `https://beacon-staging.magiconair.net` | Linked in PR comments |

## Day-to-day workflow

1. Open a PR from a branch **in this repo** (forks are rejected).
2. Comment **`/deploy-staging`** on the PR (OWNER / MEMBER / COLLABORATOR only).
3. Actions deploys that PR’s head; the container boots with `--wipe-schema`, migrates, bootstraps.
4. Smoke-test staging (passkeys are host-bound to the staging hostname).
5. Merge (or close) the PR → Actions redeploys **`main`** to staging (wipe + migrate again).
6. Promote production when ready: Railway **production** → Deploy / Redeploy `main` (no wipe flag).

Staging is **shared**: the latest `/deploy-staging` (or a PR close restoring `main`) wins. Only one deploy runs at a time (`concurrency: staging-deploy`).

Note: staging wipes on **every** container start (including crash restarts) while the start command includes `--wipe-schema`. That is intentional for pre-pilot shared staging.

## Production triggers (recommended for now)

On the **production** environment app service:

- Keep the GitHub repo connected.
- Prefer **manual Deploy** / “Redeploy” after you have verified staging.
- **Do not** set `BEACON_ALLOW_SCHEMA_WIPE` or `--wipe-schema` on production.
- Turn on auto-deploy to `main` only after you trust the pipeline.

## Wipe a database (allowed while pre-pilot)

Staging: use `/app/beacon --wipe-schema` with `BEACON_ALLOW_SCHEMA_WIPE=true` (see above).

Local optional helper (needs a reachable `DATABASE_URL`):

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
- [ ] Staging: `BEACON_ALLOW_SCHEMA_WIPE=true` + start command `/app/beacon --wipe-schema`; Railway auto-deploy **off**  
- [ ] Production: **no** wipe flag / allow env  
