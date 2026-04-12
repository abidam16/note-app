# Task 26 GET Notifications Stream Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `GET /api/v1/notifications/stream` so authenticated users can receive near-real-time unread-count updates and inbox invalidation signals over SSE.

**Architecture:** This task adds a lightweight real-time channel on top of the stable notification read model. REST remains the source of truth for inbox items. The stream sends only an initial snapshot plus lightweight change events. To support multiple API instances without introducing new infrastructure, the backend uses PostgreSQL `LISTEN/NOTIFY` as the fanout transport. Notification write paths publish best-effort user-scoped invalidation signals after successful commits, and a stream service translates those signals into SSE events by reading the current unread counter and comparing it against the last count sent to the connection. Because SSE is long-lived, the stream route must bypass the normal request timeout path, and the HTTP server write timeout must no longer cap streaming responses.

**Tech Stack:** Go, PostgreSQL, `pgx`, `net/http`, `chi`, Server-Sent Events, SQL repositories, PostgreSQL `LISTEN/NOTIFY`, table-driven tests

---

## 1. Scope

### In Scope
- Add one new endpoint:
  - `GET /api/v1/notifications/stream`
- Stream lightweight notification events over SSE
- Send an initial unread-count snapshot on connect
- Send unread-count change events when the unread value changes
- Send inbox invalidation events when the user's inbox may need refresh, even if unread count did not change
- Use PostgreSQL `LISTEN/NOTIFY` for cross-instance fanout
- Publish best-effort user-scoped invalidation signals from notification write paths
- Keep REST inbox and unread-count endpoints as the source of truth
- Add application, infrastructure, repository, handler, server, and docs tests
- Update app wiring and timeout configuration to support long-lived SSE

### Out Of Scope
- No WebSocket endpoint
- No replayable event log
- No persistent stream cursor or event history table
- No change to inbox item DTOs
- No change to notification write semantics beyond best-effort publish hooks
- No per-type stream subscriptions
- No UI behavior or frontend implementation

### Prerequisites
- Task 12 inbox API exists
- Task 14 unread-count endpoint and counter table exist
- Task 15 batch mark-read may exist
- Notification projectors and write paths already update the inbox read model correctly

---

## 2. Detailed Spec

## 2.1 Objective

Polling `GET /api/v1/notifications` for freshness is wasteful once the inbox read model is stable. The stream endpoint gives the frontend a cheap way to know:
- the latest unread badge value
- when the inbox should be refetched

This stream is intentionally not the source of truth. It only tells the client when to refresh. Clients must still use:
- `GET /api/v1/notifications`
- `GET /api/v1/notifications/unread-count`

to get canonical state.

## 2.2 Endpoint

### `GET /api/v1/notifications/stream`

- Auth: yes
- Authorization: authenticated actor only
- Transport: `text/event-stream`

### Request Payload
- none

### Query Parameters
- none in v1

### Header Rules
- clients may send `Accept: text/event-stream`
- clients may send `Last-Event-ID`, but the server does not replay missed events in v1
- if `Last-Event-ID` is present, the server ignores it and still sends a fresh snapshot

## 2.3 Response Contract

### Positive Response
- `200 OK`
- headers:
  - `Content-Type: text/event-stream`
  - `Cache-Control: no-cache`
  - `Connection: keep-alive`
  - `X-Accel-Buffering: no`

### Stream Event Types

#### 1. `snapshot`
Sent immediately after the stream opens.

Example:

```text
event: snapshot
data: {"unread_count":12,"sent_at":"2026-04-04T10:00:00Z"}

```

Payload:

```json
{
  "unread_count": 12,
  "sent_at": "2026-04-04T10:00:00Z"
}
```

#### 2. `unread_count`
Sent only when the current unread count differs from the last count sent on this connection.

Example:

```text
event: unread_count
data: {"unread_count":11,"sent_at":"2026-04-04T10:00:05Z"}

```

Payload:

```json
{
  "unread_count": 11,
  "sent_at": "2026-04-04T10:00:05Z"
}
```

#### 3. `inbox_invalidated`
Sent whenever the user's inbox may need refetch, even if unread count stayed the same.

