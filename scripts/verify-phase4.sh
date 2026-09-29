#!/usr/bin/env bash
# Phase 4 local verification: audit route + operator docs (identity vault removed).
# Does NOT drop the database (unlike phase 1–3 verifies).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "OK: $*"; }

echo "==> Beacon Phase 4 verification"
echo

export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
export DATABASE_URL="${DATABASE_URL:-postgres://beacon:beacon@127.0.0.1:5433/beacon?sslmode=disable}"
export BOOTSTRAP_ADMIN_EMAIL="${BOOTSTRAP_ADMIN_EMAIL:-rhc@example.com}"
export BASE_URL="${BASE_URL:-http://localhost:8080}"
export WEBAUTHN_RP_ID="${WEBAUTHN_RP_ID:-localhost}"
export WEBAUTHN_RP_ORIGINS="${WEBAUTHN_RP_ORIGINS:-http://localhost:8080,http://127.0.0.1:8080}"
export SECURE_COOKIES=false

chmod +x scripts/compose.sh scripts/verify-phase4.sh

./scripts/compose.sh up -d db
echo "Waiting for Postgres..."
for _ in $(seq 1 40); do
  if ./scripts/compose.sh exec -T db pg_isready -U beacon -d beacon >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

go test ./...
pass "go test ./..."

[[ -f OPERATOR.md ]] || fail "OPERATOR.md missing"
grep -q 'Invite a manager' OPERATOR.md || fail "OPERATOR.md incomplete"
! grep -qi 'break-glass' OPERATOR.md || fail "OPERATOR.md still mentions break-glass"
pass "OPERATOR.md present"

if command -v lsof >/dev/null 2>&1; then
  pids="$(lsof -t -iTCP:8080 -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$pids" ]]; then
    kill $pids >/dev/null 2>&1 || true
    sleep 0.5
  fi
fi

pkill -f '/tmp/beacon-phase4' >/dev/null 2>&1 || true
go build -o /tmp/beacon-phase4 ./cmd/beacon
/tmp/beacon-phase4 server > /tmp/beacon-phase4.log 2>&1 &
APP_PID=$!
cleanup() {
  kill "$APP_PID" >/dev/null 2>&1 || true
  wait "$APP_PID" 2>/dev/null || true
}
trap cleanup EXIT

ready=0
for _ in $(seq 1 40); do
  if ! kill -0 "$APP_PID" 2>/dev/null; then
    echo "---- app log ----"; cat /tmp/beacon-phase4.log; fail "app exited before ready"
  fi
  if curl -fsS http://127.0.0.1:8080/readyz >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.25
done
[[ "$ready" -eq 1 ]] || { echo "---- app log ----"; cat /tmp/beacon-phase4.log; fail "app not ready"; }

code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/admin/audit)"
[[ "$code" == "303" || "$code" == "302" ]] || fail "audit should redirect when logged out (got $code)"
pass "audit requires auth"

code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/admin/break-glass)"
[[ "$code" == "404" ]] || fail "break-glass should be gone (got $code)"
pass "break-glass removed"

echo
echo "Phase 4 local verification passed."
echo "Note: this script does NOT drop the database."
