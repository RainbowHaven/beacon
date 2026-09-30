#!/usr/bin/env bash
# Tag the current commit and deploy it to Railway.
#
# Staging (annotated tag <latest version>-staging-<suffix>, environment staging):
#   make publish-staging SUFFIX=menu.1
#
# Production (annotated semver tag, environment production):
#   make publish-patch
#   make publish-minor
#   make publish-major
#
# Railway tokens are read from the macOS login keychain:
#   security add-generic-password -s RAILWAY_BEACON_STAGING_TOKEN -a "$USER" -w
#   security add-generic-password -s RAILWAY_BEACON_PRODUCTION_TOKEN -a "$USER" -w
set -euo pipefail

RAILWAY_PROJECT_ID="c8091dad-9100-4147-a749-f5baea7a3372"
RAILWAY_SERVICE="beacon"
STAGING_TOKEN_SERVICE="RAILWAY_BEACON_STAGING_TOKEN"
PRODUCTION_TOKEN_SERVICE="RAILWAY_BEACON_PRODUCTION_TOKEN"

TAG=""
TAGGED=0
PUSHED=0

cleanup() {
  local status=$?
  if [[ $status -ne 0 && $TAGGED -eq 1 && $PUSHED -eq 0 && -n "$TAG" ]]; then
    git tag -d "$TAG" >/dev/null 2>&1 || true
    echo "Removed local tag ${TAG} after a failed publish." >&2
  fi
}

# semver_base prints vMAJOR.MINOR.PATCH from a tag like v1.2.3 or v1.2.3-staging-menu.1.
semver_base() {
  local raw="$1"
  if [[ "$raw" =~ ^(v[0-9]+\.[0-9]+\.[0-9]+) ]]; then
    printf '%s\n' "${BASH_REMATCH[1]}"
    return 0
  fi
  printf '%s\n' "$raw"
}

# latest_described_tag is `git describe --tags --abbrev=0`, or v0.0.0 when none exist.
latest_described_tag() {
  local raw
  raw="$(git describe --tags --abbrev=0 2>/dev/null || true)"
  if [[ -z "$raw" ]]; then
    printf '%s\n' "v0.0.0"
    return 0
  fi
  printf '%s\n' "$raw"
}

# staging_tag appends -staging-<suffix> to the semver of the latest tag reachable from HEAD.
# v1.2.3 or v1.2.3-staging-older both become v1.2.3-staging-<suffix>.
staging_tag() {
  local suffix="$1"
  local raw base
  raw="$(latest_described_tag)"
  base="$(semver_base "$raw")"
  printf '%s-staging-%s\n' "$base" "$suffix"
}

# highest_release_tag is the greatest exact vMAJOR.MINOR.PATCH tag, or v0.0.0.
highest_release_tag() {
  local tag
  while IFS= read -r tag; do
    [[ -z "$tag" ]] && continue
    if [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      printf '%s\n' "$tag"
      return 0
    fi
  done < <(git tag --list --sort=-v:refname)
  printf '%s\n' "v0.0.0"
}

bump_semver() {
  local base="$1"
  local kind="$2"
  local ver major minor patch
  ver="${base#v}"
  IFS=. read -r major minor patch <<<"$ver"
  case "$kind" in
    major) major=$((major + 1)); minor=0; patch=0 ;;
    minor) minor=$((minor + 1)); patch=0 ;;
    patch) patch=$((patch + 1)) ;;
    *)
      echo "Unknown release kind: ${kind}" >&2
      return 1
      ;;
  esac
  printf 'v%d.%d.%d\n' "$major" "$minor" "$patch"
}

valid_suffix() {
  [[ "$1" =~ ^[A-Za-z0-9]+([._-][A-Za-z0-9]+)*$ ]]
}

keychain_token() {
  local service="$1"
  local token
  if ! token="$(security find-generic-password -s "$service" -w 2>/dev/null)"; then
    echo "Keychain item '${service}' was not found." >&2
    echo "Store the Railway token with:" >&2
    echo "  security add-generic-password -s ${service} -a \"\$USER\" -w" >&2
    return 1
  fi
  if [[ -z "$token" ]]; then
    echo "Keychain item '${service}' is empty." >&2
    return 1
  fi
  printf '%s' "$token"
}

require_clean_tree() {
  local dirty
  dirty="$(git status --porcelain)"
  if [[ -n "$dirty" ]]; then
    echo "Working tree is dirty. Commit or stash before publishing." >&2
    printf '%s\n' "$dirty" >&2
    return 1
  fi
}

