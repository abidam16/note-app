# Task 13 POST Notification Read Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade `POST /api/v1/notifications/{notificationID}/read` into an idempotent inbox-read endpoint that marks one notification as read and returns the updated inbox item DTO.

**Architecture:** This task upgrades the existing mark-read endpoint to the notification inbox v2 contract introduced by Task 12. The transport layer accepts only the path parameter, the service validates the authenticated actor and notification id, and the repository performs an ownership-scoped idempotent update that sets read state only on the first unread-to-read transition. The repository returns the same inbox item projection shape as the list endpoint so the frontend can update one row without refetching the full inbox.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, explicit SQL repositories, table-driven tests, PostgreSQL-backed repository tests

---

## 1. Scope

### In Scope
- Redesign one existing endpoint:
  - `POST /api/v1/notifications/{notificationID}/read`
- Keep mark-read idempotent
- Return the inbox item DTO instead of the legacy notification shape
- Preserve the original read timestamp on repeated calls
- Update `is_read`, `read_at`, and `updated_at` on the first successful read
- Enforce own-notification scope
- Add service, repository, handler, and HTTP tests
- Update API docs and checkpoint

### Out Of Scope
- No request body
- No unread-count field in this response
- No batch mark-read endpoint
- No standalone unread-count endpoint
- No unread counter table in this task
- No SSE or WebSocket delivery
- No inbox list behavior changes beyond keeping Task 12 compatibility

---

## 2. Detailed Spec

## 2.1 Objective

After Task 12, the frontend reads notifications as inbox items. Mark-read must now return the same model so the client can update one notification card in place.

The endpoint must stay safe under retries and duplicate user actions:
- first call marks the row as read
- repeated calls return the same already-read row
- other users cannot read or probe someone else's notification

## 2.2 Endpoint

### `POST /api/v1/notifications/{notificationID}/read`

- Auth: yes
- Authorization: notification owner only

This endpoint marks one notification as read for the authenticated actor.

## 2.3 Request Payload

This endpoint has no request JSON payload.

### Request Rules
- the request body is ignored
- clients do not send `is_read`, `read_at`, or any partial update fields
- the only input is the path parameter:
  - `notificationID`

## 2.4 Response Payload

This codebase wraps successful responses in the standard success envelope:

```json
{
  "data": {
    "id": "uuid",
    "workspace_id": "uuid",
    "type": "invitation",
    "actor_id": "uuid",
    "actor": {
      "id": "uuid",
      "email": "owner@example.com",
      "full_name": "Owner"
    },
    "title": "Workspace invitation",
    "content": "You have a new workspace invitation",
    "is_read": true,
    "read_at": "2026-04-04T09:00:00Z",
    "actionable": true,
    "action_kind": "invitation_response",
    "resource_type": "invitation",
    "resource_id": "uuid",
    "payload": {
      "invitation_id": "uuid",
      "workspace_id": "uuid",
      "email": "invitee@example.com",
      "role": "editor",
      "status": "pending",
      "version": 3,
      "can_accept": true,
      "can_reject": true
    },
    "created_at": "2026-04-04T08:00:00Z",
    "updated_at": "2026-04-04T09:00:00Z"
  }
}
```

Response `200` payload inside `data` is the same public inbox item DTO used by `GET /api/v1/notifications`.

### Response Rules
- `is_read` must be `true`
- `read_at` must be non-null
- first successful mark-read:
  - sets `read_at` to the server timestamp used by the update
  - sets `updated_at` to that same effective read timestamp
- repeated mark-read:
  - keeps the original `read_at`
  - keeps the original `updated_at`
  - returns `200`
- all other notification fields stay unchanged
- actor metadata rules stay the same as Task 12:
  - if actor user row is missing, `actor` may be `null`
  - stored `actor_id` should still be returned when available

## 2.5 Validation Rules

