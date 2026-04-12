# Task 15 POST Notifications Read Batch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `POST /api/v1/notifications/read` so the frontend can mark many notifications as read in one request and receive both the number of newly updated rows and the actor's current unread count.

**Architecture:** This task builds on the inbox v2 and unread-counter work from Tasks 13 and 14. The transport layer accepts one JSON object containing notification ids, the service validates the actor and request semantics, and the repository performs an all-or-nothing ownership-scoped batch update in one transaction. The repository decrements the unread counter only for first-time unread-to-read transitions, returns the count of rows actually changed, and then returns the actor's current unread count from the counter table.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, SQL-backed repositories, table-driven tests, PostgreSQL-backed repository tests

---

## 1. Scope

### In Scope
- Add one new endpoint:
  - `POST /api/v1/notifications/read`
- Accept a bounded batch of notification ids
- Mark many notifications as read in one request
- Keep batch mark-read idempotent
- Return:
  - `updated_count`
  - `unread_count`
- Use the unread counter table from Task 14
- Enforce own-notification scope for every requested id
- Make the batch update atomic:
  - all ids valid and owned => update allowed
  - any missing or foreign id => no updates and `404`
- Add service, repository, handler, and HTTP tests
- Update API docs and checkpoint

### Out Of Scope
- No per-item response payload
- No partial-success response
- No per-type unread breakdown
- No SSE or WebSocket push
- No notification preference logic
- No inbox list changes

### Prerequisite
- Task 14 unread counter table and maintenance behavior must exist before implementing this task

---

## 2. Detailed Spec

## 2.1 Objective

Inbox UX should not require one request per notification. The frontend needs one endpoint that can mark a page of notifications as read and immediately get the refreshed unread badge value.

This endpoint must stay predictable:
- valid batch => one atomic update
- repeated batch => `200` with `updated_count = 0` when everything was already read
- mixed valid and foreign or missing ids => `404` with no partial updates

## 2.2 Endpoint

### `POST /api/v1/notifications/read`

- Auth: yes
- Authorization: every requested notification must belong to the authenticated actor

## 2.3 Request Payload

Request JSON:

```json
{
  "notification_ids": [
    "uuid-1",
    "uuid-2",
    "uuid-3"
  ]
}
```

### Fields
- `notification_ids`
  - required
  - array of UUID strings
  - minimum length `1`
  - maximum length `100`

### Request Shape Rules
- request body must contain exactly one JSON object
- unknown fields are invalid
- empty body is invalid
- `notification_ids` must be present
- every id must be a non-empty UUID string
- duplicate ids are invalid

## 2.4 Response Payload

This codebase wraps successful responses in the standard success envelope:

```json
{
  "data": {
    "updated_count": 3,
    "unread_count": 9
  }
}
```

### Response Rules
- `updated_count` is the number of notifications that changed from unread to read in this request
- `updated_count` is not the number of ids submitted
- `unread_count` is the actor's current total unread count after the transaction commits
- repeated requests against already-read rows return:
  - `200`
  - `updated_count = 0`
  - current `unread_count`
- response does not include per-item notification rows

## 2.5 Validation Rules

1. Actor must be authenticated
2. Actor id from auth context must resolve to an existing user record
3. request body must be valid JSON
4. `notification_ids` must be present
5. `notification_ids` must contain at least one id
6. `notification_ids` must contain at most `100` ids
7. every id must be a valid UUID string
8. duplicate ids are not allowed
9. every requested notification must exist and belong to the authenticated actor

## 2.6 Behavior Rules

### Actor Validation
- resolve actor with `users.GetByID`
- if actor id does not resolve:
  - return `401 unauthorized`

### Batch Validation Rule
- validate the entire request before any repository update
- semantic input validation errors return `422 validation_failed`

Examples:
- missing `notification_ids`
- empty array
- oversized array
- invalid UUID string
- duplicate ids

### Ownership And Existence Rule
- the repository must verify that all requested ids belong to the actor before performing any update
- if any id is missing or belongs to another user:
  - return `404 not_found`
  - update nothing

This keeps the endpoint non-disclosing and atomic.

### Atomicity Rule
- the repository must treat the request as all-or-nothing
- if the request passes validation and ownership checks:
  - update all eligible unread rows in one transaction
