# Cyber Security Hardening Roadmap

## Summary
This roadmap tracks backend-only cybersecurity hardening for authentication, authorization, transport security, database access safety, and deployment-sensitive security controls.

This roadmap does not change `frontend-repo/API_CONTRACT.md` up front. It focuses first on server-side protections that reduce real risk without changing product behavior. If a later task intentionally changes response semantics, that should be reviewed separately before implementation.

## Diagnosis
The codebase already has useful protections:
- authenticated routes are grouped behind bearer-token auth
- main page, workspace, thread, comment, search, and revision flows consistently check membership in the application layer
- refresh tokens are opaque random values and stored hashed
- SQL execution is mostly parameterized
- request body limits, rate limiting, overload shedding, and log sanitization are already in place

The remaining security gaps are more specific, but materially important:
- some service flows leak resource existence by checking membership after loading the target object
- JWT parsing is not strict enough about issuer and exact signing method
- rate limiting depends on client IP trust assumptions that are unsafe without an explicitly trusted proxy boundary
- the HTTP transport still lacks core security headers and explicit HTTPS boundary rules
- database connection security is not enforced by configuration validation
- secret and password policy are acceptable, but not strong enough for a higher-security baseline

## Guiding Policies
- Favor boring hardening over speculative redesign. Use the current Go, Chi, pgx, and PostgreSQL stack unless it cannot solve the problem cleanly.
- Fix broken-access-control risk before adding deeper defense-in-depth features.
- Treat proxy trust, transport security, and token validation as explicit security boundaries, not deployment assumptions hidden in docs.
- Keep security changes test-first and scoped to one slice at a time.
- Prefer stable transport behavior unless a security change clearly requires a contract adjustment.
- Reduce information leakage where possible by defaulting unauthorized foreign resources toward `404` semantics where appropriate.

## Status Model
- `not_started`
- `in_progress`
- `done`
- `blocked`

## Current Task
- `done` Task 9: security verification pass

## Tasks
1. `done` Token and secret hardening
   - tighten JWT validation in `internal/infrastructure/auth/token.go`
   - require exact expected signing algorithm, not any HMAC family method
   - validate issuer explicitly during token parsing
   - raise `JWT_SECRET` minimum strength in `internal/infrastructure/config/config.go`
   - add focused tests for malformed issuer, wrong algorithm, and weak secret rejection

2. `done` Resource existence leak reduction
   - audit page, thread, comment, and revision service flows that fetch by ID before membership checks
   - decide which paths should collapse unauthorized foreign resources to `not_found`
   - implement the change consistently in the application layer
   - add transport and service tests that verify no cross-workspace existence leak remains on those routes

3. `done` Trusted proxy and IP-based limiter hardening
   - define whether forwarded client IP headers are trusted
   - stop implicitly trusting spoofable forwarded IPs when not behind a trusted proxy
   - make rate-limit key derivation explicit and deployment-safe
   - add tests for direct-connect and trusted-proxy scenarios

4. `done` HTTP transport header hardening
  - add security headers middleware for API responses
  - cover at least:
    - `X-Content-Type-Options: nosniff`
    - `X-Frame-Options: DENY` or equivalent frame denial
    - `Referrer-Policy`
     - `Cache-Control: no-store` on sensitive auth responses if appropriate
   - document which HTTPS/HSTS behavior is enforced in app versus expected at the reverse proxy
   - add focused middleware tests

5. `done` Database connection security hardening
  - define minimum production expectations for PostgreSQL DSN security
  - reject clearly insecure production DSN configurations where feasible
  - keep local development workable without forcing production-only constraints locally
  - add tests around DSN validation and startup failure behavior

6. `done` Authentication abuse and account-enumeration review
  - review whether auth and invitation flows reveal too much about account existence
  - decide which user-facing conflicts are acceptable product behavior and which should be generalized
  - keep login generic and consider whether registration/invitation flows should become less revealing
  - add tests for the chosen behavior

7. `done` Password policy and credential lifecycle review
   - assess whether current password rules and bcrypt defaults are strong enough for the target threat model
   - decide whether to raise password requirements or move cost settings explicitly into configuration
   - avoid breaking existing auth flows casually; any stricter rule should be intentional and documented

8. `done` Authorization defense-in-depth review
   - evaluate whether selected high-risk repository reads/writes should gain stronger ownership scoping
   - consider whether PostgreSQL row-level security is warranted or unnecessary complexity for this app
   - if not adopted, document why application-layer authorization remains the chosen boundary

