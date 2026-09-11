#!/usr/bin/env bash
# Phase 5 local verification: org/houses admin + tenancy isolation tests.
# Does NOT drop the database.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "OK: $*"; }

echo "==> Beacon Phase 5 verification"
echo

export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
export DATABASE_URL="${DATABASE_URL:-postgres://beacon:beacon@127.0.0.1:5433/beacon?sslmode=disable}"
export BOOTSTRAP_ADMIN_EMAIL="${BOOTSTRAP_ADMIN_EMAIL:-rhc@example.com}"
export BASE_URL="${BASE_URL:-http://localhost:8080}"
export WEBAUTHN_RP_ID="${WEBAUTHN_RP_ID:-localhost}"
export WEBAUTHN_RP_ORIGINS="${WEBAUTHN_RP_ORIGINS:-http://localhost:8080,http://127.0.0.1:8080}"
export SECURE_COOKIES=false
export RECEIPT_DIR="${RECEIPT_DIR:-/tmp/beacon-phase5-receipts}"
# shellcheck disable=SC1091
source "$ROOT/scripts/lib/identity-env.sh"
identity_env_prepare "$ROOT"
unset IDENTITY_PRIVATE_KEY_B64

chmod +x scripts/compose.sh scripts/verify-phase5.sh
mkdir -p "$RECEIPT_DIR"

./scripts/compose.sh up -d db
echo "Waiting for Postgres..."
for _ in $(seq 1 40); do
  if ./scripts/compose.sh exec -T db pg_isready -U beacon -d beacon >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

go test ./internal/server/ -run 'TestOrgHousesAndUserScope|TestInviteRegisterLoginLock|TestOccupant' -count=1
pass "org + scoping tests"

go test ./...
pass "go test ./..."

[[ -f web/templates/admin_houses.html ]] || fail "admin_houses.html missing"
[[ -f web/templates/admin_user_edit.html ]] || fail "admin_user_edit.html missing"
pass "org templates present"

if command -v lsof >/dev/null 2>&1; then
  pids="$(lsof -t -iTCP:8080 -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$pids" ]]; then
    kill $pids >/dev/null 2>&1 || true
    sleep 0.5
  fi
fi

pkill -f '/tmp/beacon-phase5' >/dev/null 2>&1 || true
go build -o /tmp/beacon-phase5 ./cmd/beacon
/tmp/beacon-phase5 > /tmp/beacon-phase5.log 2>&1 &
APP_PID=$!
cleanup() {
  kill "$APP_PID" >/dev/null 2>&1 || true
  wait "$APP_PID" 2>/dev/null || true
}
trap cleanup EXIT

ready=0
for _ in $(seq 1 40); do
  if ! kill -0 "$APP_PID" 2>/dev/null; then
    echo "---- app log ----"; cat /tmp/beacon-phase5.log; fail "app exited before ready"
  fi
  if curl -fsS http://127.0.0.1:8080/readyz >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.25
done
[[ "$ready" -eq 1 ]] || { echo "---- app log ----"; cat /tmp/beacon-phase5.log; fail "app not ready"; }

for path in /admin/houses /admin/users; do
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:8080$path")"
  [[ "$code" == "303" || "$code" == "302" ]] || fail "$path should redirect when logged out (got $code)"
done
pass "houses and users require auth"

echo
echo "Phase 5 local verification passed."
echo "Manual: RHC → /admin/houses add second house; invite manager scoped to it; confirm isolation."
