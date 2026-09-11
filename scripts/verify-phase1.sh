#!/usr/bin/env bash
# Phase 1 local verification: migrations, WebAuthn invite/login/lock tests, HTTP smoke.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "OK: $*"; }

echo "==> Beacon Phase 1 verification"
echo

export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
export DATABASE_URL="${DATABASE_URL:-postgres://beacon:beacon@127.0.0.1:5433/beacon?sslmode=disable}"
export BOOTSTRAP_ADMIN_EMAIL="${BOOTSTRAP_ADMIN_EMAIL:-rhc@example.com}"
export BASE_URL="${BASE_URL:-http://localhost:8080}"
export WEBAUTHN_RP_ID="${WEBAUTHN_RP_ID:-localhost}"
export WEBAUTHN_RP_ORIGINS="${WEBAUTHN_RP_ORIGINS:-http://localhost:8080,http://127.0.0.1:8080}"
export SECURE_COOKIES=false
# shellcheck disable=SC1091
source "$ROOT/scripts/lib/identity-env.sh"
identity_env_prepare "$ROOT"
unset IDENTITY_PRIVATE_KEY_B64

chmod +x scripts/compose.sh scripts/verify-phase1.sh

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

# Fresh schema so bootstrap invite is deterministic for this run.
./scripts/compose.sh exec -T db psql -U beacon -d beacon -v ON_ERROR_STOP=1 <<'SQL'
DROP TABLE IF EXISTS expenses, occupants, audit_events, sessions, webauthn_challenges, webauthn_credentials, invites, users, safe_houses, rhls, schema_migrations CASCADE;
DROP TYPE IF EXISTS user_status, user_role CASCADE;
SQL

# Free :8080 in case a previous beacon (or Compose app) is still listening.
if command -v lsof >/dev/null 2>&1; then
  pids="$(lsof -t -iTCP:8080 -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$pids" ]]; then
    kill $pids >/dev/null 2>&1 || true
    sleep 0.5
  fi
fi
pkill -f '/tmp/beacon-phase1' >/dev/null 2>&1 || true
pkill -f '/tmp/beacon-phase2' >/dev/null 2>&1 || true
go build -o /tmp/beacon-phase1 ./cmd/beacon
/tmp/beacon-phase1 > /tmp/beacon-phase1.log 2>&1 &
APP_PID=$!
cleanup() {
  kill "$APP_PID" >/dev/null 2>&1 || true
  wait "$APP_PID" 2>/dev/null || true
}
trap cleanup EXIT

ready=0
for _ in $(seq 1 40); do
  if curl -fsS http://127.0.0.1:8080/readyz >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.25
done
[[ "$ready" -eq 1 ]] || { echo "---- app log ----"; cat /tmp/beacon-phase1.log; fail "app not ready"; }

curl -fsS http://127.0.0.1:8080/healthz | grep -q ok || fail "healthz"
pass "GET /healthz"

curl -fsS http://127.0.0.1:8080/login | grep -qi passkey || fail "login page"
pass "GET /login"

INVITE_URL="$(awk '{for (i=1;i<=NF;i++) if ($i ~ /^invite_url=/) { sub(/^invite_url=/,"",$i); print $i }}' /tmp/beacon-phase1.log | tail -1)"
[[ -n "$INVITE_URL" ]] || { cat /tmp/beacon-phase1.log; fail "bootstrap invite_url missing from log"; }
curl -fsS "$INVITE_URL" | grep -qi 'Enroll passkey' || fail "invite page for $INVITE_URL"
pass "bootstrap invite page"

code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/admin/users)"
[[ "$code" == "303" || "$code" == "302" ]] || fail "admin users should redirect when logged out (got $code)"
pass "admin users requires auth"

echo
echo "Phase 1 local verification passed."
echo "Manual browser check: open $INVITE_URL on localhost and enroll a passkey."
