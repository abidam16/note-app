# Task 12 GET Notifications Inbox V2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign `GET /api/v1/notifications` into the canonical inbox API with read/unread filtering, type filtering, cursor pagination, unread count, and the latest invitation live state.

**Architecture:** This task upgrades the current raw `Notification[]` endpoint into an inbox read model endpoint. The transport layer parses `status`, `type`, `limit`, and `cursor`; the service validates the authenticated actor and delegates to a repository query that returns notification inbox items, an unread count, and a cursor page. The repository reads from the v2 notification schema, joins actor user metadata, and returns invitation rows already maintained by the live projector from Task 11. This task changes the public response contract of `GET /api/v1/notifications`, but does not yet change mark-read or add the standalone unread-count endpoint.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, explicit SQL repositories, table-driven tests, PostgreSQL-backed repository tests

---

## 1. Scope

### In Scope
- Redesign one existing endpoint:
  - `GET /api/v1/notifications`
- Add `status` filter:
  - `all|read|unread`
- Add `type` filter:
  - `all|invitation|comment|mention`
- Add bounded cursor pagination
- Return `unread_count` with every page
- Expose the new public inbox notification DTO
- Join actor user metadata into the response
- Add service, repository, handler, and HTTP tests
- Update API docs and checkpoint

### Out Of Scope
- No unread-count-only endpoint
- No batch mark-read endpoint
- No change to `POST /api/v1/notifications/{notificationID}/read` yet
- No SSE or WebSocket delivery
- No unread counter table in this task
- No frontend implementation

---

## 2. Detailed Spec

## 2.1 Objective

The frontend needs one canonical inbox API that returns:
- all relevant notifications for the actor
- clear read/unread state
- latest invitation live state
- sender information
- cursor pagination
- current unread count

This task upgrades the current list endpoint from a raw persistence shape into that inbox DTO.

## 2.2 Endpoint

### `GET /api/v1/notifications`

- Auth: yes
- Authorization: own inbox only

This endpoint returns the authenticated actor's inbox only. It is not workspace-scoped.

## 2.3 Query Parameters

- `status` optional
  - allowed values:
    - `all`
    - `read`
    - `unread`
  - default:
    - `all`

- `type` optional
  - allowed values:
    - `all`
    - `invitation`
    - `comment`
    - `mention`
  - default:
    - `all`

- `limit` optional
  - positive integer
  - default `50`
  - max `100`

- `cursor` optional
  - opaque pagination cursor returned by a previous response

## 2.4 Response Payload

This codebase wraps successful responses in the standard success envelope:

```json
{
  "data": {
    "items": [
      {
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
        "is_read": false,
        "read_at": null,
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
        "updated_at": "2026-04-04T08:00:00Z"
      }
    ],
    "unread_count": 12,
    "next_cursor": "opaque-cursor",
    "has_more": true
  }
}
```

Response `200` payload inside `data`:
- `items`
- `unread_count`
- `next_cursor` optional
- `has_more`

### Notification Item Rules
- `actor` is optional
  - if `actor_id` is null or actor row no longer exists, `actor = null`
- `is_read` is the canonical inbox read status
- `read_at` is null for unread rows
- invitation lifecycle state is exposed in `payload.status`
- `payload` is type-specific and must be returned as JSON object

### Ordering Rules
- order by `created_at DESC, id DESC`
- cursor must continue from the last item using that same ordering

### Pagination Rules
- forward-only pagination
- query uses `limit + 1` fetch strategy
- if more items remain than `limit`, return:
  - `has_more = true`
  - `next_cursor = <opaque token>`
- on final page:
  - `has_more = false`
  - omit `next_cursor`

### Unread Count Rules
- `unread_count` is the actor's current total unread count across the whole inbox
- it is not filtered by the current page or current `type` filter
- it is filtered only by `user_id = actor`

In this task, `unread_count` may come from a dedicated count query. The cheap standalone unread endpoint is a later task.

## 2.5 Public Notification DTO Rules

This task changes the public list DTO from the old minimal notification shape to a new inbox DTO.

### Public Fields In Scope
- `id`
- `workspace_id`
- `type`
- `actor_id`
- `actor`
- `title`
- `content`
- `is_read`
- `read_at`
- `actionable`
- `action_kind`
- `resource_type`
- `resource_id`
- `payload`
- `created_at`
- `updated_at`

### Public Fields Removed From List Response
- `user_id`
- `event_id`
- `message`

Those remain internal compatibility fields for storage and older write paths, but the list endpoint should no longer expose them.

