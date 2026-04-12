# Task 2 POST Workspace Invitations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade `POST /api/v1/workspaces/{workspaceID}/invitations` to use the new invitation state model publicly, keep the security-hardened allowance for unregistered emails, and prevent inviting a user who is already a workspace member.

**Architecture:** This task is an endpoint-focused slice built on Task 1. The transport contract for invitation creation becomes the first public surface that exposes the explicit invitation state fields. The application layer keeps owner-only authorization and email normalization, adds a member-existence guard, and reuses the Task 1 repository foundation so creation persists `pending` invitations with versioned state.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, table-driven Go tests, PostgreSQL-backed repository tests

---

## 1. Dependencies

- Task 1 invitation state schema must be complete first.
- The database must already contain:
  - `workspace_invitations.status`
  - `workspace_invitations.version`
  - `workspace_invitations.updated_at`
  - pending uniqueness based on `status = 'pending'`

This plan assumes Task 1 landed and is stable.

---

## 2. Scope

### In Scope
- Update the create-invitation endpoint contract
- Expose invitation state fields publicly on create response
- Keep unregistered invitee emails allowed
- Reject invitation when the target email already belongs to an existing workspace member
- Keep current notification hook behavior unchanged for now
- Update tests and docs for the true endpoint contract

### Out Of Scope
- No new endpoints
- No invitation list endpoints
- No invitation update, reject, or cancel flows
- No outbox or projector work
- No unread notification changes
- No frontend implementation

---

## 3. Detailed Spec

## 3.1 Endpoint

### `POST /api/v1/workspaces/{workspaceID}/invitations`

- Auth: yes
- Authorization: workspace `owner` only

## 3.2 Request Payload

```json
{
  "email": "invitee@example.com",
  "role": "viewer"
}
```

### Fields
- `email`
  - required
  - string
  - normalized before persistence
- `role`
  - required
  - one of `owner|editor|viewer`

## 3.3 Response Payload

Response `201` returns `WorkspaceInvitation` with the explicit state fields now public:

```json
{
  "id": "uuid",
  "workspace_id": "uuid",
  "email": "invitee@example.com",
  "role": "viewer",
  "status": "pending",
  "version": 1,
  "invited_by": "uuid",
  "created_at": "2026-04-04T08:00:00Z",
  "updated_at": "2026-04-04T08:00:00Z",
  "accepted_at": null,
  "responded_by": null,
  "responded_at": null,
  "cancelled_by": null,
  "cancelled_at": null
}
```

### Response Rules
- `status` must always be `pending` for successful create
- `version` must always be `1` for successful create
- `updated_at` must equal `created_at` for successful create
- `accepted_at`, `responded_by`, `responded_at`, `cancelled_by`, and `cancelled_at` must be omitted or null according to the project’s JSON encoding pattern

## 3.4 Validation Rules

1. Actor must be authenticated
2. Actor must be a member of the workspace
3. Actor must be `owner`
4. `role` must be a valid workspace role
5. `email` must be a valid email format
6. `email` must be normalized before duplicate checks and persistence
7. If the normalized email belongs to an existing user who is already a member of the workspace, return conflict
8. If there is already a pending invitation for the same `(workspace_id, email)`, return conflict
9. Unregistered email is allowed
10. Unknown JSON fields remain invalid because `DecodeJSON` already disallows them

## 3.5 Behavior Rules

### Allowed Unregistered Email
This is the true backend behavior and must stay true:
- if `users.GetByEmail(normalizedEmail)` returns `not_found`
- create invitation anyway
- do not fail with validation error

Reason:
- current backend intentionally avoids user-existence leakage on invitation creation

### Existing Member Conflict
If `users.GetByEmail(normalizedEmail)` returns a user and that user's id already appears in `workspaces.ListMembers(workspaceID)`, invitation creation must fail with conflict.

### Duplicate Pending Invitation Conflict
If `workspaces.GetActiveInvitationByEmail(workspaceID, normalizedEmail)` finds a pending invitation, invitation creation must fail with conflict.

### Notification Side Effect
Keep the current behavior for now:
- `NotifyInvitationCreated` is still called after persistence
- notification failure still bubbles up in this task because outbox is a later task

