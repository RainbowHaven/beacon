#!/usr/bin/env bash
# Wipe the public schema of a Postgres database (pre-pilot / staging only).
# Usage: DATABASE_URL='postgres://…' ./scripts/wipe-postgres-schema.sh
#
# For Railway: use the Postgres *public TCP proxy* URL (Connect → public URL),
# not postgres.railway.internal — GitHub Actions cannot reach the private host.
set -euo pipefail

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "DATABASE_URL is required" >&2
  exit 1
fi

case "$DATABASE_URL" in
  *railway.internal*)
    echo "DATABASE_URL uses railway.internal (private network)." >&2
    echo "GitHub Actions cannot reach it. In Railway → staging Postgres →" >&2
    echo "Connect / Networking, enable the public TCP proxy and put that URL" >&2
    echo "in the STAGING_DATABASE_URL GitHub secret. Keep the app's DATABASE_URL" >&2
    echo "as \${{Postgres.DATABASE_URL}} (private) for the service itself." >&2
    exit 1
    ;;
esac

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