Example:

```text
event: inbox_invalidated
data: {"reason":"notifications_changed","sent_at":"2026-04-04T10:00:05Z"}

```

Payload:

```json
{
  "reason": "notifications_changed",
  "sent_at": "2026-04-04T10:00:05Z"
}
```

### Heartbeat Rule
To keep proxies and clients from treating the connection as idle, the server must send a heartbeat comment at least every `25s`.

Example:

```text
: keep-alive

```

Heartbeat comments are not part of the public event payload contract.

## 2.4 Validation Rules

### Authentication Rules
1. actor must be authenticated
2. missing, invalid, or expired auth returns `401 unauthorized`

### Stream Capability Rule
If the server cannot flush streaming responses because the `ResponseWriter` does not implement `http.Flusher`:
- return `500 internal_error` before sending stream headers

### Startup Rules
If the server cannot:
- load the actor
- load initial unread count
- subscribe the actor to the stream backend

before the stream starts:
- return `500 internal_error`

### Post-Start Failure Rule
If a later stream-internal error happens after headers are sent, such as:
- unread-count lookup failure on a broker event
- listener connection failure

then:
- the server closes the stream
- the client must reconnect and refresh from REST

No JSON error is written after stream headers are sent.

## 2.5 Reconnect Rules

### No Replay Rule In V1
This task does not implement replayable event history.

Therefore:
- `Last-Event-ID` is ignored
- reconnecting clients always receive a new `snapshot`

### Client Reconnect Guidance
On reconnect, clients should:
1. open the stream again
2. accept the new `snapshot`
3. refetch `GET /api/v1/notifications` if the UI needs canonical inbox items

This is acceptable because:
- the stream is only an invalidation channel
- REST remains the source of truth

## 2.6 Event Semantics

### Snapshot Rule
One `snapshot` event must be sent on every successful connection before any live update events.

### Unread-Count Rule
When a write causes the user's unread count to change, the connection should eventually receive:
- one `unread_count` event with the latest value
- one `inbox_invalidated` event

### Invalidation Rule
When a write changes inbox-visible data without changing unread count, such as:
- updating an existing live invitation notification while preserving read state

the connection should receive:
- no `unread_count` event if the count is unchanged
- one `inbox_invalidated` event

### Coalescing Rule
If multiple write events happen close together:
- it is acceptable for a connection to receive fewer events than writes
- the backend may coalesce changes implicitly because it always reads the latest unread count on notification

This is valid because the stream is not a durable event log.

## 2.7 Backend Signal Transport

Use PostgreSQL `LISTEN/NOTIFY` as the cross-instance invalidation channel.

### Channel Name
Recommended:
- `notification_stream`

### Notify Payload
Recommended JSON payload:

```json
{
  "user_id": "uuid",
  "reason": "notifications_changed",
  "sent_at": "2026-04-04T10:00:05Z"
}
```

### Payload Rules
1. `user_id` must be present and non-empty
2. `reason` is always `notifications_changed` in v1
3. `sent_at` should be RFC3339 UTC

### Delivery Semantics
- `LISTEN/NOTIFY` is best effort
- missed NOTIFY events are acceptable because reconnect always produces a fresh snapshot
- stream publisher failure must not fail the original notification write

## 2.8 Publish Triggers

After successful inbox-affecting writes, publish a user-scoped invalidation signal.

### Publish On These Effective Changes
1. single notification insert that actually inserts a row
2. batch notification insert for each user who actually got at least one inserted row
3. invitation live-notification insert or update that changes inbox-visible state
4. single mark-read that actually changes unread state
5. batch mark-read from Task 15 for each user whose unread state actually changed

### Do Not Publish On These No-Op Cases
1. insert conflict where no row is created
2. repeated mark-read that leaves the row already read
3. batch mark-read request where no row changes

### Best-Effort Rule
Publish happens after the database write succeeds.

If publishing fails:
- log the failure
- do not fail the request or projector batch

## 2.9 Application Stream Service

Add a stream-focused application service.

Recommended service:

```go
type NotificationStreamService struct {
    users     UserRepository
    counters  NotificationUnreadCountRepository
    broker    NotificationStreamBroker
    now       func() time.Time
}
```

