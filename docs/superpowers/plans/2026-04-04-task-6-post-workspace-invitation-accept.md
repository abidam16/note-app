# Task 6 POST Workspace Invitation Accept Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add version-checked `POST /api/v1/workspace-invitations/{invitationID}/accept` so the invited user can accept a pending invitation safely under concurrency and receive both the accepted invitation state and the created membership.

**Architecture:** This task upgrades the existing accept-invitation path from a simple membership insert into an explicit invitation state transition. The transport layer now accepts `{ version }`, the service resolves the authenticated user, loads the invitation, hides foreign invitations as `not_found`, and delegates to a repository method that atomically locks the invitation row, verifies `pending + version match`, creates the membership, and marks the invitation `accepted`. This task keeps notifications and outbox work out of scope so the change stays focused on the invitation acceptance contract and race safety.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, explicit SQL repositories, table-driven tests, PostgreSQL-backed repository tests

---

## 1. Dependencies

- Task 1 invitation state schema must be complete.
- Task 2 create-invitation contract must already expose invitation state fields publicly.
- Task 5 invitation update plan defines the shared version-concurrency behavior and should be treated as the same invitation lifecycle model.

This task assumes invitations now persist at least:
- `status`
- `version`
- `updated_at`
- `responded_by`
- `responded_at`
- `accepted_at`

---

## 2. Scope

### In Scope
- Upgrade one existing endpoint:
  - `POST /api/v1/workspace-invitations/{invitationID}/accept`
- Require a JSON body with `version`
- Return both:
  - accepted invitation
  - created membership
- Enforce optimistic concurrency through `version`
- Enforce invited-user-only acceptance
- Make acceptance atomic with membership creation
- Add service, repository, handler, and HTTP tests
- Update API docs and checkpoint

### Out Of Scope
- No invitation reject behavior
- No invitation cancel behavior
- No invitation notification projection work
- No outbox work in this task
- No inbox changes
- No frontend implementation

---

## 3. Detailed Spec

## 3.1 Endpoint

### `POST /api/v1/workspace-invitations/{invitationID}/accept`

- Auth: yes
- Authorization: invited user only

The invitation id remains in the URL. The request body now carries the invitation `version` for concurrency control.

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

Response `200` returns both the accepted invitation and the created membership:

```json
{
  "invitation": {
    "id": "uuid",
    "workspace_id": "uuid",
    "email": "invitee@example.com",
    "role": "editor",
    "status": "accepted",
    "version": 4,
    "invited_by": "uuid",
    "created_at": "2026-04-04T08:00:00Z",
    "updated_at": "2026-04-04T09:00:00Z",
    "accepted_at": "2026-04-04T09:00:00Z",
    "responded_by": "uuid",
    "responded_at": "2026-04-04T09:00:00Z",
    "cancelled_by": null,
    "cancelled_at": null
  },
  "membership": {
    "id": "uuid",
    "workspace_id": "uuid",
    "user_id": "uuid",
    "role": "editor",
    "created_at": "2026-04-04T09:00:00Z"
  }
}
```

### Response Rules
- `invitation.status` becomes `accepted`
- `invitation.version` increments by `1`
- `invitation.updated_at` equals the acceptance timestamp
- `invitation.responded_by` equals the authenticated user id
- `invitation.responded_at` equals the acceptance timestamp
- `invitation.accepted_at` equals the acceptance timestamp
- `membership.role` must match the accepted invitation role
- `membership.workspace_id` must match the invitation workspace
- `membership.user_id` must match the authenticated user id

## 3.4 Validation Rules

1. Actor must be authenticated
2. Actor id from auth context must resolve to an existing user record
3. `version` must be present and greater than `0`
4. Invitation must exist
5. Actor email must match the invitation target email
6. Invitation must be in `pending` state
7. Request `version` must equal current invitation version
8. Actor must not already be an active member of the workspace

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
- only `pending` invitations may be accepted
- if invitation is already `accepted`, `rejected`, or `cancelled`, return conflict

### Version Check
- if request version does not equal current version, return conflict
- this is the concurrency guard for:
  - accept vs update
  - accept vs reject
  - accept vs cancel
  - accept vs accept retry with stale state

### Atomic Acceptance
- membership creation and invitation state transition must happen in one transaction
- if membership insert fails, invitation state must not move to `accepted`
- if invitation state update fails, membership insert must not remain committed