- if ownership count does not match requested id count:
  - abort with `404`
  - leave all rows unchanged

### Idempotency Rule
- unread rows transition once:
  - `is_read = TRUE`
  - `read_at = now`
  - `updated_at = now`
- already-read rows remain unchanged
- repeated calls with the same valid ids are allowed and return `200`

### Updated Count Rule
- `updated_count` counts only first-time unread-to-read transitions
- already-read rows contribute `0`
- duplicates never reach this stage because they are validation errors

### Unread Count Rule
- `unread_count` comes from the unread counter table after the batch update
- decrement the counter only by the number of rows that actually transitioned to read
- counter must never become negative

### Performance Rule
- do not loop one SQL update per id
- do not load all candidate rows into application memory and update them one-by-one
- use one transaction and set-based SQL operations
- max batch size `100` keeps statement size bounded and predictable

## 2.7 Positive And Negative Cases

### Positive Cases

1. Authenticated user marks three unread notifications as read
- Result: `200`
- `updated_count = 3`
- `unread_count` decreases by `3`

2. Authenticated user marks a mix of unread and already-read notifications
- Result: `200`
- `updated_count` equals only the unread subset
- `unread_count` decreases only by that subset

3. Authenticated user repeats the same successful request
- Result: `200`
- `updated_count = 0`
- `unread_count` stays unchanged

4. Authenticated user marks a single valid notification through the batch endpoint
- Result: `200`
- works the same as the single mark-read endpoint, but returns counts only

### Negative Cases

1. Missing auth token
- Result: `401 unauthorized`

2. Invalid auth token
- Result: `401 unauthorized`

3. Authenticated actor id no longer resolves to a user
- Result: `401 unauthorized`

4. Malformed JSON
- Result: `400 invalid_json`

5. Missing `notification_ids`
- Result: `422 validation_failed`

6. Empty `notification_ids`
- Result: `422 validation_failed`

7. More than `100` ids
- Result: `422 validation_failed`

8. Invalid UUID in `notification_ids`
- Result: `422 validation_failed`

9. Duplicate id in `notification_ids`
- Result: `422 validation_failed`

10. Any requested notification does not exist
- Result: `404 not_found`
- no rows are updated

11. Any requested notification belongs to another user
- Result: `404 not_found`
- no rows are updated

---

## 3. File Structure And Responsibilities

### Modify
- `internal/domain/notification.go`
  - add batch-read response DTO if it does not already exist
- `internal/application/notification_service.go`
  - add batch-read service method and validation
- `internal/application/notification_service_test.go`
- `internal/application/notification_service_additional_test.go`
- `internal/repository/postgres/notification_repository.go`
  - add set-based batch mark-read method with ownership verification and unread counter update
- `internal/repository/postgres/content_repository_test.go`
  - add integration coverage for batch behavior
- `internal/transport/http/handlers.go`
  - add batch-read request parsing and response
- `internal/transport/http/server.go`
  - register the new route
- `internal/transport/http/server_test.go`
  - add success and ownership tests
- `internal/transport/http/server_invalid_json_test.go`
  - add malformed JSON coverage for the new endpoint
- `frontend-repo/API_CONTRACT.md`
  - document the new endpoint
- `docs/checkpoint.md`

### Files Explicitly Not In Scope
- `internal/application/workspace_service.go`
- `internal/application/thread_service.go`
- `internal/transport/http/response.go`
  - success envelope stays unchanged

---

## 4. Test Matrix

## 4.1 Application Service Tests

Add or update tests in:
- `internal/application/notification_service_test.go`
- `internal/application/notification_service_additional_test.go`

### Positive Cases

1. Service marks a valid batch and returns counts
- Expect:
  - actor is validated through `users.GetByID`
  - repository receives the same ordered ids or normalized ids as defined by implementation
  - returned result has `updated_count` and `unread_count`

2. Service returns `updated_count = 0` for repeated valid reads

### Negative Cases

3. Unknown actor returns `domain.ErrUnauthorized`

4. Missing `notification_ids` returns `domain.ErrValidation`

5. Empty `notification_ids` returns `domain.ErrValidation`

6. Oversized `notification_ids` returns `domain.ErrValidation`

