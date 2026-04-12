# Task 20 Mention Schema Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the database schema foundation for explicit thread-message mentions so later thread create and reply endpoints can persist mention data safely and idempotently.

**Architecture:** This task is schema-first and compatibility-first. It adds one normalized junction table between `page_comment_messages` and `users`, enforces one mention row per `(message_id, mentioned_user_id)`, and updates the integration test harness so the new table is covered by repository cleanup. Public thread endpoints and response payloads remain unchanged in this task; mention write and read behavior will land in Tasks 21 and 22 on top of this schema.

**Tech Stack:** Go, PostgreSQL, `pgx`, SQL migrations, repository integration tests, Go domain types

---

## 1. Scope

### In Scope
- Add a dedicated mention table for thread messages
- Enforce one mention row per `(message_id, mentioned_user_id)`
- Add indexes needed for future message-to-mentions and user-to-mentions lookups
- Add a small domain type for message mentions if it improves later task clarity
- Update repository integration reset logic for the new table
- Add DB-backed tests for constraints, foreign keys, and cascade behavior
- Update checkpoint

### Out Of Scope
- No new public endpoint
- No request payload changes
- No response payload changes
- No mention persistence in thread create yet
- No mention persistence in thread reply yet
- No mention notification projector yet
- No changes to `GET /api/v1/threads/{threadID}`
- No changes to `GET /api/v1/notifications`
- No repository methods for creating or loading mentions yet unless a minimal helper is required strictly for tests

### Prerequisites
- Task 17 thread-create outbox integration exists
- Task 18 thread-reply outbox integration exists
- Current thread tables from migration `000012_threaded_discussions` exist

---

## 2. Detailed Spec

## 2.1 Objective

Mention notifications should come from explicit stored data, not from parsing comment text later. To make that possible, the backend needs a durable relation between a thread message and each mentioned user.

This task adds that relation only. It does not yet let any API write or read mentions.

## 2.2 Public API Impact

### New Endpoints
- None

### Existing Endpoints In Scope
- None

### Public Request Payload Changes
- None

### Public Response Payload Changes
- None

### Public Validation Changes
- None

### Public Response Code Changes
- None

### Compatibility Rule
All current thread, notification, and invitation endpoints must keep their current observable behavior after this task. This task only adds storage foundation.

## 2.3 Data Model Spec

### New Table
- `page_comment_message_mentions`

### Columns
- `message_id UUID NOT NULL REFERENCES page_comment_messages(id) ON DELETE CASCADE`
- `mentioned_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT`

### Primary Key
- composite primary key:
  - `(message_id, mentioned_user_id)`

This enforces the product rule:
- one row per mentioned user per message

### Indexes
Add:
- primary key index from the composite primary key
- secondary index:
  - `page_comment_message_mentions_user_message_idx`
  - columns: `(mentioned_user_id, message_id)`

Rationale:
- the primary key supports message-local mention reads efficiently
- the secondary index supports future user-targeted mention lookups and projector/reconciliation flows

### Normalization Rule
This is a normalized many-to-many bridge:
- one message may mention many users
- one user may be mentioned in many messages

## 2.4 Referential And Integrity Rules

### Message Foreign Key Rule
- every mention row must reference an existing `page_comment_messages.id`
- deleting a message must delete its mention rows automatically

### User Foreign Key Rule
- every mention row must reference an existing `users.id`
- deleting a user is restricted at the DB level through the foreign key

### Duplicate Rule
- the same `mentioned_user_id` cannot appear twice for the same `message_id`

### Membership Rule
Do not enforce workspace membership in the database schema.

Reason:
- membership depends on joining:
  - message -> thread -> page -> workspace
  - workspace -> workspace_members
- that validation belongs in application logic for Tasks 21 and 22

## 2.5 Domain Model Spec

Add one focused domain type in `internal/domain/thread.go`:

```go
type PageCommentMessageMention struct {
    MessageID       string `json:"message_id"`
    MentionedUserID string `json:"mentioned_user_id"`
}
```

### Domain Rules
- do not add mentions to `PageCommentThreadDetail` yet
- do not add mention fields to `PageCommentThreadMessage` yet
- do not expose any new public thread-detail JSON in this task

This keeps Task 20 schema-only and avoids leaking incomplete API behavior before Tasks 21 and 22.

## 2.6 Repository Behavior After Task 20

No current repository method must change public behavior.

### Current Repositories
- `ThreadRepository.CreateThread`
- `ThreadRepository.AddReply`
- `ThreadRepository.GetThread`

