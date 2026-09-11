#!/usr/bin/env bash
# Shared identity key loading for verify scripts.
# Public key may come from env or .local/; private key is never committed.
# shellcheck shell=bash

identity_env_prepare() {
  local root="${1:-.}"
  local local_dir="$root/.local"
  local pub_file="$local_dir/identity.pub.b64"
  local priv_file="$local_dir/identity.priv.b64"

  if [[ -z "${IDENTITY_PUBLIC_KEY_B64:-}" && -f "$pub_file" ]]; then
    IDENTITY_PUBLIC_KEY_B64="$(tr -d '[:space:]' <"$pub_file")"
    export IDENTITY_PUBLIC_KEY_B64
  fi
  if [[ -z "${IDENTITY_PRIVATE_KEY_B64:-}" && -f "$priv_file" ]]; then
    IDENTITY_PRIVATE_KEY_B64="$(tr -d '[:space:]' <"$priv_file")"
    export IDENTITY_PRIVATE_KEY_B64
  fi

  # Ephemeral pair for this process when nothing is configured (CI / fresh clone).
  if [[ -z "${IDENTITY_PUBLIC_KEY_B64:-}" || -z "${IDENTITY_PRIVATE_KEY_B64:-}" ]]; then
    eval "$(GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" go run "$root/scripts/internal/genkeys" -export)"
  fi

  export IDENTITY_KEY_ID="${IDENTITY_KEY_ID:-local-dev-1}"
  # Server must never receive the private key from these scripts' app process env
  # beyond crypto round-trip helpers that run as separate commands.
}
