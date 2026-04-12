# Task 8 POST Workspace Invitation Cancel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add version-checked `POST /api/v1/workspace-invitations/{invitationID}/cancel` so a current workspace owner can cancel a pending invitation safely under concurrency and receive the updated invitation state.

**Architecture:** This task adds the owner-managed cancel branch of the invitation state machine. The transport layer accepts `{ version }`, the service resolves the authenticated actor, loads the invitation, hides foreign invitations from non-members as `not_found`, returns `forbidden` for same-workspace non-owner members, and delegates to a repository method that atomically locks the invitation row, verifies `pending + version match`, and marks the invitation `cancelled`. This task does not add notification projection or outbox work; it only establishes the authoritative cancel contract and race-safe persistence behavior.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, explicit SQL repositories, table-driven tests, PostgreSQL-backed repository tests

---

## 1. Dependencies

- Task 1 invitation state schema must be complete.
- Task 2 create-invitation contract must already expose invitation state fields publicly.
- Task 5 invitation update plan defines the shared version-concurrency behavior.
- Task 6 invitation accept plan defines the version-checked terminal transition style.
- Task 7 invitation reject plan defines the non-accept terminal transition pattern.

This task assumes invitations now persist at least:
- `status`
- `version`
- `updated_at`
- `responded_by`
- `responded_at`
- `cancelled_by`
- `cancelled_at`

---

## 2. Scope

### In Scope
- Add one new endpoint:
  - `POST /api/v1/workspace-invitations/{invitationID}/cancel`
- Require a JSON body with `version`
- Return the updated invitation
- Enforce optimistic concurrency through `version`
- Enforce workspace-owner-only cancellation
- Hide foreign invitation ids from non-members
- Add service, repository, handler, and HTTP tests
- Update API docs and checkpoint

### Out Of Scope
- No invitation accept changes
- No invitation reject changes
- No membership creation or deletion
- No invitation notification projection work
- No outbox work in this task
- No frontend implementation

---

## 3. Detailed Spec

## 3.1 Endpoint

### `POST /api/v1/workspace-invitations/{invitationID}/cancel`

- Auth: yes
- Authorization: current workspace owner only

The invitation id remains in the URL. The request body carries the invitation `version` for concurrency control.

## 3.2 Request Payload

```json
{
  "version": 3
}
```

### Fields
- `version`
  - required
  - integer
  - must be greater than `0`
  - must equal the current invitation version

### Request Shape Rules
- request body must contain exactly one JSON object
- unknown fields are invalid
- empty body is invalid
- `role` is not accepted here
- `email` is not accepted here

## 3.3 Response Payload

Response `200` returns the cancelled `WorkspaceInvitation`:

```json
{
  "id": "uuid",
  "workspace_id": "uuid",
  "email": "invitee@example.com",
  "role": "editor",
  "status": "cancelled",
  "version": 4,
  "invited_by": "uuid",
  "created_at": "2026-04-04T08:00:00Z",
  "updated_at": "2026-04-04T09:00:00Z",
  "accepted_at": null,
  "responded_by": null,
  "responded_at": null,
  "cancelled_by": "uuid",
  "cancelled_at": "2026-04-04T09:00:00Z"
}
```

### Response Rules
- `status` becomes `cancelled`
- `version` increments by `1`
- `updated_at` equals the cancellation timestamp
- `cancelled_by` equals the authenticated owner id
- `cancelled_at` equals the cancellation timestamp
- `responded_by` stays `null`
- `responded_at` stays `null`
- `accepted_at` stays `null`
- `email` does not change
- `role` does not change

## 3.4 Validation Rules

1. Actor must be authenticated
2. Actor id from auth context must resolve to an existing user record
3. `version` must be present and greater than `0`
4. Invitation must exist
5. Actor must be a member of the invitation workspace to access it
6. Actor must have `owner` role in the invitation workspace
7. Invitation must be in `pending` state
8. Request `version` must equal current invitation version

## 3.5 Behavior Rules

### Actor Resolution
- load the actor from `users.GetByID`
- if the token subject no longer maps to a user record:
  - return `401 unauthorized`

