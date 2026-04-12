# Task 18 POST Thread Replies Outbox Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Change `POST /api/v1/threads/{threadID}/replies` so successful reply creation writes a `thread_reply_created` outbox event in the same transaction and no longer performs synchronous notification fanout in the request path.

**Architecture:** This task mirrors Task 17 for thread replies. The application service still validates the actor, thread visibility, and reply body, but instead of calling `NotifyThreadReplyCreated` after persistence, it constructs one `thread_reply_created` outbox event and passes it into the repository reply path. The PostgreSQL `AddReply` transaction already updates thread state, inserts the reply message, and records thread lifecycle events; this task extends that transaction so the outbox row is committed together with the reply, including any auto-reopen event. After this task, thread creation and thread replies both use transactional outbox persistence and no synchronous thread notification publisher remains in the request path.

**Tech Stack:** Go, PostgreSQL, `pgx`, SQL repositories, transactional outbox, `net/http`, `chi`, table-driven tests, PostgreSQL-backed repository tests

---

## 1. Scope

### In Scope
- Change one existing endpoint's internal persistence behavior:
  - `POST /api/v1/threads/{threadID}/replies`
- Keep the current request and response contract for thread reply
- Create one outbox event with:
  - topic `thread_reply_created`
  - aggregate type `thread_message`
  - aggregate id = reply message id
- Persist the outbox event in the same database transaction as:
  - thread state update
  - reply message row
  - reply thread event row
  - optional auto-reopen thread event row
- Remove direct `NotifyThreadReplyCreated` fanout from the request path
- Remove now-dead synchronous thread notification dependency from `ThreadService`
- Add service, repository, and HTTP tests for the new behavior
- Update API docs and checkpoint

### Out Of Scope
- No outbox worker execution
- No comment notification projector yet
- No mention persistence yet
- No mention notification delivery yet
- No request or response payload change for thread reply
- No thread resolve or reopen outbox work

### Prerequisites
- Task 10 outbox foundation exists
- Task 17 thread-create outbox integration exists

---

## 2. Detailed Spec

## 2.1 Objective

Today, reply creation persists the reply successfully and then calls a synchronous notification publisher. If that publisher fails, the API returns an error even though the reply already exists.

This task fixes that reliability gap for replies:
- success means the reply update and one outbox event are committed together
- failure means neither the reply changes nor the outbox event commits

## 2.2 Endpoint

### `POST /api/v1/threads/{threadID}/replies`

- Auth: yes
- Existing authorization and validation rules remain unchanged

This task does not change the public endpoint shape. It changes how notification work is recorded after a successful reply.

## 2.3 Request Payload

Request JSON stays unchanged:

```json
{
  "body": "Follow-up reply"
}
```

### Public Validation Rules
Keep all existing validation exactly as-is:
1. actor must be authenticated
2. actor must be able to see the thread's page through workspace membership
3. `body` is required after trim

### Request Shape Rules
- request body must contain exactly one JSON object
- unknown fields are invalid
- malformed JSON returns the existing `400 invalid_json`

## 2.4 Response Payload

Response `201` remains the existing thread-detail success envelope:

```json
{
  "data": {
    "thread": {
      "id": "uuid",
      "page_id": "uuid",
      "thread_state": "open",
      "anchor_state": "active",
      "created_by": "uuid",
      "created_at": "2026-04-04T08:00:00Z",
      "reopened_by": "uuid",
      "reopened_at": "2026-04-04T09:00:00Z",
      "last_activity_at": "2026-04-04T09:00:00Z",
      "reply_count": 2
    },
    "messages": [
      {
        "id": "message-1",
        "thread_id": "thread-1",
        "body": "Initial comment",
        "created_by": "uuid",
        "created_at": "2026-04-04T08:00:00Z"
      },
      {
        "id": "message-2",
        "thread_id": "thread-1",
        "body": "Follow-up reply",
        "created_by": "uuid",
        "created_at": "2026-04-04T09:00:00Z"
      }
    ],
    "events": [
      {
        "id": "event-replied",
        "thread_id": "thread-1",
        "type": "replied",
        "actor_id": "uuid",
        "message_id": "message-2",
        "created_at": "2026-04-04T09:00:00Z"
      }
    ]
  }
}
```

### Response Rules
- success response payload is unchanged from current thread-reply behavior
- auto-reopen behavior remains unchanged:
  - replying to a resolved thread reopens it
  - response still reflects reopened state
- outbox persistence is not exposed in the response body

## 2.5 Outbox Event Contract

On every successful reply create, persist exactly one outbox row with:

- `topic = thread_reply_created`
- `aggregate_type = thread_message`
- `aggregate_id = message.ID`
- `idempotency_key = "thread_reply_created:" + message.ID`
- `status = pending`
- `available_at = message.CreatedAt`

