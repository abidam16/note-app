# Task 3 GET Workspace Invitations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `GET /api/v1/workspaces/{workspaceID}/invitations` so workspace owners can list invitations from the authoritative invitation table with status filtering and bounded cursor pagination.

**Architecture:** This task adds one owner-only list endpoint on top of the invitation state model from Tasks 1 and 2. The service layer should authorize against workspace membership, normalize and validate query parameters, and delegate to a repository method that applies invitation status filtering plus forward-only cursor pagination. The endpoint returns invitation records directly from source-of-truth data, not from notifications.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, explicit SQL repositories, table-driven tests

---

## 1. Dependencies

- Task 1 invitation state schema must be complete.
- Task 2 create-invitation contract should already expose:
  - `status`
  - `version`
  - `updated_at`
- Invitation persistence must already use explicit `status` values.

This plan assumes the invitation schema and public invitation JSON shape are already in place.

---

## 2. Scope

### In Scope
- Add one new endpoint:
  - `GET /api/v1/workspaces/{workspaceID}/invitations`
- Add owner-only authorization for this endpoint
- Add status filtering
- Add bounded cursor pagination
- Add repository SQL for workspace-scoped invitation listing
- Add API contract and checkpoint updates

### Out Of Scope
- No my-invitations endpoint
- No update/reject/cancel endpoints
- No invitation notification changes
- No inbox changes
- No outbox work
- No frontend implementation

---

## 3. Detailed Spec

## 3.1 Endpoint

### `GET /api/v1/workspaces/{workspaceID}/invitations`

- Auth: yes
- Authorization: workspace `owner` only

This endpoint is for workspace management. It is not the invited user inbox and must not reuse notification rules.

## 3.2 Query Parameters

- `status` optional
  - allowed values:
    - `pending`
    - `accepted`
    - `rejected`
    - `cancelled`
    - `all`
  - default:
    - `all`
- `limit` optional
  - positive integer
  - default `50`
  - max `100`
- `cursor` optional
  - opaque pagination cursor returned by a previous response

## 3.3 Response Payload

Response `200`:

```json
{
  "items": [
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
  ],
  "next_cursor": "opaque-cursor",
  "has_more": true
}
```

### Ordering Rules
- sort by `created_at DESC, id DESC`
- cursor must continue from the last item in that exact ordering

### Pagination Rules
- forward-only pagination
- if more items remain than `limit`, return:
  - `has_more = true`
  - `next_cursor = <opaque token>`
- if final page:
  - `has_more = false`
  - omit `next_cursor`

## 3.4 Validation Rules

1. Actor must be authenticated
2. Actor must be a workspace member
3. Actor must have role `owner`
4. `status` must be one of:
   - `pending|accepted|rejected|cancelled|all`
5. `limit` must be:
   - omitted, or
   - positive integer, and
   - `<= 100`
6. `cursor` must be:
   - valid base64 or equivalent opaque token according to implementation choice
   - well-formed for this endpoint
   - tied to the same ordering and filter semantics

## 3.5 Behavior Rules

### Workspace Scope
- only invitations whose `workspace_id` matches the requested workspace are returned

### Owner Scope
- only owners may list workspace invitations
- editors and viewers receive `403`

### Status Filter Semantics
- `status=pending`
  - only pending invitations
- `status=accepted`
  - only accepted invitations
- `status=rejected`
  - only rejected invitations
- `status=cancelled`
  - only cancelled invitations
- `status=all`
  - no status filter

### Cursor Semantics
Recommended cursor payload:
- `created_at`
- `id`
- `status` filter used by the current request

The endpoint must reject invalid cursors rather than silently fallback to page 1.

### Empty Results
- empty result set is valid
- response still returns `200`
- `items = []`
- `has_more = false`
- `next_cursor` omitted

## 3.6 Positive And Negative Cases

### Positive Cases

1. Owner lists invitations without filters
- Result: `200`
- Returns all workspace invitations sorted newest-first

2. Owner lists only pending invitations
- Result: `200`
- Returns only pending invitations

3. Owner lists accepted invitations with limit
- Result: `200`
- Returns paginated accepted invitations

