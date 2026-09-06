# beacon

Safe house operations for Rainbow Haven.

## Requirements

- Go 1.27+ (`GOTOOLCHAIN=auto` will fetch it if needed)
- Docker **or** Podman (this machine uses Podman when OrbStack/Docker is absent)

## Quick start (local)

```bash
# Postgres on host port 5433
./scripts/compose.sh up -d db

export DATABASE_URL='postgres://beacon:beacon@127.0.0.1:5433/beacon?sslmode=disable'
export BOOTSTRAP_ADMIN_EMAIL='rhc@example.com'
export BASE_URL='http://localhost:8080'
export WEBAUTHN_RP_ID=localhost
export WEBAUTHN_RP_ORIGINS='http://localhost:8080,http://127.0.0.1:8080'
export SECURE_COOKIES=false

go run ./cmd/beacon
```

On first boot (or whenever `BOOTSTRAP_ADMIN_EMAIL` is still pending without a passkey), the server prints an **invite URL** to the terminal (`=== Beacon invite ===`). Open it on `http://localhost:8080` (not only `127.0.0.1`) and enroll a passkey.

If you already created users earlier and see no invite, restart with:

```bash
export BOOTSTRAP_REISSUE=true
export BOOTSTRAP_ADMIN_EMAIL='rhc@example.com'
```

That mints a **new** invite URL (old links stop working). Or reset the DB:

```bash
./scripts/compose.sh exec -T db psql -U beacon -d beacon -c \
  "DROP TABLE IF EXISTS audit_events, sessions, webauthn_challenges, webauthn_credentials, invites, users, safe_houses, rhls, schema_migrations CASCADE; DROP TYPE IF EXISTS user_status, user_role CASCADE;"
```

Use only one server process on :8080 (either `go run` **or** Compose `app`, not both).

Health checks:

- `GET /healthz` — process up
- `GET /readyz` — database reachable

RHC admin (after login): `/admin/users` — invite, lock, re-invite (passkey recovery).

## Verify

```bash
./scripts/verify-phase1.sh       # Phase 1: WebAuthn flow tests + HTTP smoke
./scripts/verify-bootstrap.sh   # Compose full stack (optional)
./scripts/verify-cla.sh
go test ./...
```

## License

Apache License 2.0 — see [LICENSE](./LICENSE).

## Contributing

External contributions require signing the [CLA](./CLA.md). See [CONTRIBUTING.md](./CONTRIBUTING.md).

Agent handoff: [AGENTS.md](./AGENTS.md).
