# Task 17 POST Page Threads Outbox Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Change `POST /api/v1/pages/{pageID}/threads` so successful thread creation writes a `thread_created` outbox event in the same transaction and no longer performs synchronous notification fanout in the request path.

**Architecture:** This task moves thread-create notification production from an in-request side effect to transactional outbox persistence. The application service still validates and builds the thread, but instead of calling `NotifyThreadCreated` after persistence, it constructs one `thread_created` outbox event and passes it into the repository create path. The PostgreSQL thread repository already owns the create transaction, so it becomes responsible for persisting the thread row, starter message, created-event row, and outbox event atomically. If outbox insert fails, the whole transaction rolls back and the endpoint returns an error without leaving a partially-created thread behind.

**Tech Stack:** Go, PostgreSQL, `pgx`, SQL repositories, transactional outbox, `net/http`, `chi`, table-driven tests, PostgreSQL-backed repository tests

---

## 1. Scope

### In Scope
- Change one existing endpoint's internal persistence behavior:
  - `POST /api/v1/pages/{pageID}/threads`
- Keep the current request and response contract for thread creation
- Create one outbox event with:
  - topic `thread_created`
  - aggregate type `thread`
  - aggregate id = thread id
- Persist the outbox event in the same database transaction as:
  - thread row
  - starter message row
  - thread `created` event row
- Remove direct `NotifyThreadCreated` fanout from the request path
- Add service, repository, and HTTP tests for the new behavior
- Update API docs and checkpoint

### Out Of Scope
- No outbox worker execution
- No comment notification projector yet
- No reply-path outbox integration yet
- No mention persistence yet
- No mention notification delivery yet
- No thread reply behavior change in this task
- No thread create request or response payload change

### Prerequisites
- Task 10 outbox foundation exists
- Task 16 relevant-recipient resolver policy is already defined, but this task does not yet project comment notifications from the outbox

---

## 2. Detailed Spec

## 2.1 Objective

Today, thread creation persists the thread successfully and then calls a synchronous notification publisher. If that publisher fails, the API returns an error even though the thread already exists.

This task fixes that reliability gap for thread creation:
- success means the thread and one outbox event are committed together
- failure means neither the thread nor the outbox event commits

## 2.2 Endpoint

### `POST /api/v1/pages/{pageID}/threads`

- Auth: yes
- Existing authorization and validation rules remain unchanged

This task does not change the public endpoint shape. It changes how notification work is recorded after a successful create.

## 2.3 Request Payload

Request JSON stays unchanged:

```json
{
  "body": "Please revise this line",
  "anchor": {
    "type": "block",
    "block_id": "uuid",
    "quoted_text": "hello",
    "quoted_block_text": "hello world"
  }
}
```

