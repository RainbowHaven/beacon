---
name: deploy
description: >-
  Publish Beacon to Railway staging or production from a laptop. Use when the
  user asks to deploy, publish, ship, or release Beacon, or mentions staging,
  production, publish-staging, publish-patch, publish-minor, or publish-major.
---

# Deploy Beacon

Publish only when the user asks to deploy. One-time Railway setup stays in [deploy/README.md](../../../deploy/README.md).

## Choose the target

- **Staging** needs a suffix from the user. If they did not give one, ask and stop. Do not invent a suffix.
- **Production** needs a bump level from the user: `patch`, `minor`, or `major`. If they did not name one, ask and stop. Do not guess.
- Publish only the environment they named. If they named both, publish staging first and publish production only after staging succeeds.

A suffix matches `menu.1`: letters and digits, with optional `.`, `_`, or `-` between parts. The publish script rejects anything else.

## Before you run

1. `git status` must be clean. If it is dirty, stop. Do not stash, commit, or discard as part of the deploy.
2. Fetch `origin`. Production publishes `HEAD`. `HEAD` must be `main`, match `origin/main`, and already contain the previous exact `vMAJOR.MINOR.PATCH` tag. Check out another commit only when the user names it and the tree is clean.
3. Do not run `railway up`, `railway variable set`, `git tag`, or `git push` yourself. The Makefile target creates the annotated tag, pushes that tag, stamps the version, and uploads.
4. Check the Railway token before the publish command. The Makefile names the macOS keychain items: `RAILWAY_BEACON_STAGING_TOKEN` for staging and `RAILWAY_BEACON_PRODUCTION_TOKEN` for production. The publish script copies the chosen item into `RAILWAY_TOKEN` for the Railway CLI. Confirm the item exists and is non-empty, and discard the secret:

   ```bash
   security find-generic-password -s RAILWAY_BEACON_STAGING_TOKEN -w 2>/dev/null | grep -q .
   security find-generic-password -s RAILWAY_BEACON_PRODUCTION_TOKEN -w 2>/dev/null | grep -q .
   ```

   Run only the line for the environment you are about to publish. Do not print the token, the command substitution, or `-w` output. If the check fails, show the matching `security add-generic-password` line from [deploy/README.md](../../../deploy/README.md) and stop. Do not run `make publish-*`.
5. Do not wipe a database. Do not set `BEACON_ALLOW_SCHEMA_WIPE` or a wipe pre-deploy on production. Do not run `scripts/wipe-postgres-schema.sh` or `make verify-phaseN` against Railway.

Say the environment, the tag shape, and the exact command, then run it. The token check in step 4 must already have passed. A staging publish does not delete the database. Existing rows and passkeys stay, and the server migrates on start.

## Commands

From the repo root:

```bash
make publish-staging SUFFIX=menu.1
make publish-patch
make publish-minor
make publish-major
```

The staging tag is `<latest semver>-staging-<suffix>`. `v1.2.3` and `v1.2.3-staging-older` both use base `v1.2.3`. With no tags, the base is `v0.0.0`. Production bumps the highest exact `vMAJOR.MINOR.PATCH`: patch `v1.2.3` → `v1.2.4`, minor → `v1.3.0`, major → `v2.0.0`.

A tag means the publish was attempted, not that Railway succeeded. If the deploy fails, leave the tag on `origin`. The next staging publish needs a new suffix. The next production publish uses the next bump. Do not delete or move tags.

## After

Report the tag, the environment, and whether the upload succeeded. Production health check: `https://beacon.magiconair.net/healthz`. Staging host: `beacon-staging.magiconair.net`, or the Railway domain in the log. Passkeys are bound to the hostname.