## 2.6 Validation Rules

1. Actor must be authenticated
2. Actor id from auth context must resolve to an existing user record
3. `status` must be one of:
   - `all|read|unread`
4. `type` must be one of:
   - `all|invitation|comment|mention`
5. `limit` must be:
   - omitted, or
   - positive integer, and
   - `<= 100`
6. `cursor` must be valid for this endpoint
7. Cursor filter metadata must match the current request filters

## 2.7 Behavior Rules

### Actor Validation
- resolve actor with `users.GetByID`
- if actor id does not resolve:
  - return `401 unauthorized`

### Own-Inbox Scope
- always filter by `user_id = actorID`
- do not accept a user id query parameter
- do not allow workspace-level notification listing here

### Status Filter Semantics
- `status=all`
  - no read-state filter
- `status=read`
  - only `is_read = TRUE`
- `status=unread`
  - only `is_read = FALSE`

### Type Filter Semantics
- `type=all`
  - no notification-type filter
- `type=invitation`
  - only invitation notifications
- `type=comment`
  - only comment notifications
- `type=mention`
  - only mention notifications

### Empty Result Rules
- empty list is valid
- return:
  - `items = []`
  - `unread_count = <current unread total>`
  - `has_more = false`
  - omit `next_cursor`

### Invitation Row Rules
- invitation list items come from the live notification row maintained by Task 11
- no duplicate invitation cards for the same invitation should appear
- terminal invitation notifications remain in the inbox without buttons

### Actor Metadata Rules
- join actor metadata from `users`
- if `actor_id` is null:
  - return `actor_id = null`
  - return `actor = null`
- if actor user row no longer exists:
  - return `actor_id` as stored if available
  - return `actor = null`

## 2.8 Cursor Format Rules

The cursor must encode:
- `created_at`
- `id`
- `status_filter`
- `type_filter`

Recommended format:
- base64url-encoded JSON object

### Cursor Validation Rules
- malformed base64 => validation error
- malformed JSON => validation error
- missing `created_at` or `id` => validation error
- filter mismatch between cursor and request => validation error

## 2.9 Positive And Negative Cases

### Positive Cases

1. Authenticated user lists all notifications
- Result: `200`
- returns newest-first inbox page

2. Authenticated user filters unread notifications
- Result: `200`
- only unread rows returned

3. Authenticated user filters invitation notifications
- Result: `200`
- only invitation rows returned

4. Authenticated user paginates with cursor
- Result: `200`
- next page begins after the cursor row

5. Authenticated user has empty inbox
- Result: `200`
- returns empty list and current unread count `0`

### Negative Cases

1. Missing auth token
- Result: `401 unauthorized`

2. Invalid auth token
- Result: `401 unauthorized`

3. Authenticated actor id does not resolve to a user
- Result: `401 unauthorized`

4. Invalid `status`
- Result: `422 validation_failed`

5. Invalid `type`
- Result: `422 validation_failed`

6. Invalid `limit`
- Result: `422 validation_failed`

7. Invalid `cursor`
- Result: `422 validation_failed`

---

## 3. File Structure And Responsibilities

### Modify
- `internal/domain/notification.go`
  - add public inbox DTO types if domain remains the shared response-model layer
- `internal/application/notification_service.go`
  - replace raw list call with inbox list input/output
- `internal/application/notification_service_test.go`
- `internal/application/notification_service_additional_test.go`
- `internal/repository/postgres/notification_repository.go`
  - add inbox list query, unread count query, cursor encode/decode, actor join
- `internal/repository/postgres/content_repository_test.go`
  - add notification inbox integration coverage if that remains the notification test location
- `internal/transport/http/handlers.go`
  - parse query params and return the new response shape
- `internal/transport/http/server_test.go`
  - update notification endpoint tests for the new contract
- `frontend-repo/API_CONTRACT.md`
- `docs/checkpoint.md`

### Test Fake Updates Required
- `internal/application/notification_service_test.go`
  - fake notification repo list method must return the new inbox result
- `internal/application/notification_service_additional_test.go`
  - stub methods for inbox list and unread count behavior
- `internal/transport/http/server_test.go`
  - `testNotificationRepo` must support filtered list behavior and unread count expectations

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

1. Service lists inbox page with default filters
- Expect:
  - default `status = all`
  - default `type = all`
  - default `limit = 50`

2. Service validates actor existence before listing
- Expect:
  - unknown actor => `domain.ErrUnauthorized`

3. Service passes filters and cursor into repository list call

### Negative Cases

4. Invalid `status` returns validation error

