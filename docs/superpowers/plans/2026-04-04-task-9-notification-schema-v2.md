# Task 9 Notification Schema V2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce the notification inbox schema foundation so notifications can later support actor metadata, title/content, read status, invitation action state, resource linkage, and future notification types, while keeping the current notification endpoints working without a breaking API change.

**Architecture:** This task is a compatibility-first schema migration. It enriches `notifications` with v2 inbox fields, backfills existing rows safely, preserves the current `event_id` and `message` columns for compatibility, and updates repository/service writes so new notifications populate both the old and new schema. Public notification API shape should stay unchanged in this task; the new fields are internal foundation for later inbox, unread-count, and live invitation notification tasks.

**Tech Stack:** Go, PostgreSQL, `pgx`, SQL migrations, repository integration tests, Go unit tests, HTTP regression tests

---

## 1. Scope

### In Scope
- Add notification v2 columns to `notifications`
- Expand notification type support to include `mention`
- Backfill existing notification rows safely
- Keep the current endpoints functioning:
  - `GET /api/v1/notifications`
  - `POST /api/v1/notifications/{notificationID}/read`
- Update notification repository SQL to read and write the new schema
- Update notification service writes so current invitation/comment/thread notifications populate the new internal fields
- Add tests for migration-compatible behavior and repository correctness

### Out Of Scope
- No new notification endpoints
- No unread-count endpoint
- No batch mark-read endpoint
- No cursor pagination changes
- No notification filtering changes
- No outbox or projector implementation
- No thread-recipient redesign in this task
- No frontend changes

---

## 2. Detailed Spec

## 2.1 Objective

Replace the current minimal notification model:
- `type`
- `event_id`
- `message`
- `created_at`
- `read_at`

with a richer inbox foundation that can later support:
- sender information
- title and body content
- explicit read state
- actionable invitation cards
- deep links through resource metadata
- future notification types such as `mention`

This task only introduces the schema and persistence foundation. It does not expose the new notification fields in public API responses yet.

## 2.2 Public API Impact

### New Endpoints
- None

### Existing Endpoints In Scope
- `GET /api/v1/notifications`
- `POST /api/v1/notifications/{notificationID}/read`

### Public Request Payload Changes
- None

### Public Response Payload Changes
- None intended for this task

### Public Validation Changes
- None intended at the transport layer

### Public Response Code Changes
- None intended

### Compatibility Rule
The current notification endpoints must keep their current observable behavior. Internally, repository and domain code may carry the new v2 fields, but those fields must not become part of the public JSON contract in this task.

## 2.3 Data Model Spec

### Existing Table
- `notifications`

### Existing Columns Kept For Compatibility
- `id UUID PRIMARY KEY`
- `user_id UUID NOT NULL`
- `workspace_id UUID NOT NULL`
- `type TEXT NOT NULL`
- `event_id UUID NOT NULL`
- `message TEXT NOT NULL`
- `created_at TIMESTAMPTZ NOT NULL`
- `read_at TIMESTAMPTZ NULL`

`event_id` and `message` stay in place so the current repository, tests, and public API can remain compatible during the transition.

### New Columns
- `actor_id UUID NULL REFERENCES users(id) ON DELETE SET NULL`
- `title TEXT NOT NULL`
- `content TEXT NOT NULL`
- `is_read BOOLEAN NOT NULL DEFAULT FALSE`
- `actionable BOOLEAN NOT NULL DEFAULT FALSE`
- `action_kind TEXT NULL`
- `resource_type TEXT NULL`
- `resource_id UUID NULL`
- `payload JSONB NOT NULL DEFAULT '{}'::jsonb`
- `updated_at TIMESTAMPTZ NOT NULL`

### Notification Type Rules
Expand `type` validation to:
- `invitation`
- `comment`
- `mention`

### Action Kind Rules
For this task, allowed `action_kind` values are:
- `NULL`
- `invitation_response`

Do not add more action kinds yet. Future types require an explicit migration.

### Resource Type Rules
For this task, allowed `resource_type` values are:
- `NULL`
- `invitation`
- `page_comment`
- `thread`
- `thread_message`

This set is enough to back current invitation/comment/thread writes without over-designing future resource types.

