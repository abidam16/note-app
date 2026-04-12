# Task 23 Mention Notification Projector Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Project explicit mention targets from thread outbox events into append-only `mention` notifications for mentioned users only, with retry-safe idempotent behavior.

**Architecture:** This task does not introduce a second competing outbox consumer. The outbox foundation is single-consumer, so mention projection must extend the existing thread-event projection path from Task 19 rather than claim `thread_created` and `thread_reply_created` in a separate worker. The application layer adds a focused mention projection component that maps validated thread-event payloads into `mention` notifications, filters recipients against current workspace membership, and inserts mention rows idempotently. The existing thread-event projector marks an outbox row processed only after both comment and mention projection work for that event finishes successfully or is intentionally skipped.

**Tech Stack:** Go, PostgreSQL, `pgx`, SQL-backed repositories, application services, PostgreSQL-backed repository tests, Go unit tests

---

## 1. Scope

### In Scope
- Add mention-notification projection logic for thread events
- Consume explicit mention metadata from these existing outbox topics:
  - `thread_created`
  - `thread_reply_created`
- Create one append-only `mention` notification row per explicitly mentioned recipient and message event
- Filter mention recipients against current workspace membership
- Exclude the acting user from mention notifications
- Keep mention notification inserts idempotent under retries
- Ensure unread counters increase only for newly inserted mention rows
- Extend the existing thread-event projector path so one claimed outbox event can produce:
  - zero or more `comment` notifications
  - zero or more `mention` notifications
- Add application, repository, and orchestration tests
- Update API contract behavior notes and checkpoint

### Out Of Scope
- No new public HTTP endpoint
- No new outbox topic such as `mention_created`
- No change to `POST /api/v1/pages/{pageID}/threads`
- No change to `POST /api/v1/threads/{threadID}/replies`
- No invitation notification changes
- No thread notification preference or mute logic
- No inbox UI deduplication or grouping
- No background worker bootstrap in `cmd/api`

### Prerequisites
- Task 10 outbox foundation exists
- Task 12 inbox v2 API exists
- Task 14 unread-counter support exists
- Task 19 comment notification projector exists
- Task 20 mention schema exists
- Task 21 thread-create mention support exists
- Task 22 thread-reply mention support exists

---

## 2. Detailed Spec

## 2.1 Objective

Comment notifications and mention notifications serve different product purposes:
- `comment` means a relevant discussion event happened
- `mention` means the actor explicitly targeted the user in a message

After this task, a thread message with mentions may produce:
- regular `comment` notifications for relevant users from Task 19
- separate `mention` notifications for explicitly mentioned users

These streams are intentionally independent. If a user is both a relevant comment recipient and an explicit mention target for the same message, the inbox may contain:
- one `comment` notification
- one `mention` notification

This task adds only the mention projection path. It does not change request-path write behavior.

## 2.2 Public API Impact

### New Endpoints
- None

### Existing Endpoints With Data Population Change
- `GET /api/v1/notifications`
- `GET /api/v1/notifications/unread-count`

### Public Request Payload Changes
- None

### Public Response Shape Changes
- None

### Public Behavior Change
After this task, inbox queries may return `type = mention` rows populated asynchronously from thread outbox events.

Examples:
- `GET /api/v1/notifications?type=mention`
- `GET /api/v1/notifications?type=all`

No request-path thread endpoint should write mention notifications directly.

## 2.3 Event Source Rules

This task does **not** add a new outbox topic.

Mention projection uses existing thread event topics:
- `thread_created`
- `thread_reply_created`

Mention recipient data comes from the `mention_user_ids` field already added to those event payloads by Tasks 21 and 22.

### Single-Consumer Rule
Because outbox rows are single-consumer:
- do not build a second worker that separately claims `thread_created` and `thread_reply_created`
- do not mark a thread event processed before mention projection finishes

Implementation rule:
- extend the Task 19 thread-event projector so one claimed outbox event can run both projections before the outbox row transitions to `processed`

## 2.4 Thread Event Payload Contract

Mention projection expects the already validated thread-event payload shape from Task 19:
- `thread_id`
- `message_id`
- `page_id`
- `workspace_id`
- `actor_id`
- `occurred_at`
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

