# Task 28 Concurrency And Load Verification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add focused concurrency, retry, idempotency, and benchmark coverage that proves the invitation and notification system stays correct under competing actions and repeated high-volume event processing.

**Architecture:** This task is verification-only. It does not add a public endpoint or new product behavior. It adds repository integration tests for invitation races and DB-backed replay semantics, application-level retry and recipient-dedupe tests, and targeted benchmarks for the hottest notification paths. The tests use the exact domain rules from earlier tasks: invitation transitions are version-checked, only one terminal transition may win, comment and mention notifications are idempotent append-only projections, invitation notifications are one live row per invitation, and unread counters must reflect final row truth rather than retry count.

**Tech Stack:** Go, PostgreSQL, `pgx`, Go integration tests, Go unit tests, Go benchmarks, table-driven tests

---

## 1. Scope

### In Scope
- Add concurrency verification for invitation state transitions:
  - update vs accept
  - update vs reject
  - cancel vs accept
  - cancel vs reject
- Add duplicate-delivery and retry verification for:
  - invitation projector
  - comment projector
  - mention projector
- Add DB-backed idempotency verification for notification persistence under replay
- Add unread-counter correctness verification under:
  - duplicate insert replay
  - retry after partial failure
  - repeated read transitions
- Add recipient-dedupe verification for thread notifications
- Add targeted benchmarks for hot notification paths
- Add race-detector verification commands for in-process concurrency paths
- Add operator-facing verification guidance
- Update checkpoint

### Out Of Scope
- No public HTTP endpoint
- No schema change
- No new application feature
- No scheduler or worker runtime change
- No frontend change
- No strict production SLA threshold enforced in CI

### Prerequisites
- Tasks 5 through 8 invitation lifecycle endpoints and repository behavior exist
- Task 11 invitation projector exists
- Task 14 unread counters exist
- Task 19 comment projector exists
- Task 23 mention projector exists
- Task 27 is not a hard dependency for correctness in this task
- if Task 27 implementation has already introduced a reusable thread-history helper, reuse it in the benchmark suite
- otherwise, benchmark the equivalent pure recipient-derivation helper introduced in this task's test harness

---

## 2. Detailed Spec

## 2.1 Objective

The roadmap already defines the correct behavior. This task proves that behavior holds when:
- two actors race on the same invitation
- the same outbox event is delivered more than once
- a projector fails mid-run and retries later
- recipient sets overlap through thread creator, prior repliers, and mentions
- unread counters are updated under repeated writes and retries

This task is complete only when the verification suite makes those guarantees explicit and repeatable.

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

### Public Behavior Rule
This task verifies existing contracts. It does not introduce any new API behavior.

## 2.3 Verification Surface

This task adds three kinds of verification:

### 1. Repository Integration Concurrency Tests
Purpose:
- prove invitation row locking and version checks behave correctly with a real database

### 2. Application And Repository Retry/Idempotency Tests
Purpose:
- prove projector duplicate delivery and retries do not create duplicate rows or inflate unread counts

### 2a. DB-Backed Replay Tests
Purpose:
- prove PostgreSQL uniqueness and unread-counter persistence rules hold under duplicate replay, not just stubbed application logic

### 3. Benchmarks
Purpose:
- measure hot-path cost and provide a repeatable regression signal for future changes

Benchmarks are informational in this task:
- they must run successfully
- they do not fail based on a fixed time threshold in CI

## 2.4 Invitation Concurrency Rules To Prove

These tests must use real concurrent goroutines against the PostgreSQL repository, not fake repositories.

### Shared Preconditions
All race tests start from:
- one `pending` invitation
- known initial `version`
- target user not yet a member unless the test explicitly changes it

### Case 1: Update vs Accept
Competing operations:
- inviter updates role with `version = N`
- invitee accepts with `version = N`

Expected result:
- exactly one operation succeeds
- the loser returns conflict at the authoritative layer
- final state is one of:
  - updated pending invitation with `version = N+1` and no membership
  - accepted invitation with `version = N+1` and one membership

