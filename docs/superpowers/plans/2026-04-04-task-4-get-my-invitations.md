# Task 4 GET My Invitations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `GET /api/v1/my/invitations` so the authenticated user can list invitations addressed to their email from the authoritative invitation table with status filtering and bounded cursor pagination.

**Architecture:** This task adds a user-scoped invitation list endpoint. Unlike Task 3, authorization does not depend on workspace ownership. The service layer resolves the authenticated actor to their canonical email, validates the query, and delegates to a repository method that filters invitations by normalized email plus optional status. The endpoint returns source-of-truth invitation data and must not depend on notifications.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, explicit SQL repositories, table-driven tests

---

## 1. Dependencies

- Task 1 invitation state schema must be complete.
- Task 2 create-invitation contract must already expose invitation state fields publicly.
- Task 3 should already introduce:
  - invitation list DTO
  - invitation list filter style
  - bounded cursor pagination semantics

This task should reuse Task 3 conventions where practical instead of introducing a second pagination style.

---

## 2. Scope

### In Scope
- Add one new endpoint:
  - `GET /api/v1/my/invitations`
- Add status filtering
- Add bounded cursor pagination
- Resolve visibility by authenticated user email
- Add repository SQL for email-scoped invitation listing
- Add API contract and checkpoint updates

### Out Of Scope
- No workspace-owner invitation list changes beyond reuse
- No invitation update, accept, reject, or cancel changes
- No notification changes
- No inbox changes
- No outbox work
- No frontend implementation

---

## 3. Detailed Spec

## 3.1 Endpoint

### `GET /api/v1/my/invitations`

- Auth: yes
- Authorization: any authenticated user

Visibility rule:
- return only invitations whose normalized `email` matches the authenticated user’s canonical email

This endpoint is the user’s source-of-truth invitation list. It is not the notification inbox.

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
      "email": "member@example.com",
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
- cursor must continue from the last item using the same ordering

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
2. Actor must resolve to an existing user record
3. The email used for listing must be the canonical normalized email from `users.GetByID(actorID)`
4. `status` must be one of:
   - `pending|accepted|rejected|cancelled|all`
5. `limit` must be:
   - omitted, or
   - positive integer, and
   - `<= 100`
6. `cursor` must be valid for this endpoint

## 3.5 Behavior Rules

### Email Scope
- the endpoint must list invitations by the authenticated user’s normalized email
- do not trust any client-supplied email
- do not expose invitations for any other email

### Cross-Workspace Scope
- if a user has invitations from many workspaces, all matching invitations are included
- this endpoint is not limited to a single workspace

### Status Filter Semantics
- `status=pending`
  - only pending invitations for the actor email
- `status=accepted`
  - only accepted invitations for the actor email
- `status=rejected`
  - only rejected invitations for the actor email
- `status=cancelled`
  - only cancelled invitations for the actor email
- `status=all`
  - no status filter

### Empty Results
- empty result set is valid
- response still returns `200`
- `items = []`
- `has_more = false`
- `next_cursor` omitted

### Unknown Or Deleted User
If the actor id in auth context does not resolve to a current user record:
- return `401 unauthorized`

This keeps behavior aligned with other user-scoped endpoints.

## 3.6 Positive And Negative Cases

### Positive Cases

1. Authenticated user lists all invitations addressed to their email
- Result: `200`
- Returns all matching invitations across workspaces

2. Authenticated user filters pending invitations
- Result: `200`
- Returns only pending invitations

3. Authenticated user uses pagination
- Result: `200`
- Returns bounded page with `next_cursor`

4. Authenticated user has no invitations
- Result: `200`
- Returns empty list

### Negative Cases

1. Missing auth token
- Result: `401 unauthorized`

2. Invalid auth token
- Result: `401 unauthorized`

3. Authenticated actor id does not resolve to a user
- Result: `401 unauthorized`

4. Invalid `status`
- Result: `422 validation_failed`

5. Invalid `limit`
- Result: `422 validation_failed`

6. Invalid `cursor`
- Result: `422 validation_failed`

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
        "email": "member@example.com",
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
- missing auth token
- invalid auth token
- actor user record does not exist

### `422 validation_failed`
- invalid `status`
- invalid `limit`
- invalid `cursor`

No `403` is expected here because this is a self-scoped endpoint rather than a role-gated workspace management endpoint.

---

## 5. File Structure And Responsibilities

### Modify
- `internal/application/workspace_service.go`
  - add user-scoped invitation list input and service method
- `internal/application/workspace_service_test.go`
- `internal/application/workspace_service_additional_test.go`
- `internal/repository/postgres/workspace_repository.go`
  - add email-scoped invitation list query and cursor handling
- `internal/repository/postgres/user_workspace_refresh_repository_test.go`
  - or the current PostgreSQL-backed workspace invitation test file
