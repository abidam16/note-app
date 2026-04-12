# Task 27 Notification Reconciliation Job Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a dedicated admin reconciliation job that rebuilds and repairs invitation, comment, and mention notifications from source-of-truth tables and then resets unread counters to exact values.

**Architecture:** This task adds an internal CLI command, not a public endpoint. The command opens one reconciliation run, acquires a PostgreSQL advisory lock, captures a cutoff timestamp, scans authoritative invitation and thread-message data in batches, computes the expected notification projection, repairs managed notification rows idempotently, deletes only orphaned managed rows that are safe to remove for the captured cutoff, and finally recomputes unread counters from the notification table. The job supports dry-run mode and workspace-scoped execution so operators can inspect or limit impact without inventing a second recovery system.

**Tech Stack:** Go, PostgreSQL, `pgx`, CLI flags, SQL repositories, application services, PostgreSQL advisory locks, table-driven tests

---

## 1. Scope

### In Scope
- Add one internal admin command:
  - `go run ./cmd/notification-reconcile`
- Add one application reconciliation service
- Add one PostgreSQL reconciliation repository
- Rebuild and repair these managed notification classes:
  - invitation live notifications
  - comment append-only notifications for thread messages
  - mention append-only notifications for thread messages
- Delete orphaned managed notification rows when they do not match source-of-truth state for the captured cutoff
- Recompute unread counters exactly from the repaired notification table
- Support:
  - full-database reconciliation
  - one-workspace reconciliation
  - dry-run mode
  - bounded batched scans
- Publish best-effort inbox invalidation signals after effective non-dry-run repair changes
- Add command, repository, application, and documentation tests
- Update checkpoint

### Out Of Scope
- No public HTTP endpoint
- No scheduler, cron wiring, or automatic background execution
- No replayable outbox reprocessing command
- No new notification types beyond:
  - `invitation`
  - `comment`
  - `mention`
- No mutation of legacy flat-comment notification rows
- No UI or frontend changes
- No new stream endpoint, broker protocol, or SSE payload change
- No new migration in this task

### Prerequisites
- Task 11 invitation live notification projector exists
- Task 14 unread counter table exists
- Task 19 comment notification projector exists
- Task 20 explicit mention schema exists
- Task 23 mention notification projector exists
- Task 26 notification stream exists, but this task does not depend on stream delivery for correctness

---

## 2. Detailed Spec

## 2.1 Objective

The notification read model is intentionally projected data, not the source of truth. That means the backend needs one explicit repair path for cases such as:
- an invitee registers after the original invitation event and never received the live invitation notification
- a projector bug created wrong recipients or wrong payloads
- old workspace-wide thread notifications must be repaired to relevant-user notifications
- mention rows or comment rows are missing after partial failures or earlier defects
- unread counters drift from notification row state

This task adds a safe reconciliation job so operators can repair the read model from:
- invitations
- thread messages
- explicit mention rows
- current notification rows

The reconciliation job must be idempotent. Running it twice with no new source changes must produce:
- the same managed notification rows
- the same unread counters
- a zero-change summary on the second run

## 2.2 Public API Impact

### New Endpoints
- None

### Existing Endpoints In Scope
- None directly

### Public Request Payload Changes
- None

### Public Response Payload Changes
- None

### Public Response Code Changes
- None

### Public Behavior Impact
After this task, operators have one supported repair path for the notification read model. Public API contracts do not change, but later support work may use this command to:
- backfill missing invitation notifications for now-registered users
- remove stale managed notification rows
- restore exact unread counts

### Stream Freshness Rule
Task 26 defines best-effort invalidation publish after effective inbox changes. This task must stay compatible with that rule.

Therefore, after any non-dry-run batch that produces effective notification or unread-counter changes:
- publish best-effort user-scoped invalidation for affected users
- do not fail the reconciliation run if publish fails
- log publish failure and continue

Dry-run mode must never publish invalidation signals.

## 2.3 Execution Surface

This task adds one internal CLI command.

### Command

```powershell
go run ./cmd/notification-reconcile
```

### Flags

- `-env-file`
  - default: `.env`
- `-workspace-id`
  - default: empty
  - optional
  - when set, only source rows and managed notification rows for that workspace are reconciled