### Payload

Payload must be a JSON object containing:

```json
{
  "thread_id": "uuid",
  "message_id": "uuid",
  "page_id": "uuid",
  "workspace_id": "uuid",
  "actor_id": "uuid",
  "occurred_at": "2026-04-04T09:00:00Z"
}
```

### Payload Rules
- `thread_id` equals the replied thread id
- `message_id` equals the new reply message id
- `page_id` equals the thread page id
- `workspace_id` equals the page workspace id
- `actor_id` equals the authenticated actor id
- `occurred_at` equals the reply create timestamp

## 2.6 Transaction Rules

The repository reply path must commit these records together:
1. `page_comment_threads` update
2. `page_comment_messages` insert
3. `page_comment_thread_events` insert for `replied`
4. optional `page_comment_thread_events` insert for `reopened`
5. `outbox_events` insert

### Atomicity Rule
- if any step fails:
  - rollback the transaction
  - return an error
  - no reply message may remain committed
  - no partial thread state update may remain committed

### Auto-Reopen Atomicity Rule
If a reply auto-reopens a resolved thread:
- thread state change
- reopened event
- replied event
- reply message
- outbox event
must all commit together or all roll back together

### Failure Classification Rule
- validation failures before persistence still return the existing `422`
- repository or outbox persistence failure returns `500 internal_error`
- an unexpected outbox idempotency-key conflict for the newly generated reply id is treated as an internal failure, not as public `409`

## 2.7 Service Behavior Rules

### Thread Reply Path
- service still loads the thread first
- service still checks page visibility through the thread page
- service still validates trimmed reply body
- service still derives auto-reopen state exactly as before
- service must build the `thread_reply_created` outbox event before repository reply persistence
- service must pass that event into the repository reply path
- service must not call `NotifyThreadReplyCreated` after repository success

### Thread Create Path
- unchanged in this task
- create-path outbox behavior from Task 17 remains in place

### Cleanup Rule
After this task:
- `ThreadService` no longer needs a synchronous thread notification publisher dependency
- any now-unused thread notification publisher interface and dead thread notification service methods should be removed or clearly deprecated as part of the same cleanup

## 2.8 Public Positive And Negative Cases

### Positive Cases

1. Valid reply request succeeds
- Result: `201`
- reply changes and one outbox event are committed

2. Reply to a resolved thread succeeds
- Result: `201`
- auto-reopen behavior remains unchanged
- outbox event is still written once

### Negative Cases

1. Malformed JSON
- Result: `400 invalid_json`

2. Missing or invalid auth
- Result: existing `401 unauthorized`

3. Actor cannot access the thread page
- Result: existing `404 not_found`

4. Invalid reply body
- Result: existing `422 validation_failed`

5. Reply persistence fails
- Result: `500 internal_error`
- no reply changes are committed

6. Outbox persistence fails during reply transaction
- Result: `500 internal_error`
- no reply changes are committed

---

## 3. File Structure And Responsibilities

### Modify
- `internal/application/thread_service.go`
  - build the `thread_reply_created` outbox event and remove post-reply synchronous publisher call
- `internal/application/thread_service_test.go`
  - replace the current reply notification-failure expectation with outbox-aware behavior
- `internal/application/notification_events.go`
  - remove the now-unused `ThreadNotificationEventPublisher` interface if nothing still depends on it
- `internal/application/notification_service.go`
  - remove dead `NotifyThreadCreated` and `NotifyThreadReplyCreated` code if it has no remaining callers
- `internal/application/notification_service_test.go`
  - remove or replace dead synchronous thread notification tests if those methods are removed
- `internal/application/notification_service_additional_test.go`
  - remove or replace dead synchronous thread notification tests if those methods are removed
- `internal/repository/postgres/thread_repository.go`
  - persist the outbox event inside the existing reply transaction
- `internal/repository/postgres/outbox_repository.go`
  - add a transaction-aware insert helper only if needed
- `internal/repository/postgres/content_repository_test.go`
  - add thread-reply plus outbox integration coverage
- `internal/transport/http/server_test.go`
  - update reply failure expectations to reflect transactional outbox behavior
- `frontend-repo/API_CONTRACT.md`
  - update behavior notes for thread reply notification delivery
- `docs/checkpoint.md`

### Modify If Needed For Shared Types
- `internal/domain/outbox.go`
  - only if `thread_reply_created` constants or helpers are missing from Task 10 implementation

### Files Explicitly Not In Scope
- `internal/transport/http/handlers.go`
- `internal/transport/http/server.go`
  - route stays the same
- `internal/application/comment_service.go`
- `internal/repository/postgres/notification_repository.go`

---

## 4. Test Matrix