- `internal/transport/http/handlers.go`
  - add handler and query parsing
- `internal/transport/http/server.go`
  - register route
- `internal/transport/http/server_auth_workspace_test.go`
  - add end-to-end coverage
- `frontend-repo/API_CONTRACT.md`
- `docs/checkpoint.md`

### Reuse If Already Present
- `internal/domain/workspace.go`
  - reuse `WorkspaceInvitationList` from Task 3 if already added
- invitation list filter normalization helpers from Task 3 if already added

### Files Explicitly Not In Scope
- `internal/application/notification_service.go`
- `internal/repository/postgres/notification_repository.go`
- `frontend-repo/CONTEXT.md`

---

## 6. Test Plan

## 6.1 Application Service Tests

Add or update tests in:
- `internal/application/workspace_service_test.go`
- `internal/application/workspace_service_additional_test.go`

### Positive Service Cases

1. Actor lists all invitations addressed to their email
- Seed:
  - user `member@example.com`
  - invitations in multiple workspaces for `member@example.com`
  - invitations for other emails
- Expect:
  - only matching-email invitations returned

2. Actor filters by pending status
- Expect:
  - only pending invitations returned

3. Actor lists empty invitation set
- Expect:
  - empty result

4. Actor passes explicit limit
- Expect:
  - service forwards normalized limit

### Negative Service Cases

5. Unknown actor id returns unauthorized

6. Invalid status returns validation error

7. Invalid limit `0` returns validation error

8. Invalid limit `>100` returns validation error

9. Repository invalid cursor error propagates as validation error

## 6.2 Repository Integration Tests

Add DB-backed tests in the PostgreSQL workspace repository invitation test area.

### Positive Repository Cases

10. List invitations by email returns all matching invitations across workspaces

11. List invitations by email excludes invitations for other emails

12. Status filter `pending` returns only pending invitations

13. Status filter `accepted` returns only accepted invitations

14. Limit produces paginated result with `has_more` and `next_cursor`

15. Cursor returns next page without duplicates or gaps

### Negative Repository Cases

16. Invalid cursor returns `domain.ErrValidation`

17. Cursor with mismatched filter metadata returns `domain.ErrValidation`

## 6.3 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_auth_workspace_test.go`

### Positive HTTP Cases

18. Authenticated user lists all matching invitations with `200`

19. Authenticated user filters pending invitations with `200`

20. Authenticated user paginates with `limit=1`
- Assert:
  - `has_more = true`
  - `next_cursor` present

21. Authenticated user follows cursor successfully
- Assert:
  - second page result
  - final page omits `next_cursor`

22. Authenticated user with no invitations gets empty list

### Negative HTTP Cases

23. Missing auth returns `401`

24. Unknown actor user returns `401`

25. Invalid `status` returns `422`

26. Invalid `limit` returns `422`

27. Invalid `cursor` returns `422`

## 6.4 Documentation Tests

28. `frontend-repo/API_CONTRACT.md` documents:
- endpoint
- query params
- response shape
- validation rules
- pagination semantics
- self-scoped visibility by authenticated user email

29. `docs/checkpoint.md` records:
- new my-invitations endpoint
- email-scoped visibility
- status filter and pagination

---

## 7. Execution Plan

### Task 1: Define failing service tests for self-scoped invitation listing

**Files:**
- Modify: `internal/application/workspace_service_test.go`
- Modify: `internal/application/workspace_service_additional_test.go`

- [ ] **Step 1: Add failing service tests for email-scoped invitation listing**

Cover:
- actor gets invitations for their email
- invitations for other emails are excluded
- empty result case

- [ ] **Step 2: Add failing service tests for validation and unknown actor**

Cover:
- unknown actor id returns unauthorized
- invalid status returns validation error
- invalid limit returns validation error

- [ ] **Step 3: Run targeted service tests**

Run:
```powershell
go test ./internal/application -run "TestWorkspaceService" -count=1
```

Expected:
- FAIL because the self-scoped list method does not exist yet

- [ ] **Step 4: Commit**

```bash
git add internal/application/workspace_service_test.go internal/application/workspace_service_additional_test.go
git commit -m "test: define my invitations service behavior"
```

### Task 2: Implement service input, user resolution, and validation

**Files:**
- Modify: `internal/application/workspace_service.go`

- [ ] **Step 1: Extend the repository interface**

Add:
- `ListInvitationsByEmail(ctx context.Context, email string, status *domain.WorkspaceInvitationStatus, limit int, cursor string) (domain.WorkspaceInvitationList, error)`

- [ ] **Step 2: Add service input type**

Add:
```go
type ListMyInvitationsInput struct {
    Status string
    Limit  int
    Cursor string
}
```

- [ ] **Step 3: Implement `WorkspaceService.ListMyInvitations`**