- `-dry-run`
  - default: `false`
  - when `true`, compute and print the summary without persisting notification or counter changes
- `-batch-size`
  - default: `500`
  - valid range: `1` to `2000`

### Scope Rules

1. when `-workspace-id` is empty:
   - reconcile all workspaces
2. when `-workspace-id` is non-empty:
   - reconcile only that workspace's invitation, comment, and mention source data
   - unread counters are still recomputed against the full notification table for affected users
3. this task does not support user-scoped reconciliation

### Exit Codes

- `0`
  - successful run
  - includes dry-run success
- `1`
  - invalid flags
  - config load failure
  - database connection failure
  - advisory lock unavailable
  - repository or application failure
  - stdout summary write failure

## 2.4 Command Input And Output Contract

### Request Payload
- none

### Input Contract
- all inputs are command-line flags
- no JSON stdin is used in this task

### Success Output
Write one JSON object to stdout.

Example:

```json
{
  "status": "ok",
  "dry_run": false,
  "workspace_id": "workspace-uuid",
  "batch_size": 500,
  "started_at": "2026-04-04T10:00:00Z",
  "cutoff_at": "2026-04-04T10:00:00Z",
  "finished_at": "2026-04-04T10:00:12Z",
  "invitations": {
    "scanned": 42,
    "unregistered_skipped": 7,
    "inserted": 3,
    "updated": 5,
    "deleted": 1
  },
  "comments": {
    "threads_scanned": 18,
    "messages_scanned": 116,
    "inserted": 9,
    "updated": 4,
    "deleted": 13
  },
  "mentions": {
    "messages_scanned": 116,
    "mention_rows_scanned": 20,
    "inserted": 2,
    "updated": 1,
    "deleted": 0
  },
  "counters": {
    "users_recomputed": 21,
    "upserted": 18,
    "deleted": 3
  }
}
```

### Output Rules

1. `status` is always `ok` on success
2. `dry_run` echoes the command flag
3. `workspace_id` is omitted or `null` for full-database runs
4. `started_at`, `cutoff_at`, and `finished_at` are RFC3339 UTC timestamps
5. counts must reflect work actually applied
6. in dry-run mode:
   - counts reflect predicted changes
   - no writes are persisted

### Error Output
- write one human-readable error line to stderr
- no JSON error payload is required

## 2.5 Concurrency And Run Isolation Rules

### Advisory Lock Rule

Only one reconciliation job may run at a time.

Use one PostgreSQL advisory lock for the whole run.

If the lock cannot be acquired:
- return an error
- exit with code `1`
- do not perform partial work

### Cutoff Rule

At run start, capture:
- `cutoff_at = now().UTC()`

All source scans and orphan-deletion eligibility checks must use this cutoff.

Purpose:
- do not race against new source rows created while the job is running
- do not delete valid managed notifications created after the run started

### Normal Write Coexistence Rule

Normal application writes may continue while reconciliation runs.

This task must not require:
- a global write freeze
- table locks on notification tables

Because the job is batched and idempotent:
- a later run may reconcile rows created after the captured cutoff
- normal projectors remain the primary write path

## 2.6 Managed Notification Scope

This job must only mutate managed V2 notification rows.

### Managed Invitation Notification Rule
Mutate only rows that match all of:
- `type = invitation`
- `resource_type = invitation`
- `resource_id` references an invitation id

### Managed Comment Notification Rule
Mutate only rows that match all of:
- `type = comment`
- `resource_type = thread_message`
- payload contains:
  - `thread_id`
  - `message_id`
  - `page_id`
  - `workspace_id`

### Managed Mention Notification Rule
Mutate only rows that match all of:
- `type = mention`
- `resource_type = thread_message`
- payload contains:
  - `thread_id`
  - `message_id`
  - `page_id`
  - `workspace_id`

### Legacy Protection Rule
Do not update or delete any legacy notification row that does not match the managed V2 signatures above.

This guard is mandatory so the reconciliation job does not damage:
- old flat-comment notifications
- old invitation rows from pre-V2 schema states
- unrelated future notification types

## 2.7 Invitation Reconciliation Rules

Invitation notifications are a live projection, not append-only.

