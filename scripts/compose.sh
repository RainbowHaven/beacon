#!/usr/bin/env bash
# Compose wrapper that prefers Docker when available, otherwise Podman machine.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  exec docker compose "$@"
fi

if command -v podman >/dev/null 2>&1; then
  sock="$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}' 2>/dev/null || true)"
  if [[ -n "$sock" && -S "$sock" ]]; then
    export DOCKER_HOST="unix://${sock}"
  fi
  if command -v docker-compose >/dev/null 2>&1; then
    exec docker-compose "$@"
  fi
  exec podman compose "$@"
fi

echo "Neither a working Docker daemon nor Podman was found." >&2
exit 1