### Payload Rules
1. `mention_user_ids` should be present after Tasks 21 and 22
2. for backward compatibility, omitted `mention_user_ids` must be treated as an empty list
3. if present, `mention_user_ids` must be a JSON array of strings
4. blank mention ids inside the payload must be ignored during recipient filtering

### Permanent Payload Error Rule
If `mention_user_ids` is present but not a JSON array of strings:
- the event payload is malformed
- the shared thread-event projector must dead-letter the outbox row

## 2.5 Mention Recipient Rules

Mention notifications are driven only by explicit mention targets in the payload.

### Recipient Resolution Inputs
- `WorkspaceID = payload.workspace_id`
- `ActorID = payload.actor_id`
- `MentionUserIDs = payload.mention_user_ids`

### Recipient Filtering Rules
1. trim each candidate id
2. ignore blank ids
3. dedupe repeated ids while preserving first-seen order
4. exclude `actor_id`
5. load current workspace members
6. keep only ids that are current active workspace members

### Current-Membership Rule
Use current membership at projection time as the delivery gate.

If a user was mentioned when the message was written but is no longer a workspace member when projection runs:
- skip that recipient
- do not dead-letter the whole event

Rationale:
- this is a delivery authorization check, not source-of-truth corruption

### Empty Recipient Rule
If no mention recipients remain after filtering:
- mention projection is a no-op
- the shared thread-event projector may still create comment notifications for the same event
- the outbox row is still eligible to complete successfully

## 2.6 Mention Notification Identity And Idempotency Rules

Mention notifications are append-only. Each recipient gets at most one mention notification per message event.

### Notification Identity
Use the message id as the append-only event identity:
- `event_id = payload.message_id`
- `resource_type = thread_message`
- `resource_id = payload.message_id`

### Unique Key Rule
Repository inserts must enforce uniqueness across:
- `user_id`
- `type = mention`
- `event_id = message_id`

The implementation may use:
- an explicit unique index introduced by Task 9, or
- `INSERT ... ON CONFLICT DO NOTHING` on an equivalent unique constraint

### Idempotency Rule
If the same outbox event is retried:
- no duplicate `mention` notification rows may be created
- unread counters must increase only for rows inserted for the first time

### Cross-Type Coexistence Rule
Do not dedupe across `comment` and `mention`.

For the same user and message:
- one `comment` notification may exist
- one `mention` notification may exist

This is valid because the two rows have different `type` values and different product meaning.

## 2.7 Notification Field Mapping

### Common Fields
For every projected mention notification:
- `user_id = recipient user id`
- `workspace_id = payload.workspace_id`
- `type = mention`
- `event_id = payload.message_id`
- `actor_id = payload.actor_id`
- `title` and `content` depend on the thread event topic
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
  - `mention_source = explicit`
- `created_at = payload.occurred_at`
- `updated_at = payload.occurred_at`

### `thread_created` Mapping
Set:
- `title = "Mentioned in a new comment thread"`
- `content = "You were mentioned in a new comment thread"`
- `payload.event_topic = "thread_created"`

### `thread_reply_created` Mapping
Set:
- `title = "Mentioned in a thread reply"`
- `content = "You were mentioned in a thread reply"`
- `payload.event_topic = "thread_reply_created"`

### Message Field Compatibility
If legacy storage still requires `message`:
- set `message = content`

## 2.8 Projection Orchestration Rules

This task must integrate with the existing thread-event projector from Task 19.

Recommended orchestration order for one claimed event:
1. validate and parse payload once
2. run comment projection
3. run mention projection
4. mark the outbox row `processed` only after both succeed or intentionally skip

### Retry Safety Rule
If comment projection succeeds but mention projection fails transiently:
- do not mark the outbox row processed
- schedule retry
- rely on comment notification idempotency to avoid duplicate comment rows on retry
- rely on mention notification idempotency to avoid duplicate mention rows if partial progress occurred

### Permanent Error Rule
Permanent mention-projection errors should dead-letter the same outbox row only when they mean the shared event payload is malformed, such as:
- invalid `mention_user_ids` shape

Skipping all mention recipients because none are eligible is not an error.

## 2.9 Mention Notification Repository Behavior

Add append-only insert support for mention notifications.

Recommended repository method:

```go
CreateMentionNotifications(ctx context.Context, notifications []domain.Notification) (inserted int, err error)
```

### Insert Requirements
1. all input rows must have:
   - `type = mention`
   - `event_id != ""`
   - `user_id != ""`