Recommended open method:

```go
Open(ctx context.Context, actorID string) (NotificationStreamSession, error)
```

Recommended session:

```go
type NotificationStreamSession interface {
    InitialUnreadCount() int64
    Events() <-chan NotificationStreamEvent
    Close() error
}
```

Recommended event:

```go
type NotificationStreamEvent struct {
    Type        string
    UnreadCount *int64
    Reason      string
    SentAt      time.Time
}
```

### Service Algorithm
1. resolve actor by id
2. load initial unread count
3. subscribe the actor to the broker
4. return a session with:
   - initial unread count
   - event channel
5. inside the session:
   - on each broker signal, fetch current unread count
   - if count changed from last sent value:
     - emit `unread_count`
   - always emit `inbox_invalidated`

### Default Count Rule
The stream service uses the unread-count repository, so missing unread-counter rows resolve to `0`.

## 2.10 HTTP Handler Behavior

Add a dedicated SSE handler instead of using `WriteJSON`.

### Handler Algorithm
1. verify streaming support via `http.Flusher`
2. open the stream session
3. set SSE headers
4. write initial `snapshot`
5. loop on:
   - request context cancellation
   - session events
   - heartbeat ticker
6. flush after each event or heartbeat
7. close the session when the request ends

### Timeout Rule
The stream route must bypass the normal `30s` handler timeout middleware.

### Server Write Timeout Rule
The HTTP server `WriteTimeout` must no longer terminate long-lived streams. Recommended:
- set `http.Server.WriteTimeout = 0`
- keep per-route timeout middleware for normal endpoints

## 2.11 Routing Rules

The current server applies `chimiddleware.Timeout(30 * time.Second)` globally, which is incompatible with SSE.

Implementation rule:
- move timeout middleware off the root router
- apply timeout only to non-stream routes
- keep `/api/v1/notifications/stream` inside the authenticated API group but outside the timed subgroup

This preserves timeout protection for normal HTTP endpoints while allowing the stream to stay open.

## 2.12 Positive And Negative Cases

### Positive Cases

1. Authenticated user connects successfully
- result: `200`
- receives one `snapshot` immediately

2. A new unread notification is inserted for the user
- result:
  - user eventually receives `unread_count`
  - user eventually receives `inbox_invalidated`

3. A live invitation notification updates without changing unread count
- result:
  - user receives `inbox_invalidated`
  - user may receive no `unread_count` event if count is unchanged

4. Client reconnects after disconnect
- result:
  - receives a fresh `snapshot`
  - uses REST for canonical state

5. Multiple API instances are running
- result:
  - an update written on one instance can notify stream connections attached to another instance through `LISTEN/NOTIFY`

### Negative Cases

1. Missing auth
- result: `401 unauthorized`

2. Invalid or expired auth
- result: `401 unauthorized`

3. `ResponseWriter` cannot flush
- result: `500 internal_error`

4. Initial unread-count lookup fails
- result: `500 internal_error`

5. Broker subscribe fails before headers are written
- result: `500 internal_error`

6. Broker or unread-count failure after the stream already started
- result:
  - server closes the stream
  - client reconnects

---

## 3. File Structure And Responsibilities

### Create
- `internal/application/notification_stream.go`
  - stream service, session abstraction, event types, and unread-count change detection
- `internal/application/notification_stream_test.go`
  - unit tests for snapshot, invalidation, unread-count change, and reconnect-related logic
- `internal/infrastructure/database/notification_stream.go`
  - PostgreSQL `LISTEN/NOTIFY` broker with local subscriber fanout
- `internal/infrastructure/database/notification_stream_test.go`
  - broker tests for payload parsing, user filtering, subscriber cleanup, and local fanout
- `internal/transport/http/sse.go`
  - small helper for writing SSE events and heartbeats

### Modify
- `internal/domain/notification.go`
  - only if a small stream payload or publisher interface type is needed as a boundary contract
- `internal/application/notification_service.go`
  - add or expose unread-count repository interface if needed by the stream service
- `internal/repository/postgres/notification_repository.go`
  - publish best-effort user invalidation signals after successful inbox-affecting mutations