All remain behaviorally unchanged in this task.

### Test Harness Update
The integration test reset must include `page_comment_message_mentions` in the truncate list so repository tests remain isolated.

### Future Repository Work
Tasks 21 and 22 will later add mention-aware write support, likely through:
- batch insert of mention rows inside the same transaction as message creation
- message-local mention reads when thread detail exposure is introduced

Do not implement those behaviors in this task.

## 2.7 Positive And Negative Cases

### Public HTTP Cases
No HTTP behavior is introduced in this task.

Expected public result:
- all existing endpoint codes and payloads remain unchanged

### Persistence Positive Cases

1. Migration succeeds on an empty database

2. Valid mention row can be inserted for:
- an existing thread message
- an existing user

3. One message may mention multiple distinct users

4. The same user may be mentioned in multiple different messages

5. Deleting a message cascades and removes its mention rows

### Persistence Negative Cases

1. Duplicate `(message_id, mentioned_user_id)` insert fails

2. Mention row with missing `message_id` fails foreign-key validation

3. Mention row with missing `mentioned_user_id` fails foreign-key validation

4. Migration failure blocks deploy

---

## 3. Files To Change

### Create
- `migrations/000023_thread_message_mentions.up.sql`
- `migrations/000023_thread_message_mentions.down.sql`

### Modify
- `internal/domain/thread.go`
- `internal/repository/postgres/integration_test.go`
- `internal/repository/postgres/content_repository_test.go`
- `docs/checkpoint.md`

### Optional Documentation Updates
- `frontend-repo/API_CONTRACT.md`
  - not required in this task because no public endpoint contract changes

### Files Explicitly Not In Scope
- `internal/application/thread_service.go`
- `internal/application/thread_service_test.go`
- `internal/repository/postgres/thread_repository.go`
- `internal/transport/http/handlers.go`
- `internal/transport/http/server.go`
- `internal/application/notification_service.go`

---

## 4. Test Plan

## 4.1 Migration And Schema Tests

Add DB-backed tests in:
- `internal/repository/postgres/content_repository_test.go`

### Positive Cases

1. Migration creates `page_comment_message_mentions`
- assert:
  - table exists
  - composite primary key exists
  - `(mentioned_user_id, message_id)` secondary index exists

2. Valid mention insert succeeds
- seed:
  - user
  - workspace
  - page
  - thread
  - message
- action:
  - direct SQL insert into `page_comment_message_mentions`
- expect:
  - one row exists

3. One message can mention multiple users
- action:
  - insert two rows with same `message_id` and different `mentioned_user_id`
- expect:
  - two rows exist

4. One user can be mentioned in multiple messages
- action:
  - insert two rows with same `mentioned_user_id` and different `message_id`
- expect:
  - two rows exist

5. Deleting a message cascades mention deletion
- action:
  - delete from `page_comment_messages`
- expect:
  - related mention rows count becomes `0`

### Negative Cases

6. Duplicate `(message_id, mentioned_user_id)` insert fails
- expect:
  - PostgreSQL unique or primary-key violation

7. Insert with missing message fails
- expect:
  - foreign-key violation

8. Insert with missing user fails
- expect:
  - foreign-key violation

## 4.2 Domain Regression Tests

9. Domain compile regression
- goal:
  - ensure adding `PageCommentMessageMention` does not break current thread tests

No dedicated unit test is required if the type is a simple struct and existing packages compile cleanly.

## 4.3 Integration Harness Tests

10. Test database reset includes mention table
- goal:
  - repeated integration tests should not leak mention rows across runs

This may be covered indirectly by existing integration suites after the truncate list is updated.

## 4.4 Documentation Tests

11. `docs/checkpoint.md` records:
- mention table added
- one row per `(message_id, mentioned_user_id)`
- no public API behavior change yet

---

## 5. Execution Plan

### Task 1: Add failing DB-backed tests for mention table behavior

**Files:**
- Modify: `internal/repository/postgres/content_repository_test.go`

- [ ] **Step 1: Add failing schema tests for mention insert, duplicate rejection, and cascade delete**

Cover:
- valid insert
- duplicate pair failure
- missing message failure
- missing user failure
- delete-message cascade

- [ ] **Step 2: Add a failing test for schema objects existing after migrations**

Assert:
- mention table exists
- composite primary key exists
- secondary user-message index exists

