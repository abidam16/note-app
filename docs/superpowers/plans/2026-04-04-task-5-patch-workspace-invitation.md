# Task 5 PATCH Workspace Invitation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `PATCH /api/v1/workspace-invitations/{invitationID}` so a workspace owner can update a pending invitation in place by changing only its role, with explicit version-based race protection.

**Architecture:** This task adds one owner-only mutation endpoint for invitation role updates. The transport layer accepts `{ role, version }`, validates both fields, and passes a focused input object into the workspace service. The service loads the invitation, authorizes the actor against the invitation's workspace, and delegates to a repository method that performs a version-checked update on a locked invitation row. This task does not introduce notification projection changes; invitation notification updates remain future work.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, explicit SQL repositories, table-driven tests, PostgreSQL-backed repository tests

---

## 1. Dependencies

- Task 1 invitation state schema must be complete.
- Task 2 create-invitation contract must already expose:
  - `status`
  - `version`
  - `updated_at`
- Task 3 and Task 4 are not required for correctness, but the public `WorkspaceInvitation` shape should already be stable.

This plan assumes invitation state is explicit and persisted with `status`, `version`, and `updated_at`.

---

## 2. Scope

### In Scope
- Add one new endpoint:
  - `PATCH /api/v1/workspace-invitations/{invitationID}`
- Update only the invitation `role`
- Enforce optimistic concurrency through `version`
- Restrict updates to pending invitations
- Restrict updates to workspace owners
- Add service, repository, handler, and HTTP tests
- Update API docs and checkpoint

### Out Of Scope
- No target email changes
- No workspace change
- No accept, reject, or cancel behavior
- No notification side-effect changes
- No outbox work
- No frontend implementation

---

## 3. Detailed Spec

## 3.1 Endpoint

### `PATCH /api/v1/workspace-invitations/{invitationID}`

- Auth: yes
- Authorization: workspace `owner` only

The workspace is derived from the invitation row, not from the URL.

## 3.2 Request Payload

```json
{
  "role": "editor",
  "version": 3
}
```

### Fields
- `role`
  - required
  - string
  - one of `owner|editor|viewer`
- `version`
  - required
  - positive integer
  - must match the current invitation version

### Request Shape Rules
- request body must contain exactly one JSON object
- unknown fields are invalid
- `email` is not accepted here
- `workspace_id` is not accepted here

## 3.3 Response Payload

Response `200` returns the updated `WorkspaceInvitation`:

```json
{
  "id": "uuid",
  "workspace_id": "uuid",
  "email": "invitee@example.com",
  "role": "editor",
  "status": "pending",
  "version": 4,
  "invited_by": "uuid",
  "created_at": "2026-04-04T08:00:00Z",
  "updated_at": "2026-04-04T09:00:00Z",
  "accepted_at": null,
  "responded_by": null,
  "responded_at": null,
  "cancelled_by": null,
  "cancelled_at": null
}
```

### Response Rules
- `status` stays `pending`
- `email` does not change
- if `role` changed:
  - `version` increments by `1`
  - `updated_at` changes to the update timestamp
- if `role` is unchanged and `version` matches:
  - return `200`
  - return the current invitation unchanged
  - do not increment `version`
  - do not change `updated_at`

## 3.4 Validation Rules

1. Actor must be authenticated
2. `role` must be a valid workspace role
3. `version` must be present and greater than `0`
4. Invitation must exist
5. Actor must be owner of the invitation's workspace
6. Invitation must be in `pending` state
7. Request `version` must equal current invitation version

## 3.5 Behavior Rules

### Pending Only
- only `pending` invitations may be updated
- if invitation is `accepted`, `rejected`, or `cancelled`, return conflict

### Version Check
- if request version does not equal current version, return conflict
- this is the concurrency guard for:
  - update vs update
  - update vs accept
  - update vs reject
  - update vs cancel

### Same-Role No-Op
- if request role equals current role and version matches:
  - return `200`
  - do not write a change
  - do not bump version

This keeps the endpoint safe for harmless resubmission without creating fake history.

### Email Immutability
- target email cannot be changed in this endpoint
- clients cannot send email because unknown fields are invalid