1. Actor must be authenticated
2. Actor id from auth context must resolve to an existing user record
3. `notificationID` path parameter must be present
4. `notificationID` must be a valid UUID string
5. Notification must exist
6. Notification must belong to the authenticated actor

## 2.6 Behavior Rules

### Actor Validation
- resolve actor with `users.GetByID`
- if actor id does not resolve:
  - return `401 unauthorized`

### Notification ID Rule
- validate `notificationID` before repository update
- if `notificationID` is not a valid UUID string:
  - return `404 not_found`

This keeps invalid ids and foreign ids on the same non-disclosing path.

### Ownership Rule
- only notifications owned by the authenticated actor can be marked read
- repository update must filter by:
  - `id = notificationID`
  - `user_id = actorID`
- if no row matches:
  - return `404 not_found`

This covers both:
- missing notification id
- existing notification owned by another user

### Idempotency Rule
- first unread-to-read transition updates:
  - `is_read = TRUE`
  - `read_at = now`
  - `updated_at = now`
- repeated mark-read on an already-read row:
  - must not overwrite `read_at`
  - must not overwrite `updated_at`
  - must still return `200`

### Response Shape Rule
- do not return the legacy `domain.Notification` JSON shape
- return the same inbox item DTO as Task 12
- repository result must include actor metadata join so the handler does not need a second lookup

### Unread Count Rule
- this endpoint does not return unread count
- this task does not introduce an unread counter table
- idempotent row transition behavior must remain compatible with future unread count work

## 2.7 Positive And Negative Cases

### Positive Cases

1. Authenticated user marks an unread notification as read
- Result: `200`
- `is_read` becomes `true`
- `read_at` is set
- `updated_at` equals `read_at`

2. Authenticated user marks an already-read notification as read again
- Result: `200`
- existing `read_at` is preserved
- existing `updated_at` is preserved

3. Authenticated user marks an invitation notification as read
- Result: `200`
- invitation-specific payload and action fields remain unchanged

4. Authenticated user marks a comment notification as read
- Result: `200`
- comment-specific payload and resource metadata remain unchanged

### Negative Cases

1. Missing auth token
- Result: `401 unauthorized`

2. Invalid auth token
- Result: `401 unauthorized`

3. Authenticated actor id no longer resolves to a user
- Result: `401 unauthorized`

4. `notificationID` is malformed
- Result: `404 not_found`

5. Notification does not exist
- Result: `404 not_found`

6. Notification belongs to another user
- Result: `404 not_found`

---

## 3. File Structure And Responsibilities

### Modify
- `internal/application/notification_service.go`
  - update mark-read service contract to return inbox item DTO and validate actor plus notification id
- `internal/application/notification_service_test.go`
- `internal/application/notification_service_additional_test.go`
- `internal/repository/postgres/notification_repository.go`
  - replace legacy mark-read return shape with inbox item projection plus actor join
- `internal/repository/postgres/content_repository_test.go`
  - add PostgreSQL-backed mark-read coverage for v2 inbox shape and idempotency
- `internal/transport/http/handlers.go`
  - keep the same route, return the new DTO shape
- `internal/transport/http/server_test.go`
  - update mark-read endpoint tests for the new contract
- `frontend-repo/API_CONTRACT.md`
  - update mark-read endpoint contract to inbox item DTO
- `docs/checkpoint.md`

### Modify If Task 12 Implementation Requires It
- `internal/domain/notification.go`
  - only if shared inbox item types are not already present from Task 12

### Files Explicitly Not In Scope
- `internal/transport/http/server.go`
  - route path stays the same
- `internal/application/workspace_service.go`
- `internal/application/thread_service.go`

---

## 4. Test Matrix

## 4.1 Application Service Tests

Add or update tests in:
- `internal/application/notification_service_test.go`
- `internal/application/notification_service_additional_test.go`

### Positive Cases

1. Service marks unread notification as read
- Expect:
  - actor is validated through `users.GetByID`
  - repository receives actor id and notification id
  - returned inbox item has `is_read = true`