### Public Validation Rules
Keep all existing validation exactly as-is:
1. actor must be authenticated
2. actor must be able to see the page through workspace membership
3. `body` is required after trim
4. `anchor.type` must be `block`
5. `anchor.block_id` must exist in the current page draft
6. `anchor.quoted_text`, if provided, must match the anchored block
7. `anchor.quoted_block_text`, if provided, must match the anchored block

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
      "anchor": {
        "type": "block",
        "block_id": "uuid",
        "quoted_text": "hello",
        "quoted_block_text": "hello world"
      },
      "thread_state": "open",
      "anchor_state": "active",
      "created_by": "uuid",
      "created_at": "2026-04-04T08:00:00Z",
      "last_activity_at": "2026-04-04T08:00:00Z",
      "reply_count": 1
    },
    "messages": [
      {
        "id": "uuid",
        "thread_id": "uuid",
        "body": "Please revise this line",
        "created_by": "uuid",
        "created_at": "2026-04-04T08:00:00Z"
      }
    ],
    "events": [
      {
        "id": "uuid",
        "thread_id": "uuid",
        "type": "created",
        "actor_id": "uuid",
        "message_id": "uuid",
        "created_at": "2026-04-04T08:00:00Z"
      }
    ]
  }
}
```

### Response Rules
- success response payload is unchanged from current thread-create behavior
- outbox persistence is not exposed in the response body
- no notification rows are returned by this endpoint

## 2.5 Outbox Event Contract

On every successful thread create, persist exactly one outbox row with:

- `topic = thread_created`
- `aggregate_type = thread`
- `aggregate_id = thread.ID`
- `idempotency_key = "thread_created:" + thread.ID`
- `status = pending`
- `available_at = thread.CreatedAt`

### Payload

Payload must be a JSON object containing:

```json
{
  "thread_id": "uuid",
  "message_id": "uuid",
  "page_id": "uuid",
  "workspace_id": "uuid",
  "actor_id": "uuid",
  "occurred_at": "2026-04-04T08:00:00Z"
}
```

### Payload Rules
- `thread_id` equals the created thread id
- `message_id` equals the starter message id
- `page_id` equals the requested page id
- `workspace_id` equals the page workspace id
- `actor_id` equals the authenticated actor id
- `occurred_at` equals the thread create timestamp

## 2.6 Transaction Rules

The repository create path must commit these records together:
1. `page_comment_threads`
2. `page_comment_messages`
3. `page_comment_thread_events`
4. `outbox_events`

### Atomicity Rule
- if any insert fails:
  - rollback the transaction
  - return an error
  - no thread data may remain committed

### Failure Classification Rule
- validation failures before persistence still return the existing `422`
- repository or outbox persistence failure returns `500 internal_error`
- an unexpected outbox idempotency-key conflict for the newly generated thread id is treated as an internal failure, not as public `409`

## 2.7 Service Behavior Rules

### Thread Create Path
- service still validates page visibility, body, and anchor rules
- service still generates thread and starter message ids
- service must build the `thread_created` outbox event before repository create
- service must pass that event into the repository create path
- service must not call `NotifyThreadCreated` after repository success

### Thread Reply Path
- unchanged in this task
- `CreateReply` still uses the current synchronous notification publisher until Task 18

### Transition Rule
After this task:
- thread creation no longer depends on the synchronous thread notification publisher
- reply creation still does until Task 18

## 2.8 Public Positive And Negative Cases

### Positive Cases

1. Valid thread create request succeeds
- Result: `201`
- thread, starter message, created event, and outbox event are committed

2. Valid thread create request with no future recipients still succeeds
- Result: `201`
- outbox event is still written

### Negative Cases

1. Malformed JSON
- Result: `400 invalid_json`

2. Missing or invalid auth
- Result: existing `401 unauthorized`

3. Actor cannot access the page
- Result: existing `404 not_found`

4. Invalid thread body or anchor fields
- Result: existing `422 validation_failed`

5. Thread persistence fails
- Result: `500 internal_error`
- no thread data is committed

6. Outbox persistence fails during thread create transaction
- Result: `500 internal_error`
- no thread data is committed

---

## 3. File Structure And Responsibilities

### Modify
- `internal/application/thread_service.go`
  - build the `thread_created` outbox event and remove post-create synchronous publisher call
- `internal/application/thread_service_test.go`
  - replace the current create-thread notification-failure expectation with outbox-aware behavior
- `internal/application/notification_events.go`
  - adjust interfaces only if a dedicated thread-create outbox dependency is introduced
- `internal/repository/postgres/thread_repository.go`
  - persist the outbox event inside the existing create-thread transaction
- `internal/repository/postgres/outbox_repository.go`
  - add a transaction-aware insert helper only if needed to avoid duplicating outbox insert logic
- `internal/repository/postgres/content_repository_test.go`
  - add thread-create plus outbox integration coverage
- `internal/transport/http/server_test.go`
  - update create-thread failure expectations to reflect transactional outbox behavior
- `frontend-repo/API_CONTRACT.md`
  - update behavior notes for thread create notification delivery
- `docs/checkpoint.md`

### Modify If Needed For Shared Types
- `internal/domain/outbox.go`
  - only if `thread_created` constants or helpers are missing from Task 10 implementation

### Files Explicitly Not In Scope
- `internal/transport/http/handlers.go`
- `internal/transport/http/server.go`
  - route stays the same
- `internal/application/notification_service.go`
  - synchronous notification service is not used for thread create after this task
- `internal/application/comment_service.go`
- `internal/repository/postgres/notification_repository.go`

---

## 4. Test Matrix

## 4.1 Application Service Tests

Add or update tests in:
- `internal/application/thread_service_test.go`

### Positive Cases

1. `CreateThread` builds one `thread_created` outbox event and passes it to the repository create path

2. `CreateThread` no longer calls the synchronous notification publisher
- use a failing publisher and assert create still succeeds when repository succeeds

3. `CreateThread` keeps the existing success payload unchanged

### Negative Cases

4. Repository create failure propagates

5. Existing body and anchor validation failures still return validation errors before any repository call

## 4.2 Repository Integration Tests

Add DB-backed tests in:
- `internal/repository/postgres/content_repository_test.go`

### Positive Cases

6. Creating a thread inserts:
- one thread row
- one starter message row
- one thread created-event row
- one pending outbox event row

7. The created outbox event has:
- topic `thread_created`
- aggregate type `thread`
- aggregate id = thread id
- correct payload values
- stable idempotency key format

### Negative Cases

8. If outbox insert fails, thread create returns an error and no thread row is committed
- seed a duplicate outbox idempotency key or invalid outbox input through the repository seam

9. Closed pool still returns an error on thread create

## 4.3 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_test.go`

