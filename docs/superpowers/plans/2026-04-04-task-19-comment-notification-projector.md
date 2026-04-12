# Task 19 Comment Notification Projector Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Project `thread_created` and `thread_reply_created` outbox events into append-only `comment` notifications for relevant users only, with idempotent retry-safe behavior.

**Architecture:** This task adds a comment notification projector on top of the outbox foundation from Task 10 and the recipient policy from Task 16. The projector claims only thread-comment topics, validates the outbox payload, loads thread detail, resolves relevant recipients, and inserts one append-only notification row per recipient and message event. It treats malformed events as permanent dead-letter cases, retries transient repository failures, and relies on idempotent append-only notification inserts so duplicate projector runs do not create duplicate inbox rows or inflate unread counters.

**Tech Stack:** Go, PostgreSQL, `pgx`, SQL-backed repositories, application services, PostgreSQL-backed repository tests, Go unit tests

---

## 1. Scope

### In Scope
- Add the comment notification projector application service
- Consume only these outbox topics:
  - `thread_created`
  - `thread_reply_created`
- Validate and map thread event payloads into `comment` notifications
- Load thread detail and reuse the Task 16 recipient resolver
- Create one append-only notification row per recipient and message event
- Make append-only comment notification inserts idempotent under retries
- Ensure unread counters increase only for newly inserted rows
- Dead-letter malformed or unsupported thread-comment events
- Retry transient processing failures through the outbox foundation
- Add projector tests and repository integration tests
- Update API contract behavior notes and checkpoint

### Out Of Scope
- No new public HTTP endpoint
- No change to `POST /api/v1/pages/{pageID}/threads`
- No change to `POST /api/v1/threads/{threadID}/replies`
- No invitation notification changes
- No mention persistence yet
- No separate mention projector yet
- No background worker startup in `cmd/api`
- No notification preference or mute logic

### Prerequisites
- Task 10 outbox foundation exists
- Task 12 inbox v2 API exists
- Task 14 unread counter support exists
- Task 16 relevant-recipient resolver exists
- Task 17 thread-create outbox integration exists
- Task 18 thread-reply outbox integration exists

---

## 2. Detailed Spec

## 2.1 Objective

Thread create and reply endpoints should no longer produce notifications directly. They should only persist outbox events. This task makes those outbox events useful by projecting them into inbox rows.

Comment notifications are append-only. Each thread message event may create zero or more inbox rows:
- zero rows if there are no relevant recipients
- one row per distinct relevant recipient otherwise

Each notification must carry:
- who acted
- a stable title
- a stable content string
- read state from the notification row
- payload fields for page, thread, and message deep-linking

## 2.2 Public API Impact

### New Endpoints
- None

### Existing Endpoints With Data Population Change
- `GET /api/v1/notifications`

### Public Request Payload Changes
- None

### Public Response Shape Changes
- None

### Public Behavior Change
After this task, inbox queries may return `type = comment` notifications created asynchronously from:
- `thread_created`
- `thread_reply_created`

No request-path thread notification fanout is reintroduced in this task.

## 2.3 Comment Topics In Scope

The projector consumes only:
- `thread_created`
- `thread_reply_created`

It must not claim:
- `invitation_created`
- `invitation_updated`
- `invitation_accepted`
- `invitation_rejected`
- `invitation_cancelled`
- `mention_created`

If a non-comment topic somehow reaches this projector:
- treat it as a permanent error
- mark the outbox row `dead_letter`
- store a clear `last_error`

## 2.4 Thread Event Payload Contract

The projector expects the outbox payload to be a JSON object containing:
- `thread_id`
- `message_id`
- `page_id`
- `workspace_id`
- `actor_id`
- `occurred_at`

Optional future-compatible field:
- `mention_user_ids`

Example:

```json
{
  "thread_id": "thread-uuid",
  "message_id": "message-uuid",
  "page_id": "page-uuid",
  "workspace_id": "workspace-uuid",
  "actor_id": "user-uuid",
  "occurred_at": "2026-04-04T09:00:00Z",
  "mention_user_ids": ["user-2", "user-3"]
}
```

### Validation Rules
1. `thread_id` must be present and non-empty
2. `message_id` must be present and non-empty
3. `page_id` must be present and non-empty
4. `workspace_id` must be present and non-empty
5. `actor_id` must be present and non-empty
6. `occurred_at` must be present and parse as RFC3339
7. `mention_user_ids` may be omitted
8. if present, `mention_user_ids` must be a JSON array of strings
9. blank mention user ids must be ignored later by the recipient resolver

