# Notification Verification

Use these commands from the repository root.

## Important Test-Run Rules

- Run PostgreSQL-backed suites serially when broadening beyond one targeted package. The integration tests share one test database, so package-parallel runs can produce false failures.
- Use the targeted commands first while iterating. Use the repeated `-count=10` passes when you need higher confidence against timing-sensitive regressions.
- Run `-race` after concurrency, projector, or shared-fixture changes. The standard correctness suite and the race-detector suite prove different failure modes.

## Targeted Correctness Commands

Run these first after changes to invitation concurrency, projector replay, unread counters, or stale-version mapping:

```powershell
go test ./internal/repository/postgres -run "TestInvitationConcurrency|TestNotificationReplayIdempotency" -count=1
go test ./internal/application -run "TestNotificationProjectionConcurrency" -count=1
go test ./internal/transport/http -run "TestWorkspaceInvitation.*Conflict|TestAcceptInvitation.*Conflict|TestRejectInvitation.*Conflict|TestCancelInvitation.*Conflict" -count=1
```

These suites prove:

- invitation update, cancel, accept, and reject races resolve to one winner only
- replayed invitation live rows preserve read state and do not inflate unread counts
- append-only comment and mention replay stays idempotent under duplicate delivery
- projector retries complete missing work without creating duplicates
- recipient overlap is deduped deterministically and self-mentions do not notify the actor
- stale invitation versions still map to `409 Conflict`

## Repeated Confidence Passes

Rerun these when a failure looks timing-sensitive or after touching locking, retry, or uniqueness semantics:

```powershell
go test ./internal/repository/postgres -run "TestInvitationConcurrency" -count=10
go test ./internal/repository/postgres -run "TestNotificationReplayIdempotency" -count=10
go test ./internal/application -run "TestNotificationProjectionConcurrency" -count=10
```

Use the repeated pass when:

- invitation version checks, row locking, or transaction shape changed
- notification uniqueness or unread-counter semantics changed
- projector retry logic or recipient derivation changed
- a one-off failure needs to be separated from a deterministic regression

## Race-Detector Commands

Run these after changes to shared state, test scaffolding, orchestration helpers, or concurrency control:

```powershell
go test -race -p 1 ./internal/application -run "TestNotificationProjectionConcurrency" -count=1
go test -race -p 1 ./internal/application ./internal/repository/postgres ./internal/transport/http -count=1
```

Use the targeted race command first when you only changed projector logic. Use the broader serial race pass before closing a task that touched multiple backend layers.

## Benchmarks

Run the targeted load checks:

```powershell
go test ./internal/application -run ^$ -bench "BenchmarkInvitationProjectorReplay|BenchmarkThreadNotificationHistoryBuilder|BenchmarkNotificationBatchProjection" -benchmem -count=1
```

These benchmarks measure:

- invitation projector replay overhead
- thread notification history builder cost
- batch notification projection cost

Benchmark results are comparative regression signals, not pass-fail correctness checks.

## Failure Interpretation

- `TestInvitationConcurrency` failure usually means version-check, row-lock, or final-state membership semantics regressed.
- `TestNotificationReplayIdempotency` failure usually means PostgreSQL uniqueness or unread-counter persistence rules regressed.
- `TestNotificationProjectionConcurrency` failure usually means projector retry, dedupe, actor exclusion, or fake-harness accounting regressed.
- stale-version HTTP conflict failure usually means a service error mapping or transport status mapping regressed.
- `-race` failure means shared in-process state is unsafe even if correctness tests still pass.
- benchmark compile or execution failure means the hot-path verification harness is broken and Task 28 is not complete.