4. Owner follows `next_cursor`
- Result: `200`
- Returns the next page without duplicates or gaps

5. Owner requests workspace with no invitations
- Result: `200`
- Returns empty list

### Negative Cases

1. Missing auth token
- Result: `401 unauthorized`

2. Non-member requests workspace invitation list
- Result: `403 forbidden`

3. Editor requests workspace invitation list
- Result: `403 forbidden`

4. Viewer requests workspace invitation list
- Result: `403 forbidden`

5. Invalid `status` query
- Result: `422 validation_failed`

6. Invalid `limit` query
- Result: `422 validation_failed`

7. Invalid `cursor` query
- Result: `422 validation_failed`

8. Workspace does not exist but membership lookup cannot resolve actor access
- Result: follow current membership-based authorization model
  - expected `403 forbidden` if membership lookup is the authoritative guard
  - do not add a separate workspace existence leak in this task

---

## 4. API Contract And Response Codes

## 4.1 Success

### `200 OK`

```json
{
  "data": {
    "items": [
      {
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
    ],
    "next_cursor": "opaque-cursor",
    "has_more": true
  }
}
```

## 4.2 Failures

### `401 unauthorized`
- missing or invalid auth token

### `403 forbidden`
- actor is not an owner of the workspace
- actor is not a member of the workspace

### `422 validation_failed`
- invalid `status`
- invalid `limit`
- invalid `cursor`

No `404` is required for normal permission failures in this task if the current workspace membership pattern already returns `403`.

---

## 5. File Structure And Responsibilities

### Modify
- `internal/domain/workspace.go`
  - add invitation list DTOs and status filter types if needed
- `internal/application/workspace_service.go`
  - add service input type and service method
  - validate filter and limit
  - enforce owner authorization
- `internal/application/workspace_service_test.go`
- `internal/application/workspace_service_additional_test.go`
- `internal/repository/postgres/workspace_repository.go`
  - add list query and cursor decode/encode logic
- `internal/repository/postgres/user_workspace_refresh_repository_test.go`
  - or the current PostgreSQL-backed workspace repository test file used for invitation flows
- `internal/transport/http/handlers.go`
  - add handler and query parsing
- `internal/transport/http/server.go`
  - register route
- `internal/transport/http/server_auth_workspace_test.go`
  - add end-to-end handler coverage
- `frontend-repo/API_CONTRACT.md`
- `docs/checkpoint.md`

### New Types Expected
- `application.ListWorkspaceInvitationsInput`
- `domain.WorkspaceInvitationList`
- optional cursor struct in repository package

### Files Explicitly Not In Scope
- `internal/application/notification_service.go`
- `internal/repository/postgres/notification_repository.go`
- `internal/transport/http/server_test.go` unless test helpers are needed
- `frontend-repo/CONTEXT.md`

---

## 6. Test Plan

## 6.1 Application Service Tests

Add or update tests in:
- `internal/application/workspace_service_test.go`
- `internal/application/workspace_service_additional_test.go`

### Positive Service Cases

1. Owner lists all invitations
- Seed:
  - workspace owner membership
  - invitations in multiple statuses
- Expect:
  - all returned in newest-first order

2. Owner lists only pending invitations
- Expect:
  - only pending results returned

3. Owner lists with explicit limit
- Expect:
  - service forwards normalized limit

4. Empty invitation list returns empty result
- Expect:
  - no error
  - empty items

### Negative Service Cases

5. Non-owner returns forbidden

6. Invalid status filter returns validation error

7. Invalid limit `0` returns validation error

8. Invalid limit `>100` returns validation error

9. Repository invalid cursor error propagates as validation error

## 6.2 Repository Integration Tests

Add DB-backed tests in the PostgreSQL workspace repository test file.

### Positive Repository Cases

10. List invitations returns all workspace invitations sorted by `created_at DESC, id DESC`

11. List invitations with status filter `pending` returns only pending

12. List invitations with status filter `accepted` returns only accepted

13. List invitations with limit returns:
- first page sized to limit
- `has_more = true`
- non-empty `next_cursor`

14. List invitations with cursor returns the next page
- no duplicates
- no skipped rows