Forbidden result:
- both operations succeed
- accepted invitation without membership
- membership created while invitation remains pending

### Case 2: Update vs Reject
Competing operations:
- inviter updates role with `version = N`
- invitee rejects with `version = N`

Expected result:
- exactly one operation succeeds
- loser conflicts
- final state is one of:
  - updated pending invitation with `version = N+1`
  - rejected invitation with `version = N+1`

Forbidden result:
- both operations succeed
- terminal rejection and pending update both committed

### Case 3: Cancel vs Accept
Competing operations:
- inviter cancels with `version = N`
- invitee accepts with `version = N`

Expected result:
- exactly one terminal transition succeeds
- loser conflicts
- final state is exactly one of:
  - `cancelled`
  - `accepted`

Additional requirement:
- membership exists only when `accepted` wins

### Case 4: Cancel vs Reject
Competing operations:
- inviter cancels with `version = N`
- invitee rejects with `version = N`

Expected result:
- exactly one terminal transition succeeds
- loser conflicts
- final state is exactly one of:
  - `cancelled`
  - `rejected`

### Conflict Mapping Rule
Repository-level concurrency verification proves the source-of-truth conflict.

This task must also keep one light transport/application regression that confirms stale-version invitation actions still surface as:
- `409 conflict`

The goal is not to retest every endpoint contract in full. The goal is to keep the conflict mapping locked while the concurrency tests prove the DB behavior underneath.

## 2.5 Projector Retry And Duplicate-Delivery Rules To Prove

### Invitation Projector

For the same invitation outbox event delivered twice:
- only one live invitation row must exist
- unread counter must increment at most once on first create
- later updates must preserve `read_at`

For transient failure then retry:
- final row must match canonical invitation projection
- unread count must reflect final row existence, not number of attempts

### Comment Projector

For the same thread message event delivered twice:
- each recipient gets at most one `comment` notification row for that message
- unread counter increments only for new rows

For transient failure after some recipients were inserted:
- retry must finish the missing work
- already-inserted rows must not duplicate
- unread counters must equal final inserted-row count

### Mention Projector

For the same thread message event delivered twice:
- each recipient gets at most one `mention` notification row for that message
- comment and mention notifications may coexist for the same user and same message
- coexistence must not collapse into one row

For transient failure then retry:
- final mention rows must be complete and unique
- unread counters must equal final mention-row count

## 2.6 DB-Backed Replay Rules To Prove

At least one PostgreSQL-backed verification slice is required for each persistence model:

### Append-Only Replay
For `comment` and `mention` notification inserts replayed with the same logical identity:
- only one row per unique `(user_id, type, event_id)` may persist
- unread count must increment at most once for that logical row

### Live Invitation Replay
For repeated invitation live-row upserts on the same invitation:
- one live row remains
- `read_at` stays preserved if already set
- unread count does not increase on update-only replay

These tests must use the real PostgreSQL repository path, not stubs.

## 2.7 Recipient Dedupe Rules To Prove

Use thread scenarios where the same user qualifies multiple ways:
- thread creator
- prior replier
- explicit mention target

Expected result:
- user appears once in the resolved comment recipient list
- actor never appears in the recipient list
- mention notifications still remain explicit and separate by type

Example scenario:
- thread creator = user A
- prior replier = user B
- current reply actor = user C
- explicit mentions = user A, user B, user C

Expected comment recipients:
- user A
- user B

Expected mention recipients:
- user A
- user B

Forbidden result:
- duplicate rows for user A or user B
- actor user C receives a mention notification for their own message

## 2.8 Unread Counter Rules To Prove

### Create Replay Rule
If the same append-only notification insert is replayed:
- unread counter must not increment twice

### Live Invitation Update Rule
If an invitation live row is updated repeatedly:
- unread counter must not increase on update-only replay
- read state stays preserved

### Retry Rule
If a projector writes some rows, then fails, then retries:
- unread counter must equal the number of final unread rows only

