# Task 25 PUT Thread Notification Preference Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `PUT /api/v1/threads/{threadID}/notification-preference` so a thread member can set their per-thread notification mode to `all`, `mentions_only`, or `mute`.

**Architecture:** This task adds the write side for the sparse per-thread preference model introduced in Task 24. The transport layer accepts one `mode` field, the application layer reuses existing thread visibility rules, validates the requested mode, and then persists the new effective state. Non-default modes are stored in `thread_notification_preferences`; the default mode `all` is represented by deleting any existing row. The endpoint returns the effective mode plus an operation timestamp and remains idempotent in effect.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, SQL repositories, table-driven tests, PostgreSQL-backed repository tests

---

## 1. Scope

### In Scope
- Add one new endpoint:
  - `PUT /api/v1/threads/{threadID}/notification-preference`
- Reuse the storage foundation from Task 24
- Validate and persist one effective per-thread notification mode for the actor
- Keep `all` as the default effective mode
- Keep the table sparse by deleting rows when the requested mode is `all`
- Return the effective mode and operation timestamp
- Reuse existing thread visibility rules
- Add service, repository, handler, and HTTP tests
- Update API contract and checkpoint

### Out Of Scope
- No schema change beyond Task 24
- No batch preference update endpoint
- No workspace-wide preference APIs
- No change to comment or mention projector delivery rules yet
- No retroactive unread or inbox mutation
- No real-time push behavior
- No preference inheritance from workspace or page scope

### Prerequisites
- Task 24 exists:
  - `thread_notification_preferences` table
  - domain mode constants
  - GET endpoint and read repository/service support

---

## 2. Detailed Spec

## 2.1 Objective

Users need to control how they receive notifications for one thread without changing the global inbox design. This endpoint sets the actor's effective mode for the specified thread:
- `all`
- `mentions_only`
- `mute`

The design stays sparse:
- `mentions_only` and `mute` are stored as rows
- `all` is the default and is represented by deleting any existing row

This keeps reads simple and prevents the table from accumulating default-state rows.

## 2.2 Endpoint

### `PUT /api/v1/threads/{threadID}/notification-preference`

- Auth: yes
- Authorization: actor must be able to access the thread page as a current workspace member

## 2.3 Request Payload

Request JSON:

```json
{
  "mode": "mentions_only"
}
```

### Request Field Rules
- request body must be one JSON object
- `mode` is required
- `mode` must decode as a JSON string
- trim surrounding whitespace before validation

### Valid Modes
- `all`
- `mentions_only`
- `mute`

### Unknown Field Rule
- unknown fields are invalid
- malformed JSON, wrong JSON types, or trailing garbage return `400 invalid_json`

## 2.4 Response Payload

This codebase uses the standard success envelope:

```json
{
  "data": {
    "thread_id": "uuid",
    "mode": "mentions_only",
    "updated_at": "2026-04-04T10:00:00Z"
  }
}
```

### Response DTO

```json
{
  "thread_id": "uuid",
  "mode": "all|mentions_only|mute",
  "updated_at": "RFC3339 timestamp"
}
```

### Response Rules
- `thread_id` must always equal the requested thread id
- `mode` must always equal the effective mode after the request
- `updated_at` is the server-side operation commit time for this request
- response does not include `user_id`
- response does not include `created_at`

### `all` Response Rule
When the request sets `mode = all`:
- response still returns `mode = all`
- response returns a valid `updated_at`
- backend must not require a persisted row to produce this response

## 2.5 Validation Rules

### Authentication Rules
1. actor must be authenticated
2. missing, invalid, or expired auth returns `401 unauthorized`

### Payload Rules
1. request body must be valid JSON object
2. `mode` must be present
3. `mode` must be a string
4. trimmed `mode` must not be blank
5. trimmed `mode` must be one of:
   - `all`
   - `mentions_only`
   - `mute`

### Thread Visibility Rules
Use the same visibility model already used by current thread endpoints:
1. thread must exist
2. actor must be able to access the thread page through current workspace membership
3. trashed-page thread access remains hidden as `404 not_found`
4. non-member access remains hidden as `404 not_found`