Required behavior:
- resolve actor with `users.GetByID`
- if user not found, return `domain.ErrUnauthorized`
- normalize status filter
- normalize limit:
  - default `50`
  - reject `<= 0`
  - reject `> 100`
- trim cursor
- delegate using the actor’s canonical email

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
git commit -m "feat: add my invitations service"
```

### Task 3: Add repository query and cursor support for email-scoped listing

**Files:**
- Modify: `internal/repository/postgres/workspace_repository.go`
- Modify: `internal/repository/postgres/user_workspace_refresh_repository_test.go`
- Modify: `internal/repository/postgres/additional_integration_test.go` if that is the better invitation-focused location

- [ ] **Step 1: Add failing repository integration tests**

Cover:
- email scoping across workspaces
- exclusion of other emails
- status filtering
- limit pagination
- cursor continuation
- invalid cursor

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestWorkspaceRepository|TestInvitation" -count=1
```

Expected:
- FAIL because `ListInvitationsByEmail` does not exist yet

- [ ] **Step 3: Implement cursor format**

Recommended cursor fields:
- `created_at`
- `id`
- `status_filter`
- normalized `email`

The repository must reject cursors whose embedded filter/email metadata does not match the current request.

- [ ] **Step 4: Implement repository `ListInvitationsByEmail`**

Requirements:
- filter by normalized email
- optional status filter
- order `created_at DESC, id DESC`
- fetch `limit + 1`
- emit `next_cursor`
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
git commit -m "feat: add my invitations repository query"
```

### Task 4: Add HTTP route, handler, and endpoint tests

**Files:**
- Modify: `internal/transport/http/server.go`
- Modify: `internal/transport/http/handlers.go`
- Modify: `internal/transport/http/server_auth_workspace_test.go`

- [ ] **Step 1: Add failing HTTP tests for the new endpoint**

Cover:
- authenticated success
- pending filter
- pagination
- empty list

- [ ] **Step 2: Add failing HTTP tests for invalid query parameters**

Cover:
- invalid status
- invalid limit
- invalid cursor

- [ ] **Step 3: Add failing HTTP test for unknown actor**

Cover:
- valid token subject but missing user repo entry
- expect `401`

- [ ] **Step 4: Register route in `server.go`**

Add:
- `r.Get("/my/invitations", s.handleListMyInvitations())`

- [ ] **Step 5: Implement `handleListMyInvitations`**

Handler requirements:
- parse query params
- call service with authenticated actor id
- map validation and unauthorized errors through existing error mapper
- return `200` with list payload

- [ ] **Step 6: Re-run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "Test.*Invitation|Test.*Invite|Test.*My" -count=1
```

Expected:
- PASS

- [ ] **Step 7: Commit**

```bash
git add internal/transport/http/server.go internal/transport/http/handlers.go internal/transport/http/server_auth_workspace_test.go
git commit -m "feat: add my invitations endpoint"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Add endpoint contract to API docs**

Document:
- auth rule
- query params
- response payload
- self-scoped visibility
- validation errors
- pagination semantics

- [ ] **Step 2: Update checkpoint**

Record:
- new `GET /api/v1/my/invitations`
- actor-email visibility rule
- supported filters and pagination

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: add my invitations contract"
```

### Task 6: Full verification for Task 4

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "TestWorkspaceService" -count=1
go test ./internal/repository/postgres -run "TestWorkspaceRepository|TestInvitation" -count=1
go test ./internal/transport/http -run "Test.*Invitation|Test.*Invite|Test.*My" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual API sanity check if local server is available**

Call:
```http
GET /api/v1/my/invitations?status=pending&limit=1
```

Verify:
- `200`
- only invitations for the authenticated email are returned
- `next_cursor` returned when page is truncated

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify my invitations task"
```

---

## 8. Acceptance Criteria

Task 4 is complete only when all are true:
- `GET /api/v1/my/invitations` exists
- it returns only invitations addressed to the authenticated user email
- it supports `status=pending|accepted|rejected|cancelled|all`
- it uses bounded cursor pagination
- invalid query params return `422`
- missing or unknown actor returns `401`
- service, repository, and HTTP tests cover the positive and negative cases above
- docs and checkpoint reflect the new endpoint

## 9. Risks And Guardrails

- Do not source this endpoint from notifications.
- Do not accept an email query parameter from clients.
- Do not weaken the endpoint by exposing invitations for other emails in the same workspace.
- Reuse Task 3 list semantics where possible so invitation list endpoints stay consistent.

## 10. Follow-On Tasks

This plan prepares for:
- Task 5 `PATCH /api/v1/workspace-invitations/{invitationID}`
- Task 6 `POST /api/v1/workspace-invitations/{invitationID}/accept`
- later invitation notification projection work