## 4.1 Application Service Tests

Add or update tests in:
- `internal/application/thread_service_test.go`

### Positive Cases

1. `CreateReply` builds one `thread_reply_created` outbox event and passes it to the repository reply path

2. `CreateReply` no longer calls the synchronous notification publisher
- use a failing publisher and assert reply still succeeds when repository succeeds

3. `CreateReply` keeps the existing success payload unchanged

4. `CreateReply` still auto-reopens resolved threads before persistence

### Negative Cases

5. Repository reply failure propagates

6. Existing reply-body validation failures still return validation errors before any repository call

## 4.2 Repository Integration Tests

Add DB-backed tests in:
- `internal/repository/postgres/content_repository_test.go`

### Positive Cases

7. Adding a reply inserts:
- one reply message row
- one `replied` thread event row
- one pending outbox event row

8. Reply to a resolved thread also persists:
- thread reopened state
- one `reopened` event row
- one `replied` event row
- one outbox event row

9. The created outbox event has:
- topic `thread_reply_created`
- aggregate type `thread_message`
- aggregate id = reply message id
- correct payload values
- stable idempotency key format

### Negative Cases

10. If outbox insert fails, reply create returns an error and no reply message is committed

11. If outbox insert fails during auto-reopen, thread state remains unchanged and no reopen event is committed

12. Closed pool still returns an error on reply create

## 4.3 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_test.go`

### Positive Cases

13. `POST /api/v1/threads/{threadID}/replies` still returns `201` with the existing payload shape

14. Reply succeeds even when a failing synchronous notification publisher is injected
- because reply no longer uses that publisher

15. Create-thread behavior remains unchanged from Task 17 while reply behavior now matches it

### Negative Cases

16. Transactional reply failure returns `500`

17. Existing `400`, `401`, `404`, and `422` cases remain unchanged

18. The old “reply notification publisher failure returns 500 after persistence” behavior is removed

## 4.4 Cleanup Tests

19. If `ThreadNotificationEventPublisher` is removed, the codebase compiles without it and no tests still depend on the old synchronous path

20. If dead notification-service thread methods are removed, no tests still reference them

## 4.5 Documentation Tests

21. `frontend-repo/API_CONTRACT.md` documents:
- thread reply request and response remain unchanged
- thread reply now records notification work via outbox
- notification delivery is asynchronous and no longer produced directly in the request path

22. `docs/checkpoint.md` records:
- thread reply now writes `thread_reply_created` outbox events transactionally
- synchronous thread-reply notification fanout was removed
- thread create and reply now both use outbox persistence

---

## 5. Execution Plan

### Task 1: Write failing service tests for thread-reply outbox behavior

**Files:**
- Modify: `internal/application/thread_service_test.go`

- [ ] **Step 1: Add failing tests for outbox event construction**

Cover:
- correct topic
- correct aggregate type
- correct payload values

- [ ] **Step 2: Add failing test proving reply no longer uses the synchronous publisher**

Use:
- a failing thread notification publisher
- a successful fake repository reply path

Expected:
- reply succeeds

- [ ] **Step 3: Re-run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestThreadServiceCreateReply" -count=1
```

Expected:
- FAIL because reply currently still calls the synchronous publisher and has no outbox event path

- [ ] **Step 4: Commit**

```bash
git add internal/application/thread_service_test.go
git commit -m "test: define thread reply outbox behavior"
```

### Task 2: Update thread service to build the reply outbox event

**Files:**
- Modify: `internal/application/thread_service.go`
- Modify: `internal/application/notification_events.go`

- [ ] **Step 1: Extend the thread repository reply contract to accept one outbox event**

Requirement:
- repository reply path must receive the event to persist in the same transaction

- [ ] **Step 2: Build the `thread_reply_created` outbox event in `CreateReply`**

Required fields:
- topic
- aggregate type
- aggregate id
- idempotency key
- payload
- timestamps

- [ ] **Step 3: Remove post-reply `NotifyThreadReplyCreated` call**

Required behavior:
- `CreateReply` no longer uses the synchronous publisher

- [ ] **Step 4: Remove dead synchronous thread notification dependency if no callers remain**

Required cleanup:
- remove `ThreadService.notifications` if unused
- remove `ThreadNotificationEventPublisher` if unused

- [ ] **Step 5: Re-run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestThreadServiceCreateReply" -count=1
```

Expected:
- PASS

- [ ] **Step 6: Commit**

```bash
git add internal/application/thread_service.go internal/application/notification_events.go
git commit -m "feat: build thread reply outbox event"
```

### Task 3: Persist the outbox event transactionally in the reply repository path

