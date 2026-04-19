# Environment Workflow

This repo supports separate environment files for development, QA/test runtime, backend database tests, and production.

## Recommended Files

- `.env.local`
  - development API on port `8082` and development migrations
- `.env.test`
  - QA/test API on port `8081`, local test-database migrations, and DB-backed backend tests
- `.env.production`
  - production runtime on port `8080` and production migrations

Copy from the committed examples:

- `.env.local.example`
- `.env.test.example`
- `.env.production.example`

## Command Support

Both entrypoints now support `-env-file`:

- `go run ./cmd/api -env-file .env.local`
- `go run ./cmd/migrate -env-file .env.local -direction up`

If `-env-file` is omitted, the commands still default to `.env`.

## Environment Model

- Production:
  - HTTP port `8080`
  - external production database
  - use `docker-compose.yml`
- QA/Test runtime:
  - HTTP port `8081`
  - local database `noteapp_test`
  - use `make qa-up`
- Development runtime:
  - HTTP port `8082`
  - local database `noteapp`
  - use `make dev-up`
- Backend DB tests:
  - no HTTP server required
  - connect directly to local database `noteapp_test`
  - use `make test-db` or `make test-db-up`

## One-Command Wrappers

For common flows, use these wrappers instead of running two commands manually.

PowerShell:

```powershell
./scripts/dev-up.ps1
./scripts/qa-up.ps1
./scripts/test-db.ps1
./scripts/production-compose-up.ps1
```

Bash:

```bash
./scripts/dev-up.sh
./scripts/qa-up.sh
./scripts/test-db.sh
./scripts/production-compose-up.sh
```

Make:

```bash
make dev-up
make qa-up
make test-db-up
make test-all
make production-compose-up
```

## Local Database Bootstrap

Development, QA/test runtime, and DB-backed backend tests all use the local PostgreSQL Compose file:

```powershell
docker compose -f docker-compose.local-db.yml up -d postgres
```

On a fresh local PostgreSQL volume, Compose initializes both:

- `noteapp`
- `noteapp_test`

`docker-compose.local-db.yml` is for local development and testing only.
`docker-compose.yml` is production-only.

## Development Runtime

Create `.env.local` from `.env.local.example`, then run:

```powershell
go run ./cmd/migrate -env-file .env.local -direction up
go run ./cmd/api -env-file .env.local
```

For frontend browser integration against the local API, set:

- `CORS_ALLOWED_ORIGINS=http://localhost:5173`

Use a comma-separated list if you need to allow more than one local frontend origin.

Equivalent `make` targets:

```bash
make migrate-local-up
make run-local
make dev-up
```

Single-command wrappers:

```powershell
./scripts/dev-up.ps1
```

```bash
./scripts/dev-up.sh
```

## QA/Test Runtime

The QA/test runtime starts the API on port `8081` and points it at `noteapp_test`.

If your local Postgres volume was created before the init script existed, create the test database once manually:

```powershell
docker exec note-app-local-postgres psql -U noteapp -d postgres -c "CREATE DATABASE noteapp_test;"
```

Create `.env.test` from `.env.test.example`, then run:

```powershell
go run ./cmd/migrate -env-file .env.test -direction up
go run ./cmd/api -env-file .env.test
```

If you are manually testing the QA/test API from the frontend dev server, keep `CORS_ALLOWED_ORIGINS=http://localhost:5173` in `.env.test`.

Equivalent `make` targets:

```bash
make migrate-test-up
make qa-up
```

Single-command wrappers:

```powershell
./scripts/qa-up.ps1
```

```bash
./scripts/qa-up.sh
```

Use this runtime for frontend integration/manual verification against the local test database.

## Backend Database Tests

Repository/database tests do not use an HTTP server on `8081`. They connect directly to PostgreSQL.

Run them against `noteapp_test`:

```powershell
go run ./cmd/migrate -env-file .env.test -direction up
go test ./internal/infrastructure/database ./internal/repository/postgres
```

Equivalent `make` targets:

```bash
make test-db
make test-db-up
make test-all
```

Single-command wrappers:

```powershell
./scripts/test-db.ps1
```

```bash
./scripts/test-db.sh
```

Full-suite wrappers:

```powershell
./scripts/test-all.ps1
```

```bash
./scripts/test-all.sh
```

Test DSN resolution prefers:

1. `TEST_POSTGRES_DSN`
2. `POSTGRES_DSN`
3. `DATABASE_URL`
4. `.env.test`
5. `.env`
6. local fallback

You can override the test env file path with:

```powershell
$env:TEST_ENV_FILE=".env.test"
go test ./internal/repository/postgres
```

## Production Migrations and Runtime

Create `.env.production` from `.env.production.example`, then run:

```powershell
go run ./cmd/migrate -env-file .env.production -preflight folder-sibling-uniqueness
go run ./cmd/migrate -env-file .env.production -direction up
docker compose --env-file .env.production up -d --build
```

Set `CORS_ALLOWED_ORIGINS` to the exact browser frontend origins that should be allowed to call the API. Do not use `*` for this auth flow.

Equivalent `make` targets:

```bash
make migrate-production-preflight
make migrate-production-up
make production-compose-up
```

Single-command wrappers:

```powershell
./scripts/production-compose-up.ps1
```

```bash
./scripts/production-compose-up.sh
```

## Recommended Discipline

- Do not point `.env.test` at a shared development or production database.
- Do not point `.env.local` at production if you want clean development separation.
- Run the same migrations in dev, test, and production so schema stays aligned across environments.
- Use `8081` for frontend QA/integration work against the local test database.
- Use DB-backed `go test` for repository/database verification; it does not require a running HTTP server.
