APP_NAME=note-app

ifeq ($(OS),Windows_NT)
DEV_UP_CMD = powershell -ExecutionPolicy Bypass -File ./scripts/dev-up.ps1
QA_UP_CMD = powershell -ExecutionPolicy Bypass -File ./scripts/qa-up.ps1
TEST_DB_UP_CMD = powershell -ExecutionPolicy Bypass -File ./scripts/test-db.ps1
TEST_ALL_CMD = powershell -ExecutionPolicy Bypass -File ./scripts/test-all.ps1
TEST_RACE_CMD = powershell -ExecutionPolicy Bypass -File ./scripts/test-race.ps1
PRODUCTION_COMPOSE_UP_CMD = powershell -ExecutionPolicy Bypass -File ./scripts/production-compose-up.ps1
TEST_DB_CMD = powershell -NoProfile -Command "$$env:TEST_ENV_FILE='.env.test'; go test ./internal/infrastructure/database ./internal/repository/postgres"
else
DEV_UP_CMD = bash ./scripts/dev-up.sh
QA_UP_CMD = bash ./scripts/qa-up.sh
TEST_DB_UP_CMD = bash ./scripts/test-db.sh
TEST_ALL_CMD = bash ./scripts/test-all.sh
TEST_RACE_CMD = bash ./scripts/test-race.sh
PRODUCTION_COMPOSE_UP_CMD = bash ./scripts/production-compose-up.sh
TEST_DB_CMD = TEST_ENV_FILE=.env.test go test ./internal/infrastructure/database ./internal/repository/postgres
endif

.PHONY: run test tidy migrate-up migrate-down run-local migrate-local-up migrate-local-down migrate-local-preflight migrate-test-up migrate-test-down migrate-test-preflight migrate-production-up migrate-production-preflight test-db test-all test-race dev-up qa-up test-db-up production-compose-up

run:
	go run ./cmd/api

test:
	go test ./...

tidy:
	go mod tidy

migrate-up:
	go run ./cmd/migrate -direction up

migrate-down:
	go run ./cmd/migrate -direction down -steps 1

run-local:
	go run ./cmd/api -env-file .env.local

migrate-local-up:
	go run ./cmd/migrate -env-file .env.local -direction up

migrate-local-down:
	go run ./cmd/migrate -env-file .env.local -direction down -steps 1

migrate-local-preflight:
	go run ./cmd/migrate -env-file .env.local -preflight folder-sibling-uniqueness

migrate-test-up:
	go run ./cmd/migrate -env-file .env.test -direction up

migrate-test-down:
	go run ./cmd/migrate -env-file .env.test -direction down -steps 1

migrate-test-preflight:
	go run ./cmd/migrate -env-file .env.test -preflight folder-sibling-uniqueness

migrate-production-up:
	go run ./cmd/migrate -env-file .env.production -direction up

migrate-production-preflight:
	go run ./cmd/migrate -env-file .env.production -preflight folder-sibling-uniqueness

test-db:
	$(TEST_DB_CMD)

test-all:
	$(TEST_ALL_CMD)

test-race:
	$(TEST_RACE_CMD)

dev-up:
	$(DEV_UP_CMD)

qa-up:
	$(QA_UP_CMD)

test-db-up:
	$(TEST_DB_UP_CMD)

production-compose-up:
	$(PRODUCTION_COMPOSE_UP_CMD)