### Notification Behavior
- do not add invitation update notification logic in this task
- this endpoint only updates authoritative invitation state
- notification projection for invitation updates belongs to later outbox/projector tasks

## 3.6 Positive And Negative Cases

### Positive Cases

1. Owner updates pending invitation from `viewer` to `editor`
- Result: `200`
- `role` changes
- `version` increments
- `updated_at` changes

2. Owner submits same role with current version
- Result: `200`
- no-op
- `version` unchanged
- `updated_at` unchanged

### Negative Cases

1. Missing auth token
- Result: `401 unauthorized`

2. Invalid JSON body
- Result: `400 invalid_json`

3. Unknown JSON field
- Result: `400 invalid_json`

4. Invalid role
- Result: `422 validation_failed`

5. Missing or non-positive version
- Result: `422 validation_failed`

6. Invitation not found
- Result: `404 not_found`

7. Actor is not owner of invitation workspace
- Result: `403 forbidden`

8. Invitation is already `accepted`
- Result: `409 conflict`

9. Invitation is already `rejected`
- Result: `409 conflict`

10. Invitation is already `cancelled`
- Result: `409 conflict`

11. Request version is stale
- Result: `409 conflict`

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
    "status": "pending",
    "version": 4,
    "invited_by": "uuid",
    "created_at": "2026-04-04T08:00:00Z",
    "updated_at": "2026-04-04T09:00:00Z"
  }
}
```

## 4.2 Failures

### `400 invalid_json`
- malformed JSON
- multiple JSON values
- unknown fields

### `401 unauthorized`
- missing or invalid auth token

### `403 forbidden`
- actor is not owner of the invitation workspace

### `404 not_found`
- invitation id does not exist

### `409 conflict`
- invitation is not pending
- version mismatch

### `422 validation_failed`
- invalid role
- missing version
- non-positive version

No `500` behavior is introduced by this task beyond unexpected internal errors already handled by the shared error mapper.

---

## 5. File Structure And Responsibilities

### Modify
- `internal/application/workspace_service.go`
  - add update input type and service method
- `internal/application/workspace_service_test.go`
- `internal/application/workspace_service_additional_test.go`
- `internal/repository/postgres/workspace_repository.go`
  - add repository update method
- `internal/repository/postgres/user_workspace_refresh_repository_test.go`
  - or the current invitation-focused PostgreSQL test file
- `internal/repository/postgres/additional_integration_test.go`
  - if this is the better location for focused invitation race cases
- `internal/transport/http/handlers.go`
  - add request DTO and handler
- `internal/transport/http/server.go`
  - register route
- `internal/transport/http/server_auth_workspace_test.go`
  - add end-to-end HTTP coverage
- `internal/transport/http/server_invalid_json_test.go`
  - add invalid JSON case for the new endpoint
- `frontend-repo/API_CONTRACT.md`
- `docs/checkpoint.md`

### Test Fake Updates Required
- `internal/application/workspace_service_test.go`
  - fake repo interface additions
- `internal/application/workspace_service_additional_test.go`
  - stub additions
- `internal/transport/http/server_auth_workspace_test.go`
  - `httpWorkspaceRepo` additions

### Files Explicitly Not In Scope
- `internal/application/notification_service.go`
- `internal/application/notification_events.go`
- `internal/repository/postgres/notification_repository.go`
- `frontend-repo/CONTEXT.md`

---

## 6. Test Plan

## 6.1 Application Service Tests

Add or update tests in:
- `internal/application/workspace_service_test.go`
- `internal/application/workspace_service_additional_test.go`

### Positive Service Cases

1. Owner updates pending invitation role successfully
- Seed:
  - invitation in `pending`
  - owner membership on invitation workspace
- Expect:
  - returned role updated
  - version incremented
  - updated_at advanced

2. Owner submits same role with matching version
- Expect:
  - success
  - invitation unchanged
  - no version bump

### Negative Service Cases

3. Invalid role returns validation error

4. Missing or zero version returns validation error

5. Invitation not found returns not found

6. Non-owner returns forbidden

7. Accepted invitation returns conflict

8. Rejected invitation returns conflict

9. Cancelled invitation returns conflict

10. Stale version returns conflict

## 6.2 Repository Integration Tests

Add DB-backed tests in the PostgreSQL invitation test area.

### Positive Repository Cases

11. Update pending invitation role changes role and increments version

12. Update pending invitation sets `updated_at`

13. Same-role update with matching version returns unchanged invitation

### Negative Repository Cases

14. Missing invitation returns `domain.ErrNotFound`

15. Accepted invitation update returns `domain.ErrConflict`

16. Rejected invitation update returns `domain.ErrConflict`

17. Cancelled invitation update returns `domain.ErrConflict`

18. Stale version returns `domain.ErrConflict`

19. Concurrent updates only allow one winner
- if repository test style supports it, add a focused race test
- otherwise cover version mismatch deterministically

## 6.3 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_auth_workspace_test.go`
- `internal/transport/http/server_invalid_json_test.go`

