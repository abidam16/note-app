# Task 11 Invitation Notification Projector Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Project invitation outbox events into one live notification row per invitation so the recipient sees the latest invitation state, correct actionability, and stable read status.

**Architecture:** This task adds an internal invitation projector that claims only invitation outbox topics, resolves the notification recipient from the invitation email, and upserts a single invitation notification row keyed by invitation id. The projector classifies permanent errors into dead-letter, retries transient failures with backoff, and preserves the notification's existing read state on updates so invitation churn does not create duplicate rows or inflate unread state. This task does not yet change the public inbox API and does not wire invitation producers into the outbox; it only builds the projector slice on top of Tasks 9 and 10.

**Tech Stack:** Go, PostgreSQL, `pgx`, SQL-backed repositories, application services, repository integration tests, Go unit tests

---

## 1. Scope

### In Scope
- Add the invitation notification projector application service
- Consume only invitation outbox topics
- Resolve recipient user by invitation email
- Upsert one live notification row per invitation
- Keep pending invitation notifications actionable
- Keep terminal invitation notifications non-actionable
- Preserve existing read state when updating a live notification
- Retry transient processing failures through the outbox foundation
- Dead-letter malformed or unsupported invitation events
- Add repository support for invitation live-row upsert
- Add tests for projector logic, upsert behavior, and topic-scoped claims
- Update checkpoint

### Out Of Scope
- No public HTTP endpoint
- No notification list API redesign yet
- No unread-count endpoint or counter table yet
- No invitation event producers yet
- No thread/comment projector yet
- No background worker startup in `cmd/api`
- No frontend changes

---

## 2. Detailed Spec

## 2.1 Objective

Invitation notifications are not append-only. The inbox should show one invitation card that reflects the latest invitation state:
- pending => actionable
- accepted => non-actionable
- rejected => non-actionable
- cancelled => non-actionable

This task adds the projector that turns invitation outbox events into that live notification row.

This task only builds the projector and repository support. It does not yet make invitation services emit outbox events and does not yet expose the v2 inbox API publicly.

## 2.2 Public API Impact

### New Endpoints
- None

### Existing Endpoints In Scope
- None

### Public Request Payload Changes
- None

### Public Response Payload Changes
- None intended for this task

### Public Validation Changes
- None

### Public Response Code Changes
- None

### Compatibility Rule
No public API behavior should change in this task. The current notification endpoints remain as they are until the inbox API task.

## 2.3 Invitation Topics In Scope

The projector consumes only these outbox topics:
- `invitation_created`
- `invitation_updated`
- `invitation_accepted`
- `invitation_rejected`
- `invitation_cancelled`

It must not claim:
- `thread_created`
- `thread_reply_created`
- `mention_created`

If a non-invitation topic somehow reaches the projector, treat it as a permanent error and dead-letter it.

## 2.4 Invitation Event Payload Contract

The projector expects the outbox payload to be a JSON object containing:
- `invitation_id`
- `workspace_id`
- `actor_id`
- `email`
- `role`
- `status`
- `version`
- `occurred_at`

### Validation Rules
1. `invitation_id` must be present and non-empty
2. `workspace_id` must be present and non-empty
3. `actor_id` must be present and non-empty
4. `email` must be present and non-empty
5. `role` must be one of `owner|editor|viewer`
6. `status` must be one of `pending|accepted|rejected|cancelled`
7. `version` must be greater than `0`
8. `occurred_at` must be present and parse as RFC3339 timestamp

If any rule fails:
- the event is permanently invalid
- mark the outbox row `dead_letter`
- store the validation reason in `last_error`

## 2.5 Recipient Resolution Rules

### Registered User Case
- lookup recipient by `email`
- if user exists:
  - project the live invitation notification for that user

### Unregistered User Case
- if `users.GetByEmail(email)` returns `domain.ErrNotFound`:
  - mark the outbox event `processed`
  - create no notification row

This matches current product behavior where invitations may target unregistered emails, but notifications are only deliverable to existing users.

### Future Recovery Note
If an invitee registers later and no new invitation event occurs, this task does not backfill their notification. That belongs to later reconciliation/rebuild work.

