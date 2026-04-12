# Task 24 GET Thread Notification Preference Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `GET /api/v1/threads/{threadID}/notification-preference` so a thread member can read their effective per-thread notification mode.

**Architecture:** This task adds the read side of per-thread notification preferences. It introduces a small thread-member preference table that Task 25 will later write to, but Task 24 itself is read-only at the API level. The application layer reuses existing thread visibility rules to ensure the actor can access the thread, then reads the actor's stored preference row if one exists and otherwise returns the default mode `all`. The endpoint uses the standard success envelope and does not create rows on read.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, SQL repositories, table-driven tests, PostgreSQL-backed repository tests

---

## 1. Scope

### In Scope
- Add one new endpoint:
  - `GET /api/v1/threads/{threadID}/notification-preference`
- Add the storage foundation for thread notification preferences
- Return the actor's effective notification mode for one thread
- Use `all` as the default mode when no stored preference row exists
- Reuse existing thread visibility rules
- Add migration, service, repository, handler, and HTTP tests
- Update API contract and checkpoint

### Out Of Scope
- No write endpoint yet
- No change to comment or mention projection logic
- No thread-level mute enforcement yet
- No batch preference APIs
- No workspace-wide notification preference APIs
- No real-time push behavior
- No UI grouping or delivery-precedence changes

### Prerequisites
- Thread endpoints from the threaded discussion feature exist
- Task 12 inbox API work may exist, but this task does not depend on inbox internals
- Task 25 will later build the write side on the same table

---

## 2. Detailed Spec

## 2.1 Objective

Users need a stable read contract for thread-level notification controls before the write endpoint is added. This endpoint returns the actor's effective mode for one thread:
- `all`
- `mentions_only`
- `mute`

If the actor has never stored a preference for that thread, the backend must return:
- `mode = all`

This task adds the read path and the schema that the later write task will use. It does not write or update preference rows.

## 2.2 Endpoint

### `GET /api/v1/threads/{threadID}/notification-preference`

- Auth: yes
- Authorization: actor must be able to access the thread page as a current workspace member

No request body is accepted.

## 2.3 Request Payload

There is no request payload.

### Path Rules
- `threadID` comes from the route path
- blank or unknown thread ids are handled through the existing thread lookup path
- this codebase does not require transport-level UUID parsing for thread ids

### Query Rules
- no query parameters are supported in this task
- unknown query parameters are ignored

## 2.4 Response Payload

This codebase uses the standard success envelope:

```json
{
  "data": {
    "thread_id": "uuid",
    "mode": "all"
  }
}
```

### Response DTO

```json
{
  "thread_id": "uuid",
  "mode": "all|mentions_only|mute"
}
```

### Response Rules
- `thread_id` must always equal the requested thread id
- `mode` must always be one of:
  - `all`
  - `mentions_only`
  - `mute`
- if no stored preference exists:
  - return `mode = all`
- the response does not include `user_id`
- the response does not include `created_at` or `updated_at` in this task

## 2.5 Validation Rules

### Authentication Rules
1. actor must be authenticated
2. missing, invalid, or expired auth returns `401 unauthorized`

### Thread Visibility Rules
Use the same visibility model already used by thread detail and reply endpoints:
1. thread must exist
2. actor must be able to access the thread page through current workspace membership
3. trashed-page thread access remains hidden as `404 not_found`
4. non-member access remains hidden as `404 not_found`

### Stored Preference Rules
1. if a preference row exists, `mode` must be one of:
   - `all`
   - `mentions_only`
   - `mute`
2. if no preference row exists, the service must return effective mode `all`
3. GET must not insert a default row into the database

### No `403` Rule In Current Scope
Although the roadmap listed `403` as a possible negative code, this task should follow the current thread resource-hiding behavior:
- same-workspace members of any role may read this endpoint
- non-members and inaccessible threads return `404`
- no explicit `403` branch is expected in the current implementation

## 2.6 Data Model

This task introduces one new internal table for future read and write support.

### `thread_notification_preferences`

Columns:
- `thread_id UUID NOT NULL REFERENCES page_comment_threads(id) ON DELETE CASCADE`
- `user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE`
- `mode TEXT NOT NULL`
- `created_at TIMESTAMPTZ NOT NULL`
- `updated_at TIMESTAMPTZ NOT NULL`

Constraints:
- primary key on `(thread_id, user_id)`
- check constraint on `mode`:
  - `mode IN ('all', 'mentions_only', 'mute')`

Indexes:
- primary key is sufficient for this task
- no extra index is required yet

