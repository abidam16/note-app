# Task 21 POST Page Threads Mention Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend `POST /api/v1/pages/{pageID}/threads` so the starter message can persist explicit mentions and include those mention targets in the `thread_created` outbox payload.

**Architecture:** This task adds mention-aware write behavior to the thread-create path only. The transport layer accepts an optional `mentions` array, the application layer normalizes and validates mention targets against current workspace membership, and the PostgreSQL thread repository persists mention rows in the same transaction as the thread, starter message, lifecycle event, and outbox event. The public thread-detail response stays unchanged in this task; mention data is stored for later read and notification work rather than exposed immediately.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, SQL repositories, transactional outbox, table-driven tests, PostgreSQL-backed repository tests

---

## 1. Scope

### In Scope
- Extend one existing endpoint:
  - `POST /api/v1/pages/{pageID}/threads`
- Add optional `mentions` request field
- Normalize and validate mentioned user ids
- Persist mention rows for the starter message
- Extend the existing `thread_created` outbox payload with mention user ids
- Keep the thread-create response shape unchanged
- Add application, repository, and HTTP tests for positive and negative flows
- Update API contract and checkpoint

### Out Of Scope
- No change to `POST /api/v1/threads/{threadID}/replies`
- No change to `GET /api/v1/threads/{threadID}`
- No thread-detail mention read API yet
- No mention notification projector yet
- No mention-specific inbox API yet
- No legacy flat-comment mention support

### Prerequisites
- Task 17 thread-create outbox integration exists
- Task 20 mention schema exists

---

## 2. Detailed Spec

## 2.1 Objective

The frontend needs a way to declare explicit mention targets when creating the first message in a thread. The backend must:
- validate those targets against the page workspace
- persist them reliably
- attach them to the existing `thread_created` outbox event

This task does not expose mention metadata in the thread-detail response yet. It only writes canonical mention data.

## 2.2 Endpoint

### `POST /api/v1/pages/{pageID}/threads`

- Auth: yes
- Authorization: workspace member, including `viewer`

This task extends the request payload only. The success response remains `PageCommentThreadDetail`.

## 2.3 Request Payload

Request JSON becomes:

```json
{
  "body": "Please revise this line",
  "anchor": {
    "type": "block",
    "block_id": "uuid",
    "quoted_text": "hello",
    "quoted_block_text": "hello world"
  },
  "mentions": ["user-id-1", "user-id-2"]
}
```

### Request Field Rules
- `body` required after trim
- `anchor` rules remain unchanged from the current contract
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
- maximum `20` unique mentioned user ids per create-thread request

If the normalized unique mention count exceeds `20`:
- return `422 validation_failed`

### Unknown Field Rule
- request body must contain exactly one JSON object
- unknown fields are invalid
- malformed JSON or wrong field types return `400 invalid_json`

## 2.4 Validation Rules

### Existing Validation Rules
Keep all current create-thread validation:
1. actor must be authenticated
2. actor must be able to see the page through workspace membership
3. `body` is required after trim
4. `anchor.type` must be `block`
5. `anchor.block_id` must exist in the current page draft
6. `anchor.quoted_text`, if provided, must match the anchored block
7. `anchor.quoted_block_text`, if provided, must match the anchored block

### New Mention Validation Rules
1. `mentions` must decode as an array of strings if present
2. each mention id must be non-empty after trim
3. after normalization, unique mention count must be `<= 20`
4. every normalized mention id must belong to a current workspace member of the page workspace