### Mark-Read Stability Rule
If a notification is marked read repeatedly after projector replay:
- unread counter must never go below zero
- repeated mark-read does not double-decrement

## 2.9 Benchmark Scope

This task adds repeatable benchmarks for the hottest internal paths.

### Benchmark 1: Invitation Projector Replay
Measure:
- repeated processing of the same invitation event payload against in-memory or stubbed dependencies

Purpose:
- catch accidental allocation spikes or heavy remapping cost

### Benchmark 2: Thread Recipient History Builder
Measure:
- building expected comment recipients from a synthetic thread with:
  - many messages
  - overlapping prior repliers
  - overlapping mention targets

Purpose:
- catch accidental O(n^2) regressions beyond the intended history walk

Implementation note:
- if Task 27 has already added a reusable production helper for history-based recipient derivation, benchmark that helper directly
- otherwise, benchmark the equivalent pure helper created in this verification task

### Benchmark 3: Notification Batch Projection
Measure:
- batch insert preparation or projector mapping for a moderate number of recipients

Purpose:
- keep append-only mapping cost visible as the notification system evolves

### Benchmark Output Rule
Benchmarks must:
- run with `go test -bench`
- report allocations
- not assert a hard time threshold in CI

## 2.10 Positive And Negative Cases

### Positive Cases

1. one invitation action wins and the competing action conflicts

2. duplicate invitation projector delivery produces one live row only

3. duplicate comment projector delivery produces one row per recipient only

4. duplicate mention projector delivery produces one row per recipient only

5. retry after partial failure completes missing rows without duplicates

6. recipient overlap is deduped deterministically

7. unread counters equal final unread row count after replay and retry

8. benchmark suite runs successfully and reports allocations

### Negative Cases

1. both competing invitation actions commit successfully
- invalid
- test must fail

2. duplicate replay increments unread count more than once
- invalid
- test must fail

3. retry after partial projector failure leaves duplicate rows
- invalid
- test must fail

4. actor receives their own mention notification
- invalid
- test must fail

5. benchmark command fails to compile or run
- invalid
- test task not complete

---

## 3. File Structure And Responsibilities

### Create
- `internal/repository/postgres/invitation_concurrency_test.go`
  - real PostgreSQL race tests for competing invitation transitions
- `internal/repository/postgres/notification_replay_idempotency_test.go`
  - real PostgreSQL replay tests for append-only inserts, live invitation upserts, and unread-counter stability
- `internal/application/notification_projection_concurrency_test.go`
  - projector duplicate-delivery, retry, recipient-dedupe, and unread-counter tests using controlled fakes for orchestration behavior
- `internal/application/notification_projection_benchmark_test.go`
  - benchmark coverage for invitation replay, recipient history building, and batch projection mapping
- `docs/operations/notification-verification.md`
  - how to run the verification suite, what each command proves, and how to interpret failures

### Modify
- `internal/transport/http/server_test.go`
  - add one light stale-version `409` regression for invitation action mapping if missing
- `docs/checkpoint.md`
  - record completion of the concurrency and load verification task

### Modify If Needed For Testability Only
- `internal/application/notification_projection_test_helpers.go`
  - create only if shared projector fixtures reduce duplication across the new concurrency tests and benchmarks

### Files Explicitly Not In Scope
- `internal/transport/http/handlers.go`
- `internal/repository/postgres/notification_repository.go`
  - unless a tiny test-only helper extraction is required
- `internal/application/workspace_service.go`
- `cmd/api/app.go`
- `frontend-repo/API_CONTRACT.md`

---

## 4. Test Matrix

## 4.1 Repository Invitation Concurrency Tests

Add tests in:
- `internal/repository/postgres/invitation_concurrency_test.go`

### Positive Cases

1. `update` vs `accept` with the same initial version yields one success and one conflict

2. `update` vs `reject` with the same initial version yields one success and one conflict

3. `cancel` vs `accept` with the same initial version yields one terminal success and one conflict

4. `cancel` vs `reject` with the same initial version yields one terminal success and one conflict

5. when `accept` wins, exactly one membership row exists