2. empty input list is valid and returns `0, nil`
3. inserts must be idempotent under the mention unique-key rule
4. only newly inserted rows may increment unread counters
5. duplicate rows caused by retries must be ignored, not treated as errors

### Shared-Helper Rule
If Task 19 already introduced `CreateCommentNotifications`:
- extract or reuse a private shared append-only batch-insert helper
- do not duplicate unread-counter logic for mention rows

### Transaction Rule
If mention inserts increment unread counters:
- the notification inserts and counter increments must happen in one repository transaction

## 2.10 Positive And Negative Cases

### Public Cases
No new HTTP endpoint is introduced in this task.

Visible public effect after successful projection:
- `GET /api/v1/notifications?type=mention` may return projected mention notifications

### Positive Projection Cases

1. `thread_created` event with two valid mention targets
- result: two `mention` notifications created

2. `thread_reply_created` event with one mention target who is also a prior replier
- result:
  - one `comment` notification may exist
  - one `mention` notification may exist

3. self-mention in the payload
- result: actor receives no mention notification

4. duplicate mention ids in the payload
- result: one mention notification per distinct recipient only

5. payload omits `mention_user_ids`
- result: mention projection skips cleanly

6. mentioned user is no longer a workspace member at projection time
- result: that recipient is skipped and the event still succeeds

7. retry of the same event after partial progress
- result: duplicate mention rows are not created

### Negative Projection Cases

1. `mention_user_ids` has invalid JSON shape
- result: outbox row dead-lettered

2. transient membership lookup failure
- result: outbox row scheduled for retry

3. transient mention repository write failure
- result: outbox row scheduled for retry

4. one event has malformed mention payload and another event in the same batch is valid
- result:
  - malformed row dead-lettered
  - valid row still processed

### Public Response Codes
- None directly for this task
- existing inbox endpoints keep their current response-code contracts

---

## 3. File Structure And Responsibilities

### Create
- `internal/application/mention_notification_projector.go`
  - mention recipient filtering, notification mapping, and mention-specific projection helper logic
- `internal/application/mention_notification_projector_test.go`
  - focused mention projection unit tests and recipient-filtering coverage

### Modify
- `internal/application/comment_notification_projector.go`
  - extend the shared thread-event processing path so it invokes mention projection before marking the outbox row processed
- `internal/application/comment_notification_projector_test.go`
  - add orchestration tests to prove comment and mention projection share one claimed event safely
- `internal/repository/postgres/notification_repository.go`
  - add idempotent append-only insert support for mention notifications, ideally through a shared helper
- `internal/repository/postgres/content_repository_test.go`
  - add database-backed mention notification insert coverage and coexistence coverage with comment rows
- `internal/repository/postgres/closed_pool_errors_test.go`
  - add closed-pool coverage for mention notification inserts
- `frontend-repo/API_CONTRACT.md`
  - document mention notification population behavior and coexistence with comment notifications
- `docs/checkpoint.md`

### Modify If Needed
- `internal/domain/notification.go`
  - only if Task 9 implementation does not yet define `NotificationTypeMention`

### Files Explicitly Not In Scope
- `internal/transport/http/handlers.go`
- `internal/transport/http/server.go`
- `internal/application/thread_service.go`
- `internal/application/workspace_service.go`
- `internal/repository/postgres/outbox_repository.go`
- `cmd/api/app.go`

---

## 4. Test Matrix

## 4.1 Mention Projector Unit Tests

Add focused tests in:
- `internal/application/mention_notification_projector_test.go`

### Positive Cases

1. valid mention payload creates one notification per distinct mentioned current member

2. actor id in `mention_user_ids` is filtered out

3. duplicate mention ids are deduped in first-seen order

4. omitted `mention_user_ids` returns zero mention notifications without error

5. mentioned users who are no longer workspace members are skipped

6. `thread_created` mention mapping is correct
- expect:
  - `type = mention`
  - `event_id = message_id`
  - `title = "Mentioned in a new comment thread"`
  - `payload.event_topic = "thread_created"`

7. `thread_reply_created` mention mapping is correct
- expect:
  - `title = "Mentioned in a thread reply"`
  - `payload.event_topic = "thread_reply_created"`

### Negative Cases

8. invalid `mention_user_ids` shape returns permanent error classification

