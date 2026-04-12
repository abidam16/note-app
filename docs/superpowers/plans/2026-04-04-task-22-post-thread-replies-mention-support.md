# Task 22 POST Thread Replies Mention Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend `POST /api/v1/threads/{threadID}/replies` so reply messages can persist explicit mentions and include those mention targets in the `thread_reply_created` outbox payload.

**Architecture:** This task adds mention-aware write behavior to the thread-reply path only. The transport layer accepts an optional `mentions` array, the application layer reuses the Task 21 mention normalization and workspace-member validation helper, and the PostgreSQL thread repository persists mention rows in the same transaction as the reply message, thread state update, lifecycle events, and reply outbox event. The public thread-detail response stays unchanged in this task; mention data is stored for later read and notification work rather than exposed immediately.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, SQL repositories, transactional outbox, table-driven tests, PostgreSQL-backed repository tests

---

## 1. Scope

### In Scope
- Extend one existing endpoint:
  - `POST /api/v1/threads/{threadID}/replies`
- Add optional `mentions` request field
- Normalize and validate mentioned user ids
- Persist mention rows for the reply message
- Extend the existing `thread_reply_created` outbox payload with mention user ids
- Keep the thread-reply response shape unchanged
- Add application, repository, and HTTP tests for positive and negative flows
- Update API contract and checkpoint

### Out Of Scope
- No change to `POST /api/v1/pages/{pageID}/threads`
- No change to `GET /api/v1/threads/{threadID}`
- No thread-detail mention read API yet
- No mention notification projector yet
- No mention-specific inbox API yet
- No legacy flat-comment mention support

### Prerequisites
- Task 18 thread-reply outbox integration exists
- Task 20 mention schema exists
- Task 21 create-thread mention helper exists and is reusable by reply logic

---

## 2. Detailed Spec

## 2.1 Objective

The frontend needs a way to declare explicit mention targets when posting a reply inside an existing thread. The backend must:
- validate those targets against the thread page workspace
- persist them reliably
- attach them to the existing `thread_reply_created` outbox event

This task does not expose mention metadata in the thread-detail response yet. It only writes canonical mention data.

## 2.2 Endpoint

### `POST /api/v1/threads/{threadID}/replies`

- Auth: yes
- Authorization: workspace member of the thread page, including `viewer`

This task extends the request payload only. The success response remains `PageCommentThreadDetail`.

## 2.3 Request Payload

Request JSON becomes:

```json
{
  "body": "Follow-up reply",
  "mentions": ["user-id-1", "user-id-2"]
}
```

### Request Field Rules
- `body` required after trim
- `mentions` is optional
- `mentions` may be omitted or `null`
- when present, `mentions` must be a JSON array of strings

### Mention Normalization Rules
Normalize in the application layer before persistence:
1. trim each user id
2. reject blank user ids after trim
3. dedupe repeated user ids
4. preserve first-seen order

Example:

Input:

```json
{
  "mentions": ["user-2", " user-3 ", "user-2"]
}
```

Normalized result:

```json
["user-2", "user-3"]
```

### Mention Limit Rule
- maximum `20` unique mentioned user ids per reply request

If the normalized unique mention count exceeds `20`:
- return `422 validation_failed`

### Unknown Field Rule
- request body must contain exactly one JSON object
- unknown fields are invalid
- malformed JSON or wrong field types return `400 invalid_json`

## 2.4 Validation Rules

### Existing Validation Rules
Keep all current reply validation:
1. actor must be authenticated
2. thread must exist
3. actor must be able to see the thread page through workspace membership
4. `body` is required after trim

### New Mention Validation Rules
1. `mentions` must decode as an array of strings if present
2. each mention id must be non-empty after trim
3. after normalization, unique mention count must be `<= 20`
4. every normalized mention id must belong to a current workspace member of the thread page workspace

### Membership Validation Rule
Use the thread page workspace as the authority:
- load the thread detail
- load the thread page visibility as today
- load current workspace members once
- build a set of valid member user ids
- every normalized mention id must exist in that set

If any normalized mention id is not a workspace member:
- return `422 validation_failed`