### Membership Validation Rule
Use the page workspace as the authority:
- load workspace membership once
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
        "id": "message-uuid",
        "thread_id": "thread-uuid",
        "body": "Please revise this line",
        "created_by": "uuid",
        "created_at": "2026-04-04T08:00:00Z"
      }
    ],
    "events": [
      {
        "id": "event-uuid",
        "thread_id": "thread-uuid",
        "type": "created",
        "actor_id": "uuid",
        "message_id": "message-uuid",
        "created_at": "2026-04-04T08:00:00Z"
      }
    ]
  }
}
```

### Response Rules
- no new mention fields are added in this task
- response still returns the created thread, starter message, and created event only
- persisted mentions are internal data for later tasks

## 2.6 Persistence Rules

On success, the create-thread transaction must commit these records together:
1. `page_comment_threads` row
2. starter `page_comment_messages` row
3. zero or more `page_comment_message_mentions` rows for the starter message
4. `page_comment_thread_events` created row
5. `outbox_events` row for `thread_created`

### Atomicity Rule
If any insert fails:
- rollback the whole transaction
- return an error
- no thread, message, mention, lifecycle event, or outbox row may remain committed

### Mention Persistence Rule
Persist one row in `page_comment_message_mentions` for each normalized unique mention id:
- `message_id = starterMessage.ID`
- `mentioned_user_id = mentionUserID`

### Duplicate Safety Rule
Because the application already dedupes mention ids:
- the primary key conflict should never happen in normal operation
- if it does happen inside the transaction, treat it as an internal error and roll back

## 2.7 Outbox Event Contract Change

This task extends the existing `thread_created` outbox payload from Task 17.

### Topic And Identity
Keep:
- `topic = thread_created`
- `aggregate_type = thread`
- `aggregate_id = thread.ID`
- `idempotency_key = "thread_created:" + thread.ID`

### Payload
Payload must now include:

```json
{
  "thread_id": "uuid",
  "message_id": "uuid",
  "page_id": "uuid",
  "workspace_id": "uuid",
  "actor_id": "uuid",
  "occurred_at": "2026-04-04T08:00:00Z",
  "mention_user_ids": ["user-id-1", "user-id-2"]
}
```

### Payload Rules
- `mention_user_ids` must always be present
- when no mentions are supplied, store `mention_user_ids: []`
- when mentions are supplied, store the normalized unique list in first-seen order

This keeps the outbox contract explicit and avoids requiring downstream projectors to infer mention data from the database at projection time.

## 2.8 Service Behavior Rules

### Thread Create Path
After existing page visibility and anchor validation:
1. normalize `mentions`
2. validate normalized mention ids against workspace members
3. create thread and starter message objects as before
4. build mention rows for the starter message
5. build the `thread_created` outbox payload including `mention_user_ids`
6. pass the thread, message, mentions, and outbox event into the repository create path

### Failure Classification
- invalid mention payload semantics return `422 validation_failed`
- repository persistence or outbox persistence failure returns `500 internal_error`

### No Synchronous Notification Rule
This task must preserve the Task 17 behavior:
- create-thread still does not call a synchronous notification publisher

## 2.9 Positive And Negative Cases

### Positive Cases

1. Valid create-thread request without mentions
- Result: `201`
- behavior unchanged except outbox payload now includes `mention_user_ids: []`

2. Valid create-thread request with one mention
- Result: `201`
- one mention row is inserted for the starter message

3. Valid create-thread request with duplicate mention ids
- Result: `201`
- duplicates are normalized away before persistence

4. Valid create-thread request with self-mention
- Result: `201`
- self mention is persisted

### Negative Cases

1. Malformed JSON
- Result: `400 invalid_json`

2. `mentions` is not an array of strings
- Result: `400 invalid_json`

3. Unknown request field
- Result: `400 invalid_json`

4. Blank mention id after trim
- Result: `422 validation_failed`

5. Mentioned user is not a workspace member
- Result: `422 validation_failed`

6. More than `20` unique mention ids
- Result: `422 validation_failed`

7. Thread create transaction fails while inserting mention rows or outbox event
- Result: `500 internal_error`
- no data is committed

---

## 3. File Structure And Responsibilities

### Create
- `internal/application/thread_mentions.go`
  - mention normalization and workspace-member validation helpers reusable by create-thread and later reply support
- `internal/application/thread_mentions_test.go`
  - focused unit tests for mention normalization and validation

### Modify
- `internal/application/thread_service.go`
  - extend `CreateThreadInput`, call mention normalization/validation, and build mention-aware create-thread persistence inputs
- `internal/application/thread_service_test.go`
  - add create-thread mention tests and update fakes for mention-aware repository input
- `internal/repository/postgres/thread_repository.go`
  - persist starter-message mention rows in the existing create-thread transaction
- `internal/repository/postgres/content_repository_test.go`
  - add DB-backed coverage for mention row persistence and rollback behavior
- `internal/repository/postgres/closed_pool_errors_test.go`
  - update create-thread signature coverage if needed
- `internal/transport/http/handlers.go`
  - accept and forward `mentions`
- `internal/transport/http/server_test.go`
  - add HTTP coverage for `mentions`
- `frontend-repo/API_CONTRACT.md`
  - document the new request field and validation rules
- `docs/checkpoint.md`

### Modify If Needed
- `internal/domain/thread.go`
  - only if Task 20's `PageCommentMessageMention` type needs a small refinement

### Files Explicitly Not In Scope
- `internal/transport/http/server.go`
- `internal/application/notification_service.go`
- `internal/repository/postgres/notification_repository.go`
- `internal/application/comment_notification_projector.go`

---

## 4. Test Matrix

## 4.1 Mention Helper Unit Tests

Add focused tests in:
- `internal/application/thread_mentions_test.go`

### Positive Cases

1. Nil or omitted mentions normalize to empty list

2. Mention ids are trimmed and deduped in first-seen order

3. Self-mention is retained

### Negative Cases

4. Blank mention id returns validation error

5. More than `20` unique mention ids returns validation error

6. Mentioned user not present in workspace member set returns validation error

## 4.2 Application Service Tests

Add or update tests in:
- `internal/application/thread_service_test.go`

### Positive Cases

7. `CreateThread` without mentions still succeeds and passes an empty mention list to the repository path

8. `CreateThread` with mentions normalizes and passes mention rows for the starter message

9. `CreateThread` includes normalized `mention_user_ids` in the `thread_created` outbox payload

10. `CreateThread` keeps the public success payload unchanged

### Negative Cases

11. Blank mention id returns `domain.ErrValidation`

12. Non-member mention id returns `domain.ErrValidation`

13. Too many unique mentions returns `domain.ErrValidation`

14. Repository create failure still propagates

## 4.3 Repository Integration Tests

Add DB-backed tests in:
- `internal/repository/postgres/content_repository_test.go`

### Positive Cases

15. Create-thread with two distinct mentions inserts:
- one thread row
- one starter message row
- two mention rows
- one created event row
- one `thread_created` outbox row

16. Mention rows reference the starter message id

17. Outbox payload stores normalized `mention_user_ids`

### Negative Cases

18. If mention insert fails, thread creation rolls back completely
- no thread row
- no message row
- no mention rows
- no outbox row

19. Closed pool still returns an error on create-thread

## 4.4 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_test.go`

