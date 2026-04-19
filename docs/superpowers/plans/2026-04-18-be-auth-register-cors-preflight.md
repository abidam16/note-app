# Auth Register CORS Preflight Backend Plan

## 1. Task Summary

- Task: Add backend CORS and preflight handling so browser registration requests from the frontend dev origin can complete successfully.
- Why this task exists: `OPTIONS /api/v1/auth/register` currently returns `405 Method Not Allowed`, which blocks the browser from sending the actual cross-origin `POST /api/v1/auth/register`.

## 2. Objective

- Objective: Make cross-origin auth endpoints accept valid browser preflight requests and return the required CORS headers for the allowed frontend origin(s).

## 3. Scope

### In Scope

- Reproduce and confirm the `OPTIONS /api/v1/auth/register` `405` failure in the backend repo.
- Identify where the backend router or middleware stack rejects `OPTIONS`.
- Add or correct CORS middleware so allowed frontend origins receive successful preflight responses.
- Ensure auth endpoints needed by the signed-out flow support the same preflight behavior where appropriate:
  - `POST /api/v1/auth/register`
  - `POST /api/v1/auth/login`
  - `POST /api/v1/auth/refresh`
  - `POST /api/v1/auth/logout`
  - `GET /api/v1/auth/me` if frontend calls it cross-origin during bootstrap
- Add backend tests or request-level verification for allowed origin, method, and headers.
- Document the expected allowed origin configuration for local development.

### Out of Scope

- Changing auth business logic, validation, or response payloads.
- Reworking token issuance, refresh semantics, or session persistence.
- Broad API gateway or reverse proxy redesign unrelated to CORS/preflight.
- Frontend request-path changes in this repo.

## 4. Detailed Specification

- Confirm the backend currently rejects browser preflight for at least one auth endpoint with `405`.
- Add or correct middleware so `OPTIONS` is handled before endpoint method matching rejects the request.
- Successful preflight responses must, at minimum, align with the actual frontend request shape:
  - allow the frontend dev origin(s), such as `http://localhost:5173` or the active Vite origin in use
  - allow methods required by the auth surface
  - allow `Content-Type` and `Authorization` headers where used
- CORS behavior must be explicit rather than wildcarding credentials-sensitive flows unless that is already an accepted backend policy.
- This backend currently uses bearer tokens in the `Authorization` header and refresh tokens in the JSON request body rather than browser cookies. Do not enable `Access-Control-Allow-Credentials` for this task unless the auth transport model changes separately.
- The implementation should prefer centralized middleware or router-level handling over per-handler ad hoc responses.
- The current transport stack does not appear to have any CORS middleware. Implement one centralized CORS handling path rather than route-by-route `OPTIONS` handlers.
- Verify that a successful preflight is followed by a successful or business-meaningful auth response path rather than transport failure.

## 5. Files / Components to Change

### Expected Changes

- Backend HTTP server bootstrap or router setup where middleware order is defined.
- Existing CORS configuration module, if present.
- Auth route registration or route-group middleware wiring, only if current routing bypasses central preflight handling.
- Backend integration tests or request-level tests that exercise preflight behavior.
- Backend environment or developer documentation describing allowed origin configuration.

### Must Not Change

- Auth DTO contracts or API envelope shape.
- Frontend code in this repo.
- Unrelated feature routes outside the affected cross-origin policy unless the backend uses a shared global CORS layer.

## 6. Validation and Test

### Validation

- `OPTIONS /api/v1/auth/register` returns `204 No Content` for an allowed frontend origin with `Origin` and `Access-Control-Request-Method` headers present.
- Preflight response includes the expected CORS headers for origin, method, and request headers.
- The subsequent `POST /api/v1/auth/register` is no longer blocked by the browser due to preflight failure.
- Other signed-out auth endpoints needed by the frontend dev flow show consistent preflight behavior.
- Disallowed origins still fail according to backend policy.

### Tests

- Add or update backend request tests for auth preflight handling.
- Add or update backend CORS tests for allowed and disallowed origins if such coverage exists.
- Manually verify with browser devtools or `curl` using `Origin`, `Access-Control-Request-Method`, and `Access-Control-Request-Headers`.

## 7. Review Checklist

- `OPTIONS` is handled before route method rejection.
- Allowed origins are explicit and environment-configurable.
- Required request headers are allowed without over-broad policy expansion.
- The fix is centralized and does not duplicate CORS logic per endpoint.
- Tests prove preflight behavior directly rather than inferring it from frontend behavior.

## 8. Trade-offs and Risks

- Overly broad CORS allowances would remove the immediate failure but weaken backend policy.
- Narrow per-route patches may fix `/auth/register` while leaving `/auth/login` and related endpoints inconsistent.
- Middleware ordering mistakes can make local tests pass in one stack path while production still rejects preflight.

## 9. Future Improvements

- Audit non-auth API groups for the same preflight gap if the backend serves the frontend cross-origin in other areas.
- Add a shared backend troubleshooting note for CORS/preflight failures in local development.

- `artifact_type: PLAN`
- `artifact_status: UPDATED`
- `decision: PROCEED_TO_IMPLEMENTATION`
- `task_summary: Backend auth CORS preflight handling`
- `objective: Ensure browser preflight succeeds for allowed frontend origins so auth endpoints are reachable cross-origin.`
- `in_scope: Central backend preflight handling, auth endpoint coverage, tests, and local-dev origin configuration notes.`
- `out_of_scope: Auth business logic, token semantics, frontend code, and unrelated infrastructure redesign.`
- `detailed_spec: Handle `OPTIONS` before method rejection, return `204 No Content` plus the required CORS headers for allowed origins, keep policy explicit and centralized, and do not enable credentialed CORS for the current bearer-token auth model.`
- `expected_changes: Backend middleware or router setup, CORS configuration, tests, and local-dev documentation.`
- `must_not_change: API envelopes, auth contracts, frontend code in this repo, and unrelated route groups unless required by shared middleware.`
- `validation_requirements: Successful preflight for allowed origins, consistent auth endpoint behavior, and preserved rejection for disallowed origins.`
- `test_requirements: Backend request/integration coverage for preflight plus manual verification of CORS headers.`
- `review_checkpoints: Centralized handling, explicit allowed origins, minimal scope, and direct proof through tests.`
- `tradeoffs_and_risks: Avoid over-broad CORS policy and avoid route-by-route patches that leave sibling endpoints inconsistent.`
- `future_improvements: Audit other cross-origin routes and add reusable backend troubleshooting guidance.`
- `why: This is the same bounded backend transport task, but the execution contract needed to be tightened so implementation can proceed without ambiguity about status, source-of-truth docs, credentials policy, or expected preflight response semantics.`
- `source_artifacts: AGENTS.md, PRD.md, ARCHITECTURE.md, docs/workflow/ARTIFACT_DECISION_MATRIX.md, docs/workflow/HANDOFF_CONTRACTS.md, frontend-repo/API_CONTRACT.md, frontend-repo/src/config/apiBaseUrl.ts, frontend-repo/src/api/client.ts, .env`
- `next_step: Implement this plan in the backend repository as one bounded task.`

Decision

Update the existing single-task backend plan for auth CORS preflight handling.

Why this decision

The task identity did not change, but the plan needed sharper execution semantics before implementation: consistent status, correct source artifacts, explicit non-credentialed CORS policy, and exact preflight response expectations.

Plan Status

`PLAN_UPDATE`

Immediate Next Step

Implement this plan in the backend repository as one bounded task.

Continuation Prompt

Proceed to implement this plan in the backend repository as one bounded task.