### Read-State Consistency Rule
Enforce database consistency:
- unread notification:
  - `is_read = FALSE`
  - `read_at IS NULL`
- read notification:
  - `is_read = TRUE`
  - `read_at IS NOT NULL`

Use a `CHECK` constraint so the database rejects mismatched state.

### Actionability Consistency Rule
Enforce database consistency:
- if `action_kind IS NOT NULL`, then `actionable = TRUE`
- `actionable = FALSE` may still have `action_kind IS NULL`

### Resource Link Consistency Rule
Enforce database consistency:
- `resource_type IS NULL` iff `resource_id IS NULL`

### Updated At Rules
- on create: `updated_at = created_at`
- on mark-read: `updated_at = read_at`
- on backfilled unread rows: `updated_at = created_at`
- on backfilled read rows: `updated_at = read_at`

## 2.4 Indexing And Uniqueness Spec

### Keep Existing Compatibility Uniqueness
Keep the current unique dedupe rule for existing writers:
- unique `(user_id, type, event_id)`

This remains necessary because current notification service code deduplicates with `event_id`.

### Keep Existing List Index
Keep:
- index on `(user_id, created_at DESC, id DESC)`

### Replace Old Unread Index
Replace the old `read_at`-based partial unread index with an `is_read`-based one:
- index on `(user_id, created_at DESC, id DESC)`
- partial predicate `WHERE is_read = FALSE`

### Add Invitation Live-Row Uniqueness Foundation
Add a new partial unique index for the later "one live notification per invitation" behavior:
- unique `(user_id, resource_id)`
- predicate:
  - `type = 'invitation'`
  - `resource_type = 'invitation'`

This index is a foundation only. Current v1 writers may continue to use `event_id` dedupe.

### Optional Future-Filter Index
Add one forward-looking index for later inbox filtering:
- index on `(user_id, type, created_at DESC, id DESC)`

This is justified because later notification list endpoints will filter by type and newest-first ordering.

## 2.5 Backfill Rules

For existing rows:

- `actor_id = NULL`
- `title`:
  - if `type = 'invitation'` => `"Workspace invitation"`
  - if `type = 'comment'` => `"Comment activity"`
- `content = message`
- `is_read = (read_at IS NOT NULL)`
- `actionable = FALSE`
- `action_kind = NULL`
- `resource_type`:
  - if `type = 'invitation'` => `'invitation'`
  - else `NULL`
- `resource_id`:
  - if `type = 'invitation'` => `event_id`
  - else `NULL`
- `payload = '{}'::jsonb`
- `updated_at = COALESCE(read_at, created_at)`

Backfill note:
- old comment notifications are ambiguous because current `event_id` can refer to legacy page comments, threads, or thread replies
- do not guess a `resource_type` for historical comment rows in this task
- old invitation rows should remain non-actionable in backfill to avoid exposing stale invitation actions if v2 fields are accidentally surfaced before the live projection work is complete

## 2.6 Repository Behavior After Task 9

### Create Notification
Repository create must:
- insert all old compatibility fields
- insert all new v2 fields
- default `payload` to `{}` when omitted
- scan all new fields back into the domain model

### Create Many Notifications
Repository batch create must:
- insert all old compatibility fields
- insert all new v2 fields
- keep `ON CONFLICT (user_id, type, event_id) DO NOTHING`
- populate v2 fields for every inserted row

### List By User ID
Repository list must:
- still order by `created_at DESC, id DESC`
- still return rows for the current `GET /notifications` endpoint
- scan all v2 fields internally

### Mark Read
Repository mark-read must:
- set `read_at = COALESCE(read_at, readAt)`
- set `is_read = TRUE`
- set `updated_at = COALESCE(read_at, readAt)` using the effective read timestamp
- remain idempotent

## 2.7 Notification Service Behavior After Task 9

The current service still exposes the old public contract, but new writes must populate internal v2 fields.

### Invitation Created Write
Current invitation-created notifications should write:
- `type = invitation`
- `event_id = invitation.ID`
- `message = "You have a new workspace invitation"`
- `actor_id = invitation.InvitedBy`
- `title = "Workspace invitation"`
- `content = "You have a new workspace invitation"`
- `is_read = FALSE`
- `actionable = TRUE`
- `action_kind = invitation_response`
- `resource_type = invitation`
- `resource_id = invitation.ID`
- `payload = '{}'::jsonb`
- `updated_at = created_at`