6. when `cancel` or `reject` wins, no membership row exists

7. final invitation version increments exactly once from the shared starting version

### Negative Cases

8. no race test may end with two successful transitions

9. no race test may leave invitation status and membership state inconsistent

## 4.2 Projector Concurrency And Retry Tests

Add tests in:
- `internal/application/notification_projection_concurrency_test.go`

### Positive Cases

10. duplicate invitation projector delivery creates or keeps one live row only

11. invitation live-row update replay preserves `read_at`

12. duplicate comment projector delivery creates one `comment` row per recipient only

13. duplicate mention projector delivery creates one `mention` row per recipient only

14. comment and mention rows may coexist for the same user and same message

15. transient invitation projector failure followed by retry results in one correct final live row

16. transient comment projector partial failure followed by retry results in one final row per recipient only

17. transient mention projector partial failure followed by retry results in one final row per recipient only

18. recipient overlap between creator, prior replier, and mention target is deduped to one comment recipient entry

19. actor is excluded from mention notifications even if self-mentioned in payload

### Negative Cases

20. duplicate replay must not create duplicate row ids in the fake or integration sink

21. retry must not inflate unread counts above final unread row count

22. malformed retry scenario must fail the test if completion still leaves missing expected recipients

## 4.3 Repository Replay And Counter Idempotency Tests

Add tests in:
- `internal/repository/postgres/notification_replay_idempotency_test.go`

### Positive Cases

23. duplicate `comment` insert replay persists one row only and increments unread count once

24. duplicate `mention` insert replay persists one row only and increments unread count once

25. repeated invitation live-row upsert preserves `read_at` and does not increment unread count on update-only replay

26. mixed comment and mention replay persists one row per type and keeps counts exact

### Negative Cases

27. no replay test may persist duplicate logical notification rows

28. no replay test may inflate unread count above the final unread-row total

## 4.4 Unread Counter Stability Tests

Add tests in:
- `internal/application/notification_projection_concurrency_test.go`

### Positive Cases

29. duplicate append-only replay increments unread count only once per unique recipient-event key

30. invitation live-row update replay does not increment unread count

31. repeated mark-read after replay leaves unread count stable at zero

32. mixed comment and mention replay results in unread count equal to total unique unread rows across both types

### Negative Cases

33. unread count never becomes negative under repeated mark-read simulation

## 4.5 HTTP Conflict Mapping Regression

Add or update tests in:
- `internal/transport/http/server_test.go`

### Positive Cases

34. stale-version invitation accept still returns `409`

35. stale-version invitation cancel returns `409`

36. stale-version invitation reject returns `409`

### Negative Cases

37. stale-version invitation action must not regress to `500` or `200`

## 4.6 Benchmarks

Add benchmarks in:
- `internal/application/notification_projection_benchmark_test.go`

### Required Benchmarks

38. `BenchmarkInvitationProjectorReplay`

39. `BenchmarkThreadNotificationHistoryBuilder`

40. `BenchmarkNotificationBatchProjection`

### Benchmark Rules

41. each benchmark must call `b.ReportAllocs()`

42. each benchmark must validate a non-empty meaningful result so it cannot optimize away the work

## 4.7 Documentation Tests

43. `docs/operations/notification-verification.md` documents:
- exact commands
- what each suite proves
- race tests vs benchmarks distinction
- how to rerun flaky-looking failures
- when to run `-race`

44. `docs/checkpoint.md` records:
- invitation concurrency verification added
- DB-backed replay verification added
- projector idempotency and retry verification added
- benchmark suite added

---

## 5. Execution Plan

### Task 1: Add failing invitation race tests

**Files:**
- Create: `internal/repository/postgres/invitation_concurrency_test.go`

- [ ] **Step 1: Write failing real-DB race tests**