### Workspace Visibility Rule
- load the invitation by id
- load membership for `invitation.workspace_id` and `actorID`
- if membership lookup fails with non-member access:
  - return `404 not_found`
- if membership exists but actor role is not `owner`:
  - return `403 forbidden`

This prevents outsiders from probing invitation ids while preserving normal owner-only semantics for existing workspace members.

### Owner Authorization Rule
- any current workspace owner may cancel a pending invitation
- cancellation is not limited to `invited_by`
- `cancelled_by` records who performed the cancellation

### Pending Only
- only `pending` invitations may be cancelled
- if invitation is already `accepted`, `rejected`, or `cancelled`, return conflict

### Version Check
- if request version does not equal current version, return conflict
- this is the concurrency guard for:
  - cancel vs update
  - cancel vs accept
  - cancel vs reject
  - cancel vs cancel retry with stale state

### Cancellation State Transition
- cancellation updates only the invitation row
- cancellation must not create or remove membership
- cancellation must not mutate invitation role or target email

### Notification Behavior
- do not add notification update or outbox writes in this task
- this task only fixes the authoritative invitation cancel contract

## 3.6 Positive And Negative Cases

### Positive Cases

1. Current owner cancels a pending invitation with the current version
- Result: `200`
- Invitation moves to `cancelled`

2. Another current owner, not equal to `invited_by`, cancels a pending invitation
- Result: `200`
- Invitation moves to `cancelled`
- `cancelled_by` records the acting owner

### Negative Cases

1. Missing auth token
- Result: `401 unauthorized`

2. Invalid JSON body
- Result: `400 invalid_json`

3. Empty JSON body
- Result: `400 invalid_json`

4. Unknown JSON field
- Result: `400 invalid_json`

5. Missing version
- Result: `422 validation_failed`

6. Non-positive version
- Result: `422 validation_failed`

7. Invitation not found
- Result: `404 not_found`

8. Actor is not a member of the invitation workspace
- Result: `404 not_found`

9. Actor is a member but not an owner
- Result: `403 forbidden`

10. Invitation already accepted
- Result: `409 conflict`

11. Invitation already rejected
- Result: `409 conflict`

12. Invitation already cancelled
- Result: `409 conflict`

13. Request version is stale
- Result: `409 conflict`

14. Token subject resolves to no user record
- Result: `401 unauthorized`

---

## 4. API Contract And Response Codes

## 4.1 Success

### `200 OK`

```json
{
  "data": {
    "id": "uuid",
    "workspace_id": "uuid",
    "email": "invitee@example.com",
    "role": "editor",
    "status": "cancelled",
    "version": 4,
    "invited_by": "uuid",
    "created_at": "2026-04-04T08:00:00Z",
    "updated_at": "2026-04-04T09:00:00Z",
    "accepted_at": null,
    "responded_by": null,
    "responded_at": null,
    "cancelled_by": "uuid",
    "cancelled_at": "2026-04-04T09:00:00Z"
  }
}
```

## 4.2 Failures

### `400 invalid_json`
- malformed JSON
- empty body
- multiple JSON values
- unknown fields

### `401 unauthorized`
- missing auth token
- invalid auth token
- actor id from token no longer resolves to a user

### `403 forbidden`
- actor is a member of the invitation workspace but does not have owner role

### `404 not_found`
- invitation id does not exist
- actor is not a member of the invitation workspace

### `409 conflict`
- invitation is not pending
- version mismatch

### `422 validation_failed`
- missing `version`
- `version <= 0`

---

## 5. File Structure And Responsibilities

### Modify
- `internal/application/workspace_service.go`
  - add cancel input and service method
- `internal/application/workspace_service_test.go`
- `internal/application/workspace_service_additional_test.go`
- `internal/repository/postgres/workspace_repository.go`
  - add repository cancel method
- `internal/repository/postgres/user_workspace_refresh_repository_test.go`
  - extend integration coverage for cancelled invitation state