### Authoritative Source
- invitations table
- current registered user lookup by invitation email

### Registered Invitee Rule
If the invitation email does not resolve to a current user row:
- no live invitation notification should exist
- the invitation still remains valid in the source table
- count it in `unregistered_skipped`

### Expected Identity
One live row per:
- `user_id`
- `type = invitation`
- `resource_id = invitation_id`

### Field Mapping
Use the same canonical mapping as Task 11:
- `workspace_id`
- `actor_id`
- `title`
- `content`
- `actionable`
- `action_kind`
- `resource_type = invitation`
- `resource_id = invitation_id`
- payload fields including:
  - `invitation_id`
  - `status`
  - `role`
  - `version`
  - `can_accept`
  - `can_reject`

### Timestamp Rules
Canonical source timestamps:
- `created_at = invitation.created_at`
- `updated_at = invitation.updated_at`

### Upsert Rules
When the expected live row exists already:
- preserve `is_read`
- preserve `read_at`
- replace other managed fields with canonical values

When the expected live row is missing:
- insert it as unread

### Delete Rules
Delete a managed invitation notification row when all are true:
1. it is in the current command scope
2. it is eligible for the captured cutoff
3. no registered invitee notification should exist for that invitation anymore

Examples:
- invitation row was deleted manually
- invitation row exists but no current user matches the invited email
- row points to the wrong recipient

## 2.8 Comment Reconciliation Rules

Comment notifications are append-only, but reconciliation may:
- insert missing rows
- update incorrect managed fields on existing rows
- delete orphaned managed rows

### Authoritative Source
- thread table
- thread message table
- explicit mention rows
- current workspace membership

### Historical Recipient Rule

Comment recipient derivation must use message history as it existed up to each message, not the final thread state.

For each thread, process messages in stable ascending order:
- `created_at ASC`
- `id ASC` as tie-breaker

For each message, expected comment recipients are:
1. current thread creator, if not the actor
2. distinct prior repliers before this message, if not the actor
3. explicit mention targets on this message, if not the actor
4. then filter to current workspace members at reconciliation time
5. dedupe in first-seen order

### First Message Rule
For the starter message of a new thread:
- there are no prior repliers
- the thread creator is also the actor
- therefore comment recipients come only from explicit mention targets

### Identity Rule
One comment notification per:
- `user_id`
- `type = comment`
- `event_id = message_id`

### Field Mapping
Use the canonical mapping from Task 19:
- `workspace_id`
- `actor_id`
- `title`
- `content`
- `actionable = false`
- `action_kind = null`
- `resource_type = thread_message`
- `resource_id = message_id`
- payload includes:
  - `workspace_id`
  - `page_id`
  - `thread_id`
  - `message_id`
  - `event_topic`

### Timestamp Rules
Canonical source timestamps:
- `created_at = message.created_at`
- `updated_at = message.created_at` for newly inserted rows

If an existing row is repaired:
- preserve `is_read`
- preserve `read_at`
- preserve `created_at`
- set `updated_at = reconciliation run time` only when managed fields changed

### Delete Rules
Delete a managed comment notification row when all are true:
1. it is in the current command scope
2. its source message is at or before the captured cutoff
3. it is not in the expected comment-notification key set for that cutoff

This must remove:
- stale workspace-wide fanout rows from earlier incorrect logic
- rows pointing to users who are no longer eligible recipients
- duplicate or malformed managed comment rows

## 2.9 Mention Reconciliation Rules

Mention notifications are append-only, but reconciliation may:
- insert missing rows
- update incorrect managed fields on existing rows
- delete orphaned managed rows

### Authoritative Source
- thread message table
- explicit message mention rows
- current workspace membership

### Recipient Rule
For each explicit mention row:
- recipient is `mentioned_user_id`
- exclude the acting user
- exclude users who are no longer current workspace members

### Identity Rule
One mention notification per:
- `user_id`
- `type = mention`
- `event_id = message_id`

### Field Mapping
Use the canonical mapping from Task 23:
- `workspace_id`
- `actor_id`
- `title`
- `content`
- `actionable = false`
- `action_kind = null`
- `resource_type = thread_message`
- `resource_id = message_id`
- payload includes:
  - `workspace_id`
  - `page_id`
  - `thread_id`
  - `message_id`
  - `event_topic`