### Self-Mention Rule
- self-mention is allowed
- it is persisted if present
- later notification tasks remain responsible for suppressing self-notifications

## 2.5 Response Payload

Response `201` remains the current success envelope:

```json
{
  "data": {
    "thread": {
      "id": "thread-uuid",
      "page_id": "page-uuid",
      "anchor": {
        "type": "block",
        "block_id": "block-uuid",
        "quoted_text": "hello",
        "quoted_block_text": "hello world"
      },
      "thread_state": "open",
      "anchor_state": "active",
      "created_by": "user-id-1",
      "created_at": "2026-04-04T08:00:00Z",
      "last_activity_at": "2026-04-04T08:05:00Z",
      "reply_count": 2
    },
    "messages": [
      {
        "id": "message-1",
        "thread_id": "thread-uuid",
        "body": "Please revise this line",
        "created_by": "user-id-1",
        "created_at": "2026-04-04T08:00:00Z"
      },
      {
        "id": "message-2",
        "thread_id": "thread-uuid",
        "body": "Follow-up reply",
        "created_by": "user-id-2",
        "created_at": "2026-04-04T08:05:00Z"
      }
    ],
    "events": [
      {
        "id": "event-created",
        "thread_id": "thread-uuid",
        "type": "created",
        "actor_id": "user-id-1",
        "message_id": "message-1",
        "created_at": "2026-04-04T08:00:00Z"
      },
      {
        "id": "event-replied",
        "thread_id": "thread-uuid",
        "type": "replied",
        "actor_id": "user-id-2",
        "message_id": "message-2",
        "created_at": "2026-04-04T08:05:00Z"
      }
    ]
  }
}
```

### Response Rules
- no new mention fields are added in this task
- response still returns the full updated thread detail
- persisted mentions are internal data for later tasks

## 2.6 Persistence Rules

On success, the reply transaction must commit these records together:
1. updated `page_comment_threads` row
2. reply `page_comment_messages` row
3. zero or more `page_comment_message_mentions` rows for the reply message
4. optional `page_comment_thread_events` reopened row
5. `page_comment_thread_events` replied row
6. `outbox_events` row for `thread_reply_created`

### Atomicity Rule
If any insert or update fails:
- rollback the whole transaction
- return an error
- no thread update, reply message, mention row, lifecycle event, or outbox row may remain committed

### Mention Persistence Rule
Persist one row in `page_comment_message_mentions` for each normalized unique mention id:
- `message_id = replyMessage.ID`
- `mentioned_user_id = mentionUserID`

### Duplicate Safety Rule
Because the application already dedupes mention ids:
- the primary key conflict should never happen in normal operation
- if it does happen inside the transaction, treat it as an internal error and roll back

## 2.7 Outbox Event Contract Change

This task extends the existing `thread_reply_created` outbox payload from Task 18.

### Topic And Identity
Keep:
- `topic = thread_reply_created`
- `aggregate_type = thread_message`
- `aggregate_id = replyMessage.ID`
- `idempotency_key = "thread_reply_created:" + replyMessage.ID`

### Payload
Payload must now include:

```json
{
  "thread_id": "uuid",
  "message_id": "uuid",
  "page_id": "uuid",
  "workspace_id": "uuid",
  "actor_id": "uuid",
  "occurred_at": "2026-04-04T08:05:00Z",
  "mention_user_ids": ["user-id-1", "user-id-2"]
}
```

### Payload Rules
- `mention_user_ids` must always be present
- when no mentions are supplied, store `mention_user_ids: []`
- when mentions are supplied, store the normalized unique list in first-seen order

This keeps the outbox contract explicit and avoids requiring downstream projectors to infer mention data from the database at projection time.

## 2.8 Service Behavior Rules

### Thread Reply Path
After existing thread lookup, page visibility check, and body validation:
1. normalize `mentions`
2. validate normalized mention ids against workspace members
3. build the reply message object as before
4. build mention rows for the reply message
5. preserve the existing auto-reopen logic for resolved threads
6. build the `thread_reply_created` outbox payload including `mention_user_ids`
7. pass the updated thread, reply message, mentions, and outbox event into the repository reply path

### Reuse Rule
- do not duplicate mention normalization logic in the reply path
- reuse the helper introduced for Task 21