9. `done` Security verification pass
   - rerun focused auth, authorization, transport, and startup security tests
   - rerun `go test ./...`
   - review residual risks and deployment assumptions that remain outside code

## Execution Order
- Start with token validation and secret policy because those are compact, high-value hardening changes.
- Reduce resource-existence leaks next, because broken access control matters more than cosmetic transport hardening.
- Fix trusted-proxy and limiter boundary assumptions before relying on the existing rate limits as a security control.
- Add transport headers and database connection hardening only after the core auth/authz boundaries are tightened.

## Success Criteria
- Access tokens are validated against the intended issuer and exact signing algorithm.
- Cross-workspace resource probing does not trivially reveal object existence through status-code differences on the audited routes.
- IP-based rate limiting has an explicit and safe trust model for proxy headers.
- HTTP responses include baseline security headers appropriate for an API service.
- Production database configuration is less likely to run in an insecure transport mode by accident.
- Focused security regressions plus `go test ./...` pass after each task.

## Notes
- This roadmap is backend-only.
- This roadmap is intentionally separate from:
  - `docs/backend-hardening-roadmap.md`
  - `docs/security-error-hardening-roadmap.md`
- Update `docs/checkpoint.md` after each completed task.
- Do not modify `frontend-repo/API_CONTRACT.md` unless an intentional API behavior change is explicitly approved.
- Current known residual risks before this roadmap starts:
  - application-layer authorization is strong in many main flows, but some routes still leak resource existence through `403` versus `404`
  - rate limiting is only as trustworthy as the proxy/IP boundary in front of the service
  - no distributed rate limiting exists for multi-instance deployments
  - transport hardening still depends partly on deployment assumptions outside the application
- Task 1 completed:
  - `internal/infrastructure/auth/token.go` now validates access tokens with:
    - exact `HS256` method acceptance
    - explicit issuer validation
  - `internal/infrastructure/config/config.go` now requires `JWT_SECRET` to be at least 32 characters
  - focused regression coverage added in:
    - `internal/infrastructure/auth/token_additional_test.go`
    - `internal/infrastructure/config/config_test.go`
  - supporting auth/config fixtures were updated to use the stronger secret baseline
- Task 2 completed:
  - added `internal/application/resource_visibility.go` to centralize `403 -> 404` hiding for foreign resource-by-id membership failures
  - applied the hiding rule across audited resource-by-id service flows in:
    - `internal/application/page_service.go`
    - `internal/application/comment_service.go`
    - `internal/application/revision_service.go`
    - `internal/application/revision_restore.go`
    - `internal/application/revision_diff.go`
    - `internal/application/thread_service.go`
  - kept workspace-scoped list semantics unchanged, including:
    - `ListPages(workspaceID)`
    - `ListWorkspaceThreads(workspaceID)`
  - added focused regression coverage in:
    - `internal/application/page_service_test.go`
    - `internal/application/comment_service_test.go`
    - `internal/application/revision_service_test.go`
    - `internal/application/thread_service_test.go`
    - `internal/transport/http/server_test.go`
  - verified with focused tests and `go test ./...`
- Task 3 completed:
  - added explicit proxy trust configuration in `internal/infrastructure/config/config.go`:
    - `TRUST_PROXY_HEADERS`
    - `TRUSTED_PROXY_CIDRS`
  - default behavior is now safe-by-default:
    - forwarded IP headers are ignored unless proxy trust is explicitly enabled
  - added `internal/transport/http/middleware/client_ip.go` for deterministic client IP resolution
  - removed unconditional `chi` `RealIP` trust from `internal/transport/http/server.go`
  - rate limiting now keys off resolved client IP, not blindly trusted forwarded headers
  - request and failure logs now include `client_ip` while preserving raw peer `remote_addr`
  - `cmd/api/app.go` now wires proxy-trust config into server startup
  - added focused regression coverage in:
    - `internal/transport/http/middleware/middleware_test.go`
    - `internal/transport/http/server_test.go`
    - `internal/infrastructure/config/config_test.go`
  - verified with focused tests and `go test ./...`