15. Invitations from another workspace are excluded

### Negative Repository Cases

16. Invalid cursor returns `domain.ErrValidation`

17. Cursor with mismatched filter metadata returns `domain.ErrValidation`

## 6.3 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_auth_workspace_test.go`

### Positive HTTP Cases

18. Owner lists all invitations with `200`
- Assert:
  - response envelope
  - ordered items

19. Owner lists filtered invitations with `status=pending`
- Assert:
  - all returned items have `status = pending`

20. Owner lists paginated invitations with `limit=1`
- Assert:
  - `has_more = true`
  - `next_cursor` present

21. Owner follows cursor successfully
- Assert:
  - second page result
  - final page omits `next_cursor`

22. Owner lists workspace with no invitations
- Assert:
  - empty list
  - `has_more = false`

### Negative HTTP Cases

23. Missing auth returns `401`

24. Editor returns `403`

25. Viewer returns `403`

26. Non-member returns `403`

27. Invalid `status` returns `422`

28. Invalid `limit` returns `422`

29. Invalid `cursor` returns `422`

## 6.4 Documentation Tests

30. `frontend-repo/API_CONTRACT.md` documents:
- endpoint
- query params
- response shape
- validation rules
- pagination semantics

31. `docs/checkpoint.md` records:
- new workspace invitation list endpoint
- owner-only access
- status filter and pagination support

---

## 7. Execution Plan

### Task 1: Define failing service tests and DTOs

**Files:**
- Modify: `internal/application/workspace_service_test.go`
- Modify: `internal/application/workspace_service_additional_test.go`
- Modify: `internal/domain/workspace.go`

- [ ] **Step 1: Add the invitation list DTOs**

Add the minimal types needed for this endpoint:
- `WorkspaceInvitationList`
- optional filter enum/type if it improves clarity

Expected shape:
```go
type WorkspaceInvitationList struct {
    Items      []WorkspaceInvitation `json:"items"`
    NextCursor *string               `json:"next_cursor,omitempty"`
    HasMore    bool                  `json:"has_more"`
}
```

- [ ] **Step 2: Add failing service tests for owner-only list behavior**

Cover:
- owner success
- non-owner forbidden
- empty result

- [ ] **Step 3: Add failing service tests for filter and limit validation**

Cover:
- invalid status
- invalid limit zero
- invalid limit too large

- [ ] **Step 4: Run targeted service tests**

Run:
```powershell
go test ./internal/application -run "TestWorkspaceService" -count=1
```

Expected:
- FAIL because list invitation method and DTOs do not exist yet

- [ ] **Step 5: Commit**

```bash
git add internal/domain/workspace.go internal/application/workspace_service_test.go internal/application/workspace_service_additional_test.go
git commit -m "test: define workspace invitation list service behavior"
```

### Task 2: Implement service input, validation, and authorization

**Files:**
- Modify: `internal/application/workspace_service.go`

- [ ] **Step 1: Extend the repository interface**

Add:
- `ListInvitations(ctx context.Context, workspaceID string, status *domain.WorkspaceInvitationStatus, limit int, cursor string) (domain.WorkspaceInvitationList, error)`

- [ ] **Step 2: Add service input type**

Add:
```go
type ListWorkspaceInvitationsInput struct {
    WorkspaceID string
    Status      string
    Limit       int
    Cursor      string
}
```

- [ ] **Step 3: Implement service method**

Add:
- `WorkspaceService.ListInvitations`

Required behavior:
- load actor membership with `GetMembershipByUserID`
- require `owner`
- normalize status filter
- normalize limit:
  - default `50`
  - reject `<=0`
  - reject `>100`
- trim cursor
- delegate to repository

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
git commit -m "feat: add workspace invitation list service"
```

### Task 3: Add repository query and cursor support

**Files:**
- Modify: `internal/repository/postgres/workspace_repository.go`
- Modify: `internal/repository/postgres/user_workspace_refresh_repository_test.go`
- Modify: `internal/repository/postgres/additional_integration_test.go` if that is the better invitation-focused location

- [ ] **Step 1: Add failing repository integration tests**

Cover:
- sort order
- status filtering
- limit pagination
- next cursor continuation
- invalid cursor
- workspace scoping

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestWorkspaceRepository|TestInvitation" -count=1
```