5. Invalid `type` returns validation error

6. Zero or oversized `limit` returns validation error

7. Repository list error propagates

8. Repository unread-count error propagates

## 4.2 Repository Integration Tests

Add DB-backed tests in the PostgreSQL notification test area.

### Positive Cases

9. List inbox default order returns newest-first rows

10. `status=unread` returns only unread rows

11. `status=read` returns only read rows

12. `type=invitation` returns only invitation rows

13. `type=comment` returns only comment rows

14. `type=mention` returns only mention rows

15. Cursor pagination returns:
- `has_more = true`
- `next_cursor`
- next page starts after prior page

16. Actor metadata is joined when `actor_id` exists

17. `actor` is null when actor metadata is unavailable

18. `unread_count` returns total unread rows for the actor independent of current filters

### Negative Cases

19. Invalid cursor returns `domain.ErrValidation`

20. Cursor filter mismatch returns `domain.ErrValidation`

21. Other user's notifications are never returned

## 4.3 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_test.go`

### Positive Cases

22. `GET /api/v1/notifications` returns the new envelope plus inbox response shape
- Assert:
  - top-level `data`
  - `data.items`
  - `data.unread_count`
  - `data.has_more`

23. `GET /api/v1/notifications?status=unread&type=invitation&limit=1` filters correctly

24. Cursor request returns next page

25. Empty inbox returns empty list and unread count `0`

### Negative Cases

26. Invalid `status` returns `422`

27. Invalid `type` returns `422`

28. Invalid `limit` returns `422`

29. Invalid `cursor` returns `422`

30. Unknown actor returns `401`

## 4.4 Documentation Tests

31. `frontend-repo/API_CONTRACT.md` documents:
- new response shape
- filters
- cursor semantics
- unread_count semantics
- new public notification DTO

32. `docs/checkpoint.md` records:
- inbox API redesign
- public notification list contract change
- read/unread and type filters
- cursor pagination and unread_count behavior

---

## 5. Execution Plan

### Task 1: Define failing service tests for inbox list semantics

**Files:**
- Modify: `internal/application/notification_service_test.go`
- Modify: `internal/application/notification_service_additional_test.go`

- [ ] **Step 1: Add failing service tests for default list behavior**

Cover:
- actor validation
- default filters
- new list result shape

- [ ] **Step 2: Add failing service tests for invalid filters and limit**

Cover:
- invalid status
- invalid type
- invalid limit
- repository error propagation

- [ ] **Step 3: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestNotificationService" -count=1
```

Expected:
- FAIL because the new inbox list contract does not exist yet

- [ ] **Step 4: Commit**

```bash
git add internal/application/notification_service_test.go internal/application/notification_service_additional_test.go
git commit -m "test: define notification inbox service behavior"
```

### Task 2: Implement notification inbox DTOs and service contract

**Files:**
- Modify: `internal/domain/notification.go`
- Modify: `internal/application/notification_service.go`

- [ ] **Step 1: Add inbox DTO types**

Recommended shared types:

```go
type NotificationActor struct {
    ID       string `json:"id"`
    Email    string `json:"email"`
    FullName string `json:"full_name"`
}

type NotificationInboxItem struct {
    ID           string          `json:"id"`
    WorkspaceID  string          `json:"workspace_id"`
    Type         NotificationType `json:"type"`
    ActorID      *string         `json:"actor_id"`
    Actor        *NotificationActor `json:"actor,omitempty"`
    Title        string          `json:"title"`
    Content      string          `json:"content"`
    IsRead       bool            `json:"is_read"`
    ReadAt       *time.Time      `json:"read_at,omitempty"`
    Actionable   bool            `json:"actionable"`
    ActionKind   *NotificationActionKind `json:"action_kind,omitempty"`
    ResourceType *NotificationResourceType `json:"resource_type,omitempty"`
    ResourceID   *string         `json:"resource_id,omitempty"`
    Payload      json.RawMessage `json:"payload"`
    CreatedAt    time.Time       `json:"created_at"`
    UpdatedAt    time.Time       `json:"updated_at"`
}

type NotificationInboxPage struct {
    Items       []NotificationInboxItem `json:"items"`
    UnreadCount int64                   `json:"unread_count"`
    NextCursor  *string                 `json:"next_cursor,omitempty"`
    HasMore     bool                    `json:"has_more"`
}
```

- [ ] **Step 2: Add list input type and validation**

Recommended:

```go
type ListNotificationsInput struct {
    Status string
    Type   string
    Limit  int
    Cursor string
}
```

- [ ] **Step 3: Implement `NotificationService.ListNotifications` with the new contract**

Required behavior:
- resolve actor with `users.GetByID`
- normalize missing actor to `domain.ErrUnauthorized`
- validate filters and limit
- delegate to repository inbox list method

- [ ] **Step 4: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestNotificationService" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/notification.go internal/application/notification_service.go
git commit -m "feat: add notification inbox service contract"
```