If any required field is missing or malformed:
- the event is permanently invalid
- mark the outbox row `dead_letter`

## 2.5 Thread Detail Load Rules

The projector must load canonical thread detail from the source-of-truth thread tables using `thread_id`.

Recommended repository dependency:

```go
type ThreadDetailReader interface {
    GetThread(ctx context.Context, threadID string) (domain.PageCommentThreadDetail, error)
}
```

### Thread Detail Rules
- load by `thread_id` from the outbox payload
- the returned detail must include:
  - `Thread`
  - `Messages`
  - `Events`
- for `thread_created`, the detail should include the starter message
- for `thread_reply_created`, the detail should include the newly created reply message

### Missing Thread Rule
If the thread detail cannot be found:
- treat the event as a permanent data-integrity error
- dead-letter the outbox row

Rationale:
- these events are produced from the same database transaction as thread writes
- missing source rows indicate corruption or manual data damage, not a user-correctable transient state

## 2.6 Recipient Resolution Rules

The projector must reuse the Task 16 resolver instead of embedding recipient logic directly.

Recommended dependency:

```go
type ThreadNotificationRecipientResolver interface {
    ResolveRecipients(ctx context.Context, input ResolveThreadNotificationRecipientsInput) ([]string, error)
}
```

Resolver input:
- `WorkspaceID = payload.workspace_id`
- `ActorID = payload.actor_id`
- `Detail = loaded thread detail`
- `ExplicitMentionUserIDs = payload.mention_user_ids` or empty list if omitted

### Recipient Output Rules
- zero recipients is valid
- actor must never be included
- duplicates must already be removed by the resolver
- only current workspace members may remain

### Empty Recipient Rule
If the resolver returns an empty recipient list:
- mark the outbox event `processed`
- create no notification rows
- return success for that event

## 2.7 Comment Notification Identity And Idempotency Rules

Comment notifications are append-only. Each recipient gets at most one notification per message event.

### Notification Identity
Use the message id as the append-only event identity for both topics:
- `event_id = payload.message_id`
- `resource_type = thread_message`
- `resource_id = payload.message_id`

This rule applies even for `thread_created`. The first thread message is still a message event, and the message id is the best stable dedupe key for notification inserts.

### Unique Key Rule
Repository inserts must enforce uniqueness across:
- `user_id`
- `type = comment`
- `event_id = message_id`

The implementation may use:
- an explicit unique index introduced in Task 9, or
- `INSERT ... ON CONFLICT DO NOTHING` on an existing equivalent unique constraint

### Idempotency Rule
If the same outbox event is processed twice:
- no duplicate `comment` notification rows may be created
- unread counters must increase only for rows inserted for the first time
- the outbox event may still be marked `processed` on the retry if notification rows already exist

## 2.8 Notification Field Mapping

### Common Fields
For every projected comment notification:
- `user_id = recipient user id`
- `workspace_id = payload.workspace_id`
- `type = comment`
- `event_id = payload.message_id`
- `actor_id = payload.actor_id`
- `title` and `content` depend on topic
- `is_read = FALSE`
- `read_at = NULL`
- `actionable = FALSE`
- `action_kind = NULL`
- `resource_type = thread_message`
- `resource_id = payload.message_id`
- `payload` must include:
  - `thread_id`
  - `message_id`
  - `page_id`
  - `workspace_id`
  - `event_topic`
- `created_at = payload.occurred_at`
- `updated_at = payload.occurred_at`

### `thread_created` Mapping
Set:
- `title = "New comment thread"`
- `content = "A new relevant comment thread was created"`
- `payload.event_topic = "thread_created"`

### `thread_reply_created` Mapping
Set:
- `title = "New thread reply"`
- `content = "A relevant comment thread has a new reply"`
- `payload.event_topic = "thread_reply_created"`

### Message Field Compatibility
If the storage model still carries legacy `message`:
- set `message = content`

Do not expose new public behavior through `message`. Public inbox clients should rely on `title`, `content`, `type`, `actor`, and `payload`.

## 2.9 Projector Processing Rules

Create an application service that processes batches of claimed comment events.

Recommended result type:

```go
type CommentNotificationProjectorResult struct {
    Claimed      int
    Processed    int
    Retried      int
    DeadLettered int
    Skipped      int
}
```

Recommended entrypoint:

```go
ProcessBatch(ctx context.Context, workerID string, limit int, leaseDuration time.Duration, now time.Time) (CommentNotificationProjectorResult, error)
```