Expected:
- FAIL because repository list method does not exist yet

- [ ] **Step 3: Implement cursor format**

Recommended cursor fields:
- `created_at`
- `id`
- `status_filter`

Use an opaque encoded JSON token, following the same spirit as the thread repository.

- [ ] **Step 4: Implement repository `ListInvitations`**

Requirements:
- workspace-scoped query
- optional status filter
- order `created_at DESC, id DESC`
- limit `n+1` rows to detect `has_more`
- emit `next_cursor` from the last returned row
- invalid cursor returns `domain.ErrValidation`

- [ ] **Step 5: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestWorkspaceRepository|TestInvitation" -count=1
```

Expected:
- PASS

- [ ] **Step 6: Commit**

```bash
git add internal/repository/postgres/workspace_repository.go internal/repository/postgres/user_workspace_refresh_repository_test.go internal/repository/postgres/additional_integration_test.go
git commit -m "feat: add workspace invitation list repository"
```

### Task 4: Add HTTP route, handler, and endpoint tests

**Files:**
- Modify: `internal/transport/http/server.go`
- Modify: `internal/transport/http/handlers.go`
- Modify: `internal/transport/http/server_auth_workspace_test.go`

- [ ] **Step 1: Add a failing HTTP test for owner list success**

Cover:
- `GET /api/v1/workspaces/{workspaceID}/invitations`
- owner token
- populated result

- [ ] **Step 2: Add failing HTTP tests for query validation**

Cover:
- invalid status
- invalid limit
- invalid cursor

- [ ] **Step 3: Add failing HTTP tests for authorization**

Cover:
- editor forbidden
- viewer forbidden
- non-member forbidden

- [ ] **Step 4: Register route in `server.go`**

Add:
- `r.Get("/workspaces/{workspaceID}/invitations", s.handleListInvitations())`

- [ ] **Step 5: Implement `handleListInvitations`**

Handler requirements:
- parse query params
- pass input to service
- map validation and authorization errors through existing error mapper
- return `200` with list payload

- [ ] **Step 6: Re-run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "Test.*Invitation|Test.*Invite" -count=1
```

Expected:
- PASS

- [ ] **Step 7: Commit**

```bash
git add internal/transport/http/server.go internal/transport/http/handlers.go internal/transport/http/server_auth_workspace_test.go
git commit -m "feat: add workspace invitation list endpoint"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Add endpoint contract to API docs**

Document:
- auth and owner-only rule
- query params
- response payload
- validation errors
- pagination semantics

- [ ] **Step 2: Update checkpoint**

Record:
- new endpoint
- owner-only list semantics
- supported status filter
- cursor pagination

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: add workspace invitation list contract"
```

### Task 6: Full verification for Task 3

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
GET /api/v1/workspaces/{workspaceID}/invitations?status=pending&limit=1
```

Verify:
- `200`
- owner-only access
- pending-only results
- `next_cursor` returned when page is truncated

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify workspace invitation list task"
```

---

## 8. Acceptance Criteria

Task 3 is complete only when all are true:
- `GET /api/v1/workspaces/{workspaceID}/invitations` exists
- only owners can use it
- it returns source-of-truth invitation records
- `status` filter supports `pending|accepted|rejected|cancelled|all`
- pagination is bounded and cursor-based
- invalid query params return `422`
- positive and negative service, repository, and HTTP tests all pass
- docs and checkpoint reflect the new endpoint

## 9. Risks And Guardrails

- Do not source this endpoint from notifications.
- Do not reuse thread cursor code directly without adapting the cursor payload to invitation ordering.
- Do not add my-invitations logic here; that belongs to the next task.
- Keep authorization consistent with existing membership behavior to avoid creating new workspace-existence leaks.

## 10. Follow-On Tasks

This plan prepares for:
- Task 4 `GET /api/v1/my/invitations`
- Task 5 `PATCH /api/v1/workspace-invitations/{invitationID}`
- later invitation notification projection work