### Failure Classification
- invalid mention payload semantics return `422 validation_failed`
- repository persistence or outbox persistence failure returns `500 internal_error`

### No Synchronous Notification Rule
This task must preserve the Task 18 behavior:
- thread reply still does not call a synchronous notification publisher

## 2.9 Positive And Negative Cases

### Positive Cases

1. Valid reply request without mentions
- Result: `201`
- behavior unchanged except outbox payload now includes `mention_user_ids: []`

2. Valid reply request with one mention
- Result: `201`
- one mention row is inserted for the reply message

3. Valid reply request with duplicate mention ids
- Result: `201`
- duplicates are normalized away before persistence

4. Valid reply request with self-mention
- Result: `201`
- self mention is persisted

5. Valid reply on a resolved thread with mentions
- Result: `201`
- thread auto-reopens as today
- reply mention rows persist
- reopened and replied events still commit with the outbox row

### Negative Cases

1. Malformed JSON
- Result: `400 invalid_json`

2. `mentions` is not an array of strings
- Result: `400 invalid_json`

3. Unknown request field
- Result: `400 invalid_json`

4. Missing or invalid auth
- Result: existing `401 unauthorized`

5. Thread not found
- Result: existing `404 not_found`

6. Actor cannot access the thread page
- Result: existing `404 not_found`

7. Blank mention id after trim
- Result: `422 validation_failed`

8. Mentioned user is not a workspace member
- Result: `422 validation_failed`

9. More than `20` unique mention ids
- Result: `422 validation_failed`

10. Reply transaction fails while inserting mention rows or outbox event
- Result: `500 internal_error`
- no data is committed

---

## 3. File Structure And Responsibilities

### Modify
- `internal/application/thread_mentions.go`
  - reuse the existing mention helper for the reply path without duplicating logic
- `internal/application/thread_mentions_test.go`
  - extend helper tests only if additional reply-specific regression coverage is needed
- `internal/application/thread_service.go`
  - extend `CreateThreadReplyInput`, call mention normalization and validation, and build mention-aware reply persistence inputs
- `internal/application/thread_service_test.go`
  - add reply mention tests and update fakes for mention-aware repository input
- `internal/repository/postgres/thread_repository.go`
  - persist reply-message mention rows in the existing reply transaction
- `internal/repository/postgres/content_repository_test.go`
  - add DB-backed coverage for reply mention row persistence and rollback behavior
- `internal/repository/postgres/closed_pool_errors_test.go`
  - update reply signature coverage if needed
- `internal/transport/http/handlers.go`
  - accept and forward `mentions` on reply create
- `internal/transport/http/server_test.go`
  - add HTTP coverage for reply `mentions`
- `frontend-repo/API_CONTRACT.md`
  - document the new reply request field and validation rules
- `docs/checkpoint.md`

### Files Explicitly Not In Scope
- `internal/transport/http/server.go`
- `internal/application/comment_notification_projector.go`
- `internal/repository/postgres/notification_repository.go`
- `internal/application/notification_service.go`

---

## 4. Test Matrix

## 4.1 Mention Helper Regression Tests

Update only if needed in:
- `internal/application/thread_mentions_test.go`

### Positive Cases

1. Existing helper behavior still normalizes and dedupes ids in first-seen order

2. Existing helper behavior still permits self-mention

### Negative Cases

3. Existing helper behavior still rejects blank ids

4. Existing helper behavior still rejects more than `20` unique ids

5. Existing helper behavior still rejects non-member ids

## 4.2 Application Service Tests

Add or update tests in:
- `internal/application/thread_service_test.go`

### Positive Cases

6. `CreateReply` without mentions still succeeds and passes an empty mention list to the repository reply path

7. `CreateReply` with mentions normalizes and passes mention rows for the reply message

8. `CreateReply` includes normalized `mention_user_ids` in the `thread_reply_created` outbox payload

9. `CreateReply` keeps the public success payload unchanged

10. `CreateReply` on a resolved thread still auto-reopens and includes mention-aware outbox metadata

### Negative Cases

11. Blank mention id returns `domain.ErrValidation`