9. transient membership lookup failure returns retry classification

10. transient repository insert failure returns retry classification

## 4.2 Shared Thread-Event Projector Orchestration Tests

Add or update tests in:
- `internal/application/comment_notification_projector_test.go`

### Positive Cases

11. one claimed thread event can produce both comment and mention notifications before being marked processed

12. event with no mention recipients still processes comment projection successfully

13. event with only mention recipients and no comment recipients still processes successfully

### Negative Cases

14. comment projection success plus transient mention projection failure causes retry, not processed

15. duplicate retry after prior successful comment insert does not create duplicate comment or mention rows

16. malformed `mention_user_ids` in payload dead-letters the outbox row

## 4.3 Notification Repository Integration Tests

Add DB-backed tests in:
- `internal/repository/postgres/content_repository_test.go`
- `internal/repository/postgres/closed_pool_errors_test.go`

### Positive Cases

17. `CreateMentionNotifications` inserts multiple distinct rows

18. duplicate input rows for the same `(user_id, type=mention, event_id)` do not create duplicates

19. repeated repository call with the same mention rows inserts zero new rows

20. same user and same message may store:
- one `comment` notification
- one `mention` notification

21. unread counters increase only for newly inserted mention rows

22. inserted mention rows are queryable through the inbox storage shape from Task 12

### Negative Cases

23. invalid mention notification input with blank `user_id` returns validation error

24. invalid mention notification input with blank `event_id` returns validation error

25. closed-pool mention insert returns error

## 4.4 Documentation Tests

26. `frontend-repo/API_CONTRACT.md` documents:
- mention notifications are produced asynchronously from thread outbox events
- inbox rows may use `type = mention`
- mention notification payload includes `workspace_id`, `page_id`, `thread_id`, `message_id`, and `event_topic`
- mention notifications target explicit mention recipients only
- a user may receive both a `comment` and a `mention` notification for the same message

27. `docs/checkpoint.md` records:
- mention projection exists on top of thread-event outbox processing
- thread events may now populate both comment and mention inbox rows
- mention notifications are append-only and idempotent

---

## 5. Execution Plan

### Task 1: Define failing mention projection unit tests

**Files:**
- Create: `internal/application/mention_notification_projector_test.go`

- [ ] **Step 1: Add failing tests for recipient filtering and mapping**

Cover:
- dedupe
- actor exclusion
- current-member filtering
- topic-specific title and payload mapping

- [ ] **Step 2: Add failing tests for retry and permanent-error classification**

Cover:
- invalid `mention_user_ids`
- transient membership lookup failure
- transient repository insert failure

- [ ] **Step 3: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestMentionNotificationProjector" -count=1
```

Expected:
- FAIL because the mention projection helper does not exist yet

- [ ] **Step 4: Commit**

```bash
git add internal/application/mention_notification_projector_test.go
git commit -m "test: define mention notification projection behavior"
```

### Task 2: Add idempotent mention notification repository support

**Files:**
- Modify: `internal/repository/postgres/notification_repository.go`
- Modify: `internal/repository/postgres/content_repository_test.go`
- Modify: `internal/repository/postgres/closed_pool_errors_test.go`
- Modify if needed: `internal/domain/notification.go`

- [ ] **Step 1: Add failing repository tests for mention notification insert semantics**

Cover:
- insert many
- duplicate suppression
- coexistence with comment rows for the same user and message
- unread-counter increment only for newly inserted mention rows
- validation for blank `user_id` and blank `event_id`

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- FAIL because mention notification insert support does not exist yet

- [ ] **Step 3: Implement `CreateMentionNotifications`**

Requirements:
- validate input rows
- insert with idempotent conflict handling
- increment unread counters only for newly inserted rows
- allow `comment` and `mention` rows to coexist for the same `(user_id, event_id)`
- reuse shared append-only insert logic where practical

- [ ] **Step 4: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/repository/postgres/notification_repository.go internal/repository/postgres/content_repository_test.go internal/repository/postgres/closed_pool_errors_test.go internal/domain/notification.go
git commit -m "feat: add idempotent mention notification inserts"
```

### Task 3: Implement mention projection helper

**Files:**
- Create: `internal/application/mention_notification_projector.go`
- Modify: `internal/application/mention_notification_projector_test.go`

