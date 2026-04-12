# Task 10 Outbox Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce the transactional outbox foundation so future invitation, comment, and mention notification producers can persist retryable events safely and future workers can claim and process them with explicit state and idempotency.

**Architecture:** This task is a schema-and-repository foundation task. It adds an `outbox_events` table, defines the canonical event metadata and payload contract, and implements PostgreSQL repository methods for create, claim, process, retry, and dead-letter transitions using `FOR UPDATE SKIP LOCKED`. This task does not yet wire producers or start a background worker in the API process; it only creates the durable queue semantics that later tasks will use.

**Tech Stack:** Go, PostgreSQL, `pgx`, SQL migrations, repository integration tests, Go unit tests

---

## 1. Scope

### In Scope
- Add `outbox_events` table
- Define outbox domain model and event contract
- Add PostgreSQL repository for:
  - create one event
  - create many events
  - claim pending events
  - mark processed
  - mark retry
  - mark dead-letter
- Add retry and dead-letter state semantics
- Add concurrency-safe claim behavior with `FOR UPDATE SKIP LOCKED`
- Add tests for migration, repository correctness, and claim concurrency
- Update checkpoint

### Out Of Scope
- No public HTTP endpoint
- No invitation, thread, or mention producers yet
- No background worker started from `cmd/api`
- No notification projector yet
- No unread-count logic
- No frontend changes

---

## 2. Detailed Spec

## 2.1 Objective

Current request handlers and services write notifications directly. That creates a reliability gap:
- domain write succeeds
- notification write fails
- request returns error after persistence already happened

This task introduces a durable outbox table so future tasks can:
- write domain state and outbox event in one transaction
- process notification side effects asynchronously
- retry safely after temporary failures
- dead-letter poisoned events explicitly

This task only introduces the storage and repository foundation. It does not change any current product behavior yet.

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
No existing public endpoint behavior should change in this task. The outbox is internal-only foundation.

## 2.3 Data Model Spec

### New Table
- `outbox_events`

### Columns
- `id UUID PRIMARY KEY`
- `topic TEXT NOT NULL`
- `aggregate_type TEXT NOT NULL`
- `aggregate_id UUID NOT NULL`
- `idempotency_key TEXT NOT NULL`
- `payload JSONB NOT NULL`
- `status TEXT NOT NULL`
- `attempt_count INTEGER NOT NULL`
- `max_attempts INTEGER NOT NULL`
- `available_at TIMESTAMPTZ NOT NULL`
- `claimed_by TEXT NULL`
- `claimed_at TIMESTAMPTZ NULL`
- `lease_expires_at TIMESTAMPTZ NULL`
- `last_error TEXT NULL`
- `processed_at TIMESTAMPTZ NULL`
- `dead_lettered_at TIMESTAMPTZ NULL`
- `created_at TIMESTAMPTZ NOT NULL`
- `updated_at TIMESTAMPTZ NOT NULL`

### Topic Rules
Allowed `topic` values in this task:
- `invitation_created`
- `invitation_updated`
- `invitation_accepted`
- `invitation_rejected`
- `invitation_cancelled`
- `thread_created`
- `thread_reply_created`
- `mention_created`

These topics are enough for the roadmap that follows. Do not add speculative topics in this task.

### Aggregate Type Rules
Allowed `aggregate_type` values in this task:
- `invitation`
- `thread`
- `thread_message`

### Status Rules
Allowed `status` values:
- `pending`
- `processing`
- `processed`
- `dead_letter`

### Idempotency Rules
- `idempotency_key` must be globally unique
- duplicate create by the same idempotency key returns conflict
- the key must be stable across safe retries of the same logical event

### Payload Rules
- `payload` must be a JSON object, not an array or scalar
- payload must contain event-specific data only
- canonical routing fields live in table columns, not only inside payload

### Attempt Rules
- `attempt_count >= 0`
- `max_attempts > 0`
- default `attempt_count = 0`
- default `max_attempts = 25`
- claim increments `attempt_count` by `1`