Do not change notification reliability semantics in Task 2.

## 3.6 Positive And Negative Cases

### Positive Cases

1. Owner invites a registered non-member email
- Result: `201`
- Invitation stored with:
  - `status = pending`
  - `version = 1`
  - `updated_at = created_at`

2. Owner invites an unregistered email
- Result: `201`
- Same response shape as above
- Notification publisher may no-op because no user exists yet

3. Email input uses mixed case or surrounding spaces
- Result: `201`
- Stored `email` is normalized

### Negative Cases

1. Missing auth token
- Result: `401 unauthorized`

2. Non-owner workspace member attempts invite
- Result: `403 forbidden`

3. Non-member attempts invite
- Result: `403 forbidden`

4. Invalid JSON body
- Result: `400 invalid_json`

5. Unknown JSON field
- Result: `400 invalid_json`

6. Invalid role
- Result: `422 validation_failed`

7. Invalid email format
- Result: `422 validation_failed`

8. Duplicate pending invitation
- Result: `409 conflict`

9. Email belongs to an existing workspace member
- Result: `409 conflict`

10. Notification publisher fails after persistence
- Result: current service behavior preserved for this task
  - application returns error
  - HTTP maps to `500 internal_error`
  - this is intentionally not fixed until outbox work

---

## 4. API Contract And Response Codes

## 4.1 Success

### `201 Created`

```json
{
  "data": {
    "id": "uuid",
    "workspace_id": "uuid",
    "email": "invitee@example.com",
    "role": "viewer",
    "status": "pending",
    "version": 1,
    "invited_by": "uuid",
    "created_at": "2026-04-04T08:00:00Z",
    "updated_at": "2026-04-04T08:00:00Z"
  }
}
```

## 4.2 Failures

### `400 invalid_json`
- malformed JSON
- multiple JSON values
- unknown fields

Example:
```json
{
  "error": {
    "code": "invalid_json",
    "message": "request body must be valid JSON",
    "request_id": "..."
  }
}
```

### `401 unauthorized`
- missing or invalid auth token

### `403 forbidden`
- actor is not workspace owner
- actor is not a member of the workspace

### `409 conflict`
- duplicate pending invitation already exists
- target email belongs to an existing workspace member

### `422 validation_failed`
- invalid email
- invalid role

### `500 internal_error`
- notification publisher failed after invitation persistence
- repository or unexpected internal error

---

## 5. Files To Change

### Modify
- `internal/domain/workspace.go`
- `internal/application/workspace_service.go`
- `internal/application/workspace_service_test.go`
- `internal/application/workspace_service_additional_test.go`
- `internal/application/notification_events_test.go`
- `internal/transport/http/server_auth_workspace_test.go`
- `frontend-repo/API_CONTRACT.md`
- `docs/checkpoint.md`

### Likely No Production Change Needed
- `internal/transport/http/handlers.go`
  - request shape stays the same
  - response write path should already serialize the returned struct

### Verify But Probably No Change Needed
- `internal/repository/postgres/workspace_repository.go`
  - Task 1 should already make create persistence state-aware

### Files Explicitly Not In Scope
- `internal/application/notification_service.go`
- `internal/repository/postgres/notification_repository.go`
- `internal/transport/http/server.go`
- `frontend-repo/CONTEXT.md`

---

## 6. Test Plan

## 6.1 Application Service Tests

Add or update tests in:
- `internal/application/workspace_service_test.go`
- `internal/application/workspace_service_additional_test.go`

### Positive Service Cases

1. Owner invites registered non-member
- Expect success
- Assert normalized email
- Assert `status = pending`
- Assert `version = 1`
- Assert `updated_at = created_at`

2. Owner invites unregistered email
- Expect success
- Assert no user existence requirement
- Assert normalized email
- Assert `status = pending`

### Negative Service Cases

3. Invalid role returns validation error

4. Non-owner returns forbidden

5. Invalid email returns validation error

6. Duplicate pending invitation returns conflict

7. Existing workspace member email returns conflict
- Seed:
  - workspace owner
  - existing member with matching email
