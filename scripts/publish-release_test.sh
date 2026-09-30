#!/usr/bin/env bash
# Unit checks for release tag calculation. Does not call Railway or the keychain.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=publish-release.sh
source "$ROOT/scripts/publish-release.sh"

fail() { echo "FAIL: $*" >&2; exit 1; }

[[ "$(semver_base "v1.2.3")" == "v1.2.3" ]] || fail "semver_base exact"
[[ "$(semver_base "v1.2.3-menu.1")" == "v1.2.3" ]] || fail "semver_base strips suffix"
[[ "$(semver_base "v1.2.3-staging-menu.1")" == "v1.2.3" ]] || fail "semver_base strips staging suffix"
[[ "$(bump_semver "v1.2.3" patch)" == "v1.2.4" ]] || fail "patch bump"
[[ "$(bump_semver "v1.2.3" minor)" == "v1.3.0" ]] || fail "minor bump"
[[ "$(bump_semver "v1.2.3" major)" == "v2.0.0" ]] || fail "major bump"
[[ "$(bump_semver "v0.0.0" patch)" == "v0.0.1" ]] || fail "patch from zero"
[[ "$(bump_semver "v0.0.0" minor)" == "v0.1.0" ]] || fail "minor from zero"
[[ "$(bump_semver "v0.0.0" major)" == "v1.0.0" ]] || fail "major from zero"
valid_suffix "menu.1" || fail "menu.1 should be valid"
if valid_suffix "menu.1 "; then fail "trailing space should be invalid"; fi
if valid_suffix ""; then fail "empty suffix should be invalid"; fi
if valid_suffix "menu/1"; then fail "slash should be invalid"; fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
git -C "$tmp" init -q -b main
echo a >"$tmp/f"
git -C "$tmp" add f
git -C "$tmp" -c user.email=test@example.com -c user.name=test -c commit.gpgsign=false commit -q -m init
(
  cd "$tmp"
  [[ "$(latest_described_tag)" == "v0.0.0" ]] || fail "missing tag should fall back to v0.0.0"
  [[ "$(staging_tag "menu.1")" == "v0.0.0-staging-menu.1" ]] || fail "first staging tag"
  [[ "$(highest_release_tag)" == "v0.0.0" ]] || fail "no release tag yet"
  git -c tag.gpgsign=false tag v1.2.3
  git -c tag.gpgsign=false tag v1.2.3-staging-menu.1
  [[ "$(staging_tag "menu.2")" == "v1.2.3-staging-menu.2" ]] || fail "staging tag on top of a staging tag"
  echo b >>f
  git -c user.email=test@example.com -c user.name=test -c commit.gpgsign=false commit -q -am 'next'
  git -c tag.gpgsign=false tag v1.10.0
  [[ "$(semver_base "$(latest_described_tag)")" == "v1.10.0" ]] || fail "describe should see v1.10.0"
  [[ "$(staging_tag "menu.2")" == "v1.10.0-staging-menu.2" ]] || fail "staging suffix on latest version"
  [[ "$(highest_release_tag)" == "v1.10.0" ]] || fail "highest release should ignore the staging suffix and pick v1.10.0"
  [[ "$(bump_semver "$(highest_release_tag)" patch)" == "v1.10.1" ]] || fail "patch after v1.10.0"
)

echo "OK: publish-release tag calculation"