- `internal/repository/postgres/additional_integration_test.go`
  - add focused terminal-state and version-conflict coverage if that is the cleaner location
- `internal/transport/http/handlers.go`
  - add request DTO and handler
- `internal/transport/http/server.go`
  - register route
- `internal/transport/http/server_auth_workspace_test.go`
  - add end-to-end cancel contract coverage
- `internal/transport/http/server_invalid_json_test.go`
  - add invalid JSON case for cancel body
- `frontend-repo/API_CONTRACT.md`
- `docs/checkpoint.md`

### Test Fake Updates Required
- `internal/application/workspace_service_test.go`
  - fake repo cancel method and invitation-state behavior
- `internal/application/workspace_service_additional_test.go`
  - stub cancel method and branch coverage
- `internal/transport/http/server_auth_workspace_test.go`
  - `httpWorkspaceRepo` cancel method and invitation-state behavior

### Files Explicitly Not In Scope
- `internal/application/notification_service.go`
- `internal/application/notification_events.go`
- `internal/repository/postgres/notification_repository.go`
- `internal/transport/http/server_notifications_test.go`

---

## 6. Test Plan

## 6.1 Application Service Tests

Add or update tests in:
- `internal/application/workspace_service_test.go`
- `internal/application/workspace_service_additional_test.go`

### Positive Service Cases

1. Owner cancels pending invitation successfully
- Seed:
  - actor user exists
  - invitation is `pending`
  - actor is an owner in the invitation workspace
  - version matches
- Expect:
  - service returns cancelled invitation

2. Different current owner can cancel invitation created by another owner
- Expect:
  - success
  - `cancelled_by` equals acting owner id

3. Service passes expected version and actor id into repository cancel method
- Expect:
  - repository seam sees:
    - `invitationID`
    - `cancelledBy`
    - `expectedVersion`
    - cancellation timestamp

### Negative Service Cases

4. Missing or zero version returns validation error

5. Invitation not found returns not found

6. Unknown actor id returns unauthorized

7. Non-member actor returns not found

8. Same-workspace non-owner returns forbidden

9. Accepted invitation returns conflict

10. Rejected invitation returns conflict

11. Cancelled invitation returns conflict

12. Stale version returns conflict

## 6.2 Repository Integration Tests

Add DB-backed tests in the PostgreSQL invitation test area.

### Positive Repository Cases

13. Cancel pending invitation returns updated invitation

14. Cancel pending invitation increments invitation version

15. Cancel pending invitation sets:
- `status = cancelled`
- `updated_at`
- `cancelled_by`
- `cancelled_at`

16. Cancel pending invitation leaves:
- `accepted_at = null`
- `responded_by = null`
- `responded_at = null`

### Negative Repository Cases

17. Missing invitation returns `domain.ErrNotFound`

18. Accepted invitation returns `domain.ErrConflict`

19. Rejected invitation returns `domain.ErrConflict`

20. Cancelled invitation returns `domain.ErrConflict`

21. Stale version returns `domain.ErrConflict`

22. Concurrent cancel/update or cancel/accept only allows one winner
- if full race test is too heavy, at minimum cover deterministic version-conflict behavior after one transition wins

## 6.3 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_auth_workspace_test.go`
- `internal/transport/http/server_invalid_json_test.go`

### Positive HTTP Cases

23. Owner cancels pending invitation with version and gets `200`
- Assert:
  - response contains invitation
  - invitation status is `cancelled`
  - invitation version increments

24. Another owner can cancel invitation created by a different owner

### Negative HTTP Cases

25. Invalid JSON returns `400`

26. Empty body returns `400`

27. Unknown field returns `400`

28. Missing version returns `422`

29. Zero version returns `422`

30. Invitation not found returns `404`

31. Non-member actor returns `404`

32. Same-workspace non-owner returns `403`

33. Accepted invitation returns `409`

34. Rejected invitation returns `409`

35. Cancelled invitation returns `409`

36. Stale version returns `409`

37. Unknown actor user returns `401`

## 6.4 Documentation Tests

