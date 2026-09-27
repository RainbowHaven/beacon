#!/usr/bin/env bash
# Backup Beacon Postgres (receipts are in the DB as BYTEA).
#
# Local Compose:
#   ./scripts/backup-prod.sh
#
# Railway (with railway CLI linked to the project):
#   DATABASE_URL="$(railway variables --service Postgres --kv | sed -n 's/^DATABASE_URL=//p')" \
#     ./scripts/backup-prod.sh
# Or open a shell against the DB and pipe:
#   railway connect Postgres   # then use pg_dump from your laptop with the public URL
#
# Copy the resulting file off the machine / out of Railway (object storage, encrypted drive).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

OUT_DIR="${BACKUP_DIR:-$ROOT/.local/backups}"
mkdir -p "$OUT_DIR"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="$OUT_DIR/beacon-$STAMP.dump"

if [[ -n "${DATABASE_URL:-}" ]]; then
  echo "Dumping via DATABASE_URL → $OUT"
  pg_dump --format=custom --no-owner --no-acl --dbname="$DATABASE_URL" --file="$OUT"
elif command -v docker >/dev/null 2>&1 || command -v podman >/dev/null 2>&1; then
  echo "Dumping via local Compose db service → $OUT"
  ./scripts/compose.sh exec -T db pg_dump -U beacon -d beacon --format=custom --no-owner --no-acl >"$OUT"
else
  echo "Set DATABASE_URL or start local Compose Postgres." >&2
  exit 1
fi

ls -lh "$OUT"
echo "OK: backup written. Store a copy off-box. Restore example:"
echo "  pg_restore --clean --if-exists --no-owner --dbname=\"\$DATABASE_URL\" $OUT"