### Positive Cases

10. `POST /api/v1/pages/{pageID}/threads` still returns `201` with the existing payload shape

11. Thread create succeeds even when a failing synchronous notification publisher is injected
- because create no longer uses that publisher

### Negative Cases

12. Transactional create failure returns `500`

13. Existing `400`, `401`, `404`, and `422` cases remain unchanged

14. Reply endpoint still propagates notification publisher failure in this task
- this guards the intended stepwise transition before Task 18

## 4.4 Documentation Tests

15. `frontend-repo/API_CONTRACT.md` documents:
- thread create request and response remain unchanged
- thread create now records notification work via outbox
- notification delivery is asynchronous and no longer produced directly in the request path

16. `docs/checkpoint.md` records:
- thread create now writes `thread_created` outbox events transactionally
- synchronous thread-create notification fanout was removed
- reply path remains synchronous until Task 18

---

## 5. Execution Plan

### Task 1: Write failing service tests for thread-create outbox behavior

**Files:**
- Modify: `internal/application/thread_service_test.go`

- [ ] **Step 1: Add failing tests for outbox event construction**

Cover:
- correct topic
- correct aggregate id
- correct payload values

- [ ] **Step 2: Add failing test proving create no longer uses the synchronous publisher**

Use:
- a failing thread notification publisher
- a successful fake repository create path

Expected:
- create succeeds

- [ ] **Step 3: Re-run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestThreadServiceCreateThread" -count=1
```

Expected:
- FAIL because create currently still calls the synchronous publisher and has no outbox event path

- [ ] **Step 4: Commit**

```bash
git add internal/application/thread_service_test.go
git commit -m "test: define thread create outbox behavior"
```

### Task 2: Update thread service to build the outbox event

**Files:**
- Modify: `internal/application/thread_service.go`
- Modify if needed: `internal/application/notification_events.go`

- [ ] **Step 1: Extend the thread repository contract or create input to accept one outbox event**

Requirement:
- repository create path must receive the event to persist in the same transaction

- [ ] **Step 2: Build the `thread_created` outbox event in `CreateThread`**

Required fields:
- topic
- aggregate type
- aggregate id
- idempotency key
- payload
- timestamps

- [ ] **Step 3: Remove post-create `NotifyThreadCreated` call**

Required behavior:
- `CreateThread` no longer uses the synchronous publisher
- `CreateReply` remains unchanged

- [ ] **Step 4: Re-run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestThreadServiceCreateThread" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/thread_service.go internal/application/notification_events.go
git commit -m "feat: build thread create outbox event"
```