## 2.6 Live Notification Projection Rules

### Live-Row Identity
There must be exactly one invitation notification row per:
- `recipient_user_id`
- `invitation_id`

Use the unique invitation live-row foundation from Task 9:
- `type = 'invitation'`
- `resource_type = 'invitation'`
- `resource_id = invitation_id`

### Event Id Rule
For invitation live rows:
- `event_id` must always equal `invitation_id`

This keeps the compatibility field stable across invitation lifecycle updates.

### Read-State Preservation Rule
When updating an existing live invitation notification:
- preserve `is_read`
- preserve `read_at`
- do not reset to unread

This prevents unread inflation on every invitation update.

### Create Rule
If no live notification row exists for that recipient and invitation:
- create one row
- `created_at = occurred_at`
- `updated_at = occurred_at`
- `is_read = FALSE`
- `read_at = NULL`

### Update Rule
If a live notification row already exists:
- update mutable fields only
- keep:
  - `id`
  - `created_at`
  - `is_read`
  - `read_at`
- set `updated_at = occurred_at`

### Missing-Previous-Row Rule
If the first event ever seen for an invitation is terminal:
- create the live notification row directly in the terminal state

This makes the projection resilient to earlier missed events.

## 2.7 Notification Field Mapping

### Common Fields
For all projected invitation notifications:
- `user_id = resolved recipient user id`
- `workspace_id = payload.workspace_id`
- `type = invitation`
- `event_id = payload.invitation_id`
- `actor_id = payload.actor_id`
- `resource_type = invitation`
- `resource_id = payload.invitation_id`
- `payload` object must include:
  - `invitation_id`
  - `workspace_id`
  - `email`
  - `role`
  - `status`
  - `version`
  - `can_accept`
  - `can_reject`

### Pending Created Mapping
Topic: `invitation_created`

Set:
- `title = "Workspace invitation"`
- `content = "You have a new workspace invitation"`
- `message = "You have a new workspace invitation"`
- `actionable = TRUE`
- `action_kind = invitation_response`
- payload:
  - `status = pending`
  - `can_accept = true`
  - `can_reject = true`

### Pending Updated Mapping
Topic: `invitation_updated`

Set:
- `title = "Workspace invitation updated"`
- `content = "Your workspace invitation was updated"`
- `message = "Your workspace invitation was updated"`
- `actionable = TRUE`
- `action_kind = invitation_response`
- payload:
  - `status = pending`
  - `can_accept = true`
  - `can_reject = true`

### Accepted Mapping
Topic: `invitation_accepted`

Set:
- `title = "Invitation accepted"`
- `content = "You accepted the workspace invitation"`
- `message = "You accepted the workspace invitation"`
- `actionable = FALSE`
- `action_kind = NULL`
- payload:
  - `status = accepted`
  - `can_accept = false`
  - `can_reject = false`

### Rejected Mapping
Topic: `invitation_rejected`

Set:
- `title = "Invitation rejected"`
- `content = "You rejected the workspace invitation"`
- `message = "You rejected the workspace invitation"`
- `actionable = FALSE`
- `action_kind = NULL`
- payload:
  - `status = rejected`
  - `can_accept = false`
  - `can_reject = false`

### Cancelled Mapping
Topic: `invitation_cancelled`

Set:
- `title = "Invitation cancelled"`
- `content = "The workspace invitation was cancelled"`
- `message = "The workspace invitation was cancelled"`
- `actionable = FALSE`
- `action_kind = NULL`
- payload:
  - `status = cancelled`
  - `can_accept = false`
  - `can_reject = false`

## 2.8 Projector Processing Rules

Create an application service that processes batches of claimed invitation events.

Recommended method:

```go
type InvitationNotificationProjectorResult struct {
    Claimed      int
    Processed    int
    Retried      int
    DeadLettered int
    Skipped      int
}
```

Recommended entrypoint:

```go
ProcessBatch(ctx context.Context, workerID string, limit int, leaseDuration time.Duration, now time.Time) (InvitationNotificationProjectorResult, error)
```

