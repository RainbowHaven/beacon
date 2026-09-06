#!/usr/bin/env bash
# Offline checks for Beacon CLA documents + GitHub Action workflow.
# Does not call GitHub. Run from repo root: ./scripts/verify-cla.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "OK: $*"; }

SIGN_PHRASE='I have read the CLA Document and I hereby sign the CLA'
DOC_URL='https://github.com/magiconair/beacon/blob/main/CLA.md'
ACTION_REF='rdkcentral/contributor-assistant_github-action@v2.7.0'

echo "==> Beacon CLA local verification"
echo

[[ -f CLA.md ]] || fail "CLA.md missing"
[[ -f CONTRIBUTING.md ]] || fail "CONTRIBUTING.md missing"
[[ -f .github/workflows/cla.yml ]] || fail ".github/workflows/cla.yml missing"
pass "required files exist"

# Signature store is owned by the CLA action (created on first run).
# If present locally, it must be the empty/action-managed JSON — not a hand-made stub.
if [[ -e signatures/version1/cla.json ]]; then
  grep -Fq 'signedContributors' signatures/version1/cla.json \
    || fail "signatures/version1/cla.json exists but does not look like CLA Assistant output"
  pass "signatures file present (action-managed)"
else
  pass "signatures file not present yet (action will create it)"
fi

grep -Fq "$SIGN_PHRASE" CLA.md || fail "CLA.md missing exact sign phrase"
grep -Fq "$SIGN_PHRASE" CONTRIBUTING.md || fail "CONTRIBUTING.md missing exact sign phrase"
grep -Fq "$SIGN_PHRASE" .github/workflows/cla.yml || fail "cla.yml missing exact sign phrase"
pass "sign phrase consistent across CLA.md, CONTRIBUTING.md, workflow"

grep -Fq "Copyright assignment" CLA.md || grep -Fq "copyright assignment" CLA.md || grep -Fq "assign to the Project Steward" CLA.md \
  || fail "CLA.md does not look like a copyright assignment"
pass "CLA.md includes copyright assignment language"

grep -Fq "relicense" CLA.md || fail "CLA.md missing relicense language"
grep -Fq "transfer" CLA.md || fail "CLA.md missing transfer language"
pass "CLA.md covers relicense + transfer"

WF=.github/workflows/cla.yml
grep -Fq "pull_request_target" "$WF" || fail "workflow must use pull_request_target"
grep -Fq "issue_comment" "$WF" || fail "workflow must listen for issue_comment"
grep -Fq "$ACTION_REF" "$WF" || fail "workflow must pin $ACTION_REF"
grep -Fq "path-to-document: $DOC_URL" "$WF" || fail "path-to-document must be $DOC_URL"
grep -Fq "path-to-signatures: signatures/version1/cla.json" "$WF" || fail "unexpected path-to-signatures"
grep -Fq "branch: main" "$WF" || fail "signature branch must be main"
grep -Fq "allowlist: magiconair,cursoragent,bot*,dependabot[bot],github-actions[bot]" "$WF" || fail "allowlist mismatch"
pass "workflow triggers, pin, document URL, signatures path, allowlist"

# Structural YAML parse via Go (no PyYAML required).
go run ./scripts/internal/checkyaml .github/workflows/cla.yml \
  || fail "cla.yml is not valid YAML"
pass "cla.yml parses as YAML"

# Optional: actionlint if installed.
if command -v actionlint >/dev/null 2>&1; then
  actionlint .github/workflows/cla.yml
  pass "actionlint clean"
else
  echo "SKIP: actionlint not installed (brew install actionlint)"
fi

# Optional: act dry-run if installed.
if command -v act >/dev/null 2>&1; then
  echo
  echo "==> act dry-run (synthetic pull_request_target)"
  act pull_request_target \
    -W .github/workflows/cla.yml \
    -e testdata/cla/pull_request_target.json \
    -n \
    || fail "act dry-run failed"
  pass "act dry-run accepted workflow"
else
  echo "SKIP: act not installed (brew install act) — needed for local Actions dry-run"
fi

echo
echo "All offline CLA checks passed."
echo "Next (on GitHub, after push): open a PR from a non-allowlisted account to exercise signing."