### Migration File Names
- `migrations/000024_thread_notification_preferences.up.sql`
- `migrations/000024_thread_notification_preferences.down.sql`

If the actual migration sequence changed before implementation, keep the pattern and use the next available number.

### Read Rule
The table is sparse:
- only users who explicitly change their mode later will need a row
- GET must treat row absence as default `all`

## 2.7 Domain Model

Add these domain concepts in `internal/domain/thread.go`:

- `type ThreadNotificationMode string`
- constants:
  - `ThreadNotificationModeAll`
  - `ThreadNotificationModeMentionsOnly`
  - `ThreadNotificationModeMute`
- `type ThreadNotificationPreference struct`
  - `ThreadID string`
  - `UserID string`
  - `Mode ThreadNotificationMode`
  - `CreatedAt time.Time`
  - `UpdatedAt time.Time`
- `type ThreadNotificationPreferenceView struct`
  - `ThreadID string`
  - `Mode ThreadNotificationMode`

### Default Mode Rule
Use:
- `ThreadNotificationModeAll`

as the canonical default returned by the service when no row exists.

## 2.8 Repository Behavior

Add read support for one actor and one thread.

Recommended repository method:

```go
GetThreadNotificationPreference(ctx context.Context, threadID, userID string) (*domain.ThreadNotificationPreference, error)
```

### Repository Rules
1. return `(*Preference, nil)` when a row exists
2. return `(nil, nil)` when no row exists
3. do not synthesize a default row in the repository
4. closed-pool behavior returns an error

### Service Ownership Rule
The service, not the repository, owns the effective-default rule:
- `nil` preference from the repository becomes `mode = all`

## 2.9 Service Behavior

Add a new thread-service read method.

Recommended input:

```go
type GetThreadNotificationPreferenceInput struct {
    ThreadID string
}
```

Recommended method:

```go
GetNotificationPreference(ctx context.Context, actorID string, input GetThreadNotificationPreferenceInput) (domain.ThreadNotificationPreferenceView, error)
```

### Service Algorithm
1. validate access to the thread using existing thread visibility rules
2. read the actor's stored preference row for that thread
3. if a row exists:
   - return its mode
4. if no row exists:
   - return `mode = all`

### Visibility Rule
To stay consistent with current thread endpoints, the service should reuse the existing thread-access path instead of inventing new authorization semantics.

Recommended approach:
- reuse `threadDetailWithMembership` or an equivalent existing visibility helper

This may load more thread data than the endpoint strictly needs, but it keeps the first implementation consistent and simple. A future optimization can introduce a lighter metadata reader if profiling shows this path is hot.

### Failure Classification
- thread not found or inaccessible thread page:
  - `404 not_found`
- preference repository read failure:
  - `500 internal_error`

## 2.10 Positive And Negative Cases

### Positive Cases

1. Stored preference row exists with `mode = mentions_only`
- result: `200`
- response mode is `mentions_only`

2. Stored preference row exists with `mode = mute`
- result: `200`
- response mode is `mute`

3. No stored preference row exists
- result: `200`
- response mode is `all`

4. Viewer-level workspace member requests the endpoint
- result: `200`
- viewers are allowed to read preference state

### Negative Cases

1. Missing auth
- result: `401 unauthorized`

2. Invalid or expired auth
- result: `401 unauthorized`

3. Thread does not exist
- result: `404 not_found`

4. Actor is not a member of the thread workspace
- result: `404 not_found`

5. Thread page is trashed or otherwise hidden by current visibility rules
- result: `404 not_found`

6. Repository read failure
- result: `500 internal_error`

---

## 3. File Structure And Responsibilities

### Create
- `migrations/000024_thread_notification_preferences.up.sql`
  - create sparse preference table and constraints
- `migrations/000024_thread_notification_preferences.down.sql`
  - drop the preference table
- `internal/application/thread_notification_preferences.go`
  - input DTO and read-only service method for thread notification preferences
- `internal/application/thread_notification_preferences_test.go`
  - focused service tests for defaulting and visibility behavior
- `internal/repository/postgres/thread_notification_preferences.go`
  - PostgreSQL read method for one thread and one user
- `internal/repository/postgres/thread_notification_preferences_test.go`
  - focused repository integration tests for preference reads

### Modify
- `internal/domain/thread.go`
  - add notification mode and preference types
- `internal/application/thread_service.go`
  - extend the thread repository interface if needed and wire shared helpers for the new service file
- `internal/repository/postgres/integration_test.go`
  - include the new table in test database reset
