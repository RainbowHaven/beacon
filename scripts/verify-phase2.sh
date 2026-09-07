#!/usr/bin/env bash
# Phase 2 local verification: sealed-box JS↔Go roundtrip, occupant tests, HTTP smoke.
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
# Public key only on the server. Matching private key used solely in this script's crypto check.
export IDENTITY_PUBLIC_KEY_B64="${IDENTITY_PUBLIC_KEY_B64:-OM1ZQIEru2EWDGtgfLzI6tB3KIYZ30L2mVQTffmAxUQ=}"
export IDENTITY_KEY_ID="${IDENTITY_KEY_ID:-local-dev-1}"
IDENTITY_PRIVATE_KEY_B64="${IDENTITY_PRIVATE_KEY_B64:-kkaOEMtAGbjNKyXqJqzDTP9Un3oUWnyjkGqP9ZuXVbw=}"

chmod +x scripts/compose.sh scripts/verify-phase2.sh

./scripts/compose.sh up -d db
echo "Waiting for Postgres..."
for _ in $(seq 1 40); do
  if ./scripts/compose.sh exec -T db pg_isready -U beacon -d beacon >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

# JS sealed box must open with Go OpenAnonymous (browser crypto compatibility).
if command -v bun >/dev/null 2>&1; then
  CT_B64="$(bun -e '
import { readFileSync } from "fs";
eval(readFileSync("./web/static/js/vendor/nacl-fast.min.js","utf8"));
const { sealAnonymous, bytesToB64 } = await import("./web/static/js/seal.js");
const pub = process.env.IDENTITY_PUBLIC_KEY_B64;
const ct = sealAnonymous(JSON.stringify({legal_name:"Verify",refugee_id:"V-1"}), pub);
process.stdout.write(bytesToB64(ct));
')"
  go run ./scripts/internal/openseal -pub "$IDENTITY_PUBLIC_KEY_B64" -priv "$IDENTITY_PRIVATE_KEY_B64" -ct "$CT_B64" \
    | grep -q '"legal_name":"Verify"' || fail "JS→Go sealed-box roundtrip"
  pass "JS→Go sealed-box roundtrip"
else
  echo "WARN: bun not found; skipping browser seal roundtrip (go tests still cover SealAnonymous)"
fi

go test ./...
pass "go test ./..."

./scripts/compose.sh exec -T db psql -U beacon -d beacon -v ON_ERROR_STOP=1 <<'SQL'
DROP TABLE IF EXISTS occupants, audit_events, sessions, webauthn_challenges, webauthn_credentials, invites, users, safe_houses, rhls, schema_migrations CASCADE;
DROP TYPE IF EXISTS user_status, user_role CASCADE;
SQL

if command -v lsof >/dev/null 2>&1; then
  pids="$(lsof -t -iTCP:8080 -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$pids" ]]; then
    kill $pids >/dev/null 2>&1 || true
    sleep 0.5
  fi
fi
pkill -f '/tmp/beacon-phase1' >/dev/null 2>&1 || true
pkill -f '/tmp/beacon-phase2' >/dev/null 2>&1 || true
go build -o /tmp/beacon-phase2 ./cmd/beacon
/tmp/beacon-phase2 > /tmp/beacon-phase2.log 2>&1 &
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

curl -fsS "http://127.0.0.1:8080/static/js/seal.js" | grep -q sealAnonymous || fail "seal.js missing"
pass "seal.js served"

echo
echo "Phase 2 local verification passed."
echo "Manual check: log in, open /occupants/new, hand off to resident form, confirm nickname-only list."
echo "Keep IDENTITY private key offline — never set it as a server env var."