### Batch Algorithm
1. claim only invitation topics from outbox
2. for each claimed event:
   - validate topic and payload
   - resolve recipient by email
   - if user not found:
     - mark processed
     - increment `Skipped`
   - else:
     - build live invitation notification projection
     - upsert live notification row
     - mark outbox event processed
     - increment `Processed`
3. on permanent validation error:
   - mark dead-letter
   - increment `DeadLettered`
4. on transient failure:
   - mark retry with backoff
   - increment `Retried`
5. return batch summary

### Backoff Rule
For transient retry scheduling:
- next delay = `min(30s * 2^(attempt_count-1), 15m)`

Use the event's incremented `attempt_count` when calculating backoff.

### Permanent Error Rules
These conditions must dead-letter immediately:
- unsupported topic
- malformed JSON payload
- payload is not a JSON object
- missing required payload field
- invalid invitation status
- invalid role
- invalid version

### Transient Error Rules
These conditions should retry:
- database connectivity error
- notification repository write failure
- user lookup failure other than `domain.ErrNotFound`
- outbox state update failure after a successful projection attempt

## 2.9 Notification Repository Behavior After Task 11

Add invitation live-row upsert support.

Recommended repository method:

```go
UpsertInvitationLive(ctx context.Context, notification domain.Notification) (domain.Notification, error)
```

### Upsert Requirements
1. target the live-row identity:
   - `user_id`
   - `type = invitation`
   - `resource_type = invitation`
   - `resource_id = invitation_id`
2. if existing row exists:
   - update mutable fields
   - preserve read state and `created_at`
3. if no row exists:
   - insert a new row
4. if insert races and unique conflict occurs:
   - retry the update path once

### Mutable Fields On Update
- `workspace_id`
- `actor_id`
- `title`
- `content`
- `message`
- `actionable`
- `action_kind`
- `payload`
- `updated_at`

### Immutable Or Preserved Fields On Update
- `id`
- `created_at`
- `is_read`
- `read_at`
- `event_id = invitation_id`
- `type = invitation`
- `resource_type = invitation`
- `resource_id = invitation_id`

## 2.10 Outbox Repository Behavior After Task 11

The projector needs topic-scoped claims. Extend the outbox repository if Task 10 only implemented generic claims.

Recommended repository method:

```go
ClaimPendingByTopics(ctx context.Context, workerID string, topics []domain.OutboxTopic, limit int, leaseDuration time.Duration, now time.Time) ([]domain.OutboxEvent, error)
```

### Topic-Scoped Claim Rules
- `topics` must be non-empty
- only the supplied topics may be claimed
- ordering and stale-lease reclaim behavior must match Task 10
- concurrent claimers must still not receive the same row

## 2.11 Positive And Negative Cases

### Public HTTP Cases
No HTTP cases are introduced in this task.

### Projection Cases

Positive:
- pending invitation event for registered user creates one actionable notification
- invitation update event updates the same row instead of creating a second row
- accepted event updates the same row to non-actionable terminal state
- rejected event updates the same row to non-actionable terminal state
- cancelled event updates the same row to non-actionable terminal state
- read invitation notification remains read after a later update event
- terminal event can create the first live row if no earlier row exists
- unregistered invitee event is processed with no notification row created

Negative:
- malformed invitation payload is dead-lettered
- unsupported topic is dead-lettered
- duplicate projector run for the same event remains idempotent
- transient repository failure schedules retry instead of dead-letter
- projector must not claim thread or mention topics

---

## 3. File Structure And Responsibilities

### Create
- `internal/application/invitation_notification_projector.go`
- `internal/application/invitation_notification_projector_test.go`

### Modify
- `internal/repository/postgres/notification_repository.go`
  - add invitation live-row upsert
- `internal/repository/postgres/outbox_repository.go`
  - add topic-scoped claim support if not already present
- `internal/repository/postgres/outbox_repository_test.go`
  - add topic-scoped claim coverage if the repository changes
- `internal/repository/postgres/content_repository_test.go`
  - or create focused notification integration coverage if that is cleaner
- `internal/repository/postgres/closed_pool_errors_test.go`
  - add closed-pool coverage for live-row upsert and topic-scoped claim