- `internal/repository/postgres/content_repository_test.go`
  - add coverage that publish hooks trigger only on effective changes
- `internal/repository/postgres/closed_pool_errors_test.go`
  - add coverage for broker or publish integration points if those paths are exercised there
- `internal/transport/http/handlers.go`
  - add `handleNotificationsStream`
- `internal/transport/http/server.go`
  - add stream route and carve it out from timeout middleware
- `internal/transport/http/server_test.go`
  - add SSE endpoint tests
- `cmd/api/app.go`
  - build and inject the notification stream broker and service
  - adjust server write-timeout configuration for streaming
- `cmd/api/app_test.go`
  - update hardening expectations for write timeout and new wiring
- `frontend-repo/API_CONTRACT.md`
  - document the stream endpoint and event formats
- `docs/checkpoint.md`

### Files Explicitly Not In Scope
- `internal/application/comment_notification_projector.go`
- `internal/application/mention_notification_projector.go`
- `internal/application/workspace_service.go`
- `internal/application/thread_service.go`
- `migrations/`

---

## 4. Test Matrix

## 4.1 Application Stream Service Tests

Add focused tests in:
- `internal/application/notification_stream_test.go`

### Positive Cases

1. opening a session returns initial unread count

2. broker signal with changed count emits:
- one `unread_count` event
- one `inbox_invalidated` event

3. broker signal with unchanged count emits:
- no `unread_count`
- one `inbox_invalidated`

4. multiple broker signals keep using the latest last-sent count

### Negative Cases

5. unknown actor returns `domain.ErrUnauthorized`

6. initial unread-count lookup failure returns error

7. broker subscribe failure returns error

8. post-open unread-count lookup failure closes the session or stops event delivery cleanly

## 4.2 Infrastructure Broker Tests

Add focused tests in:
- `internal/infrastructure/database/notification_stream_test.go`

### Positive Cases

9. subscriber receives payloads only for their user id

10. multiple local subscribers for the same user all receive a signal

11. subscribers for different users do not receive each other's signals

12. valid PostgreSQL NOTIFY payload parses correctly

### Negative Cases

13. malformed NOTIFY payload is ignored without crashing the broker

14. unsubscribed client stops receiving events

15. publish failure returns error to the caller

## 4.3 Notification Repository Integration Tests

Add or extend tests in:
- `internal/repository/postgres/content_repository_test.go`

### Positive Cases

16. single notification insert that creates a row publishes one user invalidation

17. batch insert publishes for each user who actually received at least one inserted row

18. first unread-to-read transition publishes one user invalidation

19. live invitation update that changes inbox-visible state publishes one user invalidation

### Negative Cases

20. insert conflict publishes nothing

21. repeated mark-read publishes nothing

22. publisher failure does not fail the successful repository mutation

## 4.4 HTTP Tests

Add or update tests in:
- `internal/transport/http/server_test.go`

### Positive Cases

23. `GET /api/v1/notifications/stream` returns `200` and `Content-Type: text/event-stream`

24. stream writes one initial `snapshot` event

25. after a broker signal, stream writes:
- `unread_count` when count changed
- `inbox_invalidated`

26. stream writes heartbeat comments while idle

### Negative Cases

27. missing auth returns `401`

28. stream service open failure returns `500` before headers

29. non-flushing response writer returns `500`

30. route is not cut off by the normal 30-second request-timeout middleware

## 4.5 App Wiring Tests

Add or update tests in:
- `cmd/api/app_test.go`

### Positive Cases

31. default server wiring still builds successfully

32. HTTP server write timeout is disabled or otherwise compatible with long-lived SSE

### Negative Cases

33. invalid DB type behavior remains unchanged if buildDefaultServer still guards it

## 4.6 Documentation Tests

34. `frontend-repo/API_CONTRACT.md` documents:
- `GET /api/v1/notifications/stream`
- auth requirement
- SSE event types:
  - `snapshot`
  - `unread_count`
  - `inbox_invalidated`
- reconnect guidance
- REST inbox remains the source of truth