38. `frontend-repo/API_CONTRACT.md` documents:
- cancel endpoint
- request body with `version`
- cancelled response shape
- `404` for outsider access
- `403` for same-workspace non-owner access
- `409` for stale version and terminal states

39. `docs/checkpoint.md` records:
- new cancel endpoint
- cancel request payload
- owner-only authorization rule
- outsider-hidden visibility rule
- version concurrency rule

---

## 7. Execution Plan

### Task 1: Define failing service tests for version-checked invitation cancellation

**Files:**
- Modify: `internal/application/workspace_service_test.go`
- Modify: `internal/application/workspace_service_additional_test.go`

- [ ] **Step 1: Add failing service test for successful cancellation**

Cover:
- pending invitation
- owner authorization
- valid version
- cancelled invitation result

- [ ] **Step 2: Add failing service tests for authorization and conflict branches**

Cover:
- unknown actor
- invitation not found
- non-member returns `not_found`
- non-owner member returns `forbidden`
- accepted/rejected/cancelled conflict
- stale version conflict

- [ ] **Step 3: Run targeted service tests**

Run:
```powershell
go test ./internal/application -run "TestWorkspaceService" -count=1
```

Expected:
- FAIL because cancel input/service method and repository seam do not exist yet

- [ ] **Step 4: Commit**

```bash
git add internal/application/workspace_service_test.go internal/application/workspace_service_additional_test.go
git commit -m "test: define invitation cancellation service behavior"
```

### Task 2: Implement service input and cancellation rules

**Files:**
- Modify: `internal/application/workspace_service.go`

- [ ] **Step 1: Extend repository interface**

Add:

```go
CancelInvitation(ctx context.Context, invitationID, cancelledBy string, expectedVersion int64, cancelledAt time.Time) (domain.WorkspaceInvitation, error)
```

- [ ] **Step 2: Add service input type**

Add:

```go
type CancelInvitationInput struct {
    InvitationID string
    Version      int64
}
```

- [ ] **Step 3: Implement `WorkspaceService.CancelInvitation`**

Required behavior:
- validate `Version > 0`
- resolve actor with `users.GetByID`
- normalize unknown actor to `domain.ErrUnauthorized`
- load invitation by id
- load actor membership on `invitation.WorkspaceID`
- if membership lookup returns non-member access, convert to `domain.ErrNotFound`
- if membership exists but actor role is not owner, return `domain.ErrForbidden`
- if invitation is not pending, return `domain.ErrConflict`
- delegate to repository with current timestamp and expected version

- [ ] **Step 4: Run targeted service tests**

Run:
```powershell
go test ./internal/application -run "TestWorkspaceService" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/workspace_service.go
git commit -m "feat: add version-checked invitation cancellation service"
```

### Task 3: Add repository cancellation transition and persistence tests

**Files:**
- Modify: `internal/repository/postgres/workspace_repository.go`
- Modify: `internal/repository/postgres/user_workspace_refresh_repository_test.go`
- Modify: `internal/repository/postgres/additional_integration_test.go`

- [ ] **Step 1: Add failing repository integration tests**

Cover:
- successful cancellation
- cancelled invitation state fields
- stale version conflict
- terminal state conflicts

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestWorkspaceRepository|TestInvitation" -count=1
```

Expected:
- FAIL because repository cancel method does not exist yet

- [ ] **Step 3: Implement `CancelInvitation` as an atomic transition**

Recommended repository approach:
- begin transaction
- `SELECT ... FOR UPDATE` invitation row
- if no row: `ErrNotFound`
- if `status != pending`: `ErrConflict`
- if `version != expectedVersion`: `ErrConflict`
- update invitation fields:
  - `status = cancelled`
  - `version = version + 1`
  - `updated_at = cancelledAt`
  - `cancelled_by = cancelledBy`
  - `cancelled_at = cancelledAt`
- keep:
  - `accepted_at = NULL`
  - `responded_by = NULL`
  - `responded_at = NULL`
- commit transaction
- return updated invitation

- [ ] **Step 4: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestWorkspaceRepository|TestInvitation" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/repository/postgres/workspace_repository.go internal/repository/postgres/user_workspace_refresh_repository_test.go internal/repository/postgres/additional_integration_test.go
git commit -m "feat: add invitation cancellation repository transition"
```