### Timestamp Rules
Canonical source timestamps:
- `created_at = message.created_at`
- `updated_at = message.created_at` for inserts

If an existing row is repaired:
- preserve `is_read`
- preserve `read_at`
- preserve `created_at`
- set `updated_at = reconciliation run time` only when managed fields changed

### Delete Rules
Delete a managed mention notification row when all are true:
1. it is in the current command scope
2. its source message is at or before the captured cutoff
3. it is not in the expected mention-notification key set for that cutoff

## 2.10 Read-State Preservation Rules

Reconciliation corrects projection content, not user read intent.

Therefore:
- never reset a read notification to unread
- never clear a stored `read_at`
- never increment or decrement unread counters by delta math in the application layer
- always rebuild unread counters from notification row truth after repair writes finish

## 2.11 Unread Counter Rebuild Rules

Unread counters must be rebuilt after notification repair.

### Affected User Rule
Recompute counters only for affected users:
- users whose managed notification rows were inserted
- users whose managed notification rows were updated
- users whose managed notification rows were deleted
- users who already own managed rows in the current workspace scope

For a full-database run:
- affected users are every user with at least one managed notification row
- plus every user who already has a counter row

### Counter Source Rule
Unread count comes only from the notification table:
- count notifications for that user where `read_at IS NULL`

### Counter Persistence Rule
For each affected user:
- if unread count is greater than zero:
  - upsert the exact count into `notification_unread_counters`
- if unread count is zero:
  - delete the counter row if it exists

### No Incremental Math Rule
Do not attempt:
- `+1`
- `-1`
- delta aggregation

This task is a repair path. It must set exact values.

## 2.12 Post-Repair Invalidation Rules

### Effective-Change Rule
For non-dry-run execution, publish best-effort user-scoped invalidation only when a batch produced an effective change for that user:
- managed notification row inserted
- managed notification row updated
- managed notification row deleted
- unread counter value changed

### No-Op Rule
Do not publish invalidation for:
- dry-run execution
- scanned rows that produced no persisted change
- users whose final unread counter stayed identical and whose inbox rows were unchanged

### Failure Rule
If invalidation publish fails:
- log the error
- continue the reconciliation run
- do not roll back already committed repair writes

## 2.13 Repository Behavior

Create one focused PostgreSQL reconciliation repository instead of bloating the existing notification repository.

Recommended file:
- `internal/repository/postgres/notification_reconciliation_repository.go`

Recommended responsibilities:
- acquire and release advisory lock
- scan source invitations in batches
- scan thread ids in batches
- load ordered thread message history and explicit mention rows for one thread
- list existing managed notification keys for the current scope and cutoff
- upsert repaired invitation live rows
- upsert repaired comment rows
- upsert repaired mention rows
- delete managed rows by id
- list affected users for counter rebuild
- set exact unread counter values

### Repository Rules
1. batch scans must use stable ordering
2. repository methods must accept `context.Context`
3. repository writes must be transaction-safe per batch
4. repository must never mutate unmanaged notification rows
5. closed-pool behavior returns errors

## 2.14 Application Service Behavior

Create one application service:
- `NotificationReconciliationService`

Recommended method:

```go
Run(ctx context.Context, input RunNotificationReconciliationInput) (NotificationReconciliationSummary, error)
```

Recommended input:

```go
type RunNotificationReconciliationInput struct {
    WorkspaceID string
    DryRun      bool
    BatchSize   int
}
```

### Service Algorithm

1. validate input
2. acquire advisory lock
3. capture `started_at` and `cutoff_at`
4. reconcile invitation live notifications in batches
5. reconcile comment notifications in thread-history batches
6. reconcile mention notifications in thread-history batches
7. collect affected users
8. rebuild unread counters exactly
9. capture `finished_at`
10. return summary

### Dry-Run Rule
When `DryRun = true`:
- perform all source scans
- compute all would-change counts
- do not write notifications
- do not write unread counters
- still return a normal success summary

### Failure Rule
If any non-dry-run batch write fails:
- stop the run
- return error
- keep already committed earlier batches