- `docs/checkpoint.md`

### Files Explicitly Not In Scope
- `cmd/api/app.go`
- `internal/transport/http/server.go`
- `internal/application/workspace_service.go`
- `internal/application/thread_service.go`
- `frontend-repo/API_CONTRACT.md`

---

## 4. Test Matrix

## 4.1 Application Projector Tests

### Positive Cases

1. `invitation_created` for registered user creates one live notification
- Expect:
  - `actionable = TRUE`
  - `action_kind = invitation_response`
  - payload `can_accept = true`
  - payload `can_reject = true`

2. `invitation_updated` updates the same notification row
- Expect:
  - same notification id
  - same `created_at`
  - updated title/content/message

3. `invitation_accepted` updates the same row to terminal state
- Expect:
  - `actionable = FALSE`
  - `action_kind = NULL`
  - payload status `accepted`

4. `invitation_rejected` updates the same row to terminal state

5. `invitation_cancelled` updates the same row to terminal state

6. existing read notification stays read after update
- Seed:
  - `is_read = TRUE`
  - `read_at != NULL`
- Expect:
  - still read after upsert

7. unregistered invitee event is processed with no notification created

8. terminal invitation event creates row when none exists yet

### Negative Cases

9. malformed payload is dead-lettered

10. unsupported topic is dead-lettered

11. transient user repository failure triggers retry

12. transient notification repository failure triggers retry

13. topic claim with no invitation events returns empty batch without error

## 4.2 Notification Repository Integration Tests

### Positive Cases

14. `UpsertInvitationLive` inserts a new invitation notification

15. `UpsertInvitationLive` updates existing invitation notification in place
- Expect:
  - same row id
  - same `created_at`
  - updated mutable fields

16. update path preserves read state and `read_at`

17. insert-on-terminal-state works when row does not yet exist

### Negative Cases

18. concurrent upsert attempts do not create duplicate invitation live rows

19. closed-pool live upsert returns error

## 4.3 Outbox Repository Integration Tests

### Positive Cases

20. topic-scoped claim returns only invitation topics

21. topic-scoped claim still respects ready ordering and `SKIP LOCKED`

22. stale-lease reclaim still works with topic filter

### Negative Cases

23. blank topic list returns validation error

24. closed-pool topic-scoped claim returns error

## 4.4 Documentation Tests

25. `docs/checkpoint.md` records:
- invitation projector exists
- one live row per invitation
- read-state preservation on update
- unregistered invitee events process without notification creation
- no public API change in this task

---

## 5. Execution Plan

### Task 1: Define failing projector tests and invitation projection mapping

**Files:**
- Create: `internal/application/invitation_notification_projector_test.go`

- [ ] **Step 1: Add failing projector tests for live-row creation and update**

Cover:
- pending create
- pending update
- accepted
- rejected
- cancelled
- read-state preservation

- [ ] **Step 2: Add failing projector tests for retry and dead-letter branches**

Cover:
- malformed payload
- unsupported topic
- transient user lookup failure
- transient notification upsert failure
- unregistered invitee skip

