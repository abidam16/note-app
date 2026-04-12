# Task 7 POST Workspace Invitation Reject Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add version-checked `POST /api/v1/workspace-invitations/{invitationID}/reject` so the invited user can reject a pending invitation safely under concurrency and receive the updated invitation state.

**Architecture:** This task adds the reject branch of the invitation state machine. The transport layer accepts `{ version }`, the service resolves the authenticated user, loads the invitation, hides foreign invitations as `not_found`, and delegates to a repository method that atomically locks the invitation row, verifies `pending + version match`, and marks the invitation `rejected`. This task does not add notification projection or outbox work; it only establishes the authoritative reject contract and race-safe persistence behavior.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, explicit SQL repositories, table-driven tests, PostgreSQL-backed repository tests

---

## 1. Dependencies

- Task 1 invitation state schema must be complete.
- Task 2 create-invitation contract must already expose invitation state fields publicly.
- Task 5 invitation update plan defines the shared version-concurrency behavior.
- Task 6 invitation accept plan defines the target-user visibility and version-check semantics that reject should mirror.

This task assumes invitations now persist at least:
- `status`
- `version`
- `updated_at`
- `responded_by`
- `responded_at`
- `accepted_at`
- `cancelled_at`

---

## 2. Scope

### In Scope
- Add one new endpoint:
  - `POST /api/v1/workspace-invitations/{invitationID}/reject`
- Require a JSON body with `version`
- Return the updated invitation
- Enforce optimistic concurrency through `version`
- Enforce invited-user-only rejection
- Add service, repository, handler, and HTTP tests
- Update API docs and checkpoint

### Out Of Scope
- No membership creation
- No invitation accept changes
- No invitation cancel changes
- No invitation notification projection work
- No outbox work in this task
- No frontend implementation

---

## 3. Detailed Spec

## 3.1 Endpoint

### `POST /api/v1/workspace-invitations/{invitationID}/reject`

- Auth: yes
- Authorization: invited user only

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

Response `200` returns the rejected `WorkspaceInvitation`:

```json
{
  "id": "uuid",
  "workspace_id": "uuid",
  "email": "invitee@example.com",
  "role": "editor",
  "status": "rejected",
  "version": 4,
  "invited_by": "uuid",
  "created_at": "2026-04-04T08:00:00Z",
  "updated_at": "2026-04-04T09:00:00Z",
  "accepted_at": null,
  "responded_by": "uuid",
  "responded_at": "2026-04-04T09:00:00Z",
  "cancelled_by": null,
  "cancelled_at": null
}
```

### Response Rules
- `status` becomes `rejected`
- `version` increments by `1`
- `updated_at` equals the rejection timestamp
- `responded_by` equals the authenticated user id
- `responded_at` equals the rejection timestamp
- `accepted_at` stays `null`
- `cancelled_by` stays `null`
- `cancelled_at` stays `null`
- `email` does not change
- `role` does not change

## 3.4 Validation Rules

1. Actor must be authenticated
2. Actor id from auth context must resolve to an existing user record
3. `version` must be present and greater than `0`
4. Invitation must exist
5. Actor email must match the invitation target email
6. Invitation must be in `pending` state
7. Request `version` must equal current invitation version

## 3.5 Behavior Rules

### Actor Resolution
- load the actor from `users.GetByID`
- if the token subject no longer maps to a user record:
  - return `401 unauthorized`

### Invitation Visibility Rule
- load the invitation by id
- if the authenticated user email does not match `invitation.email`:
  - return `404 not_found`

This keeps the endpoint from confirming invitation existence to the wrong user.

### Pending Only
- only `pending` invitations may be rejected
- if invitation is already `accepted`, `rejected`, or `cancelled`, return conflict

### Version Check
- if request version does not equal current version, return conflict
- this is the concurrency guard for:
  - reject vs update
  - reject vs accept
  - reject vs cancel
  - reject vs reject retry with stale state