### Batch Algorithm
1. claim only `thread_created` and `thread_reply_created`
2. for each claimed event:
   - validate topic and payload
   - load thread detail by `thread_id`
   - resolve recipients
   - if zero recipients:
     - mark processed
     - increment `Skipped`
   - else:
     - build one `comment` notification per recipient
     - insert notifications idempotently
     - mark processed
     - increment `Processed`
3. on permanent error:
   - mark dead-letter
   - increment `DeadLettered`
4. on transient error:
   - schedule retry with backoff
   - increment `Retried`
5. continue processing other claimed rows even if one row fails
6. return batch summary

### Backoff Rule
For transient retry scheduling:
- next delay = `min(30s * 2^(attempt_count-1), 15m)`

### Permanent Error Rules
These conditions must dead-letter immediately:
- unsupported topic
- malformed JSON payload
- payload is not a JSON object
- missing required payload field
- invalid `occurred_at`
- invalid `mention_user_ids` shape
- thread detail not found
- internal resolver validation error caused by malformed source data

### Transient Error Rules
These conditions should retry:
- database connectivity failure
- thread detail read failure other than not found
- recipient resolver membership lookup failure
- notification repository insert failure
- outbox state transition failure after notification insert attempt

## 2.10 Notification Repository Behavior After Task 19

Add idempotent append-only insert support for comment notifications.

Recommended method:

```go
CreateCommentNotifications(ctx context.Context, notifications []domain.Notification) (inserted int, err error)
```

### Insert Requirements
1. all input rows must have:
   - `type = comment`
   - `event_id != ""`
   - `user_id != ""`
2. empty input list is valid and returns `0, nil`
3. inserts must be idempotent under the unique key rule
4. only newly inserted rows may increment unread counters
5. duplicate rows caused by retries must be ignored, not treated as errors

### Transaction Rule
If this method increments unread counters:
- the notification inserts and counter increments must happen in one repository transaction

### Failure Rule
If the repository cannot tell which rows were newly inserted:
- do not guess
- return an error and let the projector retry

## 2.11 Positive And Negative Cases

### Public Cases
No new HTTP endpoint is introduced in this task.

Visible public effect after successful projection:
- `GET /api/v1/notifications?type=comment` may return projected comment notifications

### Positive Projection Cases

1. `thread_created` event with no relevant recipients
- result: outbox row processed
- notifications created: `0`

2. `thread_reply_created` event for a thread started by another user
- result: one comment notification created for the thread creator

3. `thread_reply_created` event with multiple prior repliers
- result: one notification per distinct relevant recipient

4. explicit mention ids present in the payload
- result: mentioned active members are included through the resolver

5. duplicate participants across thread creator, repliers, and mentions
- result: recipient receives one notification only

6. projector retries the same event after a partial failure
- result: duplicate notification rows are not created

### Negative Projection Cases

1. malformed payload
- result: outbox row dead-lettered

2. unsupported topic
- result: outbox row dead-lettered

3. thread row missing
- result: outbox row dead-lettered

4. transient repository write failure
- result: outbox row scheduled for retry

5. resolver membership read failure
- result: outbox row scheduled for retry

---

## 3. File Structure And Responsibilities

### Create
- `internal/application/comment_notification_projector.go`
  - comment projector service, payload validation, notification mapping, and batch processing
- `internal/application/comment_notification_projector_test.go`
  - projector unit tests and event-classification coverage

### Modify
- `internal/repository/postgres/notification_repository.go`
  - add idempotent append-only insert support for comment notifications
- `internal/repository/postgres/content_repository_test.go`
  - add database-backed notification insert coverage and projector-related repository assertions
- `internal/repository/postgres/closed_pool_errors_test.go`
  - add closed-pool error coverage for comment notification inserts
- `frontend-repo/API_CONTRACT.md`
  - document comment notification population behavior and payload fields in the inbox
- `docs/checkpoint.md`

### Modify If Needed
- `internal/repository/postgres/outbox_repository.go`
  - only if Task 11 topic-scoped claim support is not yet generic enough for thread-comment topics
- `internal/repository/postgres/outbox_repository_test.go`
  - only if outbox repository changes are required
- `internal/application/thread_notification_recipient_resolver.go`
  - only if a small interface extraction or comment-specific helper is required for reuse without changing resolver rules

### Files Explicitly Not In Scope
- `internal/transport/http/handlers.go`
- `internal/transport/http/server.go`
- `internal/application/thread_service.go`
- `internal/application/workspace_service.go`
- `cmd/api/app.go`

---