- `internal/repository/postgres/closed_pool_errors_test.go`
  - add closed-pool read coverage
- `internal/transport/http/handlers.go`
  - add `handleGetThreadNotificationPreference`
- `internal/transport/http/server.go`
  - register the new route
- `internal/transport/http/server_test.go`
  - add HTTP endpoint coverage
- `frontend-repo/API_CONTRACT.md`
  - document the new endpoint
- `docs/checkpoint.md`

### Files Explicitly Not In Scope
- `internal/application/comment_notification_projector.go`
- `internal/application/mention_notification_projector.go`
- `internal/repository/postgres/notification_repository.go`
- `internal/application/notification_service.go`
- `cmd/api/app.go`

---

## 4. Test Matrix

## 4.1 Migration And Schema Tests

Add DB-backed coverage to prove:

1. migration creates `thread_notification_preferences`

2. primary key prevents duplicate rows for the same `(thread_id, user_id)`

3. `mode` check constraint rejects invalid values

4. deleting a thread cascades preference rows

5. deleting a user cascades preference rows

## 4.2 Repository Integration Tests

Add focused tests in:
- `internal/repository/postgres/thread_notification_preferences_test.go`

### Positive Cases

6. existing row with `mode = all` is returned

7. existing row with `mode = mentions_only` is returned

8. existing row with `mode = mute` is returned

9. missing row returns `nil, nil`

### Negative Cases

10. closed pool read returns error

## 4.3 Application Service Tests

Add focused tests in:
- `internal/application/thread_notification_preferences_test.go`

### Positive Cases

11. stored `mentions_only` preference returns `mentions_only`

12. stored `mute` preference returns `mute`

13. missing preference row returns default `all`

14. viewer member can read preference successfully

### Negative Cases

15. missing thread returns `domain.ErrNotFound`

16. non-member thread access returns `domain.ErrNotFound`

17. preference repository failure propagates

## 4.4 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_test.go`

### Positive Cases

18. `GET /api/v1/threads/{threadID}/notification-preference` returns `200` with stored mode

19. same endpoint returns `200` with `mode = all` when no row exists

20. response uses the standard success envelope:
- `data.thread_id`
- `data.mode`

### Negative Cases

21. missing auth returns `401`

22. non-member access returns `404`

23. missing thread returns `404`

24. trashed-page thread access returns `404`

## 4.5 Documentation Tests

25. `frontend-repo/API_CONTRACT.md` documents:
- `GET /api/v1/threads/{threadID}/notification-preference`
- no request body
- response payload `{ "thread_id": "...", "mode": "all|mentions_only|mute" }`
- default mode is `all`
- non-member access follows thread visibility and returns `404`

26. `docs/checkpoint.md` records:
- sparse preference table added
- read endpoint added
- default `all` behavior when no row exists
- no write endpoint yet

---

## 5. Execution Plan

### Task 1: Add failing migration and repository tests

**Files:**
- Create: `migrations/000024_thread_notification_preferences.up.sql`
- Create: `migrations/000024_thread_notification_preferences.down.sql`
- Create: `internal/repository/postgres/thread_notification_preferences_test.go`
- Modify: `internal/repository/postgres/integration_test.go`

- [ ] **Step 1: Write failing migration and repository tests**

Cover:
- table creation
- valid row reads
- missing row returns `nil`
- invalid mode rejected by constraint
- test reset includes the new table

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestThreadNotificationPreferenceRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- FAIL because the schema and repository do not exist yet

- [ ] **Step 3: Commit**

```bash
git add migrations/000024_thread_notification_preferences.up.sql migrations/000024_thread_notification_preferences.down.sql internal/repository/postgres/thread_notification_preferences_test.go internal/repository/postgres/integration_test.go
git commit -m "test: define thread notification preference read schema"
```

### Task 2: Implement schema, domain, and repository read path

**Files:**
- Modify: `internal/domain/thread.go`
- Create: `internal/repository/postgres/thread_notification_preferences.go`
- Modify: `internal/repository/postgres/thread_notification_preferences_test.go`
- Modify: `internal/repository/postgres/integration_test.go`
- Modify: `internal/repository/postgres/closed_pool_errors_test.go`

- [ ] **Step 1: Add the migration**

Required behavior:
- create sparse preference table
- enforce valid `mode`
- cascade on thread and user deletion

- [ ] **Step 2: Add domain mode and preference types**

Required behavior:
- define canonical mode constants
- define stored row and response view types

- [ ] **Step 3: Implement repository read support**

