#!/usr/bin/env bash
# Wipe the public schema of a Postgres database (pre-pilot / staging only).
# Usage: DATABASE_URL='postgres://…' ./scripts/wipe-postgres-schema.sh
set -euo pipefail

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "DATABASE_URL is required" >&2
  exit 1
fi

# Refuse obvious production hostnames unless FORCE_WIPE=1
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
