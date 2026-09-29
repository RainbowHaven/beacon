#!/usr/bin/env bash
# Phase 2 local verification: occupant create/list tests + HTTP smoke.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "OK: $*"; }

echo "==> Beacon Phase 2 verification"
echo

export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
export DATABASE_URL="${DATABASE_URL:-postgres://beacon:beacon@127.0.0.1:5433/beacon?sslmode=disable}"
export BOOTSTRAP_ADMIN_EMAIL="${BOOTSTRAP_ADMIN_EMAIL:-rhc@example.com}"
export BASE_URL="${BASE_URL:-http://localhost:8080}"
export WEBAUTHN_RP_ID="${WEBAUTHN_RP_ID:-localhost}"
export WEBAUTHN_RP_ORIGINS="${WEBAUTHN_RP_ORIGINS:-http://localhost:8080,http://127.0.0.1:8080}"
export SECURE_COOKIES=false

chmod +x scripts/compose.sh scripts/verify-phase2.sh

./scripts/compose.sh up -d db
echo "Waiting for Postgres..."
for _ in $(seq 1 40); do
  if ./scripts/compose.sh exec -T db pg_isready -U beacon -d beacon >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

go test ./internal/server/ -run 'TestOccupantCreateScoped' -count=1
pass "occupant tests"

./scripts/compose.sh exec -T db psql -U beacon -d beacon -v ON_ERROR_STOP=1 <<'SQL'
DROP TABLE IF EXISTS expenses, occupants, audit_events, sessions, webauthn_challenges, webauthn_credentials, invites, users, safe_houses, rhls, schema_migrations CASCADE;
DROP TYPE IF EXISTS user_status, user_role CASCADE;
SQL

if command -v lsof >/dev/null 2>&1; then
  pids="$(lsof -t -iTCP:8080 -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$pids" ]]; then
    kill $pids >/dev/null 2>&1 || true
    sleep 0.5
  fi
fi
pkill -f '/tmp/beacon-phase2' >/dev/null 2>&1 || true
go build -o /tmp/beacon-phase2 ./cmd/beacon
/tmp/beacon-phase2 server > /tmp/beacon-phase2.log 2>&1 &
APP_PID=$!
cleanup() {
  kill "$APP_PID" >/dev/null 2>&1 || true
  wait "$APP_PID" 2>/dev/null || true
}
trap cleanup EXIT

ready=0
for _ in $(seq 1 40); do
  if ! kill -0 "$APP_PID" 2>/dev/null; then
    echo "---- app log ----"; cat /tmp/beacon-phase2.log; fail "app exited before ready"
  fi
  if curl -fsS http://127.0.0.1:8080/readyz >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.25
done
[[ "$ready" -eq 1 ]] || { echo "---- app log ----"; cat /tmp/beacon-phase2.log; fail "app not ready"; }

curl -fsS http://127.0.0.1:8080/healthz | grep -q ok || fail "healthz"
pass "GET /healthz"

code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/occupants)"
[[ "$code" == "303" || "$code" == "302" ]] || fail "occupants should redirect when logged out (got $code)"
pass "occupants requires auth"

code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/admin/break-glass)"
[[ "$code" == "404" ]] || fail "break-glass should be gone (got $code)"
pass "break-glass removed"

echo
echo "Phase 2 local verification passed."
echo "Manual check: log in, add an occupant with nickname + arrival only."
