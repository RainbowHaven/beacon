# beacon

Safe house operations for Rainbow Haven — occupancy, expenses, and RHC reporting without letting field managers recover resident legal identities under duress.

## Requirements

- Go 1.27+ (`GOTOOLCHAIN=auto` fetches it if needed)
- Docker **or** Podman
- Optional: [Bun](https://bun.sh) for browser sealed-box round-trips in verify scripts

## Quick start (collaborators)

```bash
git clone git@github.com:magiconair/beacon.git
cd beacon
make setup         # brew: go, cloudflared, bun (optional)
make help
make run           # generate local keys → .env → Postgres → app on :8080
```

`make run` creates **gitignored** files:

| File | Purpose |
|------|---------|
| `.local/identity.pub.b64` | Sealed-box **public** key (safe for local `.env`) |
| `.local/identity.priv.b64` | Sealed-box **private** key for break-glass only |
| `.env` | Local server config (**public key only**) |

**Never commit** `.env`, `.local/`, or any identity private key. Regenerating keys with `rm -rf .local && make keys` invalidates previously sealed occupant identities in your local DB.

On first boot the server prints an invite URL (`=== Beacon invite ===`). Open it on `http://localhost:8080` and enroll a passkey.

Need a fresh invite for the bootstrap admin:

```bash
# in .env
BOOTSTRAP_REISSUE=true
BOOTSTRAP_ADMIN_EMAIL=rhc@example.com
make run
```

## Make targets

```bash
make setup           # brew install go, cloudflared, bun
make keys            # identity keypair → .local/ (no-op if already present)
make env             # write .env from public key
make db-up           # Postgres on host port 5433
make run             # keys + env + db + go run ./cmd/beacon
make tunnel          # Cloudflare quick tunnel (phone); writes .local/tunnel.env
make run-tunnel      # app with tunnel WebAuthn host (use with make tunnel)
make test            # go test ./...
make verify-phase1   # … through verify-phase6
make verify          # Compose full-stack smoke
make verify-cla      # CLA docs/workflow check
make compose-up      # app + db via Compose (uses .env)
make compose-down    # stop and remove volumes
```

## Phone access (cloudflared)

Passkeys are bound to the **hostname**. A quick tunnel works, but you must run the app with that host as WebAuthn RP ID:

```bash
# Terminal 1 — app on localhost first is fine; you’ll restart with tunnel env:
make run

# Terminal 2 — prints https://….trycloudflare.com and writes .local/tunnel.env
make tunnel

# Terminal 1 — stop the app (Ctrl-C), then:
make run-tunnel
```

Open the printed HTTPS URL on your phone. Enroll a **new** passkey on the tunnel host (localhost passkeys won’t work there). Quick tunnel URLs change each run — update/restart when the URL changes.

## Tests & verification

Prefer Make so everyone runs the same commands:

```bash
make test
make verify-phase5   # org/houses + scope isolation (does not drop DB)
make verify-phase6   # receipts-in-DB + Docker image + Railway deploy config
make verify-phase4   # break-glass routes (does not drop DB)
make verify-phase3   # drops schema — use a disposable local DB
make verify-phase2
make verify-phase1
make verify-cla
```

Phases 1–3 **drop** the local schema. Do not point them at data you care about.

## Production (Railway)

Deploy target: **Railway** + managed Postgres.

- **Production:** `https://beacon.magiconair.net`
- **Staging:** separate Railway environment; auto-deploy on push to `main` (see [deploy/README.md](./deploy/README.md))
- Env templates: [deploy/.env.example](./deploy/.env.example), [deploy/.env.staging.example](./deploy/.env.staging.example)
- Cutover, promote, backups: [OPERATOR.md](./OPERATOR.md)

Passkeys are bound to each hostname — enroll separately on staging and production.
Receipts are stored **in Postgres** (no file volume).

## After login

- `/occupants` — headcount, nicknames, depart
- `/occupants/new` — handoff (legal name / UN ID sealed in-browser)
- `/expenses` — amounts, notes, optional receipts
- `/reports` — monthly headcount + expense totals (no private key)
- `/admin/users` — list / lock / re-invite / edit role & scope; `/admin/users/invite` to invite
- `/admin/houses` — create/edit RHLs and safe houses (currency, active)
- `/admin/break-glass` — RHC browser-only decrypt (paste private key locally)
- `/admin/audit` — audit trail

Operator procedures: [OPERATOR.md](./OPERATOR.md).

## Keys (do not check in)

- **Public** key: `IDENTITY_PUBLIC_KEY_B64` + `IDENTITY_KEY_ID` in server env / `.env`
- **Private** key: offline only (password manager / printed). Paste into break-glass in the browser; never set as a server env var, never commit
- Local helpers: `make keys` writes both under `.local/`; only the public half is copied into `.env`

## Sharing the repo

1. Invite the person as a GitHub collaborator (or they fork and open PRs).
2. They clone and run `make run` — each machine gets its **own** local keypair.
3. External contributors must sign the [CLA](./CLA.md) on their first PR — see [CONTRIBUTING.md](./CONTRIBUTING.md).

## License

Apache License 2.0 — see [LICENSE](./LICENSE).

Agent handoff: [AGENTS.md](./AGENTS.md).