- Expect:
  - `domain.ErrConflict`

8. Notification publisher failure is still propagated
- Expect:
  - returned error from `NotifyInvitationCreated`

## 6.2 HTTP Handler Tests

Add or update tests in:
- `internal/transport/http/server_auth_workspace_test.go`
- `internal/transport/http/server_invalid_json_test.go` only if coverage gap exists

### Positive HTTP Cases

9. Invite registered non-member returns `201`
- Assert response payload includes:
  - `status = pending`
  - `version = 1`
  - `updated_at`

10. Invite unregistered email returns `201`
- Assert response payload includes:
  - normalized email
  - `status = pending`
  - `version = 1`

### Negative HTTP Cases

11. Invalid JSON returns `400`

12. Unknown field returns `400`

13. Non-owner returns `403`

14. Invalid role returns `422`

15. Invalid email returns `422`

16. Existing member email returns `409`

17. Duplicate pending invitation returns `409`

18. Notification publisher failure returns `500`
- Only if coverage for this endpoint is missing

## 6.3 Regression Cases

19. Existing invite-accept flow still works end to end
- Create invitation
- Accept invitation
- Verify membership created

20. Existing mismatch-email accept test still passes
- No regression to invitation acceptance security behavior

## 6.4 Documentation Tests

21. Update `frontend-repo/API_CONTRACT.md`
- Remove stale statement that unregistered emails are rejected
- Add `status`, `version`, and `updated_at` to `WorkspaceInvitation`
- Keep wording aligned with real backend behavior

22. Update `docs/checkpoint.md`
- Record create-invitation contract change
- Record existing-member conflict rule
- Record unregistered email remains allowed

---

## 7. Execution Plan

### Task 1: Write or update failing service tests for the new create behavior

**Files:**
- Modify: `internal/application/workspace_service_test.go`
- Modify: `internal/application/workspace_service_additional_test.go`

- [ ] **Step 1: Add a failing service test for returned pending invitation state**

Add assertions that successful invite returns:
- `Status == pending`
- `Version == 1`
- `UpdatedAt == CreatedAt`

- [ ] **Step 2: Add a failing service test for existing-member conflict**

Seed:
- workspace owner membership
- existing member user with target email

Expect:
- `InviteMember` returns `domain.ErrConflict`

- [ ] **Step 3: Run targeted service tests to confirm failure**

Run:
```powershell
go test ./internal/application -run "TestWorkspaceService|TestNotificationEvents" -count=1
```

Expected:
- fail because create flow does not yet enforce all new behavior or tests do not compile against the updated contract

- [ ] **Step 4: Commit**

```bash
git add internal/application/workspace_service_test.go internal/application/workspace_service_additional_test.go
git commit -m "test: define create invitation endpoint behavior"
```

### Task 2: Implement service-layer validation and returned state

**Files:**
- Modify: `internal/application/workspace_service.go`
- Modify: `internal/domain/workspace.go` if Task 1 kept new fields internal-only

- [ ] **Step 1: Expose the new invitation state fields in the public JSON model**

Required fields to make public now:
- `status`
- `version`
- `updated_at`
- `responded_by`
- `responded_at`
- `cancelled_by`
- `cancelled_at`

Keep:
- `accepted_at`

- [ ] **Step 2: Implement member-existence conflict check in `InviteMember`**

Recommended approach:
- normalize email
- try `users.GetByEmail`
- if user exists:
  - call `workspaces.ListMembers`
  - if any member `UserID == user.ID`, return `domain.ErrConflict`
- if user not found:
  - continue
- if another user repo error occurs:
  - return that error

- [ ] **Step 3: Keep the existing allowed-unregistered-email behavior**

Requirement:
- `users.GetByEmail` returning `domain.ErrNotFound` must not fail the request

- [ ] **Step 4: Run targeted service tests**

Run:
```powershell
go test ./internal/application -run "TestWorkspaceService|TestNotificationEvents" -count=1
```

Expected:
- PASS for the updated service coverage

- [ ] **Step 5: Commit**

```bash
git add internal/application/workspace_service.go internal/domain/workspace.go
git commit -m "feat: enforce create invitation member checks and public state fields"
```