12. Non-member mention id returns `domain.ErrValidation`

13. Too many unique mentions returns `domain.ErrValidation`

14. Repository reply failure still propagates

15. Existing reply body validation failures still return validation errors before any repository call

## 4.3 Repository Integration Tests

Add DB-backed tests in:
- `internal/repository/postgres/content_repository_test.go`

### Positive Cases

16. Reply with two distinct mentions inserts:
- updated thread row
- one reply message row
- two mention rows for the reply message
- one replied event row
- one `thread_reply_created` outbox row

17. Reply on a resolved thread with mentions inserts:
- updated open thread state
- one reply message row
- mention rows for the reply message
- one reopened event row
- one replied event row
- one `thread_reply_created` outbox row

18. Mention rows reference the reply message id, not the starter message id

19. Outbox payload stores normalized `mention_user_ids`

### Negative Cases

20. If mention insert fails, reply creation rolls back completely
- no thread state update remains
- no reply message row remains
- no mention rows remain
- no new lifecycle events remain
- no outbox row remains

21. Closed pool still returns an error on reply create

## 4.4 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_test.go`

### Positive Cases

22. `POST /api/v1/threads/{threadID}/replies` accepts a valid `mentions` array and returns `201`

23. Duplicate mention ids in the request still return `201`
- behavior:
  - duplicates are normalized internally

24. Omitted `mentions` behaves the same as today

### Negative Cases

25. Non-array `mentions` returns `400 invalid_json`

26. Unknown field still returns `400 invalid_json`

27. Blank mention id returns `422 validation_failed`

28. Non-member mention id returns `422 validation_failed`

29. Too many unique mentions returns `422 validation_failed`

30. Non-member access to the thread page still returns the existing `404 not_found`

## 4.5 Documentation Tests

31. `frontend-repo/API_CONTRACT.md` documents:
- optional `mentions` array on reply create
- mention ids are user ids, not emails
- duplicate ids are normalized
- non-member mentions return `422 validation_failed`
- response shape remains unchanged

32. `docs/checkpoint.md` records:
- reply messages now persist explicit mention rows
- `thread_reply_created` outbox payload now includes `mention_user_ids`
- response DTO unchanged in this task

---

## 5. Execution Plan

### Task 1: Add failing reply mention service tests

**Files:**
- Modify: `internal/application/thread_service_test.go`

- [ ] **Step 1: Add failing `CreateReply` tests for mention-aware behavior**

Cover:
- reply without mentions passes an empty list
- reply with mentions normalizes and dedupes ids
- normalized `mention_user_ids` are included in the outbox payload
- auto-reopen behavior remains intact when replying to a resolved thread with mentions

- [ ] **Step 2: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestThreadServiceCreateReply" -count=1
```

Expected:
- FAIL because reply does not yet accept or validate mentions

- [ ] **Step 3: Commit**

```bash
git add internal/application/thread_service_test.go
git commit -m "test: define thread reply mention support behavior"
```

### Task 2: Implement mention-aware reply service wiring

**Files:**
- Modify: `internal/application/thread_mentions.go`
- Modify: `internal/application/thread_mentions_test.go`
- Modify: `internal/application/thread_service.go`
- Modify: `internal/application/thread_service_test.go`

- [ ] **Step 1: Reuse the existing mention helper for reply validation**

Required behavior:
- no duplicated normalization logic
- validate mentions against current workspace members
- preserve first-seen order

- [ ] **Step 2: Extend `CreateThreadReplyInput` with `Mentions []string`**

Requirement:
- no thread-detail response change in this task

- [ ] **Step 3: Update `CreateReply`**

Required behavior:
- validate mentions after thread visibility is known
- build reply-message mention rows
- add normalized mention ids to the existing `thread_reply_created` outbox payload
- preserve the current auto-reopen behavior

- [ ] **Step 4: Re-run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestThreadServiceCreateReply" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/thread_mentions.go internal/application/thread_mentions_test.go internal/application/thread_service.go internal/application/thread_service_test.go
git commit -m "feat: validate thread reply mentions"
```

### Task 3: Persist reply mention rows in the reply transaction

