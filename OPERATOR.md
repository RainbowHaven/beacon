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

## Passkeys: what people need to know

- Beacon has no passwords. Each person signs in with a **passkey** on their own phone or computer.
- Point anyone unsure about passkeys to `/help/passkeys` (linked from the sign-in and invite pages).
- Everyone can see and manage their own passkeys under **Account & passkeys** (profile menu → `/account`): name them, see when each was added and last used, add another one, and remove old ones. The last passkey cannot be removed by its owner.
- **Never share accounts or devices.** Every person gets their own invite.

## Backup passkey (do this for every user)

1. The user signs in on a second device (laptop or spare phone). If that device has no passkey yet, the browser can use the phone by scanning a QR code.
2. **Account & passkeys** → type a name (e.g. “Work laptop”) → **Add passkey** → confirm with fingerprint, face, or screen lock.
3. Both passkeys now work. Losing one device no longer locks them out.

RHC admins should always have a backup passkey, and there should be at least **two** RHC admins.

## Lost, stolen, or replaced device

**The user still has another working passkey:** they sign in with it, open **Account & passkeys**, and **Remove** the lost device’s passkey. All their other sessions are signed out.

**The user has no other passkey, or you want to act for them:**

1. **Users** → **Edit** on that account → **Passkeys**.
2. **Remove** the passkey of the lost device (the name and “last used” date help identify it). All of that user’s sessions end immediately.
3. If that was their only passkey: back on **Users**, click **New invite** and send them the link. They open it on the new device and create a passkey.

**Replacing a phone (old one still works):** the user adds a passkey on the new phone first, then removes the old one.

**Stolen device or possible misuse:** **Lock** the account first (see below), then clean up passkeys and send a **New invite** when it is safe.

**New invite** always removes *all* existing passkeys of that user and ends their sessions.

## Revoke access / lock a compromised account

1. **Users** → **Lock**.
2. All sessions end immediately and the user cannot sign in, even with a valid passkey.
3. To restore access later, click **New invite** (this unlocks the account, removes the old passkeys, and issues a fresh invite link).

Passkey removals, renames, additions, locks, and invites all appear in **Audit** (`auth.passkey.*`, `admin.passkey.remove`, `admin.lock`, `admin.reinvite`).

## Change a user’s email

1. **Users** → **Edit** on that account → **Email** → enter the new address → **Change email**.
2. The address is stored in lower case and must not belong to another user.
3. Their passkeys keep working; they now type the new email to sign in. Their phone may still show the old email next to the passkey — that is harmless.
4. The change is recorded in **Audit** as `admin.user.email` with the old and new email.

You cannot change your own email; ask another RHC admin.

## Recover the only RHC admin

Prevention: keep at least two RHC admins, each with a backup passkey. Then any admin can fix another with **Remove passkey** or **New invite** as above.

If the only RHC admin can no longer sign in:

1. **They still have another passkey:** sign in with it and remove the lost one under **Account & passkeys**.
2. **The admin account is locked, or never finished enrolling (pending):** use the bootstrap invite.
   1. Set `BOOTSTRAP_ADMIN_EMAIL` to that admin’s email and `BOOTSTRAP_REISSUE=true`, then redeploy / restart.
   2. On start, Beacon removes that account’s passkeys, sets it to pending, ends its sessions, and logs a new invite URL (`bootstrap refreshed invite for pending admin`). Audit: `bootstrap.reinvite`.
   3. Open the URL on the real host and create a passkey.
   4. Set `BOOTSTRAP_REISSUE=false` and redeploy. While it stays `true`, every restart mints another invite until the passkey is enrolled.
3. **The admin account is active but every passkey is lost:** the bootstrap does **not** help here (it skips an active admin who has a passkey). Run the one-off recovery command where the app’s `DATABASE_URL` and `BASE_URL` are set:
   - Railway: `railway ssh` into the app service, then `/app/beacon recover-admin rhc@example.org`.
   - Laptop: `DATABASE_URL='postgres://…' BASE_URL='https://beacon.magiconair.net' go run ./cmd/beacon recover-admin rhc@example.org`.

   It only works for `rhc_admin` accounts. It removes that admin’s passkeys, sets the account to pending, ends its sessions, and prints a new invite URL (`=== Beacon invite ===`). Audit: `recovery.admin.reinvite`. Open the URL on the real host and create a passkey.

