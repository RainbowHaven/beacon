#!/usr/bin/env bash
# Compose wrapper that prefers Docker when available, otherwise Podman machine.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  exec docker compose "$@"
fi

if command -v podman >/dev/null 2>&1; then
  if ! podman info >/dev/null 2>&1; then
    if podman machine inspect >/dev/null 2>&1; then
      echo "Starting Podman machine..." >&2
      podman machine start >&2
      ready=0
      for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25 26 27 28 29 30; do
        if podman info >/dev/null 2>&1; then
          ready=1
          break
        fi
        sleep 1
      done
      if [[ "$ready" -ne 1 ]]; then
        echo "Podman machine started but the API is not ready." >&2
        exit 1
      fi
    else
      echo "Podman is installed but no machine exists. Run: podman machine init && podman machine start" >&2
      exit 1
    fi
  fi

  # Prefer the macOS docker.sock symlink when the machine API is up.
  if [[ -S /var/run/docker.sock ]] && curl -sf --max-time 2 --unix-socket /var/run/docker.sock http://localhost/_ping >/dev/null 2>&1; then
    export DOCKER_HOST="unix:///var/run/docker.sock"
  else
    sock="$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}' 2>/dev/null || true)"
    if [[ -n "$sock" && -S "$sock" ]]; then
      export DOCKER_HOST="unix://${sock}"
    fi
  fi
  if podman compose version >/dev/null 2>&1; then
    exec podman compose "$@"
  fi
  if command -v docker-compose >/dev/null 2>&1; then
    exec docker-compose "$@"
  fi
  exec podman compose "$@"
fi

echo "Neither a working Docker daemon nor Podman was found." >&2
exit 1