- [ ] **Step 3: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestInvitationNotificationProjector" -count=1
```

Expected:
- FAIL because the projector service does not exist yet

- [ ] **Step 4: Commit**

```bash
git add internal/application/invitation_notification_projector_test.go
git commit -m "test: define invitation notification projector behavior"
```

### Task 2: Add notification live-row upsert support

**Files:**
- Modify: `internal/repository/postgres/notification_repository.go`
- Modify: `internal/repository/postgres/content_repository_test.go`
- Modify: `internal/repository/postgres/closed_pool_errors_test.go`

- [ ] **Step 1: Add failing integration tests for `UpsertInvitationLive`**

Cover:
- insert
- update in place
- preserve read state
- no duplicate rows under repeated upsert

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- FAIL because live-row upsert does not exist yet

- [ ] **Step 3: Implement `UpsertInvitationLive`**

Recommended repository approach:
- try `UPDATE ... RETURNING` for the live-row key
- if no row updated:
  - try `INSERT`
- if insert hits the invitation live-row unique conflict:
  - retry the update path once

- [ ] **Step 4: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- PASS for live-row repository coverage

- [ ] **Step 5: Commit**

```bash
git add internal/repository/postgres/notification_repository.go internal/repository/postgres/content_repository_test.go internal/repository/postgres/closed_pool_errors_test.go
git commit -m "feat: add invitation live notification upsert"
```

### Task 3: Add topic-scoped outbox claim support if needed

**Files:**
- Modify: `internal/repository/postgres/outbox_repository.go`
- Modify: `internal/repository/postgres/outbox_repository_test.go`
- Modify: `internal/repository/postgres/closed_pool_errors_test.go`

- [ ] **Step 1: Add failing tests for invitation-topic claims**

Cover:
- invitation-only topic filter
- stale-lease reclaim with topic filter
- validation for empty topic list

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestOutboxRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- FAIL if topic-scoped claim does not exist yet

- [ ] **Step 3: Implement `ClaimPendingByTopics`**

Requirements:
- validate non-empty topics
- preserve Task 10 claim ordering and lease semantics
- use `FOR UPDATE SKIP LOCKED`

- [ ] **Step 4: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestOutboxRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/repository/postgres/outbox_repository.go internal/repository/postgres/outbox_repository_test.go internal/repository/postgres/closed_pool_errors_test.go
git commit -m "feat: add topic-scoped outbox claims"
```

### Task 4: Implement the invitation projector batch processor

**Files:**
- Create: `internal/application/invitation_notification_projector.go`
- Modify: `internal/application/invitation_notification_projector_test.go`

- [ ] **Step 1: Add projector interfaces**

Recommended dependencies:
- user lookup by email
- outbox claim + state transition repository
- invitation live notification upsert repository

- [ ] **Step 2: Implement payload validation and mapping**

Requirements:
- validate required payload fields
- build title/content/message/actionability by topic and status
- build payload with `can_accept` and `can_reject`

- [ ] **Step 3: Implement `ProcessBatch`**

Requirements:
- claim only invitation topics
- process each event independently
- mark processed on success
- dead-letter permanent errors
- retry transient errors with exponential backoff
- return batch summary

- [ ] **Step 4: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestInvitationNotificationProjector" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/invitation_notification_projector.go internal/application/invitation_notification_projector_test.go
git commit -m "feat: add invitation notification projector"
```

### Task 5: Update checkpoint documentation

**Files:**
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Record projector behavior**

Document:
- invitation outbox topics project into one live notification row
- updates preserve read state
- malformed events dead-letter
- unregistered invitee events are processed without notification creation
- no public API change in this task

- [ ] **Step 2: Commit**

```bash
git add docs/checkpoint.md
git commit -m "docs: record invitation notification projector"
```

### Task 6: Full verification for Task 11

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "TestInvitationNotificationProjector" -count=1
go test ./internal/repository/postgres -run "TestOutboxRepository|TestNotification|TestContentRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual projector sanity check if local database is available**

Verify:
- two invitation events for the same invitation produce one notification row
- updating a read invitation notification does not reset it to unread
- malformed payload ends in dead-letter

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify invitation notification projector task"
```

---

## 6. Acceptance Criteria

Task 11 is complete only when all are true:
- invitation outbox events project into one live notification row per invitation
- pending invitation notifications are actionable
- accepted, rejected, and cancelled invitation notifications are non-actionable
- repeated projector runs remain idempotent
- updates preserve the existing notification read state
- unregistered invitee events are processed without notification creation
- malformed invitation events dead-letter
- transient failures retry through the outbox foundation
- repository and application tests cover the positive and negative cases above
- checkpoint reflects the projector behavior
- no public API behavior changes in this task

## 7. Risks And Guardrails

- Do not create a second invitation notification row for updates.
- Do not reset `is_read` on invitation updates.
- Do not claim non-invitation topics in this projector.
- Do not crash the batch on one bad event; process events independently.
- Do not start a background loop in `cmd/api` in this task.

## 8. Follow-On Tasks

This plan prepares for:
- Task 12 notification inbox API redesign
- later invitation producer integration with the outbox
- later thread and mention projectors