Required behavior:
- read one row by `(thread_id, user_id)`
- return `nil, nil` when absent
- no default row creation

- [ ] **Step 4: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestThreadNotificationPreferenceRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add migrations/000024_thread_notification_preferences.up.sql migrations/000024_thread_notification_preferences.down.sql internal/domain/thread.go internal/repository/postgres/thread_notification_preferences.go internal/repository/postgres/thread_notification_preferences_test.go internal/repository/postgres/integration_test.go internal/repository/postgres/closed_pool_errors_test.go
git commit -m "feat: add thread notification preference read storage"
```

### Task 3: Implement service read behavior

**Files:**
- Create: `internal/application/thread_notification_preferences.go`
- Create: `internal/application/thread_notification_preferences_test.go`
- Modify: `internal/application/thread_service.go`

- [ ] **Step 1: Add failing service tests**

Cover:
- stored mode returned
- missing row defaults to `all`
- viewer allowed
- non-member hidden as `not_found`

- [ ] **Step 2: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestGetThreadNotificationPreference" -count=1
```

Expected:
- FAIL because the service read method does not exist yet

- [ ] **Step 3: Implement the service method**

Required behavior:
- reuse existing thread visibility rules
- fetch stored preference if present
- default to `all` when absent

- [ ] **Step 4: Re-run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestGetThreadNotificationPreference" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/thread_notification_preferences.go internal/application/thread_notification_preferences_test.go internal/application/thread_service.go
git commit -m "feat: add thread notification preference read service"
```

### Task 4: Add the HTTP endpoint

**Files:**
- Modify: `internal/transport/http/handlers.go`
- Modify: `internal/transport/http/server.go`
- Modify: `internal/transport/http/server_test.go`

- [ ] **Step 1: Add failing HTTP tests**

Cover:
- `200` with stored mode
- `200` with default `all`
- `401` without auth
- `404` for non-member
- `404` for missing thread

- [ ] **Step 2: Run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "TestThreadNotificationPreferenceEndpoint" -count=1
```

Expected:
- FAIL because the route and handler do not exist yet

- [ ] **Step 3: Implement the handler and route**

Required behavior:
- register `GET /api/v1/threads/{threadID}/notification-preference`
- call the thread service
- return the standard success envelope

- [ ] **Step 4: Re-run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "TestThreadNotificationPreferenceEndpoint" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/transport/http/handlers.go internal/transport/http/server.go internal/transport/http/server_test.go
git commit -m "feat: add thread notification preference endpoint"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update API contract**

Document:
- endpoint path
- response envelope and payload
- default `all` rule
- `401` and `404` behavior
- no request body

- [ ] **Step 2: Update checkpoint**

Record:
- read endpoint added
- sparse preference table added
- GET defaults to `all` without creating rows

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: record thread notification preference read endpoint"
```

### Task 6: Full verification for Task 24

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "TestGetThreadNotificationPreference" -count=1
go test ./internal/repository/postgres -run "TestThreadNotificationPreferenceRepository|TestClosedPoolRepositories" -count=1
go test ./internal/transport/http -run "TestThreadNotificationPreferenceEndpoint" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual API sanity check if local server is available**

Call:
```http
GET /api/v1/threads/{threadID}/notification-preference
Authorization: Bearer <token>
```

Verify:
- `200`
- `data.thread_id` matches the path
- `data.mode` is `all` when no row exists
- existing stored row returns its exact mode

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify thread notification preference read task"
```

---

## 6. Acceptance Criteria

Task 24 is complete only when all are true:
- `GET /api/v1/threads/{threadID}/notification-preference` exists
- it returns the standard success envelope with `{ thread_id, mode }`
- `mode` is always one of `all|mentions_only|mute`
- missing stored row returns effective mode `all`
- GET does not create a row when none exists
- current thread visibility rules are preserved
- non-member and inaccessible thread access return `404`
- repository and HTTP tests cover positive and negative cases
- docs and checkpoint reflect the new read endpoint

## 7. Risks And Guardrails

- Do not create or update preference rows in this GET task.
- Do not invent a new thread authorization model.
- Do not return `403` for non-members; preserve existing thread resource-hiding behavior.
- Do not leak `user_id` in the response.
- Do not add `updated_at` to the public GET response yet.
- Keep `all` as the only default mode when no row exists.

## 8. Follow-On Tasks

This plan prepares for:
- Task 25 `PUT /api/v1/threads/{threadID}/notification-preference`

Task 25 should reuse the same table and domain mode constants, and it should not change the GET response shape introduced here.