35. `docs/checkpoint.md` records:
- stream endpoint added
- SSE uses PostgreSQL `LISTEN/NOTIFY`
- publish failures are best effort and do not fail writes
- timeout carve-out and write-timeout change for streaming

---

## 5. Execution Plan

### Task 1: Define failing application and broker tests

**Files:**
- Create: `internal/application/notification_stream_test.go`
- Create: `internal/infrastructure/database/notification_stream_test.go`

- [ ] **Step 1: Add failing stream-service tests**

Cover:
- initial snapshot count
- changed versus unchanged unread count behavior
- open failure paths

- [ ] **Step 2: Add failing broker tests**

Cover:
- user filtering
- local fanout
- malformed payload handling

- [ ] **Step 3: Run targeted tests**

Run:
```powershell
go test ./internal/application -run "TestNotificationStreamService" -count=1
go test ./internal/infrastructure/database -run "TestPostgresNotificationStream" -count=1
```

Expected:
- FAIL because the stream service and broker do not exist yet

- [ ] **Step 4: Commit**

```bash
git add internal/application/notification_stream_test.go internal/infrastructure/database/notification_stream_test.go
git commit -m "test: define notification stream behavior"
```

### Task 2: Implement PostgreSQL broker and stream service

**Files:**
- Create: `internal/application/notification_stream.go`
- Create: `internal/infrastructure/database/notification_stream.go`
- Modify: `internal/application/notification_service.go`
- Modify: `internal/application/notification_stream_test.go`
- Modify: `internal/infrastructure/database/notification_stream_test.go`

- [ ] **Step 1: Implement the PostgreSQL `LISTEN/NOTIFY` broker**

Required behavior:
- one channel name
- local subscriber fanout by user id
- safe unsubscribe and cleanup
- lazy or safe startup so existing app build tests still work

- [ ] **Step 2: Implement the application stream service**

Required behavior:
- authenticate actor through existing user repository
- load initial unread count
- open a broker subscription
- translate broker signals into `unread_count` and `inbox_invalidated`

- [ ] **Step 3: Re-run targeted tests**

Run:
```powershell
go test ./internal/application -run "TestNotificationStreamService" -count=1
go test ./internal/infrastructure/database -run "TestPostgresNotificationStream" -count=1
```

Expected:
- PASS

- [ ] **Step 4: Commit**

```bash
git add internal/application/notification_stream.go internal/infrastructure/database/notification_stream.go internal/application/notification_service.go internal/application/notification_stream_test.go internal/infrastructure/database/notification_stream_test.go
git commit -m "feat: add notification stream broker and service"
```

### Task 3: Add best-effort publish hooks to notification write paths

**Files:**
- Modify: `internal/repository/postgres/notification_repository.go`
- Modify: `internal/repository/postgres/content_repository_test.go`
- Modify: `internal/repository/postgres/closed_pool_errors_test.go`

- [ ] **Step 1: Add failing repository integration tests**

Cover:
- publish on effective insert
- publish on first read transition
- no publish on no-op conflict or repeated read
- publisher failure does not fail the write

- [ ] **Step 2: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository|TestClosedPoolRepositories" -count=1
```

Expected:
- FAIL because publish hooks do not exist yet

- [ ] **Step 3: Implement publisher injection and best-effort hooks**

Required behavior:
- optional publisher dependency
- publish only after successful state change
- log and continue on publish failure

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
git commit -m "feat: publish notification stream invalidations"
```

### Task 4: Add SSE transport and route timeout carve-out

**Files:**
- Create: `internal/transport/http/sse.go`
- Modify: `internal/transport/http/handlers.go`
- Modify: `internal/transport/http/server.go`
- Modify: `internal/transport/http/server_test.go`

- [ ] **Step 1: Add failing HTTP tests for the stream endpoint**

Cover:
- `200` stream open
- initial `snapshot`
- event delivery after broker signal
- `401` without auth
- non-flushing writer failure