- Task 4 completed:
  - added baseline browser-hardening response middleware in `internal/transport/http/middleware/middleware.go`
  - all responses now include:
    - `X-Content-Type-Options: nosniff`
    - `X-Frame-Options: DENY`
    - `Referrer-Policy: no-referrer`
  - added auth-route cache protection in `internal/transport/http/middleware/middleware.go`
    - `/api/v1/auth/*` responses now include:
      - `Cache-Control: no-store`
      - `Pragma: no-cache`
  - wired the security headers middleware globally and the no-store middleware only on auth routes in `internal/transport/http/server.go`
  - added focused regression coverage in:
    - `internal/transport/http/middleware/middleware_test.go`
    - `internal/transport/http/server_auth_workspace_test.go`
  - verified with focused tests and `go test ./...`
  - HTTPS and HSTS remain an explicit deployment boundary:
    - this app does not terminate TLS directly
    - HSTS should be enforced at the trusted reverse proxy / edge layer rather than guessed inside the app
- Task 5 completed:
  - added production-only PostgreSQL DSN transport validation in `internal/infrastructure/config/config.go`
  - `APP_ENV=production` now rejects insecure DSNs that:
    - disable TLS
    - allow plaintext fallback
  - non-production environments remain unchanged
  - added focused regression coverage in:
    - `internal/infrastructure/config/config_test.go`
  - verified with focused config tests and `go test ./...`
- Task 6 completed:
  - invitation creation in `internal/application/workspace_service.go` no longer requires the invitee email to already belong to a registered user
  - this removes the previous `422` user-existence leak on workspace invitation flows
  - invitation acceptance now hides mismatched-email probes as `not_found` instead of exposing invitation existence through a dedicated mismatch error
  - added focused regression coverage in:
    - `internal/application/workspace_service_additional_test.go`
    - `internal/transport/http/server_auth_workspace_test.go`
  - verified with focused auth/workspace tests and `go test ./...`
  - residual risk intentionally left in place:
    - duplicate registration still returns a conflict
    - fully hiding that would require a broader registration / email-verification redesign, not a small hardening slice
- Task 7 completed:
  - strengthened password validation in `internal/application/auth_service.go`
  - new registration password rule now requires:
    - minimum 8 characters
    - at least one uppercase letter
    - at least one lowercase letter
    - at least one number
  - made bcrypt work factor explicit in `internal/infrastructure/auth/password.go`
    - password hashing now uses `defaultBcryptCost = 12`
  - added focused regression coverage in:
    - `internal/application/auth_service_additional_test.go`
    - `internal/infrastructure/auth/password_test.go`
  - verified with focused auth/password tests and `go test ./...`
- Task 8 completed:
  - added optional repository-scoped page visibility lookup in `internal/repository/postgres/page_repository.go`
    - `GetVisibleByUserID(ctx, pageID, userID)`
  - added shared application fallback helper in `internal/application/resource_visibility.go`
    - services now use repository scoping when available
    - otherwise they still fall back to the existing application-layer membership check and `403 -> 404` hiding
  - routed page-based services through the shared helper in:
    - `internal/application/page_service.go`
    - `internal/application/comment_service.go`
    - `internal/application/revision_service.go`
    - `internal/application/revision_restore.go`
    - `internal/application/revision_diff.go`
    - `internal/application/thread_service.go`
  - added focused regression coverage in:
    - `internal/application/resource_visibility_test.go`
    - `internal/repository/postgres/content_repository_test.go`
  - explicit decision:
    - PostgreSQL row-level security is not adopted in this slice
    - the current clean-architecture boundary keeps authorization primarily in the application layer
    - selected repository scoping is used only as defense-in-depth on high-risk page-by-id access paths
  - verified with focused visibility tests and `go test ./...`
- Task 9 completed:
  - reran focused security verification across:
    - startup and build-path hardening in `cmd/api`
    - token, password, and secret-policy checks in `internal/infrastructure/auth`
    - production config validation in `internal/infrastructure/config`
    - authorization and visibility regressions in `internal/application`
    - transport, auth, and error-mapping regressions in `internal/transport/http`
    - middleware hardening regressions in `internal/transport/http/middleware`
    - repository visibility and persistence regressions in `internal/repository/postgres`
  - reran `go test ./...`
  - current roadmap scope is complete
  - residual risks intentionally left documented:
    - no distributed rate limiting for multi-instance deployments
    - TLS termination and HSTS still depend on the trusted reverse proxy / edge layer
    - duplicate registration still returns a conflict and would need a broader registration redesign to hide fully
    - application-layer authorization remains the primary control boundary instead of PostgreSQL row-level security