### Availability Rules
- `available_at` is the earliest timestamp when a pending event may be claimed
- default `available_at = created_at`

### Lease Rules
- when an event is claimed, set:
  - `status = processing`
  - `claimed_by`
  - `claimed_at`
  - `lease_expires_at`
- a future worker may reclaim stale processing rows whose lease expired
- foundation repository support for stale-claim reclaim is in scope in this task

### Timestamp Rules
- on create:
  - `created_at = now`
  - `updated_at = now`
- on claim:
  - `updated_at = claim time`
- on retry:
  - `updated_at = failure time`
- on processed:
  - `updated_at = processed_at`
- on dead-letter:
  - `updated_at = dead_lettered_at`

## 2.4 State Machine Rules

### `pending`
- claimable when `available_at <= now`
- `processed_at IS NULL`
- `dead_lettered_at IS NULL`
- `claimed_by IS NULL`
- `claimed_at IS NULL`
- `lease_expires_at IS NULL`

### `processing`
- `claimed_by IS NOT NULL`
- `claimed_at IS NOT NULL`
- `lease_expires_at IS NOT NULL`
- `processed_at IS NULL`
- `dead_lettered_at IS NULL`

### `processed`
- `processed_at IS NOT NULL`
- `dead_lettered_at IS NULL`
- `lease_expires_at IS NULL`

### `dead_letter`
- `dead_lettered_at IS NOT NULL`
- `processed_at IS NULL`
- `lease_expires_at IS NULL`

### Allowed Transitions
- `pending -> processing`
- `processing -> pending` via retry
- `processing -> processed`
- `processing -> dead_letter`

No other transitions are valid in this task.

## 2.5 Event Payload Contract

This task defines the payload contract even though producers are future work.

### Invitation Event Payload
For all invitation topics, payload must contain:
- `invitation_id`
- `workspace_id`
- `actor_id`
- `email`
- `role`
- `status`
- `version`
- `occurred_at`

### Thread Created Payload
Payload must contain:
- `thread_id`
- `message_id`
- `page_id`
- `workspace_id`
- `actor_id`
- `occurred_at`

### Thread Reply Payload
Payload must contain:
- `thread_id`
- `message_id`
- `page_id`
- `workspace_id`
- `actor_id`
- `occurred_at`

### Mention Payload
Payload must contain:
- `thread_id`
- `message_id`
- `page_id`
- `workspace_id`
- `actor_id`
- `mentioned_user_id`
- `occurred_at`

This contract is documentation and test guidance for future producer tasks. Task 10 does not implement those producers.

## 2.6 Indexing And Claim Strategy

### Required Indexes
- unique index on `idempotency_key`
- index on `(status, available_at ASC, created_at ASC, id ASC)` for pending claims
- index on `(status, lease_expires_at ASC)` for stale processing reclaim
- index on `(aggregate_type, aggregate_id, created_at ASC)` for audit/debug lookups
- optional topic index if tests or explain plans show it is useful is out of scope here

### Claim Ordering Rule
When claiming pending work, order by:
- `available_at ASC`
- `created_at ASC`
- `id ASC`

This gives deterministic oldest-ready-first processing.

### Claim Algorithm
`ClaimPending` must:
1. select rows where:
   - `status = 'pending' AND available_at <= now`
   - or `status = 'processing' AND lease_expires_at < now`
2. lock rows with `FOR UPDATE SKIP LOCKED`
3. limit by requested batch size
4. update selected rows to:
   - `status = 'processing'`
   - `claimed_by = workerID`
   - `claimed_at = now`
   - `lease_expires_at = now + leaseDuration`
   - `attempt_count = attempt_count + 1`
   - `updated_at = now`
5. return the claimed rows

### Retry Algorithm
`MarkRetry` must:
- only operate on rows currently `processing`
- require the same `claimed_by` worker id
- if `attempt_count >= max_attempts`:
  - transition to `dead_letter`
  - set `dead_lettered_at`
  - set `last_error`
