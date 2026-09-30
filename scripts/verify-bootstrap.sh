#!/usr/bin/env bash
# Phase 0 local verification: unit tests + compose stack + HTTP checks.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "OK: $*"; }

echo "==> Beacon Phase 0 verification"
echo

command -v go >/dev/null || fail "go not installed"
GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
export GOTOOLCHAIN


go test -p 1 ./...
pass "go test -p 1 ./..."

chmod +x scripts/compose.sh scripts/verify-bootstrap.sh

./scripts/compose.sh up -d --build
cleanup() { ./scripts/compose.sh down -v >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "Waiting for app readiness..."
ready=0
for _ in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:8080/readyz >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done
[[ "$ready" -eq 1 ]] || fail "app did not become ready on :8080"

health="$(curl -fsS http://127.0.0.1:8080/healthz)"
[[ "$(echo "$health" | tr -d '\r')" == "ok" ]] || fail "healthz body=$health"
pass "GET /healthz"

readyz="$(curl -fsS http://127.0.0.1:8080/readyz)"
[[ "$(echo "$readyz" | tr -d '\r')" == "ready" ]] || fail "readyz body=$readyz"
pass "GET /readyz"

home="$(curl -fsS http://127.0.0.1:8080/)"
echo "$home" | grep -q "Beacon" || fail "home page missing Beacon"
pass "GET /"

echo
echo "Phase 0 local verification passed."