- [ ] **Step 3: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestThreadMessageMentionSchema" -count=1
```

Expected:
- FAIL because the mention table and related schema do not exist yet

- [ ] **Step 4: Commit**

```bash
git add internal/repository/postgres/content_repository_test.go
git commit -m "test: define thread message mention schema behavior"
```

### Task 2: Add the mention migration and integration reset support

**Files:**
- Create: `migrations/000023_thread_message_mentions.up.sql`
- Create: `migrations/000023_thread_message_mentions.down.sql`
- Modify: `internal/repository/postgres/integration_test.go`

- [ ] **Step 1: Write the migration**

Use migration number `000023`.

Required migration shape:

```sql
CREATE TABLE IF NOT EXISTS page_comment_message_mentions (
    message_id UUID NOT NULL REFERENCES page_comment_messages(id) ON DELETE CASCADE,
    mentioned_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    PRIMARY KEY (message_id, mentioned_user_id)
);

CREATE INDEX IF NOT EXISTS page_comment_message_mentions_user_message_idx
    ON page_comment_message_mentions (mentioned_user_id, message_id);
```

Down migration requirements:
- drop the secondary index
- drop the table

- [ ] **Step 2: Update integration reset order**

Required change:
- include `page_comment_message_mentions` in the truncate list before `page_comment_messages`

- [ ] **Step 3: Run targeted repository tests again**

Run:
```powershell
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestThreadMessageMentionSchema" -count=1
```

Expected:
- tests still fail until domain and checkpoint updates are complete, or PASS immediately if no code path depends on domain additions

- [ ] **Step 4: Commit**

```bash
git add migrations/000023_thread_message_mentions.up.sql migrations/000023_thread_message_mentions.down.sql internal/repository/postgres/integration_test.go
git commit -m "feat: add thread message mention schema"
```

### Task 3: Add the small domain mention type

**Files:**
- Modify: `internal/domain/thread.go`

- [ ] **Step 1: Add `PageCommentMessageMention`**

Required struct:

```go
type PageCommentMessageMention struct {
    MessageID       string `json:"message_id"`
    MentionedUserID string `json:"mentioned_user_id"`
}
```

- [ ] **Step 2: Keep current public thread detail payload unchanged**

Do not:
- add `Mentions` to `PageCommentThreadDetail`
- add mention fields to `PageCommentThreadMessage`

- [ ] **Step 3: Run targeted compile and repository tests**

Run:
```powershell
go test ./internal/domain/... ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestThreadMessageMentionSchema" -count=1
```

Expected:
- PASS

- [ ] **Step 4: Commit**

```bash
git add internal/domain/thread.go
git commit -m "feat: add thread message mention domain type"
```

### Task 4: Update checkpoint

**Files:**
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Record the schema foundation**

Document:
- new `page_comment_message_mentions` table
- composite primary key on `(message_id, mentioned_user_id)`
- no public API behavior change yet

- [ ] **Step 2: Commit**

```bash
git add docs/checkpoint.md
git commit -m "docs: record mention schema foundation"
```

### Task 5: Full verification for Task 20

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestThreadMessageMentionSchema" -count=1
go test ./internal/application -run "TestThreadService" -count=1
go test ./internal/transport/http -run "TestThread" -count=1
go test ./cmd/migrate -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual schema sanity check if local DB is available**

Verify:
- `page_comment_message_mentions` exists
- duplicate mention pair is rejected
- deleting a message removes its mention rows

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify mention schema task"
```

---

## 6. Acceptance Criteria

Task 20 is complete only when all are true:
- migration `000023` applies cleanly
- `page_comment_message_mentions` exists with the intended foreign keys
- one row per `(message_id, mentioned_user_id)` is enforced
- user-targeted secondary index exists
- deleting a message cascades to its mention rows
- current thread endpoints keep their current public behavior
- integration reset covers the new table
- checkpoint records the schema foundation

## 7. Risks And Guardrails

- Do not add mention request payloads in this task.
- Do not expose mention fields in thread responses yet.
- Do not enforce workspace membership through a complicated DB constraint.
- Do not add a surrogate mention id unless a concrete requirement appears later.
- Keep the schema normalized and minimal so Tasks 21 and 22 can build on it cleanly.

## 8. Follow-On Task Dependency

This plan intentionally prepares for:
- Task 21 `POST /api/v1/pages/{pageID}/threads` mention support
- Task 22 `POST /api/v1/threads/{threadID}/replies` mention support
- Task 23 mention notification projector

Those tasks should assume Task 20 has already landed and should not redesign the mention storage model.
