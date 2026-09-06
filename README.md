# beacon

Safe house operations for Rainbow Haven.

## Requirements

- Go 1.27+ (`GOTOOLCHAIN=auto` will fetch it if needed)
- Docker **or** Podman (this machine uses Podman when OrbStack/Docker is absent)

## Quick start (local)

```bash
# Start Postgres + app (http://127.0.0.1:8080, Postgres on host port 5433)
./scripts/compose.sh up -d --build

# Or run the app against Compose Postgres only:
./scripts/compose.sh up -d db
export DATABASE_URL='postgres://beacon:beacon@127.0.0.1:5433/beacon?sslmode=disable'
go run ./cmd/beacon
```

Health checks:

- `GET /healthz` — process up
- `GET /readyz` — database reachable

## Verify

```bash
./scripts/verify-bootstrap.sh   # Phase 0: tests + compose + HTTP checks
./scripts/verify-cla.sh         # CLA docs + workflow shape
go test ./...
```

## License

Apache License 2.0 — see [LICENSE](./LICENSE).

## Contributing

External contributions require signing the [CLA](./CLA.md). See [CONTRIBUTING.md](./CONTRIBUTING.md).

Agent handoff: [AGENTS.md](./AGENTS.md).
