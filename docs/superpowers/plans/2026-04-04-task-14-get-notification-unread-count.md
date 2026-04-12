# Task 14 GET Notification Unread Count Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `GET /api/v1/notifications/unread-count` so the frontend can fetch a cheap, correct unread badge value without loading inbox pages.

**Architecture:** This task adds a dedicated unread counter read model backed by a small per-user table. The transport layer exposes one authenticated GET endpoint, the service validates the actor, and the repository returns the stored unread count with `0` as the empty default. To keep the endpoint cheap and correct, this task also wires transactional counter maintenance into notification insert, batch insert, invitation live-notification insert, and first-time mark-read transitions. The endpoint itself stays simple because the hard work happens at write time.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, SQL-backed repositories, PostgreSQL migrations, table-driven tests

---

## 1. Scope

### In Scope
- Add one new endpoint:
  - `GET /api/v1/notifications/unread-count`
- Add a persistent unread counter table
- Return unread count from the counter table, not from inbox listing
- Keep the response cheap for high-frequency badge polling
- Update notification write paths so the counter remains correct:
  - single notification create
  - batch notification create
  - invitation live-notification insert if Task 11 code exists
  - first unread-to-read transition
- Keep repeated mark-read idempotent with no double decrement
- Add service, repository, handler, migration, and HTTP tests
- Update API docs and checkpoint

### Out Of Scope
- No batch unread-count endpoint
- No SSE or WebSocket push
- No notification preference logic
- No reconciliation job in this task
- No change to inbox item DTO
- No full inbox endpoint redesign beyond possibly reusing the same counter source internally

---

## 2. Detailed Spec

## 2.1 Objective

The frontend needs a cheap badge endpoint for unread notifications. The inbox list endpoint from Task 12 can return unread count along with page data, but badge refresh should not require loading notification rows.

This task defines a dedicated unread-count read model:
- one row per user
- updated transactionally when unread state changes
- queried directly by the badge endpoint

## 2.2 Endpoint

### `GET /api/v1/notifications/unread-count`

- Auth: yes
- Authorization: authenticated actor only

This endpoint returns the actor's total unread notification count across all notification types.

## 2.3 Request Payload

This endpoint has no request JSON payload.

### Request Rules
- no request body
- no required query parameters
- unread count is not filterable in this task
- the count always covers the actor's full inbox

## 2.4 Response Payload

This codebase wraps successful responses in the standard success envelope:

```json
{
  "data": {
    "unread_count": 12
  }
}
```

### Response Rules
- `unread_count` is a non-negative integer
- `unread_count` is scoped only by authenticated actor
- if the actor has no unread counter row yet:
  - return `200`
  - return `unread_count = 0`
- this endpoint does not return notification rows, filters, cursors, or unread-by-type breakdowns

## 2.5 Validation Rules

1. Actor must be authenticated
2. Actor id from auth context must resolve to an existing user record

## 2.6 Behavior Rules

### Actor Validation
- resolve actor with `users.GetByID`
- if actor id does not resolve:
  - return `401 unauthorized`

### Counter Lookup Rule
- read from the dedicated unread counter table
- if no row exists for the actor:
  - treat it as `0`
- do not return `404` for missing counter rows

### Count Scope Rule
- unread count includes all unread notifications for the actor:
  - invitation
  - comment
  - mention
- the count is not filtered by current page, workspace, or type

### Performance Rule
- the endpoint must not compute unread count by scanning notification rows in application memory
- the endpoint must not depend on loading inbox pages
- the endpoint must read one small per-user row or use an equivalent O(1)-ish lookup path

### Counter Maintenance Rules

#### Single Create
- when a new unread notification row is inserted successfully:
  - increment the recipient's counter by `1`
- if insert conflicts and no notification row is created:
  - do not increment

#### Batch Create
- increment each recipient counter by the number of unread notification rows actually inserted for that user
- duplicates skipped by `ON CONFLICT DO NOTHING` must not increment counters

#### Invitation Live Notification Upsert
- if Task 11 implementation has landed and inserts a new live invitation row:
  - increment unread count by `1`
- if Task 11 updates an existing live invitation row:
  - do not increment
- if the existing live row is already read and gets updated:
  - preserve read state and do not change the counter