**Files:**
- Modify: `internal/repository/postgres/thread_repository.go`
- Modify: `internal/repository/postgres/content_repository_test.go`
- Modify: `internal/repository/postgres/closed_pool_errors_test.go`

- [ ] **Step 1: Add failing repository integration tests for reply mentions**

Cover:
- mention row insert for reply messages
- outbox payload `mention_user_ids`
- rollback when mention insert fails
- preserved auto-reopen behavior with mentions

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositories" -count=1
```

Expected:
- FAIL because reply does not yet persist mention rows

- [ ] **Step 3: Update the reply transaction**

Required behavior:
- update thread row
- insert reply message row
- insert zero or more reply mention rows
- insert optional reopened event row
- insert replied event row
- insert mention-aware outbox row
- commit only if all operations succeed

- [ ] **Step 4: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositories" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/repository/postgres/thread_repository.go internal/repository/postgres/content_repository_test.go internal/repository/postgres/closed_pool_errors_test.go
git commit -m "feat: persist reply message mentions"
```

### Task 4: Extend the HTTP request contract for reply mentions

**Files:**
- Modify: `internal/transport/http/handlers.go`
- Modify: `internal/transport/http/server_test.go`

- [ ] **Step 1: Add failing HTTP tests for the reply `mentions` request field**

Cover:
- valid mentions array
- non-array mentions
- blank mention id
- non-member mention id
- too many mentions

- [ ] **Step 2: Update `createThreadReplyRequest` and `handleCreateThreadReply`**

Required behavior:
- decode optional `mentions`
- forward them into `application.CreateThreadReplyInput`
- keep response shape unchanged

- [ ] **Step 3: Re-run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "TestThreadReplyEndpoint" -count=1
```

Expected:
- PASS

- [ ] **Step 4: Commit**

```bash
git add internal/transport/http/handlers.go internal/transport/http/server_test.go
git commit -m "feat: accept mentions on thread replies"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update the thread-reply contract**

Document:
- optional `mentions` array
- validation rules
- unchanged `201` response shape
- mention-aware `thread_reply_created` outbox behavior as backend note

- [ ] **Step 2: Update checkpoint**

Record:
- reply-message mention persistence
- mention-aware reply outbox payload
- no public response DTO change yet

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: record thread reply mention support"
```

### Task 6: Full verification for Task 22

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "TestThreadServiceCreateReply" -count=1
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositories" -count=1
go test ./internal/transport/http -run "TestThreadReplyEndpoint" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual API sanity check if local server is available**

Call:
```http
POST /api/v1/threads/{threadID}/replies
```

With:

```json
{
  "body": "Follow-up reply",
  "mentions": ["user-2", "user-3"]
}
```

Verify:
- `201`
- response shape unchanged
- two mention rows exist for the reply message
- `thread_reply_created` outbox payload includes `mention_user_ids`
- auto-reopen still works for resolved threads

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify thread reply mention support"
```

---

## 6. Acceptance Criteria

Task 22 is complete only when all are true:
- `POST /api/v1/threads/{threadID}/replies` accepts optional `mentions`
- mention ids are normalized, deduped, and validated against workspace members
- reply-message mention rows persist in the same transaction as reply creation
- `thread_reply_created` outbox payload includes normalized `mention_user_ids`
- thread-reply response shape remains unchanged
- malformed JSON still returns `400`
- semantic mention errors return `422`
- missing or inaccessible threads keep their existing `404` behavior
- repository rollback covers mention and outbox failures
- tests and docs cover the positive and negative cases above

## 7. Risks And Guardrails

- Do not change `GET /api/v1/threads/{threadID}` in this task.
- Do not expose mention metadata in `PageCommentThreadDetail` yet.
- Do not accept emails in `mentions`; they must be user ids.
- Do not silently keep blank mention ids.
- Do not reintroduce synchronous notification writes in the reply path.
- Reuse the Task 21 mention helper instead of creating a second normalization path.
- Keep mention normalization deterministic so later projector behavior is stable.

## 8. Follow-On Tasks

This plan prepares for:
- Task 23 mention notification projector

Task 23 should consume `mention_user_ids` directly from the outbox payload for both thread creation and thread reply events.