## 4. Test Matrix

## 4.1 Application Projector Tests

Add focused tests in:
- `internal/application/comment_notification_projector_test.go`

### Positive Cases

1. `thread_created` event with no recipients is processed successfully
- expect:
  - `Processed = 0`
  - `Skipped = 1`
  - no notification insert call

2. `thread_reply_created` event creates notifications for thread creator and prior repliers
- expect:
  - actor excluded
  - one notification per distinct recipient

3. optional `mention_user_ids` are forwarded to the resolver
- expect:
  - resolver receives the same ids from payload

4. notification mapping for `thread_created` is correct
- expect:
  - `type = comment`
  - `event_id = message_id`
  - `title = "New comment thread"`
  - `content = "A new relevant comment thread was created"`
  - `payload.event_topic = "thread_created"`

5. notification mapping for `thread_reply_created` is correct
- expect:
  - `title = "New thread reply"`
  - `content = "A relevant comment thread has a new reply"`
  - `payload.event_topic = "thread_reply_created"`

6. duplicate event processing remains idempotent when repository reports existing rows
- expect:
  - event still marked processed
  - no duplicate-insert error surfaced

### Negative Cases

7. malformed JSON payload is dead-lettered

8. missing `message_id` is dead-lettered

9. invalid `mention_user_ids` type is dead-lettered

10. unsupported topic is dead-lettered

11. thread detail not found is dead-lettered

12. transient thread-detail read failure retries

13. transient recipient resolver failure retries

14. transient notification insert failure retries

15. batch continues after one event dead-letters and another succeeds

## 4.2 Notification Repository Integration Tests

Add DB-backed tests in:
- `internal/repository/postgres/content_repository_test.go`
- `internal/repository/postgres/closed_pool_errors_test.go`

### Positive Cases

16. `CreateCommentNotifications` inserts multiple distinct rows

17. duplicate input rows for the same `(user_id, type, event_id)` do not create duplicates

18. repeated repository call with the same rows inserts zero new rows

19. unread counters increase only for newly inserted rows

20. inserted comment rows are queryable through the inbox list endpoint storage shape from Task 12

### Negative Cases

21. invalid notification input with blank `user_id` returns validation error

22. invalid notification input with blank `event_id` returns validation error

23. closed-pool insert returns error

## 4.3 Outbox Repository Tests

Only required if Task 19 changes outbox repository code.

### Positive Cases

24. topic-scoped claim supports `thread_created` and `thread_reply_created`

### Negative Cases

25. non-comment topics are not returned by comment-topic claims

## 4.4 Documentation Tests

26. `frontend-repo/API_CONTRACT.md` documents:
- comment notifications are delivered asynchronously from thread outbox events
- inbox rows use `type = comment`
- comment notification payload includes `page_id`, `thread_id`, and `message_id`
- comment notifications are delivered only to relevant users

27. `docs/checkpoint.md` records:
- comment projector exists
- thread create and reply events now populate the inbox asynchronously
- comment notifications are append-only and idempotent

---

## 5. Execution Plan

### Task 1: Define failing projector tests for comment event processing

**Files:**
- Create: `internal/application/comment_notification_projector_test.go`

- [ ] **Step 1: Add failing tests for zero-recipient and reply-recipient flows**

Cover:
- `thread_created` with no recipients
- `thread_reply_created` with multiple relevant recipients
- actor exclusion

- [ ] **Step 2: Add failing tests for payload validation and error classification**

Cover:
- malformed JSON
- missing required fields
- invalid `mention_user_ids`
- unsupported topic
- missing thread detail
- transient read and write failures

- [ ] **Step 3: Add failing tests for notification field mapping**

Cover:
- topic-specific title and content
- `event_id = message_id`
- payload contains page, thread, message, workspace, and topic

- [ ] **Step 4: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestCommentNotificationProjector" -count=1
```

Expected:
- FAIL because the projector does not exist yet

- [ ] **Step 5: Commit**

```bash
git add internal/application/comment_notification_projector_test.go
git commit -m "test: define comment notification projector behavior"
```

### Task 2: Add idempotent append-only repository support for comment notifications

**Files:**
- Modify: `internal/repository/postgres/notification_repository.go`
- Modify: `internal/repository/postgres/content_repository_test.go`
- Modify: `internal/repository/postgres/closed_pool_errors_test.go`

- [ ] **Step 1: Add failing repository tests for comment notification insert semantics**

Cover:
- insert many
- duplicate suppression
- unread counter increment only for inserted rows
- validation for blank `user_id` and blank `event_id`

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- FAIL because idempotent comment notification insert support does not exist yet

- [ ] **Step 3: Implement `CreateCommentNotifications`**

Requirements:
- validate input rows
- insert with idempotent conflict handling
- increment unread counters only for newly inserted rows
- keep inserts and counter updates atomic

- [ ] **Step 4: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/repository/postgres/notification_repository.go internal/repository/postgres/content_repository_test.go internal/repository/postgres/closed_pool_errors_test.go
git commit -m "feat: add idempotent comment notification inserts"
```