- else:
  - transition to `pending`
  - set `available_at = nextAvailableAt`
  - clear `claimed_by`
  - clear `claimed_at`
  - clear `lease_expires_at`
  - set `last_error`
- always set `updated_at = failure time`

### Processed Algorithm
`MarkProcessed` must:
- only operate on rows currently `processing`
- require the same `claimed_by` worker id
- transition to `processed`
- set `processed_at`
- clear `lease_expires_at`
- keep `claimed_by` and `claimed_at` as the last successful claimer audit trail
- set `updated_at = processedAt`

### Dead-Letter Algorithm
`MarkDeadLetter` must:
- only operate on rows currently `processing`
- require the same `claimed_by` worker id
- transition to `dead_letter`
- set `dead_lettered_at`
- set `last_error`
- clear `lease_expires_at`
- set `updated_at = deadLetteredAt`

## 2.7 Domain Model Spec

Create a new domain model:
- `type OutboxTopic string`
- `type OutboxAggregateType string`
- `type OutboxStatus string`

Add constants for:
- all allowed topics
- all allowed aggregate types
- all allowed statuses

Create:

```go
type OutboxEvent struct {
    ID             string
    Topic          OutboxTopic
    AggregateType  OutboxAggregateType
    AggregateID    string
    IdempotencyKey string
    Payload        json.RawMessage
    Status         OutboxStatus
    AttemptCount   int
    MaxAttempts    int
    AvailableAt    time.Time
    ClaimedBy      *string
    ClaimedAt      *time.Time
    LeaseExpiresAt *time.Time
    LastError      *string
    ProcessedAt    *time.Time
    DeadLetteredAt *time.Time
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

The outbox domain type is internal-only and has no public API JSON contract requirements in this task.

## 2.8 Repository Behavior After Task 10

### Create
Repository create must:
- validate non-empty idempotency key
- validate `payload` is a JSON object
- insert a pending row with defaults if caller omitted optional fields
- return conflict on duplicate idempotency key

### CreateMany
Repository batch create must:
- insert all rows in one statement or transaction
- reject the batch if any payload or required field is invalid
- return conflict if any idempotency key collides

Do not silently ignore duplicate outbox events in this task. Foundation code should surface the conflict so producers can decide retry behavior later.

### ClaimPending
Repository claim must:
- validate `limit > 0`
- validate non-empty `workerID`
- validate positive `leaseDuration`
- claim only ready or stale-leased rows
- return at most `limit`
- be safe under concurrent claimers

### MarkProcessed / MarkRetry / MarkDeadLetter
These methods must:
- require the row to be `processing`
- require the same `workerID` that claimed it
- return `domain.ErrNotFound` if event id does not exist
- return `domain.ErrConflict` if state or worker ownership does not match

## 2.9 Positive And Negative Cases

### Public HTTP Cases
No HTTP cases are introduced in this task.

### Persistence Cases

Positive:
- migration succeeds on empty database
- create inserts one pending outbox event
- batch create inserts multiple outbox events
- claim returns oldest ready rows in deterministic order
- concurrent claimers do not receive the same row
- mark processed transitions a claimed row to processed
- mark retry requeues a claimed row with next availability
- mark retry dead-letters when attempts are exhausted
- mark dead-letter transitions a claimed row to dead_letter
- stale leased rows can be reclaimed

Negative:
- duplicate idempotency key returns conflict
- invalid topic returns DB validation failure
- invalid aggregate type returns DB validation failure
- invalid payload shape returns validation error
- claim with invalid limit returns validation error
- mark processed with wrong worker returns conflict
- mark retry on non-processing row returns conflict
- mark dead-letter on missing row returns not found

---

## 3. File Structure And Responsibilities

### Create
- `migrations/000021_outbox_events.up.sql`
- `migrations/000021_outbox_events.down.sql`
- `internal/domain/outbox.go`
- `internal/repository/postgres/outbox_repository.go`
- `internal/repository/postgres/outbox_repository_test.go`

### Modify
- `internal/repository/postgres/integration_test.go`
  - include `outbox_events` in test database reset
- `internal/repository/postgres/closed_pool_errors_test.go`
  - add outbox repository closed-pool coverage
- `docs/checkpoint.md`

### Files Explicitly Not In Scope
- `internal/application/notification_service.go`
- `internal/application/workspace_service.go`
- `internal/application/thread_service.go`
- `cmd/api/app.go`
- `internal/transport/http/server.go`
- `frontend-repo/API_CONTRACT.md`

---

## 4. Test Matrix

## 4.1 Migration And Repository Integration Tests

### Positive Cases

1. Apply migration on empty database
- Expect:
  - `outbox_events` table exists
  - indexes exist
  - check constraints exist

2. Create one outbox event
- Expect:
  - `status = pending`
  - `attempt_count = 0`
  - `available_at = created_at`

3. Batch create multiple outbox events
- Expect:
  - all rows inserted
  - insertion order preserved through `created_at`

4. Claim pending events in oldest-ready-first order
- Seed:
  - three pending rows with different `available_at`
- Expect:
  - earliest ready rows returned first

5. Claim skips future-available rows
- Seed:
  - one ready row
  - one future row
- Expect:
  - only ready row is claimed

6. Claim updates claim metadata
- Expect:
  - `status = processing`
  - `claimed_by`
  - `claimed_at`
  - `lease_expires_at`
  - `attempt_count = 1`

7. Mark processed transitions row correctly
- Expect:
  - `status = processed`
  - `processed_at`
  - `lease_expires_at = NULL`

8. Mark retry requeues row correctly
- Expect:
  - `status = pending`
  - `available_at = nextAvailableAt`
  - claim metadata cleared
  - `last_error` set

9. Mark retry dead-letters exhausted row
- Seed:
  - `attempt_count = max_attempts`
- Expect:
  - `status = dead_letter`
  - `dead_lettered_at`
  - `last_error`

10. Mark dead-letter transitions row correctly

11. Claim can reclaim stale processing lease
- Seed:
  - processing row with expired `lease_expires_at`
- Expect:
  - row is reclaimed by new worker

12. Concurrent claimers do not claim the same row
- Use two separate DB connections or goroutines with coordination
- Expect:
  - no overlap in claimed ids

### Negative Cases

13. Duplicate idempotency key returns `domain.ErrConflict`

14. Create with scalar or array payload returns validation error

15. Claim with zero limit returns validation error

16. Claim with blank worker id returns validation error

17. Mark processed with wrong worker returns `domain.ErrConflict`

18. Mark retry on already processed row returns `domain.ErrConflict`

19. Mark dead-letter on missing id returns `domain.ErrNotFound`

20. Invalid manual row violating state-machine checks is rejected by DB constraint

## 4.2 Closed-Pool Regression Tests

21. Outbox repository create on closed pool returns error

22. Outbox repository batch create on closed pool returns error

23. Outbox repository claim on closed pool returns error

24. Outbox repository mark processed on closed pool returns error

## 4.3 Documentation Tests

25. `docs/checkpoint.md` records:
- outbox foundation added
- schema and statuses
- claim, retry, and dead-letter support
- no public API behavior change

---

## 5. Execution Plan

### Task 1: Write migration spec and failing integration tests

**Files:**
- Create: `migrations/000021_outbox_events.up.sql`
- Create: `migrations/000021_outbox_events.down.sql`
- Create: `internal/repository/postgres/outbox_repository_test.go`
- Modify: `internal/repository/postgres/integration_test.go`

- [ ] **Step 1: Add failing migration and repository tests**

Cover:
- create
- batch create
- claim
- retry
- processed
- dead-letter
- stale lease reclaim
- duplicate idempotency key

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestOutboxRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- FAIL because the outbox schema and repository do not exist yet

- [ ] **Step 3: Write the migration**

Requirements:
- use migration number `000021`
- create `outbox_events`
- add all required indexes and checks
- add reversible down migration
- update DB reset path to truncate `outbox_events`

- [ ] **Step 4: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestOutboxRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- migration-related failures remain until domain and repository code are added

- [ ] **Step 5: Commit**

```bash
git add migrations/000021_outbox_events.up.sql migrations/000021_outbox_events.down.sql internal/repository/postgres/outbox_repository_test.go internal/repository/postgres/integration_test.go
git commit -m "test: add outbox foundation migration coverage"
```

### Task 2: Add outbox domain model and repository implementation

**Files:**
- Create: `internal/domain/outbox.go`
- Create: `internal/repository/postgres/outbox_repository.go`

- [ ] **Step 1: Add the outbox domain types and constants**

Add:
- topics
- aggregate types
- statuses
- `OutboxEvent`

- [ ] **Step 2: Implement repository `Create` and `CreateMany`**

Requirements:
- validate idempotency key
- validate JSON-object payload
- map duplicate idempotency key to `domain.ErrConflict`

- [ ] **Step 3: Implement repository `ClaimPending`**

Requirements:
- validate inputs
- use `FOR UPDATE SKIP LOCKED`
- support stale-lease reclaim
- increment `attempt_count`
- set lease fields

- [ ] **Step 4: Implement repository terminal/update methods**

Methods:
- `MarkProcessed`
- `MarkRetry`
- `MarkDeadLetter`

Requirements:
- enforce worker ownership
- enforce valid state transitions
- return `ErrConflict` on wrong state or wrong worker

- [ ] **Step 5: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestOutboxRepository" -count=1
```

