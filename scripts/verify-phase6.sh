#!/usr/bin/env bash
# Phase 6 local verification: receipts-in-Postgres, Docker image, deploy config.
# Does NOT drop the database. Does not require a Railway account.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "OK: $*"; }

echo "==> Beacon Phase 6 verification"
echo

export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
export DATABASE_URL="${DATABASE_URL:-postgres://beacon:beacon@127.0.0.1:5433/beacon?sslmode=disable}"
export BOOTSTRAP_ADMIN_EMAIL="${BOOTSTRAP_ADMIN_EMAIL:-rhc@example.com}"
export BASE_URL="${BASE_URL:-http://localhost:8080}"
export WEBAUTHN_RP_ID="${WEBAUTHN_RP_ID:-localhost}"
export WEBAUTHN_RP_ORIGINS="${WEBAUTHN_RP_ORIGINS:-http://localhost:8080,http://127.0.0.1:8080}"
export SECURE_COOKIES=false

test -f railway.toml || fail "railway.toml missing"
test -f deploy/.env.example || fail "deploy/.env.example missing"
test -f deploy/.env.staging.example || fail "deploy/.env.staging.example missing"
test -f deploy/README.md || fail "deploy/README.md missing"
test -f deploy/docker-compose.yml || fail "deploy/docker-compose.yml missing"
test -f scripts/backup-prod.sh || fail "scripts/backup-prod.sh missing"
test -f scripts/wipe-postgres-schema.sh || fail "scripts/wipe-postgres-schema.sh missing"
test -f .github/workflows/staging-deploy.yml || fail "staging-deploy workflow missing"
grep -q 'beacon.magiconair.net' deploy/.env.example || fail "deploy/.env.example missing hostname"
grep -q 'pull_request' .github/workflows/staging-deploy.yml || fail "staging-deploy workflow missing pull_request trigger"
grep -q 'opened' .github/workflows/staging-deploy.yml || fail "staging-deploy workflow missing opened"
grep -q 'synchronize' .github/workflows/staging-deploy.yml || fail "staging-deploy workflow missing synchronize"
! grep -q '/deploy-staging' .github/workflows/staging-deploy.yml || fail "staging-deploy should not use /deploy-staging slash command"
grep -q 'wipe-schema' cmd/beacon/main.go || fail "beacon missing wipe-schema command"
grep -q 'case "server"' cmd/beacon/main.go || fail "beacon missing server command"
grep -q 'BEACON_ALLOW_SCHEMA_WIPE' cmd/beacon/main.go || fail "beacon missing BEACON_ALLOW_SCHEMA_WIPE gate"
grep -q 'BEACON_ALLOW_SCHEMA_WIPE' deploy/.env.staging.example || fail "staging env example missing BEACON_ALLOW_SCHEMA_WIPE"
grep -q 'wipe-schema' deploy/README.md || fail "deploy/README.md missing wipe-schema pre-deploy"
! grep -q 'STAGING_DATABASE_URL' .github/workflows/staging-deploy.yml || fail "staging workflow should not use STAGING_DATABASE_URL"
! grep -q 'Dockerfile.staging' deploy/README.md || fail "deploy/README.md should not reference Dockerfile.staging"
! grep -q 'IDENTITY_PUBLIC_KEY' deploy/.env.example || fail "deploy/.env.example still has IDENTITY_*"
grep -q 'healthcheckPath' railway.toml || fail "railway.toml missing healthcheck"
pass "deploy config present"

chmod +x scripts/compose.sh scripts/verify-phase6.sh scripts/backup-prod.sh scripts/wipe-postgres-schema.sh

./scripts/compose.sh up -d db
echo "Waiting for Postgres..."
for _ in $(seq 1 40); do
  if ./scripts/compose.sh exec -T db pg_isready -U beacon -d beacon >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

echo "==> Expense + receipt-in-DB tests"
go test ./internal/server/ -run 'TestExpenseCreateAndMonthlyTotals' -count=1
pass "expense/receipt tests"

echo "==> Dockerfile build"
if command -v docker >/dev/null 2>&1; then
  docker build -t beacon:phase6-verify .
elif command -v podman >/dev/null 2>&1; then
  podman build -t beacon:phase6-verify .
else
  fail "docker or podman required to build image"
fi
pass "image builds"

echo "==> Compose config validate (deploy)"
POSTGRES_PASSWORD=verify \
DATABASE_URL=postgres://beacon:verify@db:5432/beacon \
BOOTSTRAP_ADMIN_EMAIL=rhc@example.com \
./scripts/compose.sh -f deploy/docker-compose.yml config >/dev/null
pass "deploy compose validates"

echo
echo "Phase 6 local checks passed."
echo "Cutover: Railway project + Postgres + vars from deploy/.env.example"
echo "         custom domain beacon.magiconair.net → then enroll RHC passkey."
echo "Backup:  ./scripts/backup-prod.sh (or DATABASE_URL=… ./scripts/backup-prod.sh)"
