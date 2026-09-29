#!/usr/bin/env bash
# Optional local helper: wipe public schema via psql.
# Prefer: BEACON_ALLOW_SCHEMA_WIPE=true go run ./cmd/beacon wipe-schema
# (or Railway staging pre-deploy: /app/beacon wipe-schema).
#
# Usage: DATABASE_URL='postgres://…' ./scripts/wipe-postgres-schema.sh
set -euo pipefail

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "DATABASE_URL is required" >&2
  exit 1
fi

case "$DATABASE_URL" in
  *railway.internal*)
    echo "DATABASE_URL uses railway.internal (private network)." >&2
    echo "Run wipe inside Railway instead: pre-deploy /app/beacon wipe-schema" >&2
    echo "with BEACON_ALLOW_SCHEMA_WIPE=true on the staging service." >&2
    exit 1
    ;;
esac

if [[ "${FORCE_WIPE:-}" != "1" ]]; then
  case "$DATABASE_URL" in
    *beacon.magiconair.net*|*production*)
      echo "Refusing to wipe what looks like production. Set FORCE_WIPE=1 to override." >&2
      exit 1
      ;;
  esac
fi

psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO CURRENT_USER;
GRANT ALL ON SCHEMA public TO public;
SQL

echo "Schema wiped."
