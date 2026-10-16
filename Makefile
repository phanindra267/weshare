# WeShare monorepo task runner.
#
# `make` may not be installed on Windows. Equivalent PowerShell scripts live in
# infra/scripts/*.ps1 and are the recommended path on this platform.

SHELL := /bin/bash
API_DIR := services/api
WEB_DIR := apps/web
INFRA_DIR := infra

.PHONY: help
help:
	@echo "WeShare tasks:"
	@echo "  make db-up          Start local PostgreSQL (docker compose)"
	@echo "  make db-down        Stop PostgreSQL (keep data)"
	@echo "  make db-reset       Recreate the database volume and migrate"
	@echo "  make migrate-up     Apply pending migrations"
	@echo "  make migrate-down   Roll back the last migration"
	@echo "  make migrate-status Show migration status"
	@echo "  make api            Run the Go API"
	@echo "  make web            Run the Next.js app"
	@echo "  make test           Run all backend and frontend tests"
	@echo "  make verify         lint + typecheck + test + build"

.PHONY: db-up
db-up:
	cd $(INFRA_DIR) && docker compose up -d

.PHONY: db-down
db-down:
	cd $(INFRA_DIR) && docker compose down

.PHONY: db-reset
db-reset:
	cd $(INFRA_DIR) && docker compose down -v && docker compose up -d
	$(MAKE) migrate-up

.PHONY: migrate-up
migrate-up:
	cd $(API_DIR) && go run ./cmd/migrate up

.PHONY: migrate-down
migrate-down:
	cd $(API_DIR) && go run ./cmd/migrate down 1

.PHONY: migrate-status
migrate-status:
	cd $(API_DIR) && go run ./cmd/migrate status

.PHONY: api
api:
	cd $(API_DIR) && go run ./cmd/server

.PHONY: web
web:
	cd $(WEB_DIR) && npm run dev

.PHONY: test
test: test-api test-web

.PHONY: test-api
test-api:
	cd $(API_DIR) && go test ./...

.PHONY: test-web
test-web:
	cd $(WEB_DIR) && npm test

.PHONY: verify
verify:
	cd $(API_DIR) && go vet ./... && gofmt -l . && go test ./...
	cd $(WEB_DIR) && npm run lint && npm run typecheck && npm test && npm run build
