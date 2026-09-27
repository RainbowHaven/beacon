# Operator runbook (Beacon)

Short procedures for Rainbow Haven Coordinating (RHC). No Go knowledge required.

## Keys (identity vault)

- **Public key** lives in app config (`IDENTITY_PUBLIC_KEY_B64` + `IDENTITY_KEY_ID`). Generate locally with `make keys` / `make env`.
- **Private key** stays offline (printed / vault / `.local/identity.priv.b64` on your laptop). Never put it in env committed to git, Docker, DB, or manager devices.
- Break-glass: RHC opens `/admin/break-glass`, pastes the private key in the browser, decrypts one nickname at a time, then clicks **Clear key**.
- Each sealed-identity fetch is audited as `identity.break_glass`.

Do not commit identity private keys. Each developer’s `make keys` output is local-only.

## Invite a manager

1. Log in as RHC → **Users** → **Invite user**.
2. Create user (role + house/RHL scope) → copy invite URL (shown once on Users).
3. Manager opens the URL on `http://localhost:8080` (or the real `BASE_URL` host) and enrolls a passkey.

## Change role or house without a new invite

1. **Users** → **Edit** on that account (not your own).
2. Change role and/or house/RHL → **Save**.
3. Their sessions end immediately; passkeys keep working — they log in again with the same passkey.

## Add a safe house

1. **Houses** → add an RHL if needed → **Add safe house** (currency + active).
2. Invite or **Edit** a manager to assign them to that house.

## Lost phone / new device

1. **Users** → **Re-invite** (revokes sessions and credentials).
2. Send the new invite URL.
3. Old passkeys stop working after re-invite.

## Lock a compromised account

1. **Users** → **Lock**.
2. Sessions are revoked immediately.
3. Re-invite later when safe.

## Monthly reporting

- **Reports** needs no private key (headcount + expense totals by currency).
- Print from the browser if RHC needs a paper copy.

## Local DB tip

`go test` and `./scripts/verify-phase{1,2,3}.sh` **drop** the database schema. `verify-phase4.sh`, `verify-phase5.sh`, and `verify-phase6.sh` do not. Do not run drop scripts against a DB you care about keeping (enrolled passkeys will disappear).

## Production (Railway)

Beacon runs on **Railway** (container + managed Postgres). Receipts live in Postgres, so a DB backup covers them.

### One-time cutover

1. Create a Railway project from the `magiconair/beacon` GitHub repo (Dockerfile / `railway.toml`).
2. Add **PostgreSQL**. On the app service, set `DATABASE_URL` to the Postgres reference variable (e.g. `${{Postgres.DATABASE_URL}}`).
3. Set variables from [deploy/.env.example](./deploy/.env.example): host/WebAuthn (`BASE_URL`, `WEBAUTHN_*`, `SECURE_COOKIES=true`), `BOOTSTRAP_ADMIN_EMAIL`, and any other required app env listed there.
4. **Networking** → custom domain `beacon.magiconair.net` → add the CNAME Railway shows at your DNS. Wait until HTTPS is ready.
5. Deploy. Open `https://beacon.magiconair.net/healthz`.
6. Use the bootstrap invite from logs (set `BOOTSTRAP_REISSUE=true` once if needed), enroll the RHC **passkey on that hostname**. Localhost/tunnel passkeys do not work on production.
7. Set `BOOTSTRAP_REISSUE=false` and redeploy if you temporarily enabled it.
8. Run a backup once (below) and store the file off Railway.

### Backup / restore

```bash
DATABASE_URL='postgres://…' ./scripts/backup-prod.sh
```

Restore (scratch / disaster):

```bash
pg_restore --clean --if-exists --no-owner --dbname="$DATABASE_URL" .local/backups/beacon-….dump
```

Enable Railway Postgres backups in the service settings as well.

### After production is live

Invite managers only with URLs on `https://beacon.magiconair.net/…`.
