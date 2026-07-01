.PHONY: install-hooks test test-integration

install-hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks installed from .githooks/"

# Unit tests only (no database required); same scope as the pre-push hook.
test:
	go test -short ./...

# End-to-end / integration tests (./e2e) against a throwaway Postgres.
# Spins up and tears down a disposable container; never touches your dev DB.
test-integration:
	./scripts/integration-test.sh