### Rejection State Transition
- rejection updates only the invitation row
- rejection must not create membership
- rejection must not mutate invitation role or target email

### Notification Behavior
- do not add notification update or outbox writes in this task
- this task only fixes the authoritative invitation rejection contract

## 3.6 Positive And Negative Cases

### Positive Cases

1. Invited user rejects a pending invitation with the current version
- Result: `200`
- Invitation moves to `rejected`

2. Rejected invitation response carries final rejection metadata
- Result: `200`
- Response includes:
  - `status = rejected`
  - `responded_by`
  - `responded_at`

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

8. Authenticated user email does not match invitation email
- Result: `404 not_found`

9. Invitation already accepted
- Result: `409 conflict`

10. Invitation already rejected
- Result: `409 conflict`

11. Invitation already cancelled
- Result: `409 conflict`

12. Request version is stale
- Result: `409 conflict`

13. Token subject resolves to no user record
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
    "status": "rejected",
    "version": 4,
    "invited_by": "uuid",
    "created_at": "2026-04-04T08:00:00Z",
    "updated_at": "2026-04-04T09:00:00Z",
    "accepted_at": null,
    "responded_by": "uuid",
    "responded_at": "2026-04-04T09:00:00Z",
    "cancelled_by": null,
    "cancelled_at": null
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

### `404 not_found`
- invitation id does not exist
- actor email does not match invitation email

### `409 conflict`
- invitation is not pending
- version mismatch

### `422 validation_failed`
- missing `version`
- `version <= 0`

No `403` is used for invitation-target mismatch in this task. The endpoint should hide foreign invitations as `not_found`.

---

## 5. File Structure And Responsibilities

### Modify
- `internal/application/workspace_service.go`
  - add reject input and service method
- `internal/application/workspace_service_test.go`
- `internal/application/workspace_service_additional_test.go`
- `internal/repository/postgres/workspace_repository.go`
  - add repository reject method
- `internal/repository/postgres/user_workspace_refresh_repository_test.go`
  - extend integration coverage for rejected invitation state
- `internal/repository/postgres/additional_integration_test.go`
  - add focused terminal-state and version-conflict coverage if that is the cleaner location
- `internal/transport/http/handlers.go`
  - add request DTO and handler
- `internal/transport/http/server.go`
  - register route
- `internal/transport/http/server_auth_workspace_test.go`
  - add end-to-end reject contract coverage
- `internal/transport/http/server_invalid_json_test.go`
  - add invalid JSON case for reject body
- `frontend-repo/API_CONTRACT.md`
- `docs/checkpoint.md`

### Test Fake Updates Required
- `internal/application/workspace_service_test.go`
  - fake repo reject method and invitation-state behavior
- `internal/application/workspace_service_additional_test.go`
  - stub reject method and branch coverage
- `internal/transport/http/server_auth_workspace_test.go`
  - `httpWorkspaceRepo` reject method and invitation-state behavior

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

1. Invited user rejects pending invitation successfully
- Seed:
  - actor user exists
  - invitation is `pending`
  - invitation email matches actor email
  - version matches
- Expect:
  - service returns rejected invitation

2. Service passes expected version and actor id into repository reject method
- Expect:
  - repository seam sees:
    - `invitationID`
    - `userID`
    - `expectedVersion`
    - rejection timestamp

### Negative Service Cases

3. Missing or zero version returns validation error

4. Invitation not found returns not found

5. Unknown actor id returns unauthorized

6. Mismatched actor email returns not found

7. Accepted invitation returns conflict

8. Rejected invitation returns conflict

9. Cancelled invitation returns conflict

10. Stale version returns conflict

## 6.2 Repository Integration Tests

Add DB-backed tests in the PostgreSQL invitation test area.

### Positive Repository Cases

11. Reject pending invitation returns updated invitation

12. Reject pending invitation increments invitation version

13. Reject pending invitation sets:
- `status = rejected`
- `updated_at`
- `responded_by`
- `responded_at`