- [ ] **Step 2: Run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "TestNotificationStreamEndpoint" -count=1
```

Expected:
- FAIL because the route and handler do not exist yet

- [ ] **Step 3: Implement SSE helper, handler, and timeout carve-out**

Required behavior:
- stream route under authenticated API routes
- no normal request timeout on this route
- send headers, snapshot, live events, and heartbeats
- close cleanly on client disconnect

- [ ] **Step 4: Re-run targeted HTTP tests**

Run:
```powershell
go test ./internal/transport/http -run "TestNotificationStreamEndpoint" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/transport/http/sse.go internal/transport/http/handlers.go internal/transport/http/server.go internal/transport/http/server_test.go
git commit -m "feat: add notifications SSE endpoint"
```

### Task 5: Update app wiring and server timeouts

**Files:**
- Modify: `cmd/api/app.go`
- Modify: `cmd/api/app_test.go`

- [ ] **Step 1: Add failing app wiring tests**

Cover:
- server still builds
- write timeout is compatible with SSE

- [ ] **Step 2: Run targeted app tests**

Run:
```powershell
go test ./cmd/api -run "TestBuildDefaultServer" -count=1
```

Expected:
- FAIL because stream wiring and timeout expectations are not implemented yet

- [ ] **Step 3: Wire the broker and stream service into the app**

Required behavior:
- create broker from the PostgreSQL pool
- inject broker into notification repository and stream service
- inject stream service into the HTTP server
- remove or neutralize write timeout as required for SSE

- [ ] **Step 4: Re-run targeted app tests**

Run:
```powershell
go test ./cmd/api -run "TestBuildDefaultServer" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/api/app.go cmd/api/app_test.go
git commit -m "feat: wire notification stream support"
```

### Task 6: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update API contract**

Document:
- endpoint path
- SSE headers and event types
- reconnect guidance
- REST remains source of truth
- `401` and startup `500` cases

- [ ] **Step 2: Update checkpoint**

Record:
- SSE stream endpoint added
- PostgreSQL `LISTEN/NOTIFY` fanout selected
- publish hooks are best effort
- timeout carve-out for stream route

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: record notification stream contract"
```

### Task 7: Full verification for Task 26

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "TestNotificationStreamService" -count=1
go test ./internal/infrastructure/database -run "TestPostgresNotificationStream" -count=1
go test ./internal/repository/postgres -run "TestNotification|TestContentRepository|TestClosedPoolRepositories" -count=1
go test ./internal/transport/http -run "TestNotificationStreamEndpoint" -count=1
go test ./cmd/api -run "TestBuildDefaultServer" -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual API sanity check if local server is available**

Open:
```http
GET /api/v1/notifications/stream
Authorization: Bearer <token>
Accept: text/event-stream
```

Verify:
- `200`
- immediate `snapshot` event with unread count
- creating or reading notifications causes `inbox_invalidated`
- unread-count-changing actions also cause `unread_count`
- idle stream still receives heartbeat comments

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify notification stream task"
```

---

## 6. Acceptance Criteria

Task 26 is complete only when all are true:
- `GET /api/v1/notifications/stream` exists
- it returns `text/event-stream`
- it sends an initial `snapshot` event on connect
- it sends `unread_count` only when the count changes
- it sends `inbox_invalidated` when inbox data may need refresh
- REST inbox and unread-count endpoints remain the source of truth
- notification write paths publish best-effort user invalidation signals after effective changes
- publish failures do not fail successful writes
- the stream route bypasses the normal request-timeout middleware
- server write timeout no longer kills long-lived streams
- tests and docs cover the positive and negative cases above

## 7. Risks And Guardrails

- Do not send full inbox rows over SSE in this task.
- Do not make SSE the source of truth for notification state.
- Do not fail inbox writes or mark-read requests because stream publish failed.
- Do not leave the stream route under the normal `30s` timeout middleware.
- Do not keep `WriteTimeout: 35s` if it would terminate valid stream connections.
- Do not rely on replay or `Last-Event-ID` in v1.
- Do not publish on no-op conflicts or repeated mark-read operations.

## 8. Follow-On Tasks

This plan prepares for:
- Task 27 notification reconciliation job
- later frontend EventSource integration

If future scale or delivery guarantees outgrow `LISTEN/NOTIFY`, the stream broker can later be swapped for a dedicated pub/sub system without changing the public SSE contract defined here.