remote_tag_exists() {
  local tag="$1"
  local remote ref
  ref="refs/tags/${tag}"
  if ! remote="$(git ls-remote --tags origin "$ref")"; then
    echo "Could not list tags on origin." >&2
    return 2
  fi
  printf '%s\n' "$remote" | awk -v ref="$ref" '$2 == ref { found=1 } END { exit !found }'
}

publish_release_main() {
  local kind="${1:-}"
  local suffix="${2:-}"
  local environment token_service message commit date version

  case "$kind" in
    staging | major | minor | patch) ;;
    *)
      echo "Usage: publish-release.sh staging <suffix> | major | minor | patch" >&2
      return 1
      ;;
  esac

  local root
  root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  cd "$root"

  if [[ "$kind" == "staging" ]]; then
    if ! valid_suffix "$suffix"; then
      echo "SUFFIX is required and must look like menu.1 (letters, digits, '.', '_', '-')." >&2
      echo "Example: make publish-staging SUFFIX=menu.1" >&2
      return 1
    fi
    environment="staging"
    token_service="$STAGING_TOKEN_SERVICE"
  else
    environment="production"
    token_service="$PRODUCTION_TOKEN_SERVICE"
  fi

  if ! command -v railway >/dev/null 2>&1; then
    echo "Railway CLI is not on PATH. Install it with: brew install railway" >&2
    return 1
  fi
  if ! command -v security >/dev/null 2>&1; then
    echo "macOS keychain (security) is required to read ${token_service}." >&2
    return 1
  fi

  echo "Fetching tags from origin..."
  GIT_TERMINAL_PROMPT=0 git fetch origin --tags

  require_clean_tree

  if [[ "$kind" == "staging" ]]; then
    local described
    described="$(latest_described_tag)"
    TAG="$(staging_tag "$suffix")"
    message="Beacon staging ${TAG}"
    echo "Latest tag: ${described}"
  else
    local base
    base="$(highest_release_tag)"
    TAG="$(bump_semver "$base" "$kind")"
    message="Beacon ${TAG}"
    echo "Latest release: ${base}"
    if [[ "$base" != "v0.0.0" ]]; then
      if ! git merge-base --is-ancestor "${base}^{commit}" HEAD; then
        echo "HEAD does not contain ${base}. Publish from a commit that includes that release." >&2
        return 1
      fi
    fi
  fi

  echo "Release tag: ${TAG}"
  echo "Railway: project ${RAILWAY_PROJECT_ID} service ${RAILWAY_SERVICE} environment ${environment}"

  if git show-ref --tags --verify --quiet "refs/tags/${TAG}"; then
    echo "Tag ${TAG} already exists locally." >&2
    return 1
  fi
  local remote_status=0
  remote_tag_exists "$TAG" || remote_status=$?
  if [[ $remote_status -eq 0 ]]; then
    echo "Tag ${TAG} already exists on origin." >&2
    return 1
  elif [[ $remote_status -ne 1 ]]; then
    return "$remote_status"
  fi

  local token
  if ! token="$(keychain_token "$token_service")"; then
    return 1
  fi

  git tag -a "$TAG" -m "$message"
  TAGGED=1
  trap cleanup EXIT

  echo "Pushing ${TAG} to origin..."
  git push origin "refs/tags/${TAG}"
  PUSHED=1

  commit="$(git rev-parse --short=7 HEAD)"
  date="$(TZ=UTC git show -s --format=%cd --date=format:%Y-%m-%dT%H:%MZ HEAD)"
  version="${TAG#v}"

  export RAILWAY_TOKEN="$token"
  export RAILWAY_PROJECT_ID
  unset token

  echo "Setting build version ${version} (${commit}, ${date})"
  echo "Deploying ${TAG} to ${environment}..."
  if ! railway variable set --skip-deploys \
    --service "$RAILWAY_SERVICE" --environment "$environment" \
    "VERSION=${version}" "GIT_COMMIT=${commit}" "GIT_DATE=${date}" ||
    ! railway up --ci \
      --project "$RAILWAY_PROJECT_ID" \
      --service "$RAILWAY_SERVICE" \
      --environment "$environment" \
      --message "$message"; then
    echo "Tag ${TAG} is on origin, but the deploy to ${environment} failed." >&2
    echo "Fix the cause and publish again; the next run creates a new tag." >&2
    return 1
  fi
  echo "Published ${TAG} to ${environment}."
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  publish_release_main "$@"
fi