14. Reject pending invitation leaves:
- `accepted_at = null`
- `cancelled_by = null`
- `cancelled_at = null`

### Negative Repository Cases

15. Missing invitation returns `domain.ErrNotFound`

16. Accepted invitation returns `domain.ErrConflict`

17. Rejected invitation returns `domain.ErrConflict`

18. Cancelled invitation returns `domain.ErrConflict`

19. Stale version returns `domain.ErrConflict`

20. Concurrent reject/update or reject/accept only allows one winner
- if full race test is too heavy, at minimum cover deterministic version-conflict behavior after one transition wins

## 6.3 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_auth_workspace_test.go`
- `internal/transport/http/server_invalid_json_test.go`

### Positive HTTP Cases

21. Invited user rejects pending invitation with version and gets `200`
- Assert:
  - response contains invitation
  - invitation status is `rejected`
  - invitation version increments

22. Rejected response includes `responded_by` and `responded_at`

### Negative HTTP Cases

23. Invalid JSON returns `400`

24. Empty body returns `400`

25. Unknown field returns `400`

26. Missing version returns `422`

27. Zero version returns `422`

28. Invitation not found returns `404`

29. Mismatched actor email returns `404`

30. Accepted invitation returns `409`

31. Rejected invitation returns `409`

32. Cancelled invitation returns `409`

33. Stale version returns `409`

34. Unknown actor user returns `401`

## 6.4 Documentation Tests

35. `frontend-repo/API_CONTRACT.md` documents:
- reject endpoint
- request body with `version`
- rejected response shape
- `404` for mismatched invitation target
- `409` for stale version and terminal states

36. `docs/checkpoint.md` records:
- new reject endpoint
- reject request payload
- target-user visibility rule
- version concurrency rule

---

## 7. Execution Plan

### Task 1: Define failing service tests for version-checked invitation rejection

**Files:**
- Modify: `internal/application/workspace_service_test.go`
- Modify: `internal/application/workspace_service_additional_test.go`

- [ ] **Step 1: Add failing service test for successful rejection**

Cover:
- pending invitation
- matching actor email
- valid version
- rejected invitation result

- [ ] **Step 2: Add failing service tests for conflict and visibility branches**

Cover:
- unknown actor
- invitation not found
- mismatched email returns `not_found`
- accepted/rejected/cancelled conflict
- stale version conflict

- [ ] **Step 3: Run targeted service tests**

Run:
```powershell
go test ./internal/application -run "TestWorkspaceService" -count=1
```

Expected:
- FAIL because reject input/service method and repository seam do not exist yet

- [ ] **Step 4: Commit**

```bash
git add internal/application/workspace_service_test.go internal/application/workspace_service_additional_test.go
git commit -m "test: define invitation rejection service behavior"
```

### Task 2: Implement service input and rejection rules

**Files:**
- Modify: `internal/application/workspace_service.go`

- [ ] **Step 1: Extend repository interface**

Add:

```go
RejectInvitation(ctx context.Context, invitationID, userID string, expectedVersion int64, respondedAt time.Time) (domain.WorkspaceInvitation, error)
```

- [ ] **Step 2: Add service input type**

Add:

```go
type RejectInvitationInput struct {
    InvitationID string
    Version      int64
}
```

- [ ] **Step 3: Implement `WorkspaceService.RejectInvitation`**

Required behavior:
- validate `Version > 0`
- resolve actor with `users.GetByID`
- normalize unknown actor to `domain.ErrUnauthorized`
- load invitation by id
- if actor email does not match invitation email, return `domain.ErrNotFound`
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
git commit -m "feat: add version-checked invitation rejection service"
```

### Task 3: Add repository rejection transition and persistence tests

**Files:**
- Modify: `internal/repository/postgres/workspace_repository.go`
- Modify: `internal/repository/postgres/user_workspace_refresh_repository_test.go`
- Modify: `internal/repository/postgres/additional_integration_test.go`

- [ ] **Step 1: Add failing repository integration tests**

Cover:
- successful rejection
- rejected invitation state fields
- stale version conflict
- terminal state conflicts

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestWorkspaceRepository|TestInvitation" -count=1
```