#### Mark Read
- only the first unread-to-read transition decrements the counter
- repeated mark-read must not decrement again
- counter must never become negative

### Consistency Rule
- notification row mutation and unread counter mutation must happen in the same database transaction or same SQL statement chain
- successful notification write with failed counter update is not allowed
- successful mark-read with failed counter decrement is not allowed

## 2.7 Read Model Schema

Add a new table:

### `notification_unread_counters`

Columns:
- `user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE`
- `unread_count BIGINT NOT NULL DEFAULT 0 CHECK (unread_count >= 0)`
- `created_at TIMESTAMPTZ NOT NULL`
- `updated_at TIMESTAMPTZ NOT NULL`

### Schema Rules
- exactly one row per user
- no negative counts
- deleting a user deletes the counter row
- missing row means count `0`, not an error

### Migration File Names
- `migrations/000022_notification_unread_counters.up.sql`
- `migrations/000022_notification_unread_counters.down.sql`

If the actual migration sequence changed before implementation, keep the name pattern and use the next available migration number.

## 2.8 Public Positive And Negative Cases

### Positive Cases

1. Authenticated user with unread notifications fetches unread count
- Result: `200`
- returns current unread total

2. Authenticated user with no unread notifications fetches unread count
- Result: `200`
- returns `0`

3. Authenticated user without a counter row yet fetches unread count
- Result: `200`
- returns `0`

### Negative Cases

1. Missing auth token
- Result: `401 unauthorized`

2. Invalid auth token
- Result: `401 unauthorized`

3. Authenticated actor id no longer resolves to a user
- Result: `401 unauthorized`

---

## 3. File Structure And Responsibilities

### Create
- `migrations/000022_notification_unread_counters.up.sql`
  - create unread counter table and constraints
- `migrations/000022_notification_unread_counters.down.sql`
  - drop unread counter table

### Modify
- `internal/domain/notification.go`
  - add a small unread-count response DTO if the codebase does not already have one
- `internal/application/notification_service.go`
  - add service method for unread count
- `internal/application/notification_service_test.go`
- `internal/application/notification_service_additional_test.go`
- `internal/repository/postgres/notification_repository.go`
  - add unread-count lookup
  - maintain counters in create, create-many, mark-read, and invitation-live upsert paths
- `internal/repository/postgres/content_repository_test.go`
  - add integration coverage for counter maintenance and unread-count lookup
- `internal/transport/http/handlers.go`
  - add unread-count handler
- `internal/transport/http/server.go`
  - register the new route
- `internal/transport/http/server_test.go`
  - add endpoint coverage
- `frontend-repo/API_CONTRACT.md`
  - document the new endpoint
- `docs/checkpoint.md`

### Modify If Task 11 Code Exists By Then
- `internal/repository/postgres/notification_repository.go`
  - update `UpsertInvitationLive` to increment counters only on insert
- any Task 11 projector tests that assert live-row insertion behavior

### Files Explicitly Not In Scope
- `internal/application/workspace_service.go`
- `internal/application/thread_service.go`
- `internal/transport/http/response.go`
  - success envelope stays unchanged

---

## 4. Test Matrix

## 4.1 Migration And Schema Tests

Add or extend repository integration coverage to prove:

1. Migration creates `notification_unread_counters`

2. `unread_count >= 0` constraint rejects negative values

3. Deleting a user cascades the counter row

## 4.2 Application Service Tests

Add or update tests in:
- `internal/application/notification_service_test.go`
- `internal/application/notification_service_additional_test.go`

### Positive Cases

4. Service returns unread count for a valid actor

5. Service returns `0` when repository reports no counter row

### Negative Cases

6. Unknown actor returns `domain.ErrUnauthorized`

7. Repository lookup error propagates

## 4.3 Repository Integration Tests

Add DB-backed tests in:
- `internal/repository/postgres/content_repository_test.go`

### Counter Lookup Cases

8. `GetUnreadCount` returns `0` when no counter row exists

9. `GetUnreadCount` returns stored count for an existing actor

### Counter Maintenance Cases

10. Single notification create increments unread count by `1`

11. Single create conflict does not increment unread count