- [ ] **Step 1: Add mention projector interfaces and dependencies**

Required dependencies:
- workspace membership list reader
- idempotent mention notification insert repository

- [ ] **Step 2: Implement recipient filtering**

Requirements:
- trim ids
- drop blanks
- dedupe in first-seen order
- exclude actor
- keep only current workspace members

- [ ] **Step 3: Implement notification mapping**

Requirements:
- map both thread topics into `type = mention`
- use `message_id` as `event_id` and `resource_id`
- map title, content, and payload exactly as specified

- [ ] **Step 4: Re-run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestMentionNotificationProjector" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/mention_notification_projector.go internal/application/mention_notification_projector_test.go
git commit -m "feat: add mention notification projection helper"
```

### Task 4: Integrate mention projection into the shared thread-event projector

**Files:**
- Modify: `internal/application/comment_notification_projector.go`
- Modify: `internal/application/comment_notification_projector_test.go`

- [ ] **Step 1: Add failing orchestration tests**

Cover:
- one event produces both comment and mention rows
- transient mention failure causes retry after comment success
- malformed mention payload dead-letters the row

- [ ] **Step 2: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "Test(CommentNotificationProjector|MentionNotificationProjector)" -count=1
```

Expected:
- FAIL because the shared projector does not yet invoke mention projection

- [ ] **Step 3: Update the thread-event processing path**

Required behavior:
- parse payload once
- run comment projection
- run mention projection
- mark processed only after both paths succeed or skip
- continue to rely on idempotent inserts during retry

- [ ] **Step 4: Re-run targeted application tests**

Run:
```powershell
go test ./internal/application -run "Test(CommentNotificationProjector|MentionNotificationProjector)" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/comment_notification_projector.go internal/application/comment_notification_projector_test.go
git commit -m "feat: project mention notifications from thread events"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update inbox behavior notes for mention notifications**

Document:
- mention notifications are produced asynchronously from thread outbox events
- recipients are explicit mention targets only
- the inbox may now return `type = mention`
- mention payload fields include `workspace_id`, `page_id`, `thread_id`, `message_id`, and `event_topic`
- a user may receive both a `comment` and a `mention` row for the same message

- [ ] **Step 2: Update checkpoint**

Record:
- mention projection added
- thread events now populate mention inbox rows through the shared projector path
- append-only idempotent insert behavior for mention notifications

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: record mention notification projector behavior"
```

### Task 6: Full verification for Task 23

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "Test(CommentNotificationProjector|MentionNotificationProjector)" -count=1
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual projector sanity check if local database is available**

Verify:
- a thread event with two explicit mention targets creates two `mention` inbox rows
- replaying the same event does not create duplicate mention rows
- a recipient who is both a participant and a mention target can have one `comment` and one `mention` row for the same message
- unread counters increase only on first insert for each mention row

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify mention notification projector task"
```

---

## 6. Acceptance Criteria

Task 23 is complete only when all are true:
- explicit mention targets from thread outbox events project into append-only `mention` notification rows
- the acting user never receives a mention notification for their own message
- non-member mention targets are skipped safely at projection time
- repeated projector runs do not create duplicate mention rows
- unread counters increase only for newly inserted mention rows
- the shared thread-event projector does not mark events processed until both comment and mention projection work is done
- malformed mention payload shape dead-letters the outbox row
- transient membership and repository failures retry through the outbox foundation
- public inbox queries can surface `type = mention` rows with page, thread, and message payload metadata
- tests and docs cover the positive and negative cases above

## 7. Risks And Guardrails

- Do not create a second outbox worker that claims `thread_created` or `thread_reply_created`.
- Do not introduce a new `mention_created` outbox topic in this task.
- Do not cross-type dedupe `comment` and `mention` notifications.
- Do not notify the actor.
- Do not notify users who are no longer workspace members.
- Do not mark the outbox row processed before mention projection finishes.
- Do not increment unread counters for duplicate mention inserts during retry.

## 8. Follow-On Tasks

This plan prepares for:
- Task 24 `GET /api/v1/threads/{threadID}/notification-preference`
- Task 25 `PUT /api/v1/threads/{threadID}/notification-preference`
- later real-time inbox delivery work

Later product work may decide whether the inbox UI groups related `comment` and `mention` rows for the same message, but this task keeps the backend streams separate.