### Task 3: Implement the comment notification projector

**Files:**
- Create: `internal/application/comment_notification_projector.go`
- Modify: `internal/application/comment_notification_projector_test.go`
- Modify if needed: `internal/repository/postgres/outbox_repository.go`
- Modify if needed: `internal/repository/postgres/outbox_repository_test.go`

- [ ] **Step 1: Add projector interfaces and dependencies**

Required dependencies:
- topic-scoped outbox claim and state transition repository
- thread detail reader
- relevant-recipient resolver
- idempotent comment notification insert repository

- [ ] **Step 2: Implement payload parsing and validation**

Requirements:
- accept only `thread_created` and `thread_reply_created`
- require all mandatory payload fields
- treat `mention_user_ids` as optional

- [ ] **Step 3: Implement topic-specific notification mapping**

Requirements:
- map both topics into `type = comment`
- use `message_id` as `event_id` and `resource_id`
- map title, content, and payload exactly as specified

- [ ] **Step 4: Implement `ProcessBatch`**

Requirements:
- claim only thread-comment topics
- load thread detail
- resolve recipients
- skip cleanly when recipient list is empty
- insert notifications idempotently
- mark processed, retry, or dead-letter per classification rules
- continue after per-event failure

- [ ] **Step 5: Re-run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestCommentNotificationProjector" -count=1
```

Expected:
- PASS

- [ ] **Step 6: Re-run targeted outbox tests if repository changes were needed**

Run:
```powershell
go test ./internal/repository/postgres -run "TestOutboxRepository" -count=1
```

Expected:
- PASS

- [ ] **Step 7: Commit**

```bash
git add internal/application/comment_notification_projector.go internal/application/comment_notification_projector_test.go internal/repository/postgres/outbox_repository.go internal/repository/postgres/outbox_repository_test.go
git commit -m "feat: add comment notification projector"
```

### Task 4: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update inbox behavior notes for comment notifications**

Document:
- comment notifications are produced asynchronously from thread outbox events
- recipients are relevant users only
- the inbox may now return `type = comment`
- comment payload fields include `workspace_id`, `page_id`, `thread_id`, `message_id`, and `event_topic`

- [ ] **Step 2: Update checkpoint**

Record:
- comment projector added
- thread create and reply now populate inbox rows through the projector
- idempotent append-only insert behavior

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: record comment notification projector behavior"
```

### Task 5: Full verification for Task 19

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "TestCommentNotificationProjector" -count=1
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository|TestClosedPoolRepositories|TestOutboxRepository" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual projector sanity check if local database is available**

Verify:
- a `thread_created` event with no relevant users creates no inbox rows
- a `thread_reply_created` event with two prior participants creates two `comment` inbox rows
- replaying the same event does not create duplicate rows
- unread counters increase only on first insert

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify comment notification projector task"
```

---

## 6. Acceptance Criteria

Task 19 is complete only when all are true:
- the projector claims only `thread_created` and `thread_reply_created`
- it loads canonical thread detail and uses the Task 16 recipient resolver
- it creates append-only `comment` notifications only for relevant users
- actor is never notified
- duplicate projector runs do not create duplicate notification rows
- unread counters increase only for newly inserted rows
- malformed or unsupported events dead-letter
- transient read and write failures retry through the outbox foundation
- public inbox queries can surface `type = comment` rows with page, thread, and message payload metadata
- tests and docs cover the positive and negative cases above

## 7. Risks And Guardrails

- Do not reintroduce synchronous thread notification writes in request handlers.
- Do not create invitation-style live-row updates for comment notifications.
- Do not use `thread_id` as the notification dedupe key; use `message_id`.
- Do not notify the actor or non-members.
- Do not increment unread counters for duplicate inserts during retries.
- Do not fail the whole batch because one event is malformed.

## 8. Follow-On Tasks

This plan prepares for:
- Task 20 mention schema
- Task 21 thread-create mention support
- Task 22 thread-reply mention support
- Task 23 mention notification projector