### Task 3: Persist the outbox event transactionally in the thread repository

**Files:**
- Modify: `internal/repository/postgres/thread_repository.go`
- Modify: `internal/repository/postgres/outbox_repository.go`
- Modify: `internal/repository/postgres/content_repository_test.go`

- [ ] **Step 1: Add failing repository integration tests**

Cover:
- outbox row created on successful thread create
- correct payload and topic values
- rollback on outbox insert failure

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositories" -count=1
```

Expected:
- FAIL because thread create does not yet persist an outbox row

- [ ] **Step 3: Add transaction-aware outbox insert support if needed**

Requirement:
- avoid a second independent transaction
- reuse existing outbox insert rules from Task 10 where practical

- [ ] **Step 4: Update `ThreadRepository.CreateThread` transaction**

Required behavior:
- insert thread
- insert starter message
- insert created event
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
git commit -m "feat: persist thread create outbox events transactionally"
```

### Task 4: Update HTTP tests for the new failure boundary

**Files:**
- Modify: `internal/transport/http/server_test.go`

- [ ] **Step 1: Replace the create-thread notification-failure expectation**

Required change:
- create-thread should no longer return `500` just because a synchronous publisher is failing

- [ ] **Step 2: Add or update transactional failure coverage**

Cover:
- repository create failure still maps to `500`
- reply path still maps publisher failure to `500` in this task

- [ ] **Step 3: Re-run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "TestThread(CreateEndpoint|EndpointsPropagateNotificationFailures)" -count=1
```

Expected:
- PASS

- [ ] **Step 4: Commit**

```bash
git add internal/transport/http/server_test.go
git commit -m "test: update thread create outbox http behavior"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update thread create behavior notes**

Document:
- thread create request and response unchanged
- thread create now records `thread_created` outbox work
- direct notification fanout was removed from the request path
- reply path remains synchronous until Task 18

- [ ] **Step 2: Update checkpoint**

Record:
- transactional outbox integration for thread create
- create-path notification failure no longer happens after persistence

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: update thread create outbox behavior"
```

### Task 6: Full verification for Task 17

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "TestThreadServiceCreateThread" -count=1
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositories" -count=1
go test ./internal/transport/http -run "TestThread(CreateEndpoint|EndpointsPropagateNotificationFailures)" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual API sanity check if local server is available**

Call:
```http
POST /api/v1/pages/{pageID}/threads
```

Verify:
- `201`
- thread detail response unchanged
- one `thread_created` outbox row exists
- no synchronous notification rows are created by the request itself

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify thread create outbox integration"
```

---

## 6. Acceptance Criteria

Task 17 is complete only when all are true:
- `POST /api/v1/pages/{pageID}/threads` keeps its current public contract
- successful thread create writes one `thread_created` outbox event
- thread row, starter message row, created-event row, and outbox row commit atomically
- outbox persistence failure rolls back thread creation fully
- thread create no longer calls the synchronous notification publisher
- thread reply behavior remains unchanged until Task 18
- service, repository, and HTTP tests cover outbox creation and the changed failure boundary
- docs and checkpoint reflect the asynchronous create-path notification behavior

## 7. Risks And Guardrails

- Do not leave thread data committed when outbox insert fails.
- Do not keep synchronous `NotifyThreadCreated` in the request path.
- Do not change thread create request or response payloads in this task.
- Do not modify reply-path behavior yet.
- Do not emit more than one outbox event per successful thread create.
- Keep the outbox payload aligned with Task 10's contract exactly.

## 8. Follow-On Tasks

This plan prepares for:
- Task 18 thread reply outbox integration
- Task 19 comment notification projector
