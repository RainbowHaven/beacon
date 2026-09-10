.PHONY: test run compose-up compose-down verify verify-cla verify-phase1 verify-phase2 verify-phase3

test:
	GOTOOLCHAIN=auto go test ./...

run:
	GOTOOLCHAIN=auto go run ./cmd/beacon

compose-up:
	./scripts/compose.sh up -d --build

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

verify-cla:
	./scripts/verify-cla.sh