### No `403` Rule In Current Scope
Although the roadmap listed `403` as a possible negative code, this task should stay consistent with the current thread resource-hiding behavior:
- same-workspace members of any role may update their own preference
- non-members and inaccessible threads return `404`
- no explicit `403` branch is expected in the current implementation

## 2.6 Persistence Rules

### Sparse Storage Rule
- `all` is the default effective mode
- storing `all` as a row is not allowed in this task
- requests for `all` delete any existing row for `(thread_id, user_id)`

### Non-Default Storage Rule
- `mentions_only` and `mute` must be stored in `thread_notification_preferences`
- one row per `(thread_id, user_id)`

### Upsert Rule
For non-default modes:
- if no row exists:
  - insert one row
- if a row exists:
  - update `mode`
  - update `updated_at`

### Delete Rule For `all`
For `mode = all`:
- if a row exists:
  - delete it
- if no row exists:
  - perform no write or a harmless no-op

### Operation Timestamp Rule
The service captures one `now := time.Now().UTC()` and uses it as:
- row `created_at` and `updated_at` for new rows
- row `updated_at` for updates
- response `updated_at` for all successful requests, including `mode = all`

## 2.7 Idempotency Rules

The endpoint is idempotent in effect.

### Repeated Non-Default Request
If the actor sends `PUT` with the same non-default mode repeatedly:
- effective mode remains the same
- endpoint still returns `200`
- row may be updated with a fresh `updated_at`

### Repeated Default Request
If the actor sends `PUT` with `mode = all` repeatedly:
- effective mode remains `all`
- endpoint still returns `200`
- no row should remain stored afterward

### No Conflict Rule
This endpoint should not return `409 conflict`.
- each actor controls only their own per-thread preference
- last-write-wins is acceptable

## 2.8 Repository Behavior

Extend the repository support introduced in Task 24.

Recommended method:

```go
SetThreadNotificationPreference(ctx context.Context, preference domain.ThreadNotificationPreference) error
```

or, if you prefer explicit operations:

```go
UpsertThreadNotificationPreference(ctx context.Context, preference domain.ThreadNotificationPreference) error
DeleteThreadNotificationPreference(ctx context.Context, threadID, userID string) error
```

### Repository Validation Rules
1. `thread_id` must not be blank
2. `user_id` must not be blank
3. `mode` must be one of:
   - `all`
   - `mentions_only`
   - `mute`
4. invalid internal input should return validation error, not rely on DB constraint text

### Recommended Persistence Strategy
- for `all`:
  - call delete path
- for `mentions_only` or `mute`:
  - use `INSERT ... ON CONFLICT (thread_id, user_id) DO UPDATE`

### Delete Path Rule
Deleting a missing row is valid and returns success.

## 2.9 Service Behavior

Add a new thread-service write method.

Recommended input:

```go
type UpdateThreadNotificationPreferenceInput struct {
    ThreadID string
    Mode     string
}
```

Recommended method:

```go
UpdateNotificationPreference(ctx context.Context, actorID string, input UpdateThreadNotificationPreferenceInput) (domain.ThreadNotificationPreferenceUpdateResult, error)
```

Recommended response type:

```go
type ThreadNotificationPreferenceUpdateResult struct {
    ThreadID   string
    Mode       ThreadNotificationMode
    UpdatedAt  time.Time
}
```

### Service Algorithm
1. validate thread access using existing thread visibility rules
2. trim and validate `mode`
3. capture one `now`
4. if mode is `all`:
   - delete any existing row
   - return `{ thread_id, mode: all, updated_at: now }`
5. if mode is `mentions_only` or `mute`:
   - upsert one row with `updated_at = now`
   - return `{ thread_id, mode, updated_at: now }`

### Visibility Rule
To stay consistent with current thread endpoints, reuse the same thread-access path as Task 24:
- `threadDetailWithMembership` or equivalent existing visibility helper