### Positive Cases

20. `POST /api/v1/pages/{pageID}/threads` accepts a valid `mentions` array and returns `201`

21. Duplicate mention ids in the request still return `201`
- behavior:
  - duplicates are normalized internally

22. Omitted `mentions` behaves the same as today

### Negative Cases

23. Non-array `mentions` returns `400 invalid_json`

24. Unknown field still returns `400 invalid_json`

25. Blank mention id returns `422 validation_failed`

26. Non-member mention id returns `422 validation_failed`

27. Too many unique mentions returns `422 validation_failed`

## 4.5 Documentation Tests

28. `frontend-repo/API_CONTRACT.md` documents:
- optional `mentions` array on create-thread
- mention ids are user ids, not emails
- duplicate ids are normalized
- non-member mentions return `422 validation_failed`
- response shape remains unchanged

29. `docs/checkpoint.md` records:
- create-thread now persists explicit mention rows
- `thread_created` outbox payload now includes `mention_user_ids`
- response DTO unchanged in this task

---

## 5. Execution Plan

### Task 1: Add failing mention helper and service tests

**Files:**
- Create: `internal/application/thread_mentions_test.go`
- Modify: `internal/application/thread_service_test.go`

- [ ] **Step 1: Add failing tests for mention normalization and validation**

Cover:
- nil mentions
- trim and dedupe
- blank id failure
- too-many failure
- non-member failure

- [ ] **Step 2: Add failing create-thread service tests for mention-aware behavior**

Cover:
- normalized mention rows passed to repository create path
- normalized `mention_user_ids` included in outbox payload
- unchanged public response payload