2. Service returns already-read notification unchanged on repeated call
- Expect:
  - repository response is returned unchanged
  - repeated call stays `200`

3. Service returns inbox item DTO, not legacy notification DTO

### Negative Cases

4. Unknown actor returns `domain.ErrUnauthorized`

5. Malformed `notificationID` returns `domain.ErrNotFound`

6. Repository not-found error propagates

7. Repository failure propagates

## 4.2 Repository Integration Tests

Add DB-backed tests in:
- `internal/repository/postgres/content_repository_test.go`

### Positive Cases

8. Mark unread notification as read updates `is_read`, `read_at`, and `updated_at`

9. Mark already-read notification as read again preserves original `read_at`

10. Mark already-read notification as read again preserves original `updated_at`

11. Mark-read returns inbox item with actor metadata when actor user exists

12. Mark-read returns inbox item with `actor = null` when `actor_id` is null or actor row is unavailable

13. Mark-read preserves invitation payload, action fields, and resource fields

### Negative Cases

14. Missing notification id returns `domain.ErrNotFound`

15. Notification owned by another user returns `domain.ErrNotFound`

16. Closed pool or query failure returns wrapped repository error

## 4.3 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_test.go`

### Positive Cases

17. `POST /api/v1/notifications/{notificationID}/read` returns success envelope plus inbox item DTO
- Assert:
  - top-level `data`
  - `data.is_read = true`
  - `data.read_at` non-null
  - legacy public fields `event_id` and `message` are absent

18. Repeated mark-read returns `200` and preserves the same `read_at`

19. Invitation notification mark-read keeps invitation payload and actionability fields unchanged

### Negative Cases

20. Missing auth returns `401`

21. Unknown actor returns `401`

22. Malformed `notificationID` returns `404`

23. Other-user notification returns `404`

24. Missing notification returns `404`

## 4.4 Documentation Tests

25. `frontend-repo/API_CONTRACT.md` documents:
- success envelope
- no request payload
- inbox item response shape
- idempotent repeated-read behavior
- `404` for foreign or missing notification

26. `docs/checkpoint.md` records:
- mark-read now returns inbox item DTO
- mark-read is idempotent
- first-read versus repeated-read semantics

---

## 5. Execution Plan

### Task 1: Define failing service tests for mark-read v2 behavior

**Files:**
- Modify: `internal/application/notification_service_test.go`
- Modify: `internal/application/notification_service_additional_test.go`

- [ ] **Step 1: Add failing service tests for successful mark-read**

Cover:
- actor validation
- inbox item return type
- first-read semantics

- [ ] **Step 2: Add failing service tests for idempotency and negative cases**

Cover:
- repeated read
- malformed notification id
- unknown actor
- repository not found
- repository error propagation

- [ ] **Step 3: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestNotificationService" -count=1
```

Expected:
- FAIL because mark-read still returns the legacy contract

- [ ] **Step 4: Commit**

```bash
git add internal/application/notification_service_test.go internal/application/notification_service_additional_test.go
git commit -m "test: define notification mark-read service behavior"
```

### Task 2: Implement service mark-read validation and DTO contract

**Files:**
- Modify: `internal/application/notification_service.go`
- Modify if needed: `internal/domain/notification.go`

- [ ] **Step 1: Add or confirm shared inbox item types**

Requirement:
- reuse the Task 12 inbox item DTO
- do not introduce a second public response model for one notification row

- [ ] **Step 2: Update the service mark-read method**

Required behavior:
- resolve actor with `users.GetByID`
- return `domain.ErrUnauthorized` when actor is missing
- validate `notificationID` as UUID
- return `domain.ErrNotFound` on malformed id
- call repository mark-read with server timestamp
- return inbox item DTO

- [ ] **Step 3: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestNotificationService" -count=1
```

Expected:
- PASS

- [ ] **Step 4: Commit**

```bash
git add internal/application/notification_service.go internal/domain/notification.go
git commit -m "feat: add notification mark-read service contract"
```

