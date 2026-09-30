.PHONY: help setup env db-up db-down test run run-tunnel tunnel compose-up compose-down \
	verify verify-cla verify-phase1 verify-phase2 verify-phase3 verify-phase4 verify-phase5 verify-phase6 \
	publish-staging publish-major publish-minor publish-patch

GOTOOLCHAIN ?= auto
LOCAL_DIR := .local
ENV_FILE := .env
TUNNEL_ENV := $(LOCAL_DIR)/tunnel.env

VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null | sed 's/^v//')
ifeq ($(strip $(VERSION)),)
VERSION := 0.0.0
endif
COMMIT  ?= $(shell git rev-parse --short=7 HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell TZ=UTC git show -s --format=%cd --date=format:%Y-%m-%dT%H:%MZ HEAD 2>/dev/null || date -u +%Y-%m-%dT%H:%MZ)
LDFLAGS := -X github.com/magiconair/beacon/internal/version.Version=$(VERSION) \
	-X github.com/magiconair/beacon/internal/version.Commit=$(COMMIT) \
	-X github.com/magiconair/beacon/internal/version.Date=$(DATE)

help:
	@echo "Beacon local development"
	@echo ""
	@echo "  make setup         Install brew deps (go, cloudflared)"
	@echo "  make env           Write .env (gitignored)"
	@echo "  make db-up         Start Postgres only"
	@echo "  make run           Run the app with .env"
	@echo "  make tunnel        Cloudflare quick tunnel to :8080 (phone access)"
	@echo "  make run-tunnel    Run app using .env + .local/tunnel.env (after make tunnel)"
	@echo "  make test          go test -p 1 ./... (packages share one database)"
	@echo "  make verify-phaseN Run phase N verify script (1–6)"
	@echo "  make verify        Compose bootstrap verify"
	@echo "  make verify-cla    CLA docs/workflow check"
	@echo "  make compose-up    Full Compose stack"
	@echo "  make compose-down  Stop Compose and remove volumes"
	@echo "  make publish-staging SUFFIX=menu.1"
	@echo "                     Tag <latest>-SUFFIX and deploy Railway staging"
	@echo "  make publish-patch Tag the next patch release and deploy production"
	@echo "  make publish-minor Tag the next minor release and deploy production"
	@echo "  make publish-major Tag the next major release and deploy production"
	@echo ""
	@echo "Never commit .env"

setup:
	@command -v brew >/dev/null 2>&1 || { echo "Homebrew required: https://brew.sh"; exit 1; }
	brew install go cloudflared
	@echo ""
	@echo "Also need a container engine for Postgres:"
	@echo "  brew install --cask docker    # or OrbStack / Podman Desktop"
	@echo "Then: make run"

env:
	@mkdir -p $(LOCAL_DIR)
	@printf '%s\n' \
		'ADDR=:8080' \
		'DATABASE_URL=postgres://beacon:beacon@127.0.0.1:5433/beacon?sslmode=disable' \
		'BASE_URL=http://localhost:8080' \
		'SECURE_COOKIES=false' \
		'WEBAUTHN_RP_ID=localhost' \
		'WEBAUTHN_RP_DISPLAY_NAME=Beacon' \
		'WEBAUTHN_RP_ORIGINS=http://localhost:8080,http://127.0.0.1:8080' \
		'BOOTSTRAP_ADMIN_EMAIL=rhc@example.com' \
		'BOOTSTRAP_REISSUE=false' \
		'ARRIVAL_FUTURE_DAYS=1' \
		> $(ENV_FILE)
	@echo "Wrote $(ENV_FILE)"

db-up:
	./scripts/compose.sh up -d db

db-down:
	./scripts/compose.sh stop db

test:
	GOTOOLCHAIN=$(GOTOOLCHAIN) go test -p 1 ./...

run: env db-up
	@echo "Waiting for Postgres..."
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do \
		./scripts/compose.sh exec -T db pg_isready -U beacon -d beacon >/dev/null 2>&1 && break; \
		sleep 0.5; \
	done
	set -a; . ./$(ENV_FILE); set +a; GOTOOLCHAIN=$(GOTOOLCHAIN) go run -ldflags "$(LDFLAGS)" ./cmd/beacon server

# Phone access: terminal A → make tunnel; terminal B → make run-tunnel
tunnel:
	chmod +x scripts/tunnel.sh
	./scripts/tunnel.sh

run-tunnel: env db-up
	@test -f $(TUNNEL_ENV) || { echo "Run make tunnel first (writes $(TUNNEL_ENV))."; exit 1; }
	@echo "Waiting for Postgres..."
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do \
		./scripts/compose.sh exec -T db pg_isready -U beacon -d beacon >/dev/null 2>&1 && break; \
		sleep 0.5; \
	done
	@echo "Loading $(ENV_FILE) + $(TUNNEL_ENV)"
	set -a; . ./$(ENV_FILE); . ./$(TUNNEL_ENV); set +a; GOTOOLCHAIN=$(GOTOOLCHAIN) go run -ldflags "$(LDFLAGS)" ./cmd/beacon server

compose-up: env
	./scripts/compose.sh --env-file $(ENV_FILE) up -d --build

compose-down:
	./scripts/compose.sh down -v

verify:
	./scripts/verify-bootstrap.sh

verify-phase1:
	./scripts/verify-phase1.sh

verify-phase2:
	./scripts/verify-phase2.sh

verify-phase3:
	./scripts/verify-phase3.sh

verify-phase4:
	./scripts/verify-phase4.sh

verify-phase5:
	./scripts/verify-phase5.sh

verify-phase6:
	./scripts/verify-phase6.sh

verify-cla:
	./scripts/verify-cla.sh

# Staging: annotated tag <latest version>-$(SUFFIX), Railway environment staging.
# Production: annotated semver bump, Railway environment production.
# Tokens: macOS keychain RAILWAY_BEACON_STAGING_TOKEN / RAILWAY_BEACON_PRODUCTION_TOKEN.
publish-staging:
	@test -n "$(SUFFIX)" || { echo "Usage: make publish-staging SUFFIX=menu.1" >&2; exit 1; }
	./scripts/publish-release.sh staging "$(SUFFIX)"

publish-major:
	./scripts/publish-release.sh major

publish-minor:
	./scripts/publish-release.sh minor

publish-patch:
	./scripts/publish-release.sh patch