### Positive HTTP Cases

20. Owner updates pending invitation and gets `200`
- Assert:
  - role updated
  - status still pending
  - version incremented

21. Owner submits same role and gets `200`
- Assert:
  - same role
  - same version

### Negative HTTP Cases

22. Invalid JSON returns `400`

23. Unknown field returns `400`

24. Invalid role returns `422`

25. Missing or zero version returns `422`

26. Non-owner returns `403`

27. Invitation not found returns `404`

28. Accepted invitation returns `409`

29. Rejected invitation returns `409`

30. Cancelled invitation returns `409`

31. Stale version returns `409`

## 6.4 Documentation Tests

32. `frontend-repo/API_CONTRACT.md` documents:
- endpoint
- request payload
- pending-only update rule
- version concurrency rule
- response payload
- all positive and negative response codes

33. `docs/checkpoint.md` records:
- new invitation update endpoint
- pending-only mutation rule
- version conflict behavior
- same-role no-op semantics

---

## 7. Execution Plan

### Task 1: Define failing service tests for invitation update behavior

**Files:**
- Modify: `internal/application/workspace_service_test.go`
- Modify: `internal/application/workspace_service_additional_test.go`

- [ ] **Step 1: Add failing service tests for successful role update**

Cover:
- pending invitation
- owner authorization
- role change
- version increment

- [ ] **Step 2: Add failing service test for same-role no-op**

Expect:
- `200`-equivalent service success
- unchanged version
- unchanged updated_at

- [ ] **Step 3: Add failing service tests for conflicts and validation**

Cover:
- invalid role
- invalid version
- non-owner
- not found
- accepted/rejected/cancelled conflict
- stale version conflict

- [ ] **Step 4: Run targeted service tests**

Run:
```powershell
go test ./internal/application -run "TestWorkspaceService" -count=1
```

Expected:
- FAIL because update method and repository seam do not exist yet

- [ ] **Step 5: Commit**

```bash
git add internal/application/workspace_service_test.go internal/application/workspace_service_additional_test.go
git commit -m "test: define invitation update service behavior"
```

### Task 2: Implement service input and validation

**Files:**
- Modify: `internal/application/workspace_service.go`

- [ ] **Step 1: Extend repository interface**

Add:
- `UpdateInvitationRole(ctx context.Context, invitationID string, role domain.WorkspaceRole, expectedVersion int64, updatedAt time.Time) (domain.WorkspaceInvitation, error)`

- [ ] **Step 2: Add service input type**

Add:
```go
type UpdateInvitationInput struct {
    InvitationID string
    Role         domain.WorkspaceRole
    Version      int64
}
```

- [ ] **Step 3: Implement `WorkspaceService.UpdateInvitation`**

Required behavior:
- validate role
- validate `Version > 0`
- load invitation by id
- load actor membership for `invitation.WorkspaceID`
- require owner role
- delegate to repository update method with current timestamp

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
git commit -m "feat: add invitation update service"
```

### Task 3: Add repository update method and persistence tests

**Files:**
- Modify: `internal/repository/postgres/workspace_repository.go`
- Modify: `internal/repository/postgres/user_workspace_refresh_repository_test.go`
- Modify: `internal/repository/postgres/additional_integration_test.go`

- [ ] **Step 1: Add failing repository integration tests**

Cover:
- successful pending update
- same-role no-op
- stale version conflict
- terminal state conflicts
- missing invitation

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestWorkspaceRepository|TestInvitation" -count=1
```