12. Batch create increments each user's count by only the rows actually inserted for that user

13. Batch create duplicate rows skipped by conflict do not increment unread count

14. First mark-read decrements unread count by `1`

15. Repeated mark-read does not decrement unread count again

16. Counter never becomes negative on repeated mark-read

17. If Task 11 upsert exists, inserting a new live invitation notification increments unread count

18. If Task 11 upsert exists, updating an existing live invitation notification does not increment unread count

### Failure Cases

19. Closed pool or query failure returns wrapped repository error

## 4.4 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_test.go`

### Positive Cases

20. `GET /api/v1/notifications/unread-count` returns success envelope and unread count
- Assert:
  - top-level `data`
  - `data.unread_count`

21. Authenticated user with no counter row gets `0`

### Negative Cases

22. Missing auth returns `401`

23. Unknown actor returns `401`

## 4.5 Documentation Tests

24. `frontend-repo/API_CONTRACT.md` documents:
- endpoint path
- no request payload
- success envelope
- `unread_count` semantics
- `200` and `401` cases

25. `docs/checkpoint.md` records:
- unread counter table added
- unread count endpoint added
- counter maintenance semantics on insert and first-read

---

## 5. Execution Plan

### Task 1: Define failing service tests for unread-count behavior

**Files:**
- Modify: `internal/application/notification_service_test.go`
- Modify: `internal/application/notification_service_additional_test.go`

- [ ] **Step 1: Add failing service tests for successful unread-count lookup**

Cover:
- valid actor
- zero result
- response DTO shape

- [ ] **Step 2: Add failing service tests for negative cases**

Cover:
- unknown actor
- repository error propagation

- [ ] **Step 3: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestNotificationService" -count=1
```

Expected:
- FAIL because unread-count service contract does not exist yet

- [ ] **Step 4: Commit**

```bash
git add internal/application/notification_service_test.go internal/application/notification_service_additional_test.go
git commit -m "test: define notification unread-count service behavior"
```

### Task 2: Add unread counter migration

**Files:**
- Create: `migrations/000022_notification_unread_counters.up.sql`
- Create: `migrations/000022_notification_unread_counters.down.sql`

- [ ] **Step 1: Write the migration**

Required DDL:
- create `notification_unread_counters`
- primary key on `user_id`
- foreign key to `users(id)` with `ON DELETE CASCADE`
- `CHECK (unread_count >= 0)`

- [ ] **Step 2: Run migrations against the test database**

Run:
```powershell
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration" -count=1
```

Expected:
- FAIL later in repository logic, not on missing table DDL

- [ ] **Step 3: Commit**

```bash
git add migrations/000022_notification_unread_counters.up.sql migrations/000022_notification_unread_counters.down.sql
git commit -m "feat: add notification unread counter schema"
```

### Task 3: Implement service unread-count contract

**Files:**
- Modify: `internal/domain/notification.go`
- Modify: `internal/application/notification_service.go`

- [ ] **Step 1: Add or confirm unread-count response DTO**

Recommended shape:

```go
type NotificationUnreadCount struct {
    UnreadCount int64 `json:"unread_count"`
}
```

- [ ] **Step 2: Add service method**

Recommended signature:

```go
GetUnreadCount(ctx context.Context, actorID string) (domain.NotificationUnreadCount, error)
```

Required behavior:
- resolve actor with `users.GetByID`
- return `domain.ErrUnauthorized` if actor is missing
- delegate to repository unread-count lookup
- normalize missing counter row to `0`

- [ ] **Step 3: Re-run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestNotificationService" -count=1
```

Expected:
- PASS

- [ ] **Step 4: Commit**

```bash
git add internal/domain/notification.go internal/application/notification_service.go
git commit -m "feat: add notification unread-count service"
```

### Task 4: Implement repository unread counter maintenance and lookup

**Files:**
- Modify: `internal/repository/postgres/notification_repository.go`
- Modify: `internal/repository/postgres/content_repository_test.go`

- [ ] **Step 1: Add failing repository integration tests for counter lookup and maintenance**

