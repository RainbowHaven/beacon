# Operator runbook (Beacon)

Short procedures for Rainbow Haven Coordinating (RHC). No Go knowledge required.

## Invite a manager

1. Log in as RHC → **Users** → **Invite user**.
2. Create user (role + house/RHL scope) → copy invite URL (shown once on Users).
3. Manager opens the URL on the real `BASE_URL` host and enrolls a passkey.

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

## Residents

- Record **nickname + arrival** only. Do not enter legal names or government IDs in Beacon.
- Nicknames must **start with a letter**. Folded nickname keys are **permanently reserved** per safe house (never reused after correction or departure). Use **Correct** to fix a nickname in place (same resident; former spelling is not kept on the record).
- Arrival date may be at most `ARRIVAL_FUTURE_DAYS` ahead of today (default **1**; set `0` to disallow any future date).
- Mark departure when the resident leaves.
- Use the **profile** control in the header (email/role) to log out.

## Monthly reporting

- **Reports** builds the monthly report (formerly Document 50) for one safe house from resident records and expenses as they are when you open it. There is no draft or submit step.
- Bed-nights count each night a resident slept in the house; the departure night is not counted. For the current month only nights before today are counted and the report says "Month in progress".
- Average occupancy needs the house's approved sleeping places (set on **Houses**).
- RHL and RHC admins with several houses see an overview first; open a house for its full report.
- **Download CSV** exports one house and month.
- Print from the browser if RHC needs a paper copy.

## Local DB tip

`go test` and `./scripts/verify-phase{1,2,3}.sh` **drop** the database schema. `verify-phase4.sh`, `verify-phase5.sh`, and `verify-phase6.sh` do not. Do not run drop scripts against a DB you care about keeping (enrolled passkeys will disappear).

## Production (Railway)

Beacon runs on **Railway** (container + managed Postgres). Receipts live in Postgres, so a DB backup covers them.

Repo: [RainbowHaven/beacon](https://github.com/RainbowHaven/beacon).  
Full staging + promote workflow: [deploy/README.md](./deploy/README.md).

### Staging vs production

| | Staging | Production |
|--|---------|------------|
| Purpose | Try a commit before prod | Live `beacon.magiconair.net` |
| Deploy | `make publish-staging SUFFIX=…` | `make publish-patch` / `minor` / `major` |
| DB | Separate staging Postgres; wiped via Railway pre-deploy on each deploy | Separate production Postgres |
| Host | `beacon-staging.magiconair.net` or Railway `*.up.railway.app` | `beacon.magiconair.net` |
| Env template | [deploy/.env.staging.example](./deploy/.env.staging.example) | [deploy/.env.example](./deploy/.env.example) |

Passkeys are bound to the hostname — enroll separately on staging and production.

After deploying the identity-vault removal, **delete any leftover `IDENTITY_*` variables** from Railway and wipe/recreate the DB (or let migration `007_drop_identity_vault.sql` run on existing data).

### One-time production cutover

1. Create a Railway project from the `RainbowHaven/beacon` GitHub repo (Dockerfile / `.railway/railway.ts`).
2. Add **PostgreSQL**. On the app service, set `DATABASE_URL` to the Postgres reference variable (e.g. `${{Postgres.DATABASE_URL}}`).
3. Set variables from [deploy/.env.example](./deploy/.env.example): host/WebAuthn (`BASE_URL`, `WEBAUTHN_*`, `SECURE_COOKIES=true`), `BOOTSTRAP_ADMIN_EMAIL`.
4. **Networking** → custom domain `beacon.magiconair.net` → add the CNAME Railway shows at your DNS. Wait until HTTPS is ready.
5. Deploy. Open `https://beacon.magiconair.net/healthz`.
6. Use the bootstrap invite from logs (set `BOOTSTRAP_REISSUE=true` once if needed), enroll the RHC **passkey on that hostname**.
7. Set `BOOTSTRAP_REISSUE=false` and redeploy if you temporarily enabled it.
8. Run a backup once (below) and store the file off Railway.
9. Add the **staging** environment (see [deploy/README.md](./deploy/README.md)), store the Railway tokens in the macOS keychain, and **disable** Railway auto-deploy from GitHub on staging and production.

### Promote a change

1. From a clean checkout, run `make publish-staging SUFFIX=menu.1` (use a new suffix) and smoke-test the staging host.
2. Merge the PR.
3. From `main`, run `make publish-patch` (or `make publish-minor` / `make publish-major`).
4. Confirm `https://beacon.magiconair.net/healthz`.

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