### Failure Classification
- malformed JSON:
  - `400 invalid_json`
- invalid mode semantics:
  - `422 validation_failed`
- thread not found or inaccessible:
  - `404 not_found`
- repository failure:
  - `500 internal_error`

## 2.10 Positive And Negative Cases

### Positive Cases

1. Request sets `mode = mentions_only` with no existing row
- result: `200`
- row inserted
- response mode is `mentions_only`

2. Request changes stored mode from `mentions_only` to `mute`
- result: `200`
- existing row updated
- response mode is `mute`

3. Request sets `mode = all` when a row exists
- result: `200`
- row deleted
- response mode is `all`

4. Request sets `mode = all` when no row exists
- result: `200`
- no row stored
- response mode is `all`

5. Viewer-level workspace member updates their own preference
- result: `200`

6. Repeated request with the same mode
- result: `200`
- effective state remains correct

### Negative Cases

1. Missing auth
- result: `401 unauthorized`

2. Invalid or expired auth
- result: `401 unauthorized`

3. Malformed JSON
- result: `400 invalid_json`

4. `mode` is missing
- result: `422 validation_failed`

5. `mode` is blank after trim
- result: `422 validation_failed`

6. `mode` is not one of the allowed values
- result: `422 validation_failed`

7. `mode` is wrong JSON type
- result: `400 invalid_json`

8. Unknown request field
- result: `400 invalid_json`

9. Thread does not exist
- result: `404 not_found`

10. Actor is not a member of the thread workspace
- result: `404 not_found`

11. Thread page is trashed or otherwise hidden by current visibility rules
- result: `404 not_found`

12. Repository write failure
- result: `500 internal_error`

---

## 3. File Structure And Responsibilities

### Modify
- `internal/domain/thread.go`
  - add update-result DTO if Task 24 did not already define a suitable public response type
- `internal/application/thread_notification_preferences.go`
  - add the write input DTO and update service method
- `internal/application/thread_notification_preferences_test.go`
  - add service tests for write behavior, defaults, and sparse-delete behavior
- `internal/application/thread_service.go`
  - extend thread preference repository interface if needed and wire shared helpers
- `internal/repository/postgres/thread_notification_preferences.go`
  - add upsert and delete behavior for sparse preference writes
- `internal/repository/postgres/thread_notification_preferences_test.go`
  - add repository integration coverage for create, update, delete, and idempotent writes
- `internal/repository/postgres/closed_pool_errors_test.go`
  - add closed-pool write coverage
- `internal/transport/http/handlers.go`
  - add `handlePutThreadNotificationPreference`
- `internal/transport/http/server.go`
  - register the new route
- `internal/transport/http/server_test.go`
  - add HTTP endpoint coverage
- `frontend-repo/API_CONTRACT.md`
  - document the new endpoint
- `docs/checkpoint.md`

### Files Explicitly Not In Scope
- `migrations/000024_thread_notification_preferences.up.sql`
- `migrations/000024_thread_notification_preferences.down.sql`
- `internal/application/comment_notification_projector.go`
- `internal/application/mention_notification_projector.go`
- `internal/repository/postgres/notification_repository.go`
- `cmd/api/app.go`

---

## 4. Test Matrix

## 4.1 Repository Integration Tests

Add focused tests in:
- `internal/repository/postgres/thread_notification_preferences_test.go`

### Positive Cases

1. set `mentions_only` inserts a new row

2. set `mute` updates an existing row

3. set `all` deletes an existing row

4. set `all` with no existing row succeeds and leaves no row

5. repeated set of the same non-default mode keeps one row and updates `updated_at`

### Negative Cases

6. blank `thread_id` returns validation error

7. blank `user_id` returns validation error

8. invalid internal mode returns validation error

9. closed-pool write returns error

## 4.2 Application Service Tests

Add focused tests in:
- `internal/application/thread_notification_preferences_test.go`

### Positive Cases

10. missing row plus `mode = mentions_only` creates effective `mentions_only`