### Comment Created Write
Current page-comment notifications should write:
- `type = comment`
- `event_id = comment.ID`
- `message = "New comment on a page in your workspace"`
- `actor_id = comment.CreatedBy`
- `title = "Comment activity"`
- `content = "New comment on a page in your workspace"`
- `is_read = FALSE`
- `actionable = FALSE`
- `action_kind = NULL`
- `resource_type = page_comment`
- `resource_id = comment.ID`

### Thread Created Write
Current thread-created notifications should write:
- `type = comment`
- `event_id = thread.ID`
- `message = "New thread on a page in your workspace"`
- `actor_id = thread.CreatedBy`
- `title = "Comment activity"`
- `content = "New thread on a page in your workspace"`
- `resource_type = thread`
- `resource_id = thread.ID`

### Thread Reply Write
Current thread-reply notifications should write:
- `type = comment`
- `event_id = reply.ID`
- `message = "New reply on a page thread in your workspace"`
- `actor_id = reply.CreatedBy`
- `title = "Comment activity"`
- `content = "New reply on a page thread in your workspace"`
- `resource_type = thread_message`
- `resource_id = reply.ID`

### Mention Notifications
No service write is added in this task.

The schema must support mention later, but no mention producer is implemented yet.

## 2.8 Domain Model Spec

Add:
- `type NotificationActionKind string`
- `type NotificationResourceType string`
- `NotificationTypeMention`
- action-kind constant:
  - `invitation_response`
- resource-type constants:
  - `invitation`
  - `page_comment`
  - `thread`
  - `thread_message`

Update `domain.Notification` to include the new fields, but do not expose them in JSON yet. Use `json:"-"` for the new internal-only fields in this task:
- `ActorID`
- `Title`
- `Content`
- `IsRead`
- `Actionable`
- `ActionKind`
- `ResourceType`
- `ResourceID`
- `Payload`
- `UpdatedAt`

Keep the current public JSON tags for:
- `EventID`
- `Message`
- `CreatedAt`
- `ReadAt`

## 2.9 Positive And Negative Cases

### Public HTTP Cases
No new HTTP cases are introduced in this task.

Expected result:
- current notification list endpoint behaves exactly as before
- current mark-read endpoint behaves exactly as before
- current response codes remain unchanged

### Persistence Cases

Positive:
- migration succeeds on empty database
- migration succeeds on a database with unread and read notifications
- repository create stores both old and new notification fields
- repository batch create stores both old and new notification fields
- repository mark-read sets both `read_at` and `is_read`
- repository list still returns newest-first ordering

Negative:
- migration should fail if a backfill row would violate a new check constraint
- repository create should reject invalid enum-like values if application validation misses them
- repository create should still surface conflict on duplicate `(user_id, type, event_id)`
- repository mark-read should still return not found for another user's notification

---

## 3. File Structure And Responsibilities

### Create
- `migrations/000020_notification_schema_v2.up.sql`
- `migrations/000020_notification_schema_v2.down.sql`

### Modify
- `internal/domain/notification.go`
  - add internal v2 notification fields and new type constants
- `internal/repository/postgres/notification_repository.go`
  - read/write the new columns
- `internal/application/notification_service.go`
  - populate new internal v2 fields for current notification writers
- `internal/repository/postgres/content_repository_test.go`
  - extend integration coverage for notification repository migration behavior
- `internal/application/notification_service_test.go`
- `internal/application/notification_service_additional_test.go`
- `internal/transport/http/server_test.go`
  - keep endpoint regression coverage stable
- `frontend-repo/API_CONTRACT.md`
  - document that public notification contract is unchanged in this task, if needed in the task-history section
- `docs/checkpoint.md`

### Files Explicitly Not In Scope
- `internal/transport/http/handlers.go`
- `internal/transport/http/server.go`
- `internal/application/thread_service.go`
- `internal/application/workspace_service.go`
- `internal/repository/postgres/workspace_repository.go`

---