Expected:
- FAIL because update method does not exist yet

- [ ] **Step 3: Implement `UpdateInvitationRole`**

Recommended repository approach:
- begin transaction
- `SELECT ... FOR UPDATE` invitation row
- if no row: `ErrNotFound`
- if status != `pending`: `ErrConflict`
- if version mismatch: `ErrConflict`
- if role unchanged: return current invitation unchanged
- else `UPDATE` role, version, updated_at and return updated row

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
git commit -m "feat: add invitation update repository"
```

### Task 4: Add HTTP route, request DTO, handler, and endpoint tests

**Files:**
- Modify: `internal/transport/http/server.go`
- Modify: `internal/transport/http/handlers.go`
- Modify: `internal/transport/http/server_auth_workspace_test.go`
- Modify: `internal/transport/http/server_invalid_json_test.go`

- [ ] **Step 1: Add failing invalid JSON coverage**

Add case:
- `PATCH /api/v1/workspace-invitations/{invitationID}`

- [ ] **Step 2: Add failing HTTP tests for positive and negative flows**

Cover:
- successful update
- same-role no-op
- non-owner forbidden
- invalid role
- invalid version
- stale version
- terminal status conflict
- not found

- [ ] **Step 3: Register route in `server.go`**

Add:
- `r.Patch("/workspace-invitations/{invitationID}", s.handleUpdateInvitation())`

- [ ] **Step 4: Add request DTO and handler**

Handler requirements:
- decode JSON body
- map invalid JSON to `400`
- call service
- return `200` with updated invitation

Expected request DTO:
```go
type updateInvitationRequest struct {
    Role    string `json:"role"`
    Version int64  `json:"version"`
}
```

- [ ] **Step 5: Re-run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "Test.*Invitation|Test.*Invite" -count=1
```

Expected:
- PASS

- [ ] **Step 6: Commit**

```bash
git add internal/transport/http/server.go internal/transport/http/handlers.go internal/transport/http/server_auth_workspace_test.go internal/transport/http/server_invalid_json_test.go
git commit -m "feat: add invitation update endpoint"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Add endpoint contract to API docs**

Document:
- route
- request payload
- pending-only update rule
- version concurrency rule
- same-role no-op behavior
- response shape
- error cases

- [ ] **Step 2: Update checkpoint**

Record:
- new endpoint
- owner-only permission
- pending-only mutation
- conflict semantics for stale version and terminal states

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: add invitation update contract"
```

### Task 6: Full verification for Task 5

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
PATCH /api/v1/workspace-invitations/{invitationID}
Content-Type: application/json

{ "role": "editor", "version": 1 }
```

Verify:
- `200`
- role updated
- status still pending
- version incremented

Then retry with stale version and verify:
- `409`

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify invitation update task"
```

---

## 8. Acceptance Criteria

Task 5 is complete only when all are true:
- `PATCH /api/v1/workspace-invitations/{invitationID}` exists
- only owners of the invitation workspace can use it
- only pending invitations can be updated
- request requires valid `role` and positive `version`
- stale version returns `409`
- same-role update succeeds as a no-op
- service, repository, and HTTP tests cover all positive and negative cases above
- docs and checkpoint reflect the endpoint contract

## 9. Risks And Guardrails

- Do not add invitation notification updates in this task.
- Do not allow email mutation through this endpoint.
- Do not silently ignore stale versions; always return conflict.
- Keep the no-op rule narrow:
  - same role with matching version only
  - terminal states still conflict

## 10. Follow-On Tasks

This plan prepares for:
- Task 6 `POST /api/v1/workspace-invitations/{invitationID}/accept`
- Task 7 `POST /api/v1/workspace-invitations/{invitationID}/reject`
- Task 8 `POST /api/v1/workspace-invitations/{invitationID}/cancel`
- later invitation notification projection work