Cover:
- zero default
- single create increment
- create conflict no increment
- batch create increments by actual inserts
- first mark-read decrement
- repeated mark-read no double decrement
- non-negative counter guard
- invitation live-upsert insert versus update if available

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositoryErrors" -count=1
```

Expected:
- FAIL because counter logic does not exist yet

- [ ] **Step 3: Add repository unread-count method**

Recommended signature:

```go
GetUnreadCount(ctx context.Context, userID string) (int64, error)
```

Behavior:
- `SELECT unread_count FROM notification_unread_counters WHERE user_id = $1`
- missing row returns `0, nil`

- [ ] **Step 4: Update single-create path**

Required behavior:
- when notification insert succeeds with unread state:
  - increment recipient counter in the same transaction
- on conflict:
  - do not increment

- [ ] **Step 5: Update batch-create path**

Required behavior:
- count only rows actually inserted
- increment each affected user's counter by the inserted unread row count
- use one SQL statement chain or one transaction

- [ ] **Step 6: Update mark-read path**

Required behavior:
- decrement counter only when the row was unread before the update
- preserve idempotent repeated-read behavior
- prevent negative counts with SQL guardrails

- [ ] **Step 7: Update invitation live-upsert path if it exists**

Required behavior:
- increment only on insert
- no counter change on update

- [ ] **Step 8: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositoryErrors" -count=1
```

Expected:
- PASS

- [ ] **Step 9: Commit**

```bash
git add internal/repository/postgres/notification_repository.go internal/repository/postgres/content_repository_test.go
git commit -m "feat: add notification unread counter maintenance"
```

### Task 5: Add handler, route, and HTTP tests

**Files:**
- Modify: `internal/transport/http/handlers.go`
- Modify: `internal/transport/http/server.go`
- Modify: `internal/transport/http/server_test.go`

- [ ] **Step 1: Add failing HTTP tests for the unread-count endpoint**

Cover:
- success response
- zero response
- missing auth
- unknown actor

- [ ] **Step 2: Add `handleGetUnreadNotificationCount`**

Handler requirements:
- no request body parsing
- call service with authenticated actor id
- map errors through existing error mapper
- return success envelope with unread count DTO

- [ ] **Step 3: Register the route**

Route:

```go
r.Get("/notifications/unread-count", s.handleGetUnreadNotificationCount())
```

- [ ] **Step 4: Re-run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "TestNotificationEndpoints" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/transport/http/handlers.go internal/transport/http/server.go internal/transport/http/server_test.go
git commit -m "feat: add notification unread-count endpoint"
```

### Task 6: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update the API contract**

Document:
- `GET /api/v1/notifications/unread-count`
- no request payload
- success envelope
- actor-scoped count semantics
- `200` and `401` cases

- [ ] **Step 2: Update checkpoint**

Record:
- unread counter table introduced
- unread-count endpoint added
- create and first-read now maintain counters transactionally

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: add notification unread-count contract"
```

### Task 7: Full verification for Task 14

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
GET /api/v1/notifications/unread-count
```

Verify:
- `200`
- response includes `data.unread_count`
- count changes only once after first-time mark-read

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify notification unread-count task"
```

---

## 6. Acceptance Criteria

Task 14 is complete only when all are true:
- `GET /api/v1/notifications/unread-count` exists
- it returns `{ "data": { "unread_count": n } }`
- it returns `0` when the actor has no counter row
- it returns `401` for missing, invalid, or stale actor auth
- unread count comes from a dedicated counter table, not from loading inbox rows
- notification create, batch create, invitation live insert, and first-time mark-read maintain counters transactionally
- repeated mark-read does not double decrement the counter
- counter values never become negative
- service, repository, and HTTP tests cover the positive and negative cases above
- docs and checkpoint reflect the new endpoint and counter semantics

## 7. Risks And Guardrails

- Do not implement this endpoint as a wrapper around `GET /api/v1/notifications`.
- Do not scan the notifications table in application memory to compute unread count.
- Do not increment counters for rows skipped by insert conflict.
- Do not decrement counters on repeated mark-read.
- Do not let the counter go negative even under retries.
- Keep counter changes in the same transaction boundary as notification row changes.

## 8. Follow-On Tasks

This plan prepares for:
- Task 15 `POST /api/v1/notifications/read`
- later reconciliation work to rebuild counters from notification state if needed