Expected:
- FAIL because repository reject method does not exist yet

- [ ] **Step 3: Implement `RejectInvitation` as an atomic transition**

Recommended repository approach:
- begin transaction
- `SELECT ... FOR UPDATE` invitation row
- if no row: `ErrNotFound`
- if `status != pending`: `ErrConflict`
- if `version != expectedVersion`: `ErrConflict`
- update invitation fields:
  - `status = rejected`
  - `version = version + 1`
  - `updated_at = respondedAt`
  - `responded_by = userID`
  - `responded_at = respondedAt`
- keep:
  - `accepted_at = NULL`
  - `cancelled_by = NULL`
  - `cancelled_at = NULL`
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
git commit -m "feat: add invitation rejection repository transition"
```

### Task 4: Add HTTP request DTO, route, handler, and endpoint tests

**Files:**
- Modify: `internal/transport/http/handlers.go`
- Modify: `internal/transport/http/server.go`
- Modify: `internal/transport/http/server_auth_workspace_test.go`
- Modify: `internal/transport/http/server_invalid_json_test.go`

- [ ] **Step 1: Add failing invalid JSON tests**

Add reject endpoint coverage for:
- malformed JSON
- empty body
- unknown fields

- [ ] **Step 2: Add failing HTTP tests for positive and negative flows**

Cover:
- successful rejection
- unknown actor
- invitation not found
- mismatched actor email
- invalid version
- stale version
- terminal status conflicts

- [ ] **Step 3: Add request DTO and handler**

Expected request DTO:

```go
type rejectInvitationRequest struct {
    Version int64 `json:"version"`
}
```

Handler requirements:
- decode JSON body
- map JSON decode failures to `400 invalid_json`
- call service with `RejectInvitationInput`
- return `200` with updated invitation

- [ ] **Step 4: Register route in `server.go`**

Add:
- `r.Post("/workspace-invitations/{invitationID}/reject", s.handleRejectInvitation())`

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
git commit -m "feat: add invitation rejection endpoint"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update the reject invitation API contract**

Document:
- route
- request body with `version`
- rejected response shape
- `404` mismatch-hides-resource rule
- `409` for stale version and terminal states
- `422` for invalid version

- [ ] **Step 2: Update checkpoint**

Record:
- new reject endpoint
- reject returns the updated invitation
- rejection is version-checked and pending-only

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: add invitation rejection contract"
```

### Task 6: Full verification for Task 7

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
POST /api/v1/workspace-invitations/{invitationID}/reject
Content-Type: application/json

{ "version": 1 }
```

Verify:
- `200`
- invitation status is `rejected`
- invitation version increments
- `responded_by` and `responded_at` are set

Then retry with stale version and verify:
- `409`

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify invitation rejection task"
```

---

## 8. Acceptance Criteria

Task 7 is complete only when all are true:
- `POST /api/v1/workspace-invitations/{invitationID}/reject` exists
- it requires `{ "version": n }`
- only the invited user can reject the invitation
- wrong-user access returns `404`, not `403`
- only pending invitations can be rejected
- stale version returns `409`
- successful rejection returns the updated invitation in `rejected` state
- service, repository, and HTTP tests cover all positive and negative cases above
- docs and checkpoint reflect the new contract

## 9. Risks And Guardrails

- Do not leak invitation existence to the wrong user by returning `403` for email mismatch.
- Do not reject an invitation without version checking.
- Do not create membership during rejection.
- Do not add notification or outbox logic in this task.
- Preserve compatibility with the repo's existing error mapper:
  - unauthorized => `401`
  - not found => `404`
  - conflict => `409`
  - validation => `422`

## 10. Follow-On Tasks

This plan prepares for:
- Task 8 `POST /api/v1/workspace-invitations/{invitationID}/cancel`
- later invitation notification projection work