How the bootstrap behaves on every server start when `BOOTSTRAP_ADMIN_EMAIL` is set:

| Situation | `BOOTSTRAP_REISSUE=false` | `BOOTSTRAP_REISSUE=true` |
|--|--|--|
| Database has no users | creates the RHC admin + logs invite | same |
| Email not found (other users exist) | nothing | nothing |
| Account active with at least one passkey | nothing | nothing |
| Account pending, locked, or without passkeys | nothing | removes passkeys, new invite in logs |

The bootstrap does not check the role of an existing account with that email, so only ever set it to an RHC admin’s address.

## Residents

- Record **nickname + arrival** only. Do not enter legal names or government IDs in Beacon.
- Nicknames must **start with a letter**. Folded nickname keys are **permanently reserved** per safe house (never reused after correction or departure). Use **Correct** to fix a nickname in place (same resident; former spelling is not kept on the record).
- Arrival date may be at most `ARRIVAL_FUTURE_DAYS` ahead of today (default **1**; set `0` to disallow any future date).
- Mark departure when the resident leaves.
- Use the **profile** control in the header (email/role) to log out or open **Account & passkeys**.

## Monthly reporting

- **Reports** builds the monthly report (formerly Document 50) for one safe house from resident records and expenses as they are when you open it. There is no draft, submit or final version; corrections are made to the underlying records.
- Bed-nights count each night a resident slept in the house; the departure night is not counted. For the current month only nights before today are counted and the report says "Month in progress".
- Average occupancy needs the house's approved sleeping places (set on **Houses**).
- **Safeguarding concerns** shows how many concerns were reported in the month and lists every concern open at some point in the month, including open ones from earlier months, by incident identifier only. Details stay in Document 37 with the RHL Safeguarding Contact.
- **Operational problems or changes** lists every Operations record active during the month; resolved or closed records show their status and date.
- RHL and RHC admins with several houses see an overview first, including concerns reported and problems still open at month end; open a house for its full report.
- Once a month has ended, the Agent of the house or an RHL user confirms on the report that data entry is complete (**Confirm data entry complete**). RHC admins can see confirmations but do not confirm. Beacon records who confirmed and when, and keeps every earlier confirmation.
- If residents, expenses, safeguarding concerns, operational problems or approved sleeping places for that month change after confirmation, the report shows "Data changed after confirmation" and the month must be confirmed again. A departure or closure after the month does not count as a change.
- Monthly data are due by the end of the seventh day after the month (UTC). After that the month is overdue until its current data are confirmed.
- **Dashboard** (RHC and RHL admins, under **More** on phones) lists every active safe house with the latest month confirmed and the status of the most recent month that has ended: Confirmed, Changed after confirmation, Due or Overdue with days overdue. **History** shows the last 12 months of a house.
- **Download CSV** exports one house and month.
- Print from the browser if RHC needs a paper copy.

## Expense and operations categories

The categories offered for expenses and on **Operations** live in the database tables `expense_categories` and `operational_issue_categories` (`key`, `label`, `sort_order`, `active`). Change them only with a new numbered migration in `internal/migrate/sql/`, so every environment gets the same list. Forms offer active categories in `sort_order`; lists, reports, CSV and history always show the current label of the stored key.

- **Add:** insert a row. The key is permanent and must be lower-case snake case (`a-z`, `0-9`, `_`); pick a `sort_order` between the neighbours.

  ```sql
  INSERT INTO expense_categories (key, label, sort_order) VALUES ('medical', 'Medical', 65);
  ```

- **Rename:** change the label only. Existing records and past reports show the new label; confirmed months stay confirmed.

  ```sql
  UPDATE operational_issue_categories SET label = 'Staffing or Agent handover' WHERE key = 'staffing_agent';
  ```

- **Retire:** set `active = false`. The category disappears from the forms, existing records keep it and its label, and editing such a record asks for a current category. Set `active = true` to bring it back.

  ```sql
  UPDATE expense_categories SET active = false WHERE key = 'communication';
  ```

Do not delete categories (the database refuses while any record uses them) and do not change keys: a key change is copied to every record and marks the confirmed months of those records as changed. Keep the expense category `other`; expenses logged without a category use it.

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