### Task 4: Add HTTP request DTO, route, handler, and endpoint tests

**Files:**
- Modify: `internal/transport/http/handlers.go`
- Modify: `internal/transport/http/server.go`
- Modify: `internal/transport/http/server_auth_workspace_test.go`
- Modify: `internal/transport/http/server_invalid_json_test.go`

- [ ] **Step 1: Add failing invalid JSON tests**

Add cancel endpoint coverage for:
- malformed JSON
- empty body
- unknown fields

- [ ] **Step 2: Add failing HTTP tests for positive and negative flows**

Cover:
- successful cancellation
- second owner cancellation
- unknown actor
- invitation not found
- non-member actor
- non-owner member
- invalid version
- stale version
- terminal status conflicts

- [ ] **Step 3: Add request DTO and handler**

Expected request DTO:

```go
type cancelInvitationRequest struct {
    Version int64 `json:"version"`
}
```

Handler requirements:
- decode JSON body
- map JSON decode failures to `400 invalid_json`
- call service with `CancelInvitationInput`
- return `200` with updated invitation

- [ ] **Step 4: Register route in `server.go`**

Add:
- `r.Post("/workspace-invitations/{invitationID}/cancel", s.handleCancelInvitation())`

- [ ] **Step 5: Re-run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "Test.*Invitation|Test.*Invite" -count=1
```

Expected:
- PASS

- [ ] **Step 6: Commit**

```bash
git add internal/transport/http/handlers.go internal/transport/http/server.go internal/transport/http/server_auth_workspace_test.go internal/transport/http/server_invalid_json_test.go
git commit -m "feat: add invitation cancellation endpoint"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update the cancel invitation API contract**

Document:
- route
- request body with `version`
- cancelled response shape
- outsider-hidden `404` rule
- same-workspace non-owner `403` rule
- `409` for stale version and terminal states
- `422` for invalid version

- [ ] **Step 2: Update checkpoint**

Record:
- new cancel endpoint
- cancel returns the updated invitation
- cancellation is version-checked and pending-only
- any current owner may cancel

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: add invitation cancellation contract"
```

### Task 6: Full verification for Task 8

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "TestWorkspaceService" -count=1
go test ./internal/repository/postgres -run "TestWorkspaceRepository|TestInvitation" -count=1
go test ./internal/transport/http -run "Test.*Invitation|Test.*Invite" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual API sanity check if local server is available**

Call:
```http
POST /api/v1/workspace-invitations/{invitationID}/cancel
Content-Type: application/json

{ "version": 1 }
```

Verify:
- `200`
- invitation status is `cancelled`
- invitation version increments
- `cancelled_by` and `cancelled_at` are set

Then retry with stale version and verify:
- `409`

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify invitation cancellation task"
```

---

## 8. Acceptance Criteria

Task 8 is complete only when all are true:
- `POST /api/v1/workspace-invitations/{invitationID}/cancel` exists
- it requires `{ "version": n }`
- only current workspace owners can cancel the invitation
- outsider access returns `404`
- same-workspace non-owner access returns `403`
- only pending invitations can be cancelled
- stale version returns `409`
- successful cancellation returns the updated invitation in `cancelled` state
- service, repository, and HTTP tests cover all positive and negative cases above
- docs and checkpoint reflect the new contract

## 9. Risks And Guardrails

- Do not leak invitation existence to outsiders by returning `403` for non-member access.
- Do not cancel an invitation without version checking.
- Do not create or remove membership during cancellation.
- Do not add notification or outbox logic in this task.
- Preserve compatibility with the repo's existing error mapper:
  - unauthorized => `401`
  - forbidden => `403`
  - not found => `404`
  - conflict => `409`
  - validation => `422`

## 10. Follow-On Tasks

This plan prepares for:
- Phase 2 notification schema and inbox work
- later invitation notification projection work