## 4. Test Matrix

## 4.1 Migration And Repository Tests

Add DB-backed coverage in the PostgreSQL notification test area.

### Positive Cases

1. Apply migration on empty database
- Expect:
  - new columns exist
  - new constraints exist
  - new indexes exist

2. Apply migration on database with one unread invitation notification
- Expect:
  - `is_read = FALSE`
  - `title = "Workspace invitation"`
  - `content = message`
  - `resource_type = invitation`
  - `resource_id = event_id`
  - `updated_at = created_at`

3. Apply migration on database with one read comment notification
- Expect:
  - `is_read = TRUE`
  - `title = "Comment activity"`
  - `resource_type = NULL`
  - `resource_id = NULL`
  - `updated_at = read_at`

4. Repository create inserts invitation notification with v2 fields populated

5. Repository batch create inserts comment notifications with v2 fields populated

6. Repository mark-read sets:
- `read_at`
- `is_read = TRUE`
- `updated_at`

7. Repository list still returns newest-first ordering after schema upgrade

### Negative Cases

8. Duplicate `(user_id, type, event_id)` still returns `domain.ErrConflict`

9. Mark-read on another user's notification still returns `domain.ErrNotFound`

10. Invalid manual row with mismatched `is_read` and `read_at` is rejected by DB constraint

11. Invalid manual row with `resource_type` but null `resource_id` is rejected by DB constraint

12. Invalid manual row with `action_kind` but `actionable = FALSE` is rejected by DB constraint

## 4.2 Notification Service Tests

### Positive Cases

13. `NotifyInvitationCreated` populates:
- `actor_id = invited_by`
- `title`
- `content`
- `actionable = TRUE`
- `action_kind = invitation_response`
- `resource_type = invitation`
- `resource_id = invitation.ID`

14. `NotifyCommentCreated` populates:
- `actor_id = comment.CreatedBy`
- `title`
- `content`
- `actionable = FALSE`
- `resource_type = page_comment`

15. `NotifyThreadCreated` populates:
- `actor_id = thread.CreatedBy`
- `resource_type = thread`

16. `NotifyThreadReplyCreated` populates:
- `actor_id = reply.CreatedBy`
- `resource_type = thread_message`

### Negative Cases

17. Existing repository errors still propagate

18. Existing invitation-create duplicate conflict is still ignored by service

## 4.3 HTTP Regression Tests

### Positive Cases

19. `GET /api/v1/notifications` response shape remains `Notification[]`

20. `POST /api/v1/notifications/{notificationID}/read` still returns a notification with `read_at`

### Negative Cases

21. Marking another user's notification as read still returns `404`

No new endpoint cases are introduced in this task.

---

## 5. Execution Plan

### Task 1: Write migration spec and failing DB-backed tests

**Files:**
- Create: `migrations/000020_notification_schema_v2.up.sql`
- Create: `migrations/000020_notification_schema_v2.down.sql`
- Modify: `internal/repository/postgres/content_repository_test.go`

- [ ] **Step 1: Add failing migration coverage for notification backfill**

Cover:
- unread invitation notification
- read comment notification
- index existence
- check-constraint behavior

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository" -count=1
```

Expected:
- FAIL because the new migration and backfill behavior do not exist yet

- [ ] **Step 3: Write the migration**

Requirements:
- use migration number `000020`
- alter `notifications`
- backfill existing rows
- replace the unread index
- add the invitation live-row partial unique index
- add reversible down migration

- [ ] **Step 4: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository" -count=1
```

Expected:
- migration-related failures remain until repository scans are updated

- [ ] **Step 5: Commit**

```bash
git add migrations/000020_notification_schema_v2.up.sql migrations/000020_notification_schema_v2.down.sql internal/repository/postgres/content_repository_test.go
git commit -m "test: add notification schema v2 migration coverage"
```

### Task 2: Update domain model and repository scans

**Files:**
- Modify: `internal/domain/notification.go`
- Modify: `internal/repository/postgres/notification_repository.go`

- [ ] **Step 1: Extend the domain notification model with internal-only v2 fields**

Add:
- new type constants
- action/resource enums
- internal-only fields with `json:"-"`