11. existing row plus `mode = mute` updates effective mode to `mute`

12. existing row plus `mode = all` returns effective `all` and removes stored state

13. no existing row plus `mode = all` still returns effective `all`

14. viewer member can update preference successfully

### Negative Cases

15. missing thread returns `domain.ErrNotFound`

16. non-member thread access returns `domain.ErrNotFound`

17. blank mode returns `domain.ErrValidation`

18. invalid mode returns `domain.ErrValidation`

19. repository write failure propagates

## 4.3 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_test.go`

### Positive Cases

20. `PUT /api/v1/threads/{threadID}/notification-preference` with `mentions_only` returns `200`

21. same endpoint with `mute` returns `200`

22. same endpoint with `all` returns `200`

23. response uses the standard success envelope:
- `data.thread_id`
- `data.mode`
- `data.updated_at`

24. repeated request with the same mode remains `200`

### Negative Cases

25. missing auth returns `401`

26. malformed JSON returns `400`

27. wrong JSON type for `mode` returns `400`

28. unknown field returns `400`

29. missing mode returns `422`

30. blank mode returns `422`

31. invalid mode returns `422`

32. non-member access returns `404`

33. missing thread returns `404`

34. trashed-page thread access returns `404`

## 4.4 Documentation Tests

35. `frontend-repo/API_CONTRACT.md` documents:
- `PUT /api/v1/threads/{threadID}/notification-preference`
- request payload `{ "mode": "all|mentions_only|mute" }`
- response payload `{ "thread_id": "...", "mode": "...", "updated_at": "..." }`
- `all` is the default and clears stored preference state
- non-member access follows thread visibility and returns `404`

36. `docs/checkpoint.md` records:
- write endpoint added
- sparse preference model now supports set and reset
- `mode = all` deletes stored preference state

---

## 5. Execution Plan

### Task 1: Add failing repository write tests

**Files:**
- Modify: `internal/repository/postgres/thread_notification_preferences_test.go`
- Modify: `internal/repository/postgres/closed_pool_errors_test.go`

- [ ] **Step 1: Add failing repository tests for create, update, and delete semantics**

Cover:
- insert `mentions_only`
- update to `mute`
- delete on `all`
- no-op success for `all` without a row

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestThreadNotificationPreferenceRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- FAIL because write support does not exist yet

- [ ] **Step 3: Commit**

```bash
git add internal/repository/postgres/thread_notification_preferences_test.go internal/repository/postgres/closed_pool_errors_test.go
git commit -m "test: define thread notification preference write behavior"
```

### Task 2: Implement sparse repository write support

**Files:**
- Modify: `internal/repository/postgres/thread_notification_preferences.go`
- Modify: `internal/repository/postgres/thread_notification_preferences_test.go`
- Modify: `internal/repository/postgres/closed_pool_errors_test.go`

- [ ] **Step 1: Implement upsert and delete behavior**

Required behavior:
- non-default modes upsert one row
- `all` deletes any existing row
- invalid internal input returns validation error

- [ ] **Step 2: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestThreadNotificationPreferenceRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- PASS

- [ ] **Step 3: Commit**

```bash
git add internal/repository/postgres/thread_notification_preferences.go internal/repository/postgres/thread_notification_preferences_test.go internal/repository/postgres/closed_pool_errors_test.go
git commit -m "feat: add thread notification preference write repository"
```

### Task 3: Implement service write behavior

**Files:**
- Modify: `internal/domain/thread.go`
- Modify: `internal/application/thread_notification_preferences.go`
- Modify: `internal/application/thread_notification_preferences_test.go`
- Modify: `internal/application/thread_service.go`

- [ ] **Step 1: Add failing service tests**

Cover:
- non-default create
- non-default update
- reset to `all`
- blank mode
- invalid mode
- non-member hidden as `not_found`