Rationale:
- the run is idempotent
- rerunning the command is the repair strategy
- global rollback across the whole database is not required

## 2.15 Positive And Negative Cases

### Positive Cases

1. full run with no drift
- result: exit `0`
- summary counts show scans but zero inserts, updates, and deletes

2. invitation exists for a now-registered user and live row is missing
- result: exit `0`
- invitation notification inserted

3. existing invitation live row has stale payload or wrong actionability
- result: exit `0`
- row updated
- `read_at` preserved

4. thread message is missing one expected comment notification
- result: exit `0`
- missing comment row inserted

5. thread message has obsolete workspace-wide fanout rows from old logic
- result: exit `0`
- only relevant-user comment rows remain

6. self-mention exists in source rows
- result: exit `0`
- no mention notification for the acting user

7. unread counter drift exists
- result: exit `0`
- counter row is reset to exact notification-table value

8. dry-run on a workspace with drift
- result: exit `0`
- summary reports predicted changes
- database state is unchanged

### Negative Cases

1. `-batch-size=0`
- result: exit `1`
- no DB work begins

2. `-batch-size=5000`
- result: exit `1`
- no DB work begins

3. config load failure
- result: exit `1`

4. DB connection failure
- result: exit `1`

5. advisory lock already held by another reconciliation run
- result: exit `1`

6. source scan query failure
- result: exit `1`
- later phases do not run

7. notification repair write failure in non-dry-run mode
- result: exit `1`
- earlier committed batches remain valid

8. stdout summary write failure
- result: exit `1`

---

## 3. File Structure And Responsibilities

### Create
- `cmd/notification-reconcile/main.go`
  - parse flags, load config, wire DB and reconciliation service, print summary JSON, set process exit code
- `cmd/notification-reconcile/main_test.go`
  - command flag parsing, dependency wiring, summary output, and exit behavior tests
- `internal/application/notification_reconciliation.go`
  - reconciliation service, input validation, cutoff capture, phase orchestration, summary generation
- `internal/application/notification_reconciliation_test.go`
  - service tests for dry-run, repair behavior, affected-user collection, and failure handling
- `internal/application/thread_notification_history_builder.go`
  - pure helper that derives historical comment recipients and expected comment or mention notification keys from ordered thread history
- `internal/application/thread_notification_history_builder_test.go`
  - focused tests for first-message behavior, prior-replier logic, mention inclusion, actor exclusion, and deterministic ordering
- `internal/repository/postgres/notification_reconciliation_repository.go`
  - PostgreSQL scan, upsert, delete, advisory-lock, and exact-counter reset methods for reconciliation
- `internal/repository/postgres/notification_reconciliation_repository_test.go`
  - integration tests for scans, managed-row protection, idempotent upserts, deletes, and exact counter rebuild
- `docs/operations/notification-reconciliation.md`
  - operator guidance for safe command usage, dry-run, workspace scope, and expected outcomes

### Modify
- `internal/repository/postgres/closed_pool_errors_test.go`
  - add closed-pool coverage for reconciliation repository methods
- `internal/infrastructure/database/notification_stream.go`
  - reuse the Task 26 broker publish path for best-effort post-repair invalidation if that file owns the publish helper
- `docs/checkpoint.md`
  - record the new reconciliation command and recovery capability

### Files Explicitly Not In Scope
- `internal/transport/http/handlers.go`
- `internal/transport/http/server.go`
- `frontend-repo/API_CONTRACT.md`
- `cmd/api/app.go`
- `internal/application/notification_stream.go`
  - except for a small shared publisher interface extraction if Task 26 requires it for reuse

---

## 4. Test Matrix

## 4.1 Command Tests

Add tests in:
- `cmd/notification-reconcile/main_test.go`

### Positive Cases

1. default flags use:
- `.env`
- empty `workspace_id`
- `dry_run = false`
- `batch_size = 500`

2. `-workspace-id` is passed through to the service

3. `-dry-run` is passed through and success prints one JSON summary object

4. successful run exits with code `0`

### Negative Cases

5. invalid `-batch-size=0` returns error

6. invalid `-batch-size=2001` returns error

7. config load error exits with code `1`

8. reconciliation service error exits with code `1`