- [ ] **Step 2: Update repository `Create` to write all new columns**

Requirements:
- preserve compatibility fields
- insert and return v2 fields
- preserve conflict behavior

- [ ] **Step 3: Update repository `CreateMany` to write all new columns**

Requirements:
- extend array insert shape
- preserve `ON CONFLICT (user_id, type, event_id) DO NOTHING`

- [ ] **Step 4: Update repository `ListByUserID` and `MarkRead` scans**

Requirements:
- scan the new columns
- `MarkRead` must set `is_read = TRUE` and `updated_at`

- [ ] **Step 5: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository" -count=1
```

Expected:
- PASS

- [ ] **Step 6: Commit**

```bash
git add internal/domain/notification.go internal/repository/postgres/notification_repository.go
git commit -m "feat: upgrade notification repository to schema v2"
```

### Task 3: Update notification service writers

**Files:**
- Modify: `internal/application/notification_service.go`
- Modify: `internal/application/notification_service_test.go`
- Modify: `internal/application/notification_service_additional_test.go`

- [ ] **Step 1: Add failing service assertions for new internal notification fields**

Cover:
- invitation-created metadata
- comment metadata
- thread-created metadata
- thread-reply metadata

- [ ] **Step 2: Run targeted service tests**

Run:
```powershell
go test ./internal/application -run "TestNotificationService" -count=1
```

Expected:
- FAIL because current writers do not populate v2 fields

- [ ] **Step 3: Update notification service writes**

Requirements:
- populate both old and new fields
- keep current user-selection behavior unchanged
- keep duplicate-conflict ignore behavior unchanged for invitation create

- [ ] **Step 4: Re-run targeted service tests**

Run:
```powershell
go test ./internal/application -run "TestNotificationService" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/notification_service.go internal/application/notification_service_test.go internal/application/notification_service_additional_test.go
git commit -m "feat: populate notification v2 metadata"
```

### Task 4: Run HTTP regression coverage for current notification endpoints

**Files:**
- Modify: `internal/transport/http/server_test.go`

- [ ] **Step 1: Add or tighten regression assertions for notification endpoint compatibility**

Assert:
- list response shape is unchanged
- read response shape is unchanged
- `read_at` is still the externally visible read marker

- [ ] **Step 2: Run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "TestNotificationEndpoints" -count=1
```

Expected:
- PASS

- [ ] **Step 3: Commit**

```bash
git add internal/transport/http/server_test.go
git commit -m "test: lock notification endpoint compatibility"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update the notification task documentation**

Document:
- no public endpoint change in this task
- notification schema gained internal v2 metadata
- current public notification contract remains stable

- [ ] **Step 2: Update checkpoint**

Record:
- notification v2 schema foundation exists
- internal actor/title/content/action/resource fields added
- public notification endpoint contract unchanged

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: record notification schema v2 foundation"
```

### Task 6: Full verification for Task 9

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

- [ ] **Step 2: Manual schema sanity check if local database is available**

Verify:
- `notifications` has the new v2 columns
- old rows are backfilled
- current notification endpoints still respond with the old public shape

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify notification schema v2 task"
```

---

## 6. Acceptance Criteria

Task 9 is complete only when all are true:
- `notifications` stores actor, title, content, read-state, action, resource, payload, and updated-at metadata
- notification type validation includes `mention`
- old notification rows are backfilled safely
- current notification repository methods work against the new schema
- current notification service writes populate the new internal metadata
- current `GET /notifications` and `POST /notifications/{notificationID}/read` behavior remains unchanged
- repository, service, and HTTP tests cover the migration and compatibility cases above
- docs and checkpoint reflect the schema-foundation nature of the task

## 7. Risks And Guardrails

- Do not break the current public notification API in this task.
- Do not remove `event_id` or `message` yet.
- Do not expose new v2 fields in JSON before the inbox API task is ready.
- Do not guess deep-link resource types for historical comment rows during backfill.
- Do not add unread counters, outbox logic, or live invitation upsert behavior in this task.

## 8. Follow-On Tasks

This plan prepares for:
- Task 10 outbox foundation
- Task 11 outbox and invitation live-notification projection
- Task 12 notification inbox v2 API
- Task 14 unread-count work