### Existing Membership Rule
- if the target user is already a workspace member, return conflict
- authoritative protection belongs in the transactional repository path
- a unique membership constraint should remain the final guard even if a higher-level pre-check is added later

### Notification Behavior
- do not add notification update or outbox writes in this task
- this task only fixes the authoritative invitation acceptance contract

## 3.6 Positive And Negative Cases

### Positive Cases

1. Invited user accepts a pending invitation with the current version
- Result: `200`
- Invitation moves to `accepted`
- Membership is created

2. Accepted invitation response returns both objects
- Result: `200`
- Response includes:
  - accepted invitation
  - created membership

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

13. User is already a member of the workspace
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
    "invitation": {
      "id": "uuid",
      "workspace_id": "uuid",
      "email": "invitee@example.com",
      "role": "editor",
      "status": "accepted",
      "version": 4,
      "invited_by": "uuid",
      "created_at": "2026-04-04T08:00:00Z",
      "updated_at": "2026-04-04T09:00:00Z",
      "accepted_at": "2026-04-04T09:00:00Z",
      "responded_by": "uuid",
      "responded_at": "2026-04-04T09:00:00Z",
      "cancelled_by": null,
      "cancelled_at": null
    },
    "membership": {
      "id": "uuid",
      "workspace_id": "uuid",
      "user_id": "uuid",
      "role": "editor",
      "created_at": "2026-04-04T09:00:00Z"
    }
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
- membership already exists

### `422 validation_failed`
- missing `version`
- `version <= 0`

No `403` is used for invitation-target mismatch in this task. The endpoint should hide foreign invitations as `not_found`.

---

## 5. File Structure And Responsibilities

### Modify
- `internal/application/workspace_service.go`
  - change accept input and result contract
- `internal/application/workspace_service_test.go`
- `internal/application/workspace_service_additional_test.go`
- `internal/repository/postgres/workspace_repository.go`
  - change acceptance repository method to version-checked invitation state transition
- `internal/repository/postgres/user_workspace_refresh_repository_test.go`
  - extend integration coverage for accepted invitation state and membership creation
- `internal/repository/postgres/additional_integration_test.go`
  - add focused conflict and concurrency-path coverage if that is the cleaner location
- `internal/transport/http/handlers.go`
  - add request DTO and update accept handler response shape
- `internal/transport/http/server.go`
  - route path stays the same, but handler signature changes
- `internal/transport/http/server_auth_workspace_test.go`
  - update end-to-end accept contract coverage
- `internal/transport/http/server_invalid_json_test.go`
  - add invalid JSON case for accept body
- `frontend-repo/API_CONTRACT.md`
- `docs/checkpoint.md`

### Test Fake Updates Required
- `internal/application/workspace_service_test.go`
  - fake repo accept method signature and invitation-state behavior
- `internal/application/workspace_service_additional_test.go`
  - stub accept method signature and branch coverage
- `internal/transport/http/server_auth_workspace_test.go`
  - `httpWorkspaceRepo` accept method signature and invitation-state behavior

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

1. Invited user accepts pending invitation successfully
- Seed:
  - actor user exists
  - invitation is `pending`
  - invitation email matches actor email
  - version matches
- Expect:
  - service returns accepted invitation
  - service returns created membership

2. Service passes expected version and actor id into repository accept method
- Expect:
  - repository seam sees:
    - `invitationID`
    - `userID`
    - `expectedVersion`
    - acceptance timestamp

### Negative Service Cases

3. Missing or zero version returns validation error

4. Invitation not found returns not found

5. Unknown actor id returns unauthorized

6. Mismatched actor email returns not found

7. Accepted invitation returns conflict

8. Rejected invitation returns conflict

9. Cancelled invitation returns conflict

10. Stale version returns conflict

11. Membership already exists returns conflict

## 6.2 Repository Integration Tests

Add DB-backed tests in the PostgreSQL invitation test area.

### Positive Repository Cases

12. Accept pending invitation creates workspace membership and returns accepted invitation

13. Accept pending invitation increments invitation version

14. Accept pending invitation sets:
- `status = accepted`
- `updated_at`
- `responded_by`
- `responded_at`
- `accepted_at`