### Task 3: Update HTTP endpoint tests and contract expectations

**Files:**
- Modify: `internal/transport/http/server_auth_workspace_test.go`

- [ ] **Step 1: Add failing HTTP assertions for new response fields**

Assert create response includes:
- `status = pending`
- `version = 1`
- `updated_at` set

- [ ] **Step 2: Add failing HTTP test for inviting an existing member**

Flow:
- create workspace
- register/login target user
- invite and accept or seed direct membership
- attempt second invite to same email
- expect `409`

- [ ] **Step 3: Keep and strengthen unregistered invite test**

Assert:
- response is `201`
- payload carries pending state fields

- [ ] **Step 4: Run targeted HTTP tests to confirm failure**

Run:
```powershell
go test ./internal/transport/http -run "Test.*Invite|TestAcceptInvitation" -count=1
```

Expected:
- failures until the endpoint contract is fully aligned

- [ ] **Step 5: Apply compatibility fixes only if handler or test fakes need them**

Most likely updates:
- test fake workspace repo pending-state logic
- fake accept behavior if it now relies on `Status`

- [ ] **Step 6: Re-run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "Test.*Invite|TestAcceptInvitation" -count=1
```

Expected:
- PASS

- [ ] **Step 7: Commit**

```bash
git add internal/transport/http/server_auth_workspace_test.go
git commit -m "test: lock create invitation HTTP contract"
```

### Task 4: Update docs to match real backend behavior

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update `WorkspaceInvitation` type in API contract**

Required updates:
- add `status`
- add `version`
- add `updated_at`
- add terminal audit fields if documented globally

- [ ] **Step 2: Fix stale validation notes for unregistered invitees**

Remove:
- "invitee email must already belong to a registered user"
- "unregistered email returns 422 validation_failed"

Replace with:
- unregistered email is allowed
- duplicate pending invitations return conflict
- existing workspace member email returns conflict

- [ ] **Step 3: Update checkpoint**

Record:
- create invitation now returns explicit invitation state metadata
- create invitation rejects existing workspace members
- unregistered email remains allowed

- [ ] **Step 4: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: update create invitation contract"
```

### Task 5: Full verification for Task 2

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run exact verification commands**

Run:
```powershell
go test ./internal/application -run "TestWorkspaceService|TestNotificationEvents" -count=1
go test ./internal/transport/http -run "Test.*Invite|TestAcceptInvitation" -count=1
go test ./internal/repository/postgres -run "TestWorkspaceRepository|TestInvitation" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual API sanity check if local server is available**

Call:
```http
POST /api/v1/workspaces/{workspaceID}/invitations
```

Verify:
- response `201`
- `status = pending`
- `version = 1`
- `updated_at = created_at`
- unregistered email still accepted
- existing member email rejected with `409`

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify create invitation endpoint task"
```

---

## 8. Acceptance Criteria

Task 2 is complete only when all are true:
- `POST /api/v1/workspaces/{workspaceID}/invitations` returns the new invitation state fields publicly
- successful create always returns pending invitation state with version `1`
- unregistered emails remain allowed
- existing workspace member emails are rejected with conflict
- duplicate pending invitations are rejected with conflict
- current authorization and validation semantics remain intact
- service and HTTP tests cover all positive and negative cases above
- `frontend-repo/API_CONTRACT.md` and `docs/checkpoint.md` reflect the real endpoint behavior

## 9. Risks And Guardrails

- Do not change notification reliability semantics in this task.
- Do not add list, update, reject, or cancel invitation behavior in this task.
- Do not weaken the current security posture by reintroducing user-existence leakage through a validation error for unregistered emails.
- Be careful with fake repositories in tests:
  - some still treat pending as `AcceptedAt == nil`
  - update them only as needed for this endpoint slice

## 10. Follow-On Tasks

This plan prepares for:
- Task 3 `GET /api/v1/workspaces/{workspaceID}/invitations`
- Task 4 `GET /api/v1/my/invitations`
- Task 5 `PATCH /api/v1/workspace-invitations/{invitationID}`
- later invitation notification projection tasks
