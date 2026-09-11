.PHONY: help keys env db-up db-down test run compose-up compose-down \
	verify verify-cla verify-phase1 verify-phase2 verify-phase3 verify-phase4

GOTOOLCHAIN ?= auto
LOCAL_DIR := .local
PUB_FILE := $(LOCAL_DIR)/identity.pub.b64
PRIV_FILE := $(LOCAL_DIR)/identity.priv.b64
ENV_FILE := .env

help:
	@echo "Beacon local development"
	@echo ""
	@echo "  make keys          Generate identity keypair into .local/ (gitignored)"
	@echo "  make env           Write .env from .local keys (gitignored)"
	@echo "  make db-up         Start Postgres only"
	@echo "  make run           Run the app with .env (creates keys/env if needed)"
	@echo "  make test          go test ./..."
	@echo "  make verify-phaseN Run phase N verify script (1–4)"
	@echo "  make verify        Compose bootstrap verify"
	@echo "  make verify-cla    CLA docs/workflow check"
	@echo "  make compose-up    Full Compose stack (needs IDENTITY_* in environment/.env)"
	@echo "  make compose-down  Stop Compose and remove volumes"
	@echo ""
	@echo "Never commit .env, .local/, or any identity private key."

keys:
	@mkdir -p $(LOCAL_DIR)
	@if [ -f $(PUB_FILE) ] && [ -f $(PRIV_FILE) ]; then \
		echo "Keys already exist in $(LOCAL_DIR)/ (delete both files to regenerate)"; \
	else \
		GOTOOLCHAIN=$(GOTOOLCHAIN) go run ./scripts/internal/genkeys -out-dir $(LOCAL_DIR); \
	fi

env: keys
	@pub=$$(tr -d '[:space:]' <$(PUB_FILE)); \
	printf '%s\n' \
		'ADDR=:8080' \
		'DATABASE_URL=postgres://beacon:beacon@127.0.0.1:5433/beacon?sslmode=disable' \
		'BASE_URL=http://localhost:8080' \
		'SECURE_COOKIES=false' \
		'WEBAUTHN_RP_ID=localhost' \
		'WEBAUTHN_RP_DISPLAY_NAME=Beacon' \
		'WEBAUTHN_RP_ORIGINS=http://localhost:8080,http://127.0.0.1:8080' \
		'BOOTSTRAP_ADMIN_EMAIL=rhc@example.com' \
		'BOOTSTRAP_REISSUE=false' \
		"IDENTITY_PUBLIC_KEY_B64=$$pub" \
		'IDENTITY_KEY_ID=local-dev-1' \
		'RECEIPT_DIR=data/receipts' \
		> $(ENV_FILE)
	@echo "Wrote $(ENV_FILE) (public key only). Private key remains in $(PRIV_FILE)."

db-up:
	./scripts/compose.sh up -d db

db-down:
	./scripts/compose.sh stop db

test:
	GOTOOLCHAIN=$(GOTOOLCHAIN) go test ./...

run: env db-up
	@echo "Waiting for Postgres..."
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do \
		./scripts/compose.sh exec -T db pg_isready -U beacon -d beacon >/dev/null 2>&1 && break; \
		sleep 0.5; \
	done
	set -a; . ./$(ENV_FILE); set +a; GOTOOLCHAIN=$(GOTOOLCHAIN) go run ./cmd/beacon

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

verify-cla:
	./scripts/verify-cla.sh