Expected:
- PASS

- [ ] **Step 6: Commit**

```bash
git add internal/domain/outbox.go internal/repository/postgres/outbox_repository.go
git commit -m "feat: add transactional outbox repository foundation"
```

### Task 3: Add closed-pool regression coverage

**Files:**
- Modify: `internal/repository/postgres/closed_pool_errors_test.go`

- [ ] **Step 1: Add failing closed-pool assertions for outbox repository**

Cover:
- create
- create many
- claim
- mark processed

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestClosedPoolRepositories" -count=1
```

Expected:
- PASS

- [ ] **Step 3: Commit**

```bash
git add internal/repository/postgres/closed_pool_errors_test.go
git commit -m "test: cover outbox repository closed-pool failures"
```

### Task 4: Update checkpoint documentation

**Files:**
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Record the outbox foundation**

Document:
- `outbox_events` schema
- explicit statuses
- idempotency key uniqueness
- claim/retry/dead-letter support
- no public API change in this task

- [ ] **Step 2: Commit**

```bash
git add docs/checkpoint.md
git commit -m "docs: record outbox foundation"
```

### Task 5: Full verification for Task 10

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/repository/postgres -run "TestOutboxRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- PASS

- [ ] **Step 2: Manual schema sanity check if local database is available**

Verify:
- `outbox_events` exists
- claim query does not return the same row to two concurrent claimers
- retry and dead-letter transitions persist as expected

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify outbox foundation task"
```

---

## 6. Acceptance Criteria

Task 10 is complete only when all are true:
- `outbox_events` exists with explicit topic, aggregate, status, idempotency, retry, and audit fields
- the schema enforces valid states and idempotency-key uniqueness
- PostgreSQL repository supports create, batch create, claim, processed, retry, and dead-letter transitions
- claim uses `FOR UPDATE SKIP LOCKED`
- stale lease reclaim is supported
- duplicate idempotency keys return conflict
- repository tests cover positive, negative, and concurrent-claim cases
- checkpoint reflects the outbox foundation
- no public API behavior changes in this task

## 7. Risks And Guardrails

- Do not start a background worker in this task.
- Do not wire current invitation or thread producers to the outbox yet.
- Do not silently ignore duplicate idempotency keys.
- Do not omit lease semantics; without reclaim, events can get stuck in `processing`.
- Do not let repository methods transition rows from invalid states.

## 8. Follow-On Tasks

This plan prepares for:
- Task 11 invitation notification projector
- later invitation, thread, and mention producer integration tasks