15. Membership role equals invitation role

### Negative Repository Cases

16. Missing invitation returns `domain.ErrNotFound`

17. Accepted invitation returns `domain.ErrConflict`

18. Rejected invitation returns `domain.ErrConflict`

19. Cancelled invitation returns `domain.ErrConflict`

20. Stale version returns `domain.ErrConflict`

21. Existing membership returns `domain.ErrConflict`

22. Atomicity guard
- if membership insert conflicts, invitation state does not move to `accepted`

23. Concurrent accept/update or accept/cancel only allows one winner
- if full race test is too heavy, at minimum cover deterministic version-conflict behavior after one transition wins

## 6.3 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_auth_workspace_test.go`
- `internal/transport/http/server_invalid_json_test.go`

### Positive HTTP Cases

24. Invited user accepts pending invitation with version and gets `200`
- Assert:
  - response contains `invitation`
  - response contains `membership`
  - invitation status is `accepted`

25. Accepted response membership matches invitation role

### Negative HTTP Cases

26. Invalid JSON returns `400`

27. Empty body returns `400`

28. Unknown field returns `400`

29. Missing version returns `422`

30. Zero version returns `422`

31. Invitation not found returns `404`

32. Mismatched actor email returns `404`

33. Accepted invitation returns `409`

34. Rejected invitation returns `409`

35. Cancelled invitation returns `409`

36. Stale version returns `409`

37. Existing membership returns `409`

38. Unknown actor user returns `401`

## 6.4 Documentation Tests

39. `frontend-repo/API_CONTRACT.md` documents:
- request body with `version`
- accepted response shape `{ invitation, membership }`
- `404` for mismatched invitation target
- `409` for stale version and terminal states

40. `docs/checkpoint.md` records:
- new accept request payload
- accepted response now returns both invitation and membership
- mismatch-hides-resource rule
- version concurrency rule

---

## 7. Execution Plan

### Task 1: Define failing service tests for version-checked invitation acceptance

**Files:**
- Modify: `internal/application/workspace_service_test.go`
- Modify: `internal/application/workspace_service_additional_test.go`

- [ ] **Step 1: Add failing service test for successful acceptance**

Cover:
- pending invitation
- matching actor email
- valid version
- accepted invitation + membership result

- [ ] **Step 2: Add failing service tests for conflict and visibility branches**

Cover:
- unknown actor
- invitation not found
- mismatched email returns `not_found`
- accepted/rejected/cancelled conflict
- stale version conflict
- existing membership conflict

- [ ] **Step 3: Run targeted service tests**

Run:
```powershell
go test ./internal/application -run "TestWorkspaceService" -count=1
```

Expected:
- FAIL because accept input/result contract and repository seam have not been updated yet

- [ ] **Step 4: Commit**

```bash
git add internal/application/workspace_service_test.go internal/application/workspace_service_additional_test.go
git commit -m "test: define invitation acceptance service behavior"
```

### Task 2: Implement service input, result, and acceptance rules

**Files:**
- Modify: `internal/application/workspace_service.go`

- [ ] **Step 1: Extend repository interface**

Change accept repository method to:

```go
AcceptInvitation(ctx context.Context, invitationID, userID string, expectedVersion int64, acceptedAt time.Time) (domain.WorkspaceInvitation, domain.WorkspaceMember, error)
```

- [ ] **Step 2: Add service input and result types**

Add:

```go
type AcceptInvitationInput struct {
    InvitationID string
    Version      int64
}

type AcceptInvitationResult struct {
    Invitation domain.WorkspaceInvitation
    Membership domain.WorkspaceMember
}
```

- [ ] **Step 3: Implement `WorkspaceService.AcceptInvitation` with the new contract**

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
git commit -m "feat: add version-checked invitation acceptance service"
```

### Task 3: Add repository acceptance transition and persistence tests

**Files:**
- Modify: `internal/repository/postgres/workspace_repository.go`
- Modify: `internal/repository/postgres/user_workspace_refresh_repository_test.go`
- Modify: `internal/repository/postgres/additional_integration_test.go`

- [ ] **Step 1: Add failing repository integration tests**

Cover:
- successful acceptance
- accepted invitation state fields
- membership creation
- stale version conflict
- terminal state conflicts
- membership-exists conflict
- atomicity guard

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestWorkspaceRepository|TestInvitation" -count=1
```