Cover:
- update vs accept
- update vs reject
- cancel vs accept
- cancel vs reject

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestInvitationConcurrency" -count=1
```

Expected:
- FAIL because the race tests or helper coordination do not exist yet

- [ ] **Step 3: Commit**

```bash
git add internal/repository/postgres/invitation_concurrency_test.go
git commit -m "test: define invitation concurrency verification"
```

### Task 2: Implement invitation race test helpers and assertions

**Files:**
- Modify: `internal/repository/postgres/invitation_concurrency_test.go`

- [ ] **Step 1: Add deterministic goroutine coordination**

Required behavior:
- synchronize competing operations with a start barrier
- collect exactly two outcomes
- assert one success and one conflict

- [ ] **Step 2: Add final-state assertions**

Required behavior:
- assert invitation status
- assert version increment
- assert membership presence or absence

- [ ] **Step 3: Re-run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestInvitationConcurrency" -count=1
```

Expected:
- PASS

- [ ] **Step 4: Commit**

```bash
git add internal/repository/postgres/invitation_concurrency_test.go
git commit -m "test: verify invitation concurrency outcomes"
```

### Task 3: Add failing projector replay and unread-counter tests

**Files:**
- Create: `internal/application/notification_projection_concurrency_test.go`
- Create: `internal/repository/postgres/notification_replay_idempotency_test.go`

- [ ] **Step 1: Write failing duplicate-delivery and retry tests**

Cover:
- invitation replay
- comment replay
- mention replay
- unread counter stability
- recipient dedupe
- DB-backed replay semantics

- [ ] **Step 2: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestNotificationProjectionConcurrency" -count=1
go test ./internal/repository/postgres -run "TestNotificationReplayIdempotency" -count=1
```

Expected:
- FAIL because the verification harnesses do not exist yet

- [ ] **Step 3: Commit**

```bash
git add internal/application/notification_projection_concurrency_test.go internal/repository/postgres/notification_replay_idempotency_test.go
git commit -m "test: define notification projection concurrency verification"
```

### Task 4: Implement projector verification harnesses and assertions

**Files:**
- Modify: `internal/application/notification_projection_concurrency_test.go`
- Modify: `internal/repository/postgres/notification_replay_idempotency_test.go`
- Create if needed: `internal/application/notification_projection_test_helpers.go`

- [ ] **Step 1: Add controlled fake repositories or fixtures**

Required behavior:
- simulate duplicate delivery
- simulate transient failure on first write
- capture final notification rows
- capture unread count mutations

- [ ] **Step 2: Add DB-backed replay assertions**

Required behavior:
- replay append-only comment and mention inserts through PostgreSQL
- replay invitation live-row upserts through PostgreSQL
- assert exact unread-counter outcomes

- [ ] **Step 3: Add final-state assertions**

Required behavior:
- no duplicate logical notifications
- read state preserved on invitation live updates
- unread count equals final unread rows only
- actor excluded from self-mention delivery

- [ ] **Step 4: Re-run targeted verification tests**

Run:
```powershell
go test ./internal/application -run "TestNotificationProjectionConcurrency" -count=1
go test ./internal/repository/postgres -run "TestNotificationReplayIdempotency" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/notification_projection_concurrency_test.go internal/application/notification_projection_test_helpers.go internal/repository/postgres/notification_replay_idempotency_test.go
git commit -m "test: verify notification projection retries and replay"
```

### Task 5: Add stale-version HTTP conflict regression

**Files:**
- Modify: `internal/transport/http/server_test.go`

- [ ] **Step 1: Add or extend one stale-version regression**

Required behavior:
- stale invitation action returns `409`
- no success payload is returned

- [ ] **Step 2: Run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "TestWorkspaceInvitation.*Conflict|TestWorkspaceInvitationAccept.*Conflict" -count=1
```

Expected:
- PASS

- [ ] **Step 3: Commit**

```bash
git add internal/transport/http/server_test.go
git commit -m "test: keep invitation conflict mapping stable"
```

### Task 6: Add benchmarks

**Files:**
- Create: `internal/application/notification_projection_benchmark_test.go`

- [ ] **Step 1: Write benchmark fixtures**

Required behavior:
- synthetic invitation event replay fixture
- synthetic thread history fixture with overlap
- synthetic batch projection fixture