7. Invalid UUID returns `domain.ErrValidation`

8. Duplicate ids return `domain.ErrValidation`

9. Repository not-found error propagates

10. Repository failure propagates

## 4.2 Repository Integration Tests

Add DB-backed tests in:
- `internal/repository/postgres/content_repository_test.go`

### Positive Cases

11. Batch mark-read updates all unread rows in the request

12. Batch mark-read updates only unread rows when some requested rows are already read

13. Batch mark-read returns `updated_count = 0` when all requested rows are already read

14. Batch mark-read decrements the unread counter by exactly the number of unread rows changed

15. Batch mark-read returns the post-update unread counter value

16. Batch mark-read with one valid id works as a normal single-item batch

### Negative Cases

17. Batch with one missing notification returns `domain.ErrNotFound` and updates nothing

18. Batch with one other-user notification returns `domain.ErrNotFound` and updates nothing

19. Failed ownership verification leaves all candidate rows unchanged

20. Closed pool or query failure returns wrapped repository error

## 4.3 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_test.go`
- `internal/transport/http/server_invalid_json_test.go`

### Positive Cases

21. `POST /api/v1/notifications/read` returns success envelope and counts
- Assert:
  - top-level `data`
  - `data.updated_count`
  - `data.unread_count`

22. Repeated successful request returns `200` with `updated_count = 0`

23. Mixed unread and already-read ids returns `200` with partial updated count but full request success

### Negative Cases

24. Missing auth returns `401`

25. Unknown actor returns `401`

26. Malformed JSON returns `400`

27. Empty array returns `422`

28. Oversized array returns `422`

29. Invalid UUID returns `422`

30. Duplicate id returns `422`

31. Missing notification returns `404`

32. Other-user notification returns `404`

## 4.4 Documentation Tests

33. `frontend-repo/API_CONTRACT.md` documents:
- endpoint path
- request payload
- success envelope
- `updated_count` semantics
- `unread_count` semantics
- `400`, `401`, `404`, and `422` cases

34. `docs/checkpoint.md` records:
- batch mark-read endpoint added
- all-or-nothing ownership behavior
- idempotent repeated-read semantics

---

## 5. Execution Plan

### Task 1: Define failing service tests for batch mark-read behavior

**Files:**
- Modify: `internal/application/notification_service_test.go`
- Modify: `internal/application/notification_service_additional_test.go`

- [ ] **Step 1: Add failing service tests for successful batch mark-read**

Cover:
- valid ids
- updated count
- unread count
- repeated request behavior

- [ ] **Step 2: Add failing service tests for validation and not-found cases**

Cover:
- missing array
- empty array
- oversized array
- invalid UUID
- duplicate id
- unknown actor
- repository not found

- [ ] **Step 3: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestNotificationService" -count=1
```

Expected:
- FAIL because batch mark-read service contract does not exist yet

- [ ] **Step 4: Commit**

```bash
git add internal/application/notification_service_test.go internal/application/notification_service_additional_test.go
git commit -m "test: define notification batch mark-read service behavior"
```

### Task 2: Implement service batch mark-read contract

**Files:**
- Modify: `internal/domain/notification.go`
- Modify: `internal/application/notification_service.go`

- [ ] **Step 1: Add or confirm batch-read DTOs**

Recommended shape:

```go
type BatchMarkNotificationsReadInput struct {
    NotificationIDs []string
}

type NotificationBatchReadResult struct {
    UpdatedCount int64 `json:"updated_count"`
    UnreadCount  int64 `json:"unread_count"`
}
```

- [ ] **Step 2: Implement service validation**

Required behavior:
- resolve actor with `users.GetByID`
- require `1..100` ids
- reject duplicate ids
- validate every id as UUID
- call repository only after the full request validates

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
git commit -m "feat: add notification batch mark-read service"
```

### Task 3: Add repository batch mark-read method and integration coverage

**Files:**
- Modify: `internal/repository/postgres/notification_repository.go`
- Modify: `internal/repository/postgres/content_repository_test.go`

- [ ] **Step 1: Add failing repository integration tests for batch mark-read**