- [ ] **Step 2: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "Test(Update|Get)ThreadNotificationPreference" -count=1
```

Expected:
- FAIL because the update service method does not exist yet

- [ ] **Step 3: Implement the service update method**

Required behavior:
- reuse thread visibility rules
- trim and validate mode
- use sparse storage semantics
- return `{ thread_id, mode, updated_at }`

- [ ] **Step 4: Re-run targeted application tests**

Run:
```powershell
go test ./internal/application -run "Test(Update|Get)ThreadNotificationPreference" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/thread.go internal/application/thread_notification_preferences.go internal/application/thread_notification_preferences_test.go internal/application/thread_service.go
git commit -m "feat: add thread notification preference write service"
```

### Task 4: Add the HTTP endpoint

**Files:**
- Modify: `internal/transport/http/handlers.go`
- Modify: `internal/transport/http/server.go`
- Modify: `internal/transport/http/server_test.go`

- [ ] **Step 1: Add failing HTTP tests**

Cover:
- `200` for each valid mode
- `400` malformed JSON
- `400` wrong type and unknown field
- `422` missing, blank, or invalid mode
- `404` non-member and missing thread

- [ ] **Step 2: Run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "TestThreadUpdateNotificationPreferenceEndpoint" -count=1
```

Expected:
- FAIL because the route and handler do not exist yet

- [ ] **Step 3: Implement the handler and route**

Required behavior:
- register `PUT /api/v1/threads/{threadID}/notification-preference`
- decode one `mode` field
- call the thread service
- return the standard success envelope

- [ ] **Step 4: Re-run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "TestThreadUpdateNotificationPreferenceEndpoint" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/transport/http/handlers.go internal/transport/http/server.go internal/transport/http/server_test.go
git commit -m "feat: add thread notification preference update endpoint"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update API contract**

Document:
- endpoint path
- request payload
- response envelope and payload
- sparse `all` semantics
- `400`, `401`, `404`, and `422` behavior

- [ ] **Step 2: Update checkpoint**

Record:
- write endpoint added
- default reset behavior uses row deletion for `all`
- endpoint is idempotent in effect

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: record thread notification preference update endpoint"
```

### Task 6: Full verification for Task 25

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "Test(Update|Get)ThreadNotificationPreference" -count=1
go test ./internal/repository/postgres -run "TestThreadNotificationPreferenceRepository|TestClosedPoolRepositories" -count=1
go test ./internal/transport/http -run "TestThread(Update)?NotificationPreferenceEndpoint" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual API sanity check if local server is available**

Call:
```http
PUT /api/v1/threads/{threadID}/notification-preference
Authorization: Bearer <token>
Content-Type: application/json
```

With:

```json
{
  "mode": "mute"
}
```

Then:

```json
{
  "mode": "all"
}
```

Verify:
- first request returns `200` with `mode = mute`
- second request returns `200` with `mode = all`
- subsequent GET returns `mode = all`
- no stored row remains after resetting to `all`

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify thread notification preference write task"
```

---

## 6. Acceptance Criteria

Task 25 is complete only when all are true:
- `PUT /api/v1/threads/{threadID}/notification-preference` exists
- request payload accepts exactly one valid mode
- response returns the standard success envelope with `{ thread_id, mode, updated_at }`
- `all` resets the preference to the default effective state
- resetting to `all` deletes stored preference state
- non-default modes persist through the preference table
- current thread visibility rules are preserved
- non-member and inaccessible thread access return `404`
- repository, service, and HTTP tests cover positive and negative cases
- docs and checkpoint reflect the new write endpoint

## 7. Risks And Guardrails

- Do not store explicit `all` rows in this task.
- Do not invent a new thread authorization model.
- Do not return `403` for non-members; preserve existing thread resource-hiding behavior.
- Do not return `409`; last-write-wins is acceptable here.
- Do not leak `user_id` in the response.
- Do not change the GET response shape introduced by Task 24.

## 8. Follow-On Tasks

This plan prepares for:
- later enforcement of `mentions_only` and `mute` during comment-notification recipient calculation
- later real-time inbox refresh behavior

Future delivery logic should read the effective mode from the same sparse model without changing the API contracts introduced in Tasks 24 and 25.