- [ ] **Step 2: Add benchmarks with result guards**

Required behavior:
- call `b.ReportAllocs()`
- assert non-empty meaningful output

- [ ] **Step 3: Run targeted benchmarks**

Run:
```powershell
go test ./internal/application -run ^$ -bench "BenchmarkInvitationProjectorReplay|BenchmarkThreadNotificationHistoryBuilder|BenchmarkNotificationBatchProjection" -benchmem -count=1
```

Expected:
- PASS
- benchmark output lines for all three benchmarks

- [ ] **Step 4: Commit**

```bash
git add internal/application/notification_projection_benchmark_test.go
git commit -m "test: add notification projection benchmarks"
```

### Task 7: Update verification docs

**Files:**
- Create: `docs/operations/notification-verification.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Write the verification runbook**

Document:
- exact `go test` commands
- exact benchmark command
- what failures mean
- when to rerun with `-count=10` for race confidence
- when to run `-race` for in-process concurrency checks

- [ ] **Step 2: Update checkpoint**

Record:
- invitation race verification
- DB-backed replay verification
- projector replay verification
- benchmark availability

- [ ] **Step 3: Commit**

```bash
git add docs/operations/notification-verification.md docs/checkpoint.md
git commit -m "docs: add notification verification runbook"
```

### Task 8: Full verification for Task 28

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run repository race tests repeatedly**

Run:
```powershell
go test ./internal/repository/postgres -run "TestInvitationConcurrency" -count=10
```

Expected:
- PASS for all repetitions

- [ ] **Step 2: Run DB-backed replay tests repeatedly**

Run:
```powershell
go test ./internal/repository/postgres -run "TestNotificationReplayIdempotency" -count=10
```

Expected:
- PASS for all repetitions

- [ ] **Step 3: Run projector concurrency tests repeatedly**

Run:
```powershell
go test ./internal/application -run "TestNotificationProjectionConcurrency" -count=10
```

Expected:
- PASS for all repetitions

- [ ] **Step 4: Run race-detector verification**

Run:
```powershell
go test ./internal/application -run "TestNotificationProjectionConcurrency" -race -count=1
```

Expected:
- PASS
- no race detector failures

- [ ] **Step 5: Run HTTP conflict regression**

Run:
```powershell
go test ./internal/transport/http -run "TestWorkspaceInvitation.*Conflict|TestWorkspaceInvitationAccept.*Conflict|TestWorkspaceInvitationReject.*Conflict|TestWorkspaceInvitationCancel.*Conflict" -count=1
```

Expected:
- PASS

- [ ] **Step 6: Run benchmarks**

Run:
```powershell
go test ./internal/application -run ^$ -bench "BenchmarkInvitationProjectorReplay|BenchmarkThreadNotificationHistoryBuilder|BenchmarkNotificationBatchProjection" -benchmem -count=1
```

Expected:
- PASS
- benchmark output lines are printed

- [ ] **Step 7: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify concurrency and load coverage"
```

---

## 6. Acceptance Criteria

Task 28 is complete only when all are true:
- real PostgreSQL race tests prove one-winner invitation behavior for the four roadmap race cases
- real PostgreSQL replay tests prove append-only and live-row idempotency under duplicate delivery
- stale invitation action conflicts remain mapped to `409`
- duplicate projector delivery does not create duplicate logical notifications
- projector retries do not inflate unread counts
- recipient overlap is deduped deterministically
- self-mentions do not notify the actor
- race-detector verification passes for the in-process concurrency suite
- benchmark suite runs successfully and reports allocations
- verification commands are documented for operators and future developers
- checkpoint is updated

## 7. Risks And Guardrails

- Do not weaken concurrency tests by using fake repositories for invitation races.
- Do not set brittle benchmark time thresholds in CI.
- Do not change production code only to satisfy a test unless the change improves observability or deterministic behavior cleanly.
- Do not collapse comment and mention notifications into one logical assertion; they are separate by design.
- Do not use unordered recipient assertions where deterministic order is part of the contract.