**Files:**
- Modify: `internal/repository/postgres/thread_repository.go`
- Modify: `internal/repository/postgres/outbox_repository.go`
- Modify: `internal/repository/postgres/content_repository_test.go`

- [ ] **Step 1: Add failing repository integration tests**

Cover:
- outbox row created on successful reply
- correct payload and topic values
- rollback on outbox insert failure
- rollback of auto-reopen state on outbox failure

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositories" -count=1
```

Expected:
- FAIL because reply does not yet persist an outbox row

- [ ] **Step 3: Add transaction-aware outbox insert support if needed**

Requirement:
- avoid a second independent transaction
- reuse existing outbox insert rules from Task 10 where practical

- [ ] **Step 4: Update `ThreadRepository.AddReply` transaction**

Required behavior:
- lock and update thread state
- insert reply message
- insert optional reopened event
- insert replied event
- insert outbox event
- commit only if all inserts succeed

- [ ] **Step 5: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositories" -count=1
```

Expected:
- PASS

- [ ] **Step 6: Commit**

```bash
git add internal/repository/postgres/thread_repository.go internal/repository/postgres/outbox_repository.go internal/repository/postgres/content_repository_test.go
git commit -m "feat: persist thread reply outbox events transactionally"
```

### Task 4: Remove old synchronous thread notification tests and update HTTP behavior

**Files:**
- Modify: `internal/application/notification_service.go`
- Modify: `internal/application/notification_service_test.go`
- Modify: `internal/application/notification_service_additional_test.go`
- Modify: `internal/transport/http/server_test.go`

- [ ] **Step 1: Remove dead synchronous thread notification methods and tests if they have no remaining callers**

Required cleanup:
- delete or replace dead code rather than carrying it forward unused

- [ ] **Step 2: Replace the reply notification-failure expectation**

Required change:
- reply should no longer return `500` just because a synchronous publisher is failing

- [ ] **Step 3: Add or update transactional failure coverage**

Cover:
- repository reply failure still maps to `500`
- both thread create and reply now ignore failing synchronous publishers

- [ ] **Step 4: Re-run targeted application and HTTP tests**

Run:
```powershell
go test ./internal/application -run "Test(NotificationService|ThreadServiceCreateReply)" -count=1
go test ./internal/transport/http -run "TestThread(CreateEndpoint|ReplyEndpoint|EndpointsPropagateNotificationFailures)" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/notification_service.go internal/application/notification_service_test.go internal/application/notification_service_additional_test.go internal/transport/http/server_test.go
git commit -m "test: update thread reply outbox http behavior"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update thread reply behavior notes**

Document:
- thread reply request and response unchanged
- thread reply now records `thread_reply_created` outbox work
- direct notification fanout was removed from the request path
- thread create and reply now both use outbox persistence

- [ ] **Step 2: Update checkpoint**

Record:
- transactional outbox integration for thread reply
- reply-path notification failure no longer happens after persistence
- synchronous thread notification path removed fully

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: update thread reply outbox behavior"
```

### Task 6: Full verification for Task 18

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "Test(NotificationService|ThreadServiceCreateReply)" -count=1
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositories" -count=1
go test ./internal/transport/http -run "TestThread(CreateEndpoint|ReplyEndpoint|EndpointsPropagateNotificationFailures)" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual API sanity check if local server is available**

Call:
```http
POST /api/v1/threads/{threadID}/replies
```

Verify:
- `201`
- thread detail response unchanged
- one `thread_reply_created` outbox row exists
- no synchronous notification rows are created by the request itself
- auto-reopen behavior still works when replying to a resolved thread

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify thread reply outbox integration"
```

---

## 6. Acceptance Criteria

Task 18 is complete only when all are true:
- `POST /api/v1/threads/{threadID}/replies` keeps its current public contract
- successful reply writes one `thread_reply_created` outbox event
- reply message, thread state update, reply event, optional reopen event, and outbox row commit atomically
- outbox persistence failure rolls back reply creation fully
- thread reply no longer calls the synchronous notification publisher
- no synchronous thread notification path remains in `ThreadService` after this task
- service, repository, and HTTP tests cover outbox creation and the changed failure boundary
- docs and checkpoint reflect the asynchronous reply-path notification behavior

## 7. Risks And Guardrails

- Do not leave reply data committed when outbox insert fails.
- Do not keep synchronous `NotifyThreadReplyCreated` in the request path.
- Do not change thread reply request or response payloads in this task.
- Do not break auto-reopen behavior while moving reply persistence.
- Do not emit more than one outbox event per successful reply.
- Keep the outbox payload aligned with Task 10's contract exactly.
- Remove dead synchronous thread notification code if it truly has no remaining callers.

## 8. Follow-On Tasks

This plan prepares for:
- Task 19 comment notification projector
- later mention-aware reply events