Cover:
- full unread batch
- mixed unread and already-read rows
- repeated batch
- not-found atomic failure
- foreign-id atomic failure
- unread counter decrement

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositoryErrors" -count=1
```

Expected:
- FAIL because batch mark-read repository logic does not exist yet

- [ ] **Step 3: Implement repository batch method**

Recommended signature:

```go
BatchMarkRead(ctx context.Context, userID string, notificationIDs []string, readAt time.Time) (domain.NotificationBatchReadResult, error)
```

Required behavior:
- verify every requested id belongs to the actor
- abort with `domain.ErrNotFound` if matched row count differs from requested id count
- update unread rows only
- decrement unread counter by actual updated row count
- return final unread count from the counter table
- keep the whole operation in one transaction

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
git commit -m "feat: add notification batch mark-read query"
```

### Task 4: Add handler, route, and HTTP tests

**Files:**
- Modify: `internal/transport/http/handlers.go`
- Modify: `internal/transport/http/server.go`
- Modify: `internal/transport/http/server_test.go`
- Modify: `internal/transport/http/server_invalid_json_test.go`

- [ ] **Step 1: Add failing HTTP tests for the new endpoint**

Cover:
- success response
- repeated request
- empty array
- invalid UUID
- foreign id
- missing id

- [ ] **Step 2: Add request DTO and handler**

Recommended request type:

```go
type batchMarkNotificationsReadRequest struct {
    NotificationIDs []string `json:"notification_ids"`
}
```

Handler requirements:
- decode JSON with existing strict decoder
- map malformed JSON to `400 invalid_json`
- call service with parsed ids
- map validation, unauthorized, and not-found errors through the existing error mapper
- return success envelope with counts

- [ ] **Step 3: Register the route**

Route:

```go
r.Post("/notifications/read", s.handleBatchMarkNotificationsRead())
```

- [ ] **Step 4: Re-run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "TestNotificationEndpoints|TestHandlersInvalidJSONBranches" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/transport/http/handlers.go internal/transport/http/server.go internal/transport/http/server_test.go internal/transport/http/server_invalid_json_test.go
git commit -m "feat: add notification batch mark-read endpoint"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update the API contract**

Document:
- endpoint path
- request payload
- success envelope
- all-or-nothing semantics
- idempotent repeated-read behavior
- `400`, `401`, `404`, and `422` cases

- [ ] **Step 2: Update checkpoint**

Record:
- batch mark-read endpoint added
- unread counter now supports batch decrement
- any missing or foreign id fails the whole batch

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: add notification batch mark-read contract"
```

### Task 6: Full verification for Task 15

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "TestNotificationService" -count=1
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositoryErrors" -count=1
go test ./internal/transport/http -run "TestNotificationEndpoints|TestHandlersInvalidJSONBranches" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual API sanity check if local server is available**

Call:
```http
POST /api/v1/notifications/read
Content-Type: application/json

{ "notification_ids": ["uuid-1", "uuid-2"] }
```

Verify:
- `200`
- response includes `data.updated_count`
- response includes `data.unread_count`
- repeating the same request returns `updated_count = 0`

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify notification batch mark-read task"
```

---

## 6. Acceptance Criteria

Task 15 is complete only when all are true:
- `POST /api/v1/notifications/read` exists
- it accepts `{ "notification_ids": [...] }`
- it returns `{ "data": { "updated_count": n, "unread_count": m } }`
- it enforces `1..100` unique UUID ids
- malformed JSON returns `400`
- semantic input errors return `422`
- any missing or foreign id returns `404` and updates nothing
- repeated successful requests are idempotent and return `updated_count = 0` when nothing new changed
- unread counter decrements only by actual unread-to-read transitions
- service, repository, and HTTP tests cover the positive and negative cases above
- docs and checkpoint reflect the new endpoint and semantics

## 7. Risks And Guardrails

- Do not implement partial success for mixed valid and invalid ids.
- Do not decrement unread count for already-read rows.
- Do not perform one update per id.
- Do not leak whether a foreign notification exists.
- Do not allow duplicate ids to silently skew `updated_count`.
- Keep the whole repository operation transactional.

## 8. Follow-On Tasks

This plan prepares for:
- later real-time unread badge refresh
- later reconciliation work if unread counters ever need rebuild support
