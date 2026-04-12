# Security And Error Hardening Roadmap

## Summary
This roadmap tracks backend-only work to reduce secret leakage risk, improve log safety, and make client-facing errors more consistent and understandable.

This roadmap does not change `frontend-repo/API_CONTRACT.md` up front. The first tasks focus on backend behavior, log hygiene, and test coverage. If a later task needs an intentional API error-contract change, that should be reviewed separately before implementation.

## Diagnosis
The current codebase already avoids some common leaks:
- request logging does not record request bodies
- auth middleware does not log bearer tokens
- internal `500` responses are already generic
- password hashes are stripped from returned user payloads

The remaining problems are narrower but still important:
- some startup and request-failure logs record raw error strings
- those raw errors may expose infrastructure or implementation detail in logs
- validation and domain errors are not consistently curated for client readability
- error quality is acceptable in many places, but not controlled by one clear policy

## Guiding Policies
- Prefer safe logs over verbose logs. If a raw error may contain infrastructure detail, redact or replace it.
- Keep client-facing errors stable, short, and human-readable.
- Do not weaken debugging blindly. Move sensitive detail out of broad logs rather than deleting all observability.
- Favor the existing stack and architecture. No new platform tool unless the current stack cannot solve the problem cleanly.
- Implement one hardening slice at a time with tests first.

## Status Model
- `not_started`
- `in_progress`
- `done`
- `blocked`

## Current Task
- `done` Task 6: final verification pass

## Tasks
1. `done` Startup and infrastructure log sanitization
   - audit startup logging in `cmd/api/app.go`
   - stop logging raw infrastructure errors where they may expose DSN or connection detail
   - keep enough structured context for operators
   - add focused tests for sanitized logging behavior where practical

2. `done` Request failure log sanitization
   - review `writeMappedError` in `internal/transport/http/server.go`
   - stop emitting raw `err.Error()` for broadly visible logs where it is not needed
   - keep stable safe fields such as request id, route, status, and error code
   - add regression coverage for sanitized failure logging

3. `done` Client validation error normalization
   - identify inconsistent validation wording
   - define transport-layer rules for client-safe validation messages
   - normalize high-noise endpoints first
   - add response-shape and wording tests

4. `done` Sensitive-data regression sweep
   - add tests proving logs do not contain:
     - bearer tokens
     - refresh tokens
     - password hashes
     - raw DSN values
   - verify auth and startup paths explicitly

5. `done` Panic and crash-surface reduction
   - audit startup panic paths
   - replace avoidable panics with explicit errors where that improves safety and clarity
   - keep test-only panic expectations only where truly justified

6. `done` Final verification pass
   - rerun focused security/error tests
   - rerun `go test ./...`
   - review roadmap completion and remaining open risks

## Execution Order
- Start with log sanitization, because that reduces real leakage risk without changing API behavior.
- Normalize client validation errors only after backend log safety is improved.
- Do not mix broad error-message cleanup with unrelated feature work.

## Success Criteria
- Common logs do not expose secrets or raw infrastructure details.
- Client-facing errors are more consistent and easier to understand.
- Existing backend behavior remains stable unless an intentional contract change is explicitly approved.
- Regression tests cover the main leak-prone paths.

## Notes
- This roadmap is backend-only.
- This roadmap is intentionally separate from `docs/backend-hardening-roadmap.md`.
- Update `docs/checkpoint.md` after each completed task.
- Task 1 completed:
  - `cmd/api/app.go` now creates loggers through an injectable runtime dependency for testable startup logging
  - startup infrastructure logs no longer emit raw error strings for:
    - database connection failure
    - server listen failure
    - server shutdown failure
  - focused regression coverage added in `cmd/api/app_test.go`
- Task 2 completed:
  - `internal/transport/http/server.go` no longer logs raw `err.Error()` text for request-failure events
  - request-failure logs now keep safe structured fields such as:
    - request id
    - method
    - path
    - route
    - status
    - error code
    - failure class
  - focused regression coverage added in `internal/transport/http/server_test.go`
- Task 3 completed:
  - `internal/transport/http/server.go` now normalizes validation messages at the transport boundary
  - wrapped validation prefixes such as `validation failed:` are removed before responses are written
  - focused regression coverage added in `internal/transport/http/server_auth_workspace_test.go`
- Task 4 completed:
  - request logging now sanitizes sensitive URL-carried data before it reaches logs
  - sensitive query keys such as `password`, `refresh_token`, `access_token`, `secret`, and `api_key` are redacted in:
    - `internal/transport/http/middleware/middleware.go`
    - `internal/transport/http/server.go`
  - referer URLs now redact sensitive query and fragment values in:
    - `internal/transport/http/middleware/log_sanitizer.go`
  - focused regression coverage now proves logs do not contain:
    - raw DSN values on startup paths
    - refresh tokens or passwords from URL query logging
    - bearer tokens, refresh tokens, or stored password hashes during normal auth request logging
- Task 5 completed:
  - `cmd/api/main.go` no longer panics on:
    - env flag parsing failure
    - config load failure
  - startup failures now write a short error to stderr and exit with code `1`
  - `cmd/api/app.go` now recovers startup panics during server construction and converts them into explicit errors with build-stage context
  - focused regression coverage added in:
    - `cmd/api/app_test.go`
    - `cmd/api/main_test.go`
- Task 6 completed:
  - reran focused security/error verification across:
    - startup log sanitization
    - startup panic/crash reduction
    - request-failure log sanitization
    - validation-message normalization
    - sensitive-data log redaction
    - rate-limit and overload middleware contract checks
  - reran `go test ./...`
  - current residual risks outside this roadmap:
    - distributed rate limiting is still not implemented for multi-instance deployments
    - direct internal misuse of `buildDefaultServer` still panics, but startup execution through `run()` now converts that panic into an explicit error
    - log safety now covers the main transport/startup paths, but future logging additions still need the same review discipline