- [ ] **Step 3: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "Test(ThreadServiceCreateThread|ThreadMentionNormalization)" -count=1
```

Expected:
- FAIL because create-thread does not yet accept or validate mentions

- [ ] **Step 4: Commit**

```bash
git add internal/application/thread_mentions_test.go internal/application/thread_service_test.go
git commit -m "test: define thread create mention support behavior"
```

### Task 2: Implement mention normalization and service wiring

**Files:**
- Create: `internal/application/thread_mentions.go`
- Modify: `internal/application/thread_service.go`
- Modify: `internal/application/thread_service_test.go`

- [ ] **Step 1: Add the mention helper**

Required behavior:
- normalize mentions
- enforce max `20`
- validate against workspace members
- preserve first-seen order

- [ ] **Step 2: Extend `CreateThreadInput` with `Mentions []string`**

Requirement:
- no create-reply change in this task

- [ ] **Step 3: Update `CreateThread`**

Required behavior:
- validate mentions after page visibility is known
- build starter-message mention rows
- add normalized mention ids to the existing `thread_created` outbox payload

- [ ] **Step 4: Re-run targeted application tests**

Run:
```powershell
go test ./internal/application -run "Test(ThreadServiceCreateThread|ThreadMentionNormalization)" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/thread_mentions.go internal/application/thread_service.go internal/application/thread_service_test.go
git commit -m "feat: validate create-thread mentions"
```

### Task 3: Persist mention rows in the create-thread transaction

**Files:**
- Modify: `internal/repository/postgres/thread_repository.go`
- Modify: `internal/repository/postgres/content_repository_test.go`
- Modify: `internal/repository/postgres/closed_pool_errors_test.go`

- [ ] **Step 1: Add failing repository integration tests for starter-message mentions**

Cover:
- mention row insert
- outbox payload `mention_user_ids`
- rollback when mention insert fails

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositories" -count=1
```

Expected:
- FAIL because create-thread does not yet persist mention rows

- [ ] **Step 3: Update the create-thread transaction**

Required behavior:
- insert thread row
- insert starter message row
- insert zero or more mention rows
- insert created event row
- insert mention-aware outbox row
- commit only if all inserts succeed

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
git commit -m "feat: persist starter message mentions"
```

### Task 4: Extend the HTTP request contract

**Files:**
- Modify: `internal/transport/http/handlers.go`
- Modify: `internal/transport/http/server_test.go`

- [ ] **Step 1: Add failing HTTP tests for the `mentions` request field**

Cover:
- valid mentions array
- non-array mentions
- blank mention id
- non-member mention id
- too many mentions

- [ ] **Step 2: Update `createThreadRequest` and `handleCreateThread`**

Required behavior:
- decode optional `mentions`
- forward them into `application.CreateThreadInput`
- keep response shape unchanged

- [ ] **Step 3: Re-run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "TestThreadCreateEndpoint" -count=1
```

Expected:
- PASS

- [ ] **Step 4: Commit**

```bash
git add internal/transport/http/handlers.go internal/transport/http/server_test.go
git commit -m "feat: accept mentions on thread create"
```

### Task 5: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update the create-thread contract**

Document:
- optional `mentions` array
- validation rules
- unchanged `201` response shape
- mention-aware `thread_created` outbox behavior as backend note

- [ ] **Step 2: Update checkpoint**

Record:
- starter message mention persistence
- mention-aware outbox payload
- no public response DTO change yet

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: record thread create mention support"
```

### Task 6: Full verification for Task 21

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "Test(ThreadServiceCreateThread|ThreadMentionNormalization)" -count=1
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestClosedPoolRepositories" -count=1
go test ./internal/transport/http -run "TestThreadCreateEndpoint" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual API sanity check if local server is available**

Call:
```http
POST /api/v1/pages/{pageID}/threads
```

With:

```json
{
  "body": "Please revise this line",
  "anchor": {
    "type": "block",
    "block_id": "block-1"
  },
  "mentions": ["user-2", "user-3"]
}
```

Verify:
- `201`
- response shape unchanged
- two mention rows exist for the starter message
- `thread_created` outbox payload includes `mention_user_ids`

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify thread create mention support"
```

---

## 6. Acceptance Criteria

Task 21 is complete only when all are true:
- `POST /api/v1/pages/{pageID}/threads` accepts optional `mentions`
- mention ids are normalized, deduped, and validated against workspace members
- starter-message mention rows persist in the same transaction as thread creation
- `thread_created` outbox payload includes normalized `mention_user_ids`
- create-thread response shape remains unchanged
- malformed JSON still returns `400`
- semantic mention errors return `422`
- repository rollback covers mention and outbox failures
- tests and docs cover the positive and negative cases above

## 7. Risks And Guardrails

- Do not change `GET /api/v1/threads/{threadID}` in this task.
- Do not expose mention metadata in `PageCommentThreadDetail` yet.
- Do not accept emails in `mentions`; they must be user ids.
- Do not silently keep blank mention ids.
- Do not reintroduce synchronous notification writes in the create-thread path.
- Keep mention normalization deterministic so later projector behavior is stable.

## 8. Follow-On Tasks

This plan prepares for:
- Task 22 `POST /api/v1/threads/{threadID}/replies` mention support
- Task 23 mention notification projector

Task 22 should reuse the same mention normalization and membership-validation helper instead of re-implementing it.
