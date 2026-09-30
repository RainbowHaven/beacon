# AGENTS.md — Beacon handoff for coding agents

## Mission

Beacon helps Rainbow Haven run LGBTQ safe houses: occupancy, expenses, and RHC reporting. For residents Beacon stores a nickname, stay dates, and reporting demographics (country of origin, gender, birth year). Resident legal identity is **not** collected in Beacon.

## Hard constraints (do not violate)

- **No React. No HTMX.** Server-rendered Go multi-page HTML + minimal vanilla JS only where required (optional receipt image resize).
- **Auth:** passkeys for daily login; RHC invite / lock / recovery. No standing field passwords as the default path.
- **PII:** do not collect legal names, UNHCR file numbers, or other government IDs. Nickname + arrival/departure + reporting demographics (country, gender, birth year) only.
- **Monthly reports (occupancy, demographic aggregates, expense totals, receipt counts) must not require any private key.** The monthly report replaces Document 50 and is built in `internal/report`.
- **RBAC and authz live in Go**, not in Supabase RLS/PostgREST (Postgres is storage).
- **Local-first:** every phase must run on a laptop via Compose/`go test`/scripts before cloud deploy.
- **CLA:** external contributors must sign [CLA.md](./CLA.md); do not pre-create `signatures/version1/cla.json`.

## Stack

- Go 1.27, `cmd/beacon`, Postgres 18
- Compose: `./scripts/compose.sh` (Docker if available, else Podman); prefer `make run` / `make test` / `make verify-phaseN`
- Verify Phase 0: `make verify` (`./scripts/verify-bootstrap.sh`)
- Verify Phase 1: `make verify-phase1`
- Verify Phase 2: `make verify-phase2`
- Verify Phase 3: `make verify-phase3`
- Verify Phase 4: `make verify-phase4`
- Verify Phase 5: `make verify-phase5`
- Verify Phase 6: `make verify-phase6`
- Verify CLA docs/workflow: `make verify-cla`
- **Receipts** are stored in Postgres (`BYTEA` on expenses), not on the filesystem.
- **Production:** Railway (Dockerfile + managed Postgres). Publish from a laptop with `make publish-staging` and `make publish-{patch,minor,major}` — see [deploy/README.md](./deploy/README.md) and [OPERATOR.md](./OPERATOR.md). No GitHub Actions deploy.

## Roles

`safe_house_manager` | `rhl_admin` | `rhc_admin` — enforce server-side on every route.

## Out of MVP

MCP/AI reporting, offline write queue, expense approvals, OCR, Shamir key splitting, duress mode.
