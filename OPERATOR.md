# Operator runbook (Beacon)

Short procedures for Rainbow Haven Coordinating (RHC). No Go knowledge required.

## Keys (identity vault)

- **Public key** lives in app config (`IDENTITY_PUBLIC_KEY_B64` + `IDENTITY_KEY_ID`).
- **Private key** stays offline (printed / vault). Never put it in env, Docker, DB, or manager devices.
- Break-glass: RHC opens `/admin/break-glass`, pastes the private key in the browser, decrypts one nickname at a time, then clicks **Clear key**.
- Each sealed-identity fetch is audited as `identity.break_glass`.

Local-dev private key (NOT for production): matching the compose public key is used only in `scripts/verify-phase2.sh` / `verify-phase4.sh` for crypto checks — do not set it as a server env var.

## Invite a manager

1. Log in as RHC → **Users** → **Invite user**.
2. Create user (role + house/RHL scope) → copy invite URL (shown once on Users).
3. Manager opens the URL on `http://localhost:8080` (or the real `BASE_URL` host) and enrolls a passkey.

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

`go test` and `./scripts/verify-phase{1,2,3}.sh` **drop** the database schema. `verify-phase4.sh` does not. Do not run drop scripts against a DB you care about keeping (enrolled passkeys will disappear).