### Task 3: Add repository mark-read v2 query and integration coverage

**Files:**
- Modify: `internal/repository/postgres/notification_repository.go`
- Modify: `internal/repository/postgres/content_repository_test.go`

- [ ] **Step 1: Add failing repository tests for mark-read v2**

Cover:
- first read
- repeated read
- actor join
- other-user access
- missing id

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositoryErrors" -count=1
```

Expected:
- FAIL because mark-read still returns the legacy shape

- [ ] **Step 3: Implement repository mark-read query**

Required behavior:
- update only rows owned by the actor
- set:
  - `is_read = TRUE`
  - `read_at = COALESCE(read_at, $3)`
  - `updated_at = COALESCE(read_at, $3)`
- use a writable CTE or equivalent query so the method returns the updated row plus actor metadata
- return `domain.ErrNotFound` when no row matches
- keep the operation idempotent

- [ ] **Step 4: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositoryErrors" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/repository/postgres/notification_repository.go internal/repository/postgres/content_repository_test.go
git commit -m "feat: add notification mark-read query"
```

### Task 4: Update handler and HTTP endpoint tests

**Files:**
- Modify: `internal/transport/http/handlers.go`
- Modify: `internal/transport/http/server_test.go`

- [ ] **Step 1: Add failing HTTP tests for the new mark-read contract**

Cover:
- success envelope
- inbox item fields
- repeated read
- malformed id
- foreign notification

- [ ] **Step 2: Update `handleMarkNotificationRead`**

Handler requirements:
- keep request body unread
- pass `notificationID` path parameter to service
- map unauthorized and not-found errors through the existing error mapper
- return `200` with inbox item DTO

- [ ] **Step 3: Re-run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "TestNotificationEndpoints" -count=1
```

Expected:
- PASS

- [ ] **Step 4: Commit**

```bash
git add internal/transport/http/handlers.go internal/transport/http/server_test.go
git commit -m "feat: redesign notification mark-read endpoint"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update the mark-read API contract**

Document:
- no request payload
- success envelope plus inbox item response shape
- idempotent repeated-read behavior
- `404` for foreign and missing notification

- [ ] **Step 2: Update checkpoint**

Record:
- `POST /api/v1/notifications/{notificationID}/read` now returns inbox item DTO
- mark-read is idempotent
- first-read updates read state once

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: update notification mark-read contract"
```

### Task 6: Full verification for Task 13

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "TestNotificationService" -count=1
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositoryErrors" -count=1
go test ./internal/transport/http -run "TestNotificationEndpoints" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual API sanity check if local server is available**

Call:
```http
POST /api/v1/notifications/{notificationID}/read
```

Verify:
- `200`
- response includes `data.is_read = true`
- response includes stable `read_at` on repeated calls
- response does not include legacy public fields from the old notification DTO

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify notification mark-read task"
```

---

## 6. Acceptance Criteria

Task 13 is complete only when all are true:
- `POST /api/v1/notifications/{notificationID}/read` returns the inbox item DTO instead of the legacy notification shape
- the endpoint remains idempotent
- first read sets `is_read`, `read_at`, and `updated_at`
- repeated read preserves the original `read_at` and `updated_at`
- other users cannot read or probe someone else's notifications
- malformed notification ids return `404`
- service, repository, and HTTP tests cover all positive and negative cases above
- docs and checkpoint reflect the new contract

## 7. Risks And Guardrails

- Do not leak whether a notification exists for another user.
- Do not overwrite `read_at` on repeated calls.
- Do not return a response shape that differs from the inbox item model used by Task 12.
- Do not add unread count or batch-read behavior in this task.
- Keep the repository update ownership-scoped so the service does not rely on post-update filtering.

## 8. Follow-On Tasks

This plan prepares for:
- Task 14 `GET /api/v1/notifications/unread-count`
- Task 15 `POST /api/v1/notifications/read`
