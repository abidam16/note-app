# Backend Hardening Roadmap

## Summary
This roadmap tracks backend-only resilience and abuse-resistance work for the public HTTP API. It intentionally excludes frontend work and does not change `frontend-repo/API_CONTRACT.md`.

Goals:
- reduce the chance of resource exhaustion from malformed or abusive clients
- reduce the blast radius of retry storms and burst traffic
- harden request parsing and connection handling without changing business behavior

## Status Model
- `not_started`
- `in_progress`
- `done`
- `blocked`

## Current Task
- `done` Hardening verification pass

## Tasks
1. `done` Server socket hardening
   - add `ReadTimeout`
   - add `WriteTimeout`
   - add `IdleTimeout`
   - add `MaxHeaderBytes`
   - add focused tests in `cmd/api/app_test.go`

2. `done` JSON body size caps
   - cap request bodies before JSON decode
   - add route-safe defaults for document-heavy endpoints
   - verify oversized requests fail cleanly

3. `done` Global rate limiting
   - add per-IP throttling middleware
   - keep implementation in-process first
   - add deterministic middleware tests

4. `done` Auth route throttling
   - add stricter limiter for `/api/v1/auth/login` and `/api/v1/auth/refresh`
   - tune for brute-force and retry abuse

5. `done` Overload shedding for expensive routes
   - bound concurrent heavy operations
   - protect DB and CPU-heavy paths under burst load

6. `done` Hardening verification pass
   - add regression coverage for malformed input and limiter behavior
   - rerun full test suite

## Notes
- Implement one task at a time.
- Each task must follow TDD: failing test first, then minimal code.
- Update `docs/checkpoint.md` after each completed task.
- Task 1 completed:
  - `cmd/api/app.go` now sets `ReadTimeout=15s`, `WriteTimeout=35s`, `IdleTimeout=60s`, and `MaxHeaderBytes=1<<20`
  - regression coverage added in `cmd/api/app_test.go`
- Task 2 completed:
  - `internal/transport/http/response.go` now applies a default `1 MiB` JSON body cap
  - `PUT /api/v1/pages/{pageID}/draft` now uses an `8 MiB` decode cap for document-sized payloads
  - trailing JSON values are now rejected after the first decoded value
  - regression coverage added in `internal/transport/http/response_test.go` and `internal/transport/http/server_test.go`
- Task 3 completed:
  - `internal/transport/http/middleware/middleware.go` now provides an in-process fixed-window per-IP limiter
  - `/api/v1` is throttled at `120 requests/minute per client IP`
  - `/healthz` remains outside the limiter scope
  - opportunistic bucket cleanup prevents unbounded in-memory growth for expired clients
  - regression coverage added in `internal/transport/http/middleware/middleware_test.go` and `internal/transport/http/server_test.go`
- Task 4 completed:
  - `/api/v1/auth/login` and `/api/v1/auth/refresh` now have a stricter `5 requests/minute per client IP` limiter
  - auth throttling is layered on top of the global API limiter without affecting `/api/v1/auth/register`, `/api/v1/auth/logout`, or `/healthz`
  - regression coverage added in `internal/transport/http/server_auth_workspace_test.go`
- Task 5 completed:
  - `internal/transport/http/middleware/middleware.go` now provides semaphore-based overload shedding with JSON `503 overloaded` responses
  - the heavy-route limiter is shared across the first high-cost route set in `internal/transport/http/server.go`
  - protected heavy routes:
    - `PUT /api/v1/pages/{pageID}/draft`
    - `POST /api/v1/pages/{pageID}/revisions`
    - `GET /api/v1/pages/{pageID}/revisions/compare`
    - `POST /api/v1/pages/{pageID}/revisions/{revisionID}/restore`
  - light routes remain available while heavy-route slots are saturated
  - regression coverage added in `internal/transport/http/middleware/middleware_test.go` and `internal/transport/http/server_test.go`
- Task 6 completed:
  - fixed the overload-test shared-map race by synchronizing `testPageRepo` in `internal/transport/http/server_test.go`
  - tightened rate-limit accuracy so `429 Retry-After` reflects the remaining window instead of the full window
  - strengthened limiter and overload regression coverage for:
    - JSON error payloads
    - `Content-Type`
    - `Retry-After`
    - shared heavy-route limiter interaction across endpoints
  - reran focused limiter tests and the full Go test suite