## 4.2 Repository Integration Tests

Add tests in:
- `internal/repository/postgres/notification_reconciliation_repository_test.go`

### Positive Cases

9. advisory lock can be acquired once and released

10. invitation source scan honors:
- workspace scope
- stable ordering
- cutoff filter

11. thread-id scan honors:
- workspace scope
- stable ordering
- cutoff filter

12. ordered thread-history load returns:
- starter message first
- replies in created order
- explicit mention rows for each message

13. invitation live-row upsert inserts missing row

14. invitation live-row upsert updates managed fields while preserving `read_at`

15. comment row upsert inserts missing managed row

16. comment row repair updates title, content, actor, and payload without clearing `read_at`

17. mention row upsert inserts missing managed row

18. managed orphan delete removes only rows selected by id

19. exact unread counter rebuild upserts positive counts

20. exact unread counter rebuild deletes zero-value rows

### Negative Cases

21. second advisory lock acquisition on a different connection fails

22. managed-row listing never returns unmanaged legacy rows

23. closed pool returns errors for:
- lock acquisition
- source scans
- repair writes
- counter rebuild

## 4.3 Historical Recipient Builder Tests

Add tests in:
- `internal/application/thread_notification_history_builder_test.go`

### Positive Cases

24. starter message with no mentions produces zero comment recipients

25. starter message with one explicit mention produces one comment recipient

26. first reply by a new actor notifies only the thread creator

27. second distinct replier notifies:
- thread creator
- prior replier
- explicit mention targets

28. duplicate mention target and prior replier are deduped in first-seen order

29. self-mention is retained in source history but excluded from mention notification recipients

### Negative Cases

30. blank mention user ids are ignored

31. non-member recipients are filtered out

## 4.4 Application Service Tests

Add tests in:
- `internal/application/notification_reconciliation_test.go`

### Positive Cases

32. dry-run returns predicted invitation, comment, mention, and counter counts with no write calls

33. missing invitation notification for now-registered user is scheduled for insert

34. stale invitation live row is scheduled for update and preserves read state

35. obsolete comment fanout rows are scheduled for delete

36. missing mention row is scheduled for insert

37. affected-user set includes:
- inserted-row owners
- updated-row owners
- deleted-row owners
- full-run users with existing counter rows

38. service calls exact counter rebuild after notification repair phases

39. non-dry-run effective repair change publishes one best-effort invalidation per affected user

40. dry-run never publishes invalidation

### Negative Cases

41. invalid input batch size returns validation error

42. advisory lock acquisition failure stops the run before scans begin

43. invitation scan failure aborts the run

44. comment repair write failure aborts the run

45. dry-run never calls write methods

46. publish failure is logged and does not fail the run

## 4.5 Documentation Tests

47. `docs/operations/notification-reconciliation.md` documents:
- command path
- flags
- dry-run behavior
- workspace-scoped behavior
- exit codes
- the managed-row protection rule
- post-repair invalidation behavior

48. `docs/checkpoint.md` records:
- notification reconciliation command added
- invitation backfill for now-registered invitees supported
- exact unread counter rebuild supported
- best-effort invalidation after repair supported

---

## 5. Execution Plan

### Task 1: Add failing command and service tests

**Files:**
- Create: `cmd/notification-reconcile/main_test.go`
- Create: `internal/application/notification_reconciliation_test.go`
- Create: `internal/application/thread_notification_history_builder_test.go`

- [ ] **Step 1: Write failing command tests**

Cover:
- default flags
- invalid batch size
- dry-run pass-through
- exit code on failure

- [ ] **Step 2: Write failing historical-recipient tests**

Cover:
- starter message recipient rules
- prior replier accumulation
- mention dedupe
- actor exclusion

- [ ] **Step 3: Write failing reconciliation service tests**

Cover:
- dry-run summary
- missing invitation backfill
- orphan comment delete
- exact counter rebuild trigger

- [ ] **Step 4: Run targeted application and command tests**

Run:
```powershell
go test ./cmd/notification-reconcile -count=1
go test ./internal/application -run "TestNotificationReconciliation|TestThreadNotificationHistoryBuilder" -count=1
```

