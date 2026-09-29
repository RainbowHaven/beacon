# Deploying Beacon on Railway

Production hostname: `beacon.magiconair.net`  
Suggested staging hostname: `beacon-staging.magiconair.net` (or Railway’s `*.up.railway.app` until DNS is ready)

Repo: [RainbowHaven/beacon](https://github.com/RainbowHaven/beacon)

## Layout

Use **one Railway project** with **two environments**:

| Environment | Git branch trigger | Domain | Database |
|-------------|-------------------|--------|----------|
| **staging** | Auto-deploy on push to `main` | staging host | Staging Postgres only |
| **production** | Manual deploy (for now) | `beacon.magiconair.net` | Production Postgres only |

Do not share a Postgres instance between staging and production.

## Create staging (one-time)

1. Open the existing Beacon Railway project (the one that already serves production).
2. Create a **new environment** named `staging` (project → Environments → New).
3. In `staging`:
   - Add **PostgreSQL**.
   - Add / duplicate the **app** service from the same GitHub repo (`RainbowHaven/beacon`), Dockerfile root.
   - Set variables from [`.env.staging.example`](./.env.staging.example), including `DATABASE_URL` → staging Postgres reference.
4. **Networking**
   - Generate a Railway domain, *or* attach `beacon-staging.magiconair.net` (CNAME as Railway instructs).
   - Target port: whatever the logs show (`PORT` from Railway, often `8080`).
5. **Source / Triggers** (staging app service)
   - Branch: `main`
   - Enable **automatic deploys** on push to that branch.
6. Wait for the first deploy. Check `/healthz`, then bootstrap invite + enroll a passkey on the **staging** hostname (passkeys are host-bound).

## Production triggers (recommended for now)

On the **production** environment app service:

- Keep the GitHub repo connected.
- Prefer **manual Deploy** / “Redeploy” after you have verified staging.
- Turn on auto-deploy to `main` only after staging auto-deploy feels reliable.

## Day-to-day workflow

1. Merge a PR into `main`.
2. Staging rebuilds automatically → smoke-test on the staging URL.
3. Promote: open **production** → Deploy latest `main` (or Redeploy after merge if already tracking `main` without auto-deploy wait).
4. Smoke-test `https://beacon.magiconair.net/healthz` and a quick login.

## Wipe a database (allowed while pre-pilot)

Staging (and production, when you intentionally reset):

1. Railway → that environment’s Postgres → clear data / delete & recreate the database service, **or** drop schema via a one-off connection.
2. Redeploy the app so migrations run clean.
3. Re-bootstrap the admin invite (`BOOTSTRAP_REISSUE=true` once, then set false).

Never run local `verify-phase{1,2,3}` scripts against Railway databases.

## Checklist after any env change

- [ ] `DATABASE_URL` points at **this** environment’s Postgres  
- [ ] `BASE_URL` / `WEBAUTHN_RP_ID` / `WEBAUTHN_RP_ORIGINS` match the public HTTPS host  
- [ ] `SECURE_COOKIES=true`  
- [ ] No leftover `IDENTITY_*` variables  
- [ ] `/healthz` returns OK  
- [ ] Passkey enrolled on that host  