Expected:
- FAIL because repository accept signature and state-transition behavior do not match the tests yet

- [ ] **Step 3: Implement `AcceptInvitation` as an atomic transition**

Recommended repository approach:
- begin transaction
- `SELECT ... FOR UPDATE` invitation row
- if no row: `ErrNotFound`
- if `status != pending`: `ErrConflict`
- if `version != expectedVersion`: `ErrConflict`
- insert workspace member
- on unique membership violation: `ErrConflict`
- update invitation fields:
  - `status = accepted`
  - `version = version + 1`
  - `updated_at = acceptedAt`
  - `accepted_at = acceptedAt`
  - `responded_by = userID`
  - `responded_at = acceptedAt`
- commit transaction
- return both invitation and membership

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
git commit -m "feat: make invitation acceptance atomic and version-checked"
```

### Task 4: Add HTTP request DTO, handler update, and endpoint tests

**Files:**
- Modify: `internal/transport/http/handlers.go`
- Modify: `internal/transport/http/server.go`
- Modify: `internal/transport/http/server_auth_workspace_test.go`
- Modify: `internal/transport/http/server_invalid_json_test.go`

- [ ] **Step 1: Add failing invalid JSON tests**

Add accept endpoint coverage for:
- malformed JSON
- empty body
- unknown fields

- [ ] **Step 2: Add failing HTTP tests for positive and negative flows**

Cover:
- successful acceptance
- response shape `{ invitation, membership }`
- unknown actor
- invitation not found
- mismatched actor email
- invalid version
- stale version
- terminal status conflicts
- existing membership conflict

- [ ] **Step 3: Add request DTO and update the handler**

Expected request DTO:

```go
type acceptInvitationRequest struct {
    Version int64 `json:"version"`
}
```

Handler requirements:
- decode JSON body
- map JSON decode failures to `400 invalid_json`
- call service with `AcceptInvitationInput`
- return `200` with `map[string]any{"invitation": result.Invitation, "membership": result.Membership}`

- [ ] **Step 4: Keep the existing route path and update route wiring only if needed**

Route remains:
- `r.Post("/workspace-invitations/{invitationID}/accept", s.handleAcceptInvitation())`

Only the handler behavior changes.

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
git commit -m "feat: update invitation acceptance endpoint contract"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update the accept invitation API contract**

Document:
- request body with `version`
- accepted response shape
- `404` mismatch-hides-resource rule
- `409` for stale version and terminal states
- `422` for invalid version

- [ ] **Step 2: Update checkpoint**

Record:
- accept endpoint now requires version
- accept returns invitation plus membership
- acceptance is version-checked and pending-only

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: update invitation acceptance contract"
```

### Task 6: Full verification for Task 6

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
POST /api/v1/workspace-invitations/{invitationID}/accept
Content-Type: application/json

{ "version": 1 }
```

Verify:
- `200`
- response contains both `invitation` and `membership`
- invitation status is `accepted`
- invitation version increments

Then retry with stale version and verify:
- `409`

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify invitation acceptance task"
```

---

## 8. Acceptance Criteria

Task 6 is complete only when all are true:
- `POST /api/v1/workspace-invitations/{invitationID}/accept` requires `{ "version": n }`
- only the invited user can accept the invitation
- wrong-user access returns `404`, not `403`
- only pending invitations can be accepted
- stale version returns `409`
- successful acceptance returns both accepted invitation state and created membership
- membership creation and invitation state transition are atomic
- service, repository, and HTTP tests cover all positive and negative cases above
- docs and checkpoint reflect the new contract

## 9. Risks And Guardrails

- Do not leak invitation existence to the wrong user by returning `403` for email mismatch.
- Do not accept an invitation without version checking.
- Do not update invitation state outside the same transaction that creates membership.
- Do not add notification or outbox logic in this task.
- Preserve compatibility with the repo's existing error mapper:
  - unauthorized => `401`
  - not found => `404`
  - conflict => `409`
  - validation => `422`

## 10. Follow-On Tasks

This plan prepares for:
- Task 7 `POST /api/v1/workspace-invitations/{invitationID}/reject`
- Task 8 `POST /api/v1/workspace-invitations/{invitationID}/cancel`
- later invitation notification projection work