Expected:
- FAIL because the command and reconciliation service do not exist yet

- [ ] **Step 5: Commit**

```bash
git add cmd/notification-reconcile/main_test.go internal/application/notification_reconciliation_test.go internal/application/thread_notification_history_builder_test.go
git commit -m "test: define notification reconciliation behavior"
```

### Task 2: Add failing repository integration tests

**Files:**
- Create: `internal/repository/postgres/notification_reconciliation_repository_test.go`
- Modify: `internal/repository/postgres/closed_pool_errors_test.go`

- [ ] **Step 1: Write failing repository integration tests**

Cover:
- advisory lock exclusivity
- invitation scans
- thread-history scans
- managed-row protection
- exact unread counter rebuild

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestNotificationReconciliationRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- FAIL because the reconciliation repository does not exist yet

- [ ] **Step 3: Commit**

```bash
git add internal/repository/postgres/notification_reconciliation_repository_test.go internal/repository/postgres/closed_pool_errors_test.go
git commit -m "test: define notification reconciliation repository behavior"
```

### Task 3: Implement repository support

**Files:**
- Create: `internal/repository/postgres/notification_reconciliation_repository.go`
- Modify: `internal/repository/postgres/notification_reconciliation_repository_test.go`
- Modify: `internal/repository/postgres/closed_pool_errors_test.go`

- [ ] **Step 1: Implement advisory lock and batched source scans**

Required behavior:
- acquire and release reconciliation advisory lock
- scan invitations by scope and cutoff
- scan thread ids by scope and cutoff
- load ordered message and mention history per thread

- [ ] **Step 2: Implement managed notification repair writes**

Required behavior:
- upsert invitation live rows preserving read state
- upsert comment rows
- upsert mention rows
- delete only explicitly selected managed rows

- [ ] **Step 3: Implement exact counter rebuild methods**

Required behavior:
- upsert positive unread counts
- delete zero-count rows
- never use delta math

- [ ] **Step 4: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestNotificationReconciliationRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/repository/postgres/notification_reconciliation_repository.go internal/repository/postgres/notification_reconciliation_repository_test.go internal/repository/postgres/closed_pool_errors_test.go
git commit -m "feat: add notification reconciliation repository"
```

### Task 4: Implement historical recipient builder

**Files:**
- Create: `internal/application/thread_notification_history_builder.go`
- Modify: `internal/application/thread_notification_history_builder_test.go`

- [ ] **Step 1: Implement pure history-to-recipient logic**

Required behavior:
- process messages in stable order
- derive recipients from thread creator, prior repliers, and explicit mentions
- exclude actor
- filter to current workspace members
- dedupe in first-seen order

- [ ] **Step 2: Re-run targeted builder tests**

Run:
```powershell
go test ./internal/application -run "TestThreadNotificationHistoryBuilder" -count=1
```

Expected:
- PASS

- [ ] **Step 3: Commit**

```bash
git add internal/application/thread_notification_history_builder.go internal/application/thread_notification_history_builder_test.go
git commit -m "feat: add thread notification history builder"
```

### Task 5: Implement reconciliation service

**Files:**
- Create: `internal/application/notification_reconciliation.go`
- Modify: `internal/application/notification_reconciliation_test.go`

- [ ] **Step 1: Implement input validation and summary model**

Required behavior:
- validate batch size range
- capture timestamps
- build JSON-printable summary shape

- [ ] **Step 2: Implement invitation reconciliation phase**

Required behavior:
- backfill now-registered invitees
- update stale live rows
- preserve read state
- compute orphan deletes only for managed rows in scope and cutoff

- [ ] **Step 3: Implement comment and mention reconciliation phases**

Required behavior:
- derive expected rows from thread history
- insert missing managed rows
- repair stale managed rows
- delete orphaned managed rows

- [ ] **Step 4: Implement affected-user collection and exact counter rebuild**

Required behavior:
- collect touched users
- include existing-counter users for full runs
- rebuild exact unread counts after notification repair

- [ ] **Step 5: Re-run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestNotificationReconciliation|TestThreadNotificationHistoryBuilder" -count=1
```

Expected:
- PASS

- [ ] **Step 6: Commit**