### Task 3: Add repository inbox query, unread count, and cursor support

**Files:**
- Modify: `internal/repository/postgres/notification_repository.go`
- Modify: `internal/repository/postgres/content_repository_test.go`

- [ ] **Step 1: Add failing repository integration tests for inbox listing**

Cover:
- default order
- status filters
- type filters
- unread count
- cursor pagination
- actor join
- invalid cursor

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository" -count=1
```

Expected:
- FAIL because inbox list query does not exist yet

- [ ] **Step 3: Implement cursor encode/decode**

Required cursor fields:
- `created_at`
- `id`
- `status_filter`
- `type_filter`

- [ ] **Step 4: Implement repository inbox list query**

Requirements:
- filter by actor `user_id`
- optional `is_read` filter
- optional `type` filter
- left join actor user metadata
- order `created_at DESC, id DESC`
- fetch `limit + 1`
- compute `unread_count` with dedicated count query
- return `domain.ErrValidation` for invalid cursor

- [ ] **Step 5: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository" -count=1
```

Expected:
- PASS

- [ ] **Step 6: Commit**

```bash
git add internal/repository/postgres/notification_repository.go internal/repository/postgres/content_repository_test.go
git commit -m "feat: add notification inbox query"
```

### Task 4: Update handler and HTTP endpoint tests

**Files:**
- Modify: `internal/transport/http/handlers.go`
- Modify: `internal/transport/http/server_test.go`

- [ ] **Step 1: Add failing HTTP tests for the new response contract**

Cover:
- new response shape
- unread_count
- filters
- cursor pagination
- empty inbox

- [ ] **Step 2: Add failing HTTP tests for invalid query parameters**

Cover:
- invalid status
- invalid type
- invalid limit
- invalid cursor
- unknown actor

- [ ] **Step 3: Update `handleListNotifications`**

Handler requirements:
- parse query params
- call service with `ListNotificationsInput`
- map validation and unauthorized errors through existing error mapper
- return `200` with `NotificationInboxPage`

- [ ] **Step 4: Re-run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "TestNotificationEndpoints" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/transport/http/handlers.go internal/transport/http/server_test.go
git commit -m "feat: redesign notification inbox endpoint"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update the notification list API contract**

Document:
- filters
- success envelope plus inbox page response shape
- new public notification DTO
- unread_count semantics
- cursor semantics

- [ ] **Step 2: Update checkpoint**

Record:
- `GET /api/v1/notifications` now returns inbox page data
- filters and pagination rules
- new public notification fields

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: update notification inbox API contract"
```

### Task 6: Full verification for Task 12

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "TestNotificationService" -count=1
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository" -count=1
go test ./internal/transport/http -run "TestNotificationEndpoints" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual API sanity check if local server is available**

Call:
```http
GET /api/v1/notifications?status=unread&type=invitation&limit=1
```

Verify:
- `200`
- page response includes `items`, `unread_count`, and pagination fields
- invitation row exposes `payload.status` and `is_read`

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify notification inbox task"
```

---

## 6. Acceptance Criteria

Task 12 is complete only when all are true:
- `GET /api/v1/notifications` returns inbox page data instead of raw `Notification[]`
- it supports `status=all|read|unread`
- it supports `type=all|invitation|comment|mention`
- it uses bounded cursor pagination
- it returns total `unread_count`
- it exposes clear public read state through `is_read`
- invitation lifecycle state is visible through `payload.status`
- actor metadata is included when available
- service, repository, and HTTP tests cover all positive and negative cases above
- docs and checkpoint reflect the new contract

## 7. Risks And Guardrails

- Do not leak other users' inbox rows.
- Do not recompute unread count by loading the whole inbox into memory.
- Do not expose internal compatibility fields like `event_id` and `message` in the new list response.
- Keep cursor validation strict so clients cannot mix cursors across filter sets.
- Do not add standalone unread-count or batch-read behavior in this task.

## 8. Follow-On Tasks

This plan prepares for:
- Task 13 `POST /api/v1/notifications/{notificationID}/read`
- Task 14 `GET /api/v1/notifications/unread-count`
- Task 15 `POST /api/v1/notifications/read`
