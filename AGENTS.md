# AGENTS.md — Beacon handoff for coding agents

## Mission

Beacon helps Rainbow Haven run LGBTQ safe houses: occupancy, expenses, and RHC reporting — without letting field managers recover resident legal identities under duress.

## Hard constraints (do not violate)

- **No React. No HTMX.** Server-rendered Go multi-page HTML + minimal vanilla JS only where required (client-side sealed-box encrypt, optional receipt image resize).
- **Auth:** passkeys for daily login; RHC invite / lock / recovery. No standing field passwords as the default path.
- **PII:** seal only identity fields (legal name, refugee/UN ID) client-side with libsodium sealed boxes. Nickname + arrival/departure stay plaintext for headcount.
- **Private key never on server, DB, or manager devices.** Break-glass decrypt is RHC browser-only.
- **Monthly reports (headcount, expense totals, receipt counts) must not require the private key.**
- **RBAC and authz live in Go**, not in Supabase RLS/PostgREST (Postgres is storage).
- **Local-first:** every phase must run on a laptop via Compose/`go test`/scripts before cloud deploy.
- **CLA:** external contributors must sign [CLA.md](./CLA.md); do not pre-create `signatures/version1/cla.json`.

## Stack

- Go 1.27, `cmd/beacon`, Postgres 16
- Compose: `./scripts/compose.sh` (Docker if available, else Podman); prefer `make run` / `make test` / `make verify-phaseN`
- Verify Phase 0: `make verify` (`./scripts/verify-bootstrap.sh`)
- Verify Phase 1: `make verify-phase1`
- Verify Phase 2: `make verify-phase2`
- Verify Phase 3: `make verify-phase3`
- Verify Phase 4: `make verify-phase4`
- Verify Phase 5: `make verify-phase5`
- Verify CLA docs/workflow: `make verify-cla`
- Identity keys: `make keys` → `.local/` (gitignored). Never commit private keys.

## Roles

`safe_house_manager` | `rhl_admin` | `rhc_admin` — enforce server-side on every route.

## Out of MVP

MCP/AI reporting, offline write queue, demographic aggregates, expense approvals, OCR, Shamir key splitting, duress mode.