```bash
git add internal/application/notification_reconciliation.go internal/application/notification_reconciliation_test.go internal/application/thread_notification_history_builder.go internal/application/thread_notification_history_builder_test.go
git commit -m "feat: add notification reconciliation service"
```

### Task 6: Add the admin command

**Files:**
- Create: `cmd/notification-reconcile/main.go`
- Modify: `cmd/notification-reconcile/main_test.go`
- Modify if needed: `internal/infrastructure/database/notification_stream.go`

- [ ] **Step 1: Implement flag parsing and dependency wiring**

Required behavior:
- parse `env-file`
- parse optional `workspace-id`
- parse `dry-run`
- parse bounded `batch-size`
- wire best-effort invalidation publisher for non-dry-run repair flows

- [ ] **Step 2: Implement summary JSON output**

Required behavior:
- print one summary JSON object to stdout on success
- print one error line to stderr on failure
- exit `0` on success and `1` on failure

- [ ] **Step 3: Re-run targeted command tests**

Run:
```powershell
go test ./cmd/notification-reconcile -count=1
```

Expected:
- PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/notification-reconcile/main.go cmd/notification-reconcile/main_test.go internal/infrastructure/database/notification_stream.go
git commit -m "feat: add notification reconciliation command"
```

### Task 7: Update operational docs

**Files:**
- Create: `docs/operations/notification-reconciliation.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Write operator runbook**

Document:
- command usage
- recommended first use with `-dry-run`
- workspace-scoped repair
- advisory lock behavior
- expected exit codes
- managed-row protection and legacy-row exclusion
- best-effort post-repair invalidation behavior

- [ ] **Step 2: Update checkpoint**

Record:
- reconciliation job added
- backfill and repair behavior
- exact unread counter rebuild support

- [ ] **Step 3: Commit**

```bash
git add docs/operations/notification-reconciliation.md docs/checkpoint.md
git commit -m "docs: add notification reconciliation runbook"
```

### Task 8: Full verification for Task 27

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./cmd/notification-reconcile -count=1
go test ./internal/application -run "TestNotificationReconciliation|TestThreadNotificationHistoryBuilder" -count=1
go test ./internal/repository/postgres -run "TestNotificationReconciliationRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Run a dry-run manual sanity check if a local DB is available**

Run:
```powershell
go run ./cmd/notification-reconcile -env-file .env.test -dry-run
```

Verify:
- exit code `0`
- one JSON summary is printed
- no DB writes are persisted

- [ ] **Step 3: Run a scoped dry-run manual sanity check if a local workspace id is available**

Run:
```powershell
go run ./cmd/notification-reconcile -env-file .env.test -dry-run -workspace-id <workspace-id>
```

Verify:
- exit code `0`
- summary includes the workspace id
- counts are bounded to that workspace scan

- [ ] **Step 4: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify notification reconciliation task"
```

---

## 6. Acceptance Criteria

Task 27 is complete only when all are true:
- `go run ./cmd/notification-reconcile` exists and returns structured summary JSON on success
- the command supports:
  - full runs
  - workspace-scoped runs
  - dry-run mode
  - bounded batch size
- only one reconciliation run may execute at a time through an advisory lock
- the run captures one cutoff timestamp and uses it to avoid deleting newer rows
- invitation reconciliation backfills missing live rows for now-registered invitees
- comment reconciliation derives expected recipients from historical thread state, not final thread state
- mention reconciliation excludes the acting user and non-members
- managed notification repairs preserve read state
- unmanaged legacy notification rows are never changed
- unread counters are rebuilt exactly from notification row truth
- effective non-dry-run repair changes publish best-effort invalidation for affected users
- repository, application, command, and docs tests cover positive and negative cases
- checkpoint and operator docs are updated

## 7. Risks And Guardrails

- Do not mutate legacy flat-comment notification rows.
- Do not rebuild comment recipients from final thread state. Use message history up to each message.
- Do not delete rows created after the run's captured cutoff.
- Do not reset read rows to unread during repair.
- Do not use incremental counter math in this task.
- Do not auto-run this command from `cmd/api`.
- Do not treat the reconciliation command as a replacement for the normal projector path.
- Do not fail the repair run because invalidation publish failed after a successful repair write.
