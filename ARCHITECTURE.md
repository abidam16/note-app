# 1. System Purpose

This repository is a Go backend for a workspace-scoped document system with role-based access, mutable page drafts, immutable manual revisions, asynchronous discussion, and user-scoped notifications.

Architecturally, it prioritizes:
- server-enforced authority boundaries around workspace membership and roles
- clear separation between current draft state, historical revision state, deleted state, and projected notification state
- PostgreSQL-backed durability with explicit SQL, transactional writes for core mutations, and bounded HTTP behavior
- recoverability over aggressive deletion

Its role relative to the product is to be the authoritative backend for identity, workspace access, content state, discussion state, and notification data ownership. Frontend behavior should consume these server-owned rules rather than re-implement them.

# 2. System Shape

Runtime shape:
- One primary API binary: `cmd/api`
- One operational CLI binary: `cmd/notification-reconcile`
- One PostgreSQL database for all durable state
- No external broker, cache, or search service in the current codebase

Code shape:
- `internal/transport/http`: HTTP routing, middleware, JSON decoding, SSE framing, status mapping
- `internal/application`: use-case orchestration, validation, authorization checks, recipient resolution, anchor reevaluation, reconciliation logic
- `internal/domain`: stable data types, enums, shared error taxonomy, outbox payload validation helpers
- `internal/repository/postgres`: explicit SQL repositories and most transaction boundaries
- `internal/infrastructure/*`: config loading, PostgreSQL pool setup, JWT/password helpers, local file storage, PostgreSQL `LISTEN/NOTIFY` broker

Request flow:
1. Chi router and middleware establish request id, recovery, security headers, rate limiting, auth context, and logging.
2. Transport handlers parse and normalize request input.
3. Application services enforce membership, role rules, validation, and use-case sequencing.
4. PostgreSQL repositories execute SQL and commit transaction-scoped state.
5. Transport maps domain/application errors to HTTP responses.

Background and async shape:
- The shipped API binary does not start a background projector worker.
- The codebase contains projector implementations for invitation, comment, and mention notifications, plus an outbox repository, but no long-running worker process is wired in `cmd/api` or another shipped command.
- The shipped operational async path is `cmd/notification-reconcile`, which repairs managed notification projections and unread counters from source tables.

Streaming shape:
- `GET /api/v1/notifications/stream` is SSE.
- The stream is fed by PostgreSQL `LISTEN/NOTIFY` invalidation plus a fresh unread-count read on each signal.
- REST inbox and unread-count endpoints remain the canonical read APIs for notification state.

# 3. Architectural Boundaries and Responsibilities

Transport (`internal/transport/http`)
- Responsibilities:
  - HTTP routing and middleware
  - request decoding, query parsing, input shape normalization
  - auth token extraction and user id injection into context
  - mapping domain/application errors to HTTP status and envelope shape
  - SSE response formatting
- Must not do:
  - business validation beyond request-shape parsing
  - authorization policy decisions beyond requiring authentication
  - direct SQL or transaction control
  - projection or counter maintenance logic

Application (`internal/application`)
- Responsibilities:
  - use-case orchestration
  - authorization checks from workspace membership and role
  - document validation and block-anchor validation
  - hiding foreign resources as `not_found` where the product contract requires it
  - thread recipient resolution, anchor reevaluation, notification reconciliation
- Must not do:
  - HTTP-specific behavior
  - raw SQL
  - persistence-specific retry/locking implementation details
- Current caveat:
  - some operations orchestrate multiple repository calls without one enclosing transaction, so the application layer currently owns some non-atomic sequencing risk

Domain (`internal/domain`)
- Responsibilities:
  - stable entities, DTO-like structs, enums, and shared errors
  - lightweight invariants such as outbox payload validation helpers and enum validation
- Must not do:
  - repository access
  - transport logic
  - request orchestration
- Observed shape:
  - mostly an anemic model package, not a rich domain-logic layer

Repository / persistence (`internal/repository/postgres`)
- Responsibilities:
  - SQL queries
  - transaction boundaries for atomic writes
  - row locking, uniqueness handling, cursor pagination, and persistence mapping
  - read models such as inbox pages, unread counters, and thread list counts
- Must not do:
  - HTTP error mapping
  - client-facing auth policy decisions
- Current caveat:
  - some visibility-optimized reads fold membership filtering into SQL for leak reduction

Projection / worker / integration code
- Responsibilities:
  - projector types consume outbox events and build notification projections
  - reconciliation rebuilds managed notification rows and unread counters from authoritative tables
  - notification stream broker publishes and subscribes to user-scoped invalidation signals
- Must not do:
  - become the source of truth for invitations, pages, threads, or memberships
- Current state:
  - implemented, but only reconciliation is wired into a shipped command; projector workers are not

# 4. Core Domains and Their Technical Realization

Identity and session domain
- Main structures:
  - `users`
  - `refresh_tokens`
  - `application.AuthService`
  - `infrastructure/auth.TokenManager`
  - `infrastructure/auth.PasswordManager`
- Relationships:
  - one user can own many refresh tokens
  - access tokens are stateless JWTs; refresh tokens are opaque tokens hashed into the database
- Maturity:
  - mature for email/password auth and refresh rotation
  - no password reset, email verification, or access-token revocation layer

Workspace access and invitation domain
- Main structures:
  - `workspaces`
  - `workspace_members`
  - `workspace_invitations`
  - `application.WorkspaceService`
- Relationships:
  - workspace membership is the authoritative access boundary
  - invitations are pre-membership lifecycle records with explicit status and version
- Maturity: Transitional
  - workspace and invitation lifecycle is mature as source data
  - invitation notification delivery is still transitional; source records are stronger than the projection path

Content domain
- Main structures:
  - `folders`
  - `pages`
  - `page_drafts`
  - `revisions`
  - `trash_items`
  - `application.FolderService`, `PageService`, `RevisionService`, `SearchService`
- Relationships:
  - page metadata lives in `pages`
  - current editable content lives in exactly one `page_drafts` row per page
  - immutable checkpoints live in `revisions`
  - deletion lifecycle uses `pages.deleted_at/deleted_by` plus `trash_items`
- Maturity: Architecturally weak at the mutation edges
  - current draft, manual revision, search, and trash semantics are mature
  - restore and anchor reevaluation sequencing is still architecturally weak

Collaboration domain
- Main structures:
  - legacy `page_comments`
  - `page_comment_threads`
  - `page_comment_messages`
  - `page_comment_thread_events`
  - `page_comment_message_mentions`
  - `thread_notification_preferences`
  - `application.CommentService`, `ThreadService`
- Relationships:
  - flat comments are page-level records
  - threads are anchored discussion records with ordered messages and lifecycle events
  - mentions belong to thread messages, not to free-text parsing
  - per-thread notification preference rows are sparse overrides per `(thread_id, user_id)`
- Maturity: Legacy-compatible and under active evolution
  - threads are the more actively evolved model
  - flat comments remain live, so collaboration architecture is still dual-track
  - stored thread preferences are present but not yet applied to delivery

Notification and awareness domain
- Main structures:
  - `notifications`
  - `notification_unread_counters`
  - `outbox_events`
  - SSE stream broker and stream service
  - reconciliation service and repository
- Relationships:
  - notifications are per-user inbox items
  - unread counters are a per-user derived read model
  - outbox events represent pending async work, not business truth
- Maturity: Transitional
  - inbox read APIs, mark-read behavior, unread counters, and SSE invalidation are mature
  - end-to-end live async notification production is transitional because projector workers are not wired into shipped binaries

# 5. Data Ownership and Source of Truth

Authoritative
Source tables:
- `users`: canonical identity record
- `refresh_tokens`: canonical refresh-token revocation state
- `workspaces`: canonical workspace metadata
- `workspace_members`: canonical membership and role state
- `workspace_invitations`: canonical invitation lifecycle and optimistic-concurrency version
- `folders`: canonical folder tree
- `pages`: canonical page metadata and active-vs-trashed visibility flag via `deleted_at`
- `page_drafts`: canonical current editable document
- `revisions`: canonical immutable checkpoints
- `page_comments`: canonical legacy flat-comment records
- `page_comment_threads`: canonical thread records and anchor snapshot fields
- `page_comment_messages`: canonical thread messages
- `page_comment_thread_events`: canonical persisted thread lifecycle history
- `page_comment_message_mentions`: canonical explicit mention rows
- `thread_notification_preferences`: canonical stored per-thread override rows

Authoritative
Joint state:
- Trash state is represented by both `pages.deleted_at/deleted_by` and the active `trash_items` row.
- In normal reads, `pages.deleted_at` is the immediate visibility gate.
- `trash_items` is the restore handle and deletion audit record.
- Future changes must preserve both sides together.

Derived
- `page_drafts.search_body` is derived from structured document content
- `pages.title_search` and `page_drafts.search_body_vector` are generated search vectors
- `page_comment_threads.anchor_state` is persisted derived state from comparing thread anchor snapshots to the current draft
- recovered `block_id` values on threads are persisted results of reevaluation logic, not free-form client data
- `notification_unread_counters` is derived from unread notification rows

Advisory / projected
- `notifications` is an inbox table containing invitation and thread-derived rows plus some direct legacy flat-comment fanout rows
- SSE `snapshot`, `unread_count`, and `inbox_invalidated` events are delivery hints
- thread list `counts`, `reply_count`, inbox pagination state, and transport response DTOs are derived read models

Never authoritative
- notification inbox rows for invitation state, thread state, membership, or page visibility
- unread counters as the source of unread truth
- SSE stream events as durable state
- search vectors or extracted search text as the source document
- thread list counts or transport response fields as durable records
- the absence of a `thread_notification_preferences` row as "missing data"; absence means effective default `all`

# 6. Write Model and Read Model

Write model
Direct source writes:
- auth register/login/refresh/logout mutate `users` and `refresh_tokens`
- workspace, member role, and invitation endpoints mutate workspace source tables
- folder/page/revision/trash endpoints mutate content tables directly
- flat comments write directly to `page_comments`
- thread create/reply/resolve/reopen write directly to thread tables
- thread notification preference endpoints mutate `thread_notification_preferences`

Write model
Transactional writes already enforced:
- workspace creation: `workspaces` + owner `workspace_members`
- invitation accept: membership creation + invitation state change
- invitation reject/cancel/update: single invitation row under lock
- page creation: `pages` + `page_drafts`
- page soft delete: `pages.deleted_*` + `trash_items`
- trash restore: page undelete + trash restore markers
- thread create: thread + starter message + mentions + thread event + outbox event
- thread reply: thread update + reply message + mentions + thread events + outbox event
- notification insert / live invitation upsert / mark-read / batch mark-read: notification row changes + unread counter changes
- combined comment+mention projection writes: notification rows + unread counters

Read model
Canonical reads:
- most business reads come directly from source tables
- inbox and unread-count reads come from the projected `notifications` table plus `notification_unread_counters`
- thread list endpoints are source-table reads with derived counts and cursor pagination
- search reads come from source tables plus generated search vectors

Read model
Projection-driven reads:
- invitation notifications are intended to be one live row per invitation
- comment and mention notifications are intended to be append-only rows derived from thread events
- unread count is served from `notification_unread_counters`, not by aggregating inbox rows on every request
- SSE is a freshness layer over the read models, not an alternative source

Current reality
- invitation create still calls `NotificationService.NotifyInvitationCreated` directly in the request path
- flat comment create still performs direct synchronous notification fanout
- thread create and reply already write outbox rows transactionally
- projector code exists, but no shipped worker is consuming outbox rows continuously
- reconciliation can repair managed invitation/thread notification rows later

Eventual
- managed invitation/comment/mention notification rows may lag source tables until an external projector or reconciliation run applies them
- unread counters are only as fresh as the underlying notification projection writes

# 7. Consistency Model

Immediate
- workspace creation produces both workspace and owner membership or neither
- invitation acceptance creates membership and advances invitation status/version together
- invitation update, reject, and cancel are row-locked and version-checked before commit
- page creation produces the page and its initial empty draft together
- page delete and restore keep page visibility state and trash record state aligned inside repository transactions
- thread create and reply persist their source records and matching outbox event together
- mark-read and batch mark-read update unread counters in the same transaction as notification state
- search body extraction is stored in the same draft update statement as content

Hazard
Not atomic across the full use case:
- draft update commits the draft first, then reevaluates each thread anchor in separate repository calls
- page delete commits deletion first, then reevaluates thread anchors afterward
- trash restore commits restore first, then reevaluates thread anchors afterward
- revision restore updates the draft, then reevaluates anchors, then creates a new restore revision as a separate write
- flat comment creation persists the comment before synchronous notification fanout
- invite-member create persists the invitation before direct notification creation

Hazard
Practical consequence:
- some request handlers can return an error after the main source write has already committed
- thread anchor reevaluation can partially update some threads before a later failure stops the loop
- revision restore can leave the draft restored even if thread reevaluation or restore-revision creation fails afterward

Guarantee
- revision restore is additive at the data-model level because it creates a new revision instead of rewriting old revisions
- trash restore does not remove historical revisions
- notification reconciliation can rebuild managed invitation/comment/mention rows and unread counters from authoritative tables
- reconciliation does not claim to repair every legacy notification row shape, especially flat-comment request-path notifications

Eventual
- source tables outrank projected notifications
- REST notification endpoints outrank SSE events
- when projection state and source tables disagree, source tables are authoritative and reconciliation is the repair mechanism

# 8. Concurrency Model

Control
- optimistic concurrency is explicit via `workspace_invitations.version`
- accept/reject/cancel/update lock the invitation row with `FOR UPDATE`
- stale version requests return `conflict`
- only one pending invitation per `(workspace_id, email)` is allowed by filtered unique index
- membership uniqueness on `(workspace_id, user_id)` prevents duplicate accept races

Control
Uniqueness and dedupe:
- `workspace_members(workspace_id, user_id)` unique
- pending invitation unique index on `(workspace_id, email)` where `status='pending'`
- folder sibling uniqueness is enforced on normalized names within one parent scope
- active trash uniqueness is enforced per page
- `page_comment_message_mentions(message_id, mentioned_user_id)` primary key
- `thread_notification_preferences(thread_id, user_id)` primary key
- outbox idempotency key unique
- notifications unique by `(user_id, type, event_id)` for append-only comment/mention semantics
- invitation live notifications unique by `(user_id, resource_id)` when `type='invitation'`

Idempotency
- resolve/reopen thread endpoints are idempotent by service logic
- mark single notification read is idempotent
- batch mark-read is idempotent for already-read rows
- invitation live projection preserves one row per invitation identity
- comment and mention projection writes are retry-safe when the projector path is used

Retry
- outbox claim/retry/dead-letter uses lease-based worker claiming with `FOR UPDATE SKIP LOCKED`
- retries are safe for projector writes because uniqueness prevents duplicate visible rows and unread increments happen only for newly inserted rows
- reconciliation is intended to be idempotent and single-run via advisory lock

Hazard
- pages and drafts have no version field or compare-and-swap guard; concurrent edits are last-write-wins
- page metadata updates are also last-write-wins
- thread resolve/reopen has no client-supplied version; later writes overwrite earlier state
- thread anchor reevaluation walks threads one by one, so concurrent reevaluations can interleave and overwrite each other
- SSE has no replay cursor support; duplicate or missed invalidations are tolerated by forcing REST refetch

# 9. Integration and Async Model

Outbox model:
- `outbox_events` exists with pending, processing, processed, and dead-letter states
- thread create and reply already append outbox events in the same transaction as source writes
- projector implementations exist for:
  - invitation live notification upsert
  - relevant-user comment notifications
  - explicit mention notifications

Current shipped async behavior:
- no binary in this repo continuously runs those projectors
- `cmd/api` wires notification SSE but not outbox consumption
- `cmd/notification-reconcile` is the only shipped operational async command

Synchronous follow-up behavior still present:
- invitation create performs direct notification creation in-process
- flat page comments perform direct workspace-wide notification fanout in-process

Reconciliation model:
- scans source invitations and thread history up to a cutoff time
- repairs only managed notification rows
- recomputes unread counters from notification-table truth, not delta math
- holds one advisory lock for the full run
- publishes best-effort stream invalidation after non-dry-run changes

Streaming model:
- PostgreSQL `pg_notify` publishes user-scoped invalidation payloads
- API-side stream session subscribes, rereads unread count, emits `unread_count` only on change, and emits `inbox_invalidated` on every signal
- heartbeat comments keep the SSE connection alive
- no `Last-Event-ID` replay or durable event log is implemented

Repair and reconciliation boundaries:
- reconciliation treats invitations and thread-message notifications as rebuildable managed data
- it intentionally leaves unrelated or legacy notification shapes alone
- it is both a repair path and, today, the only fully wired path for catching missed managed notification projections

# 10. Security and Authority Boundaries

Layering summary:
- transport establishes authentication context
- application enforces authorization policy
- persistence occasionally applies visibility filtering in SQL to reduce resource-existence leaks

Auth context establishment:
- `Authenticate` middleware parses `Authorization: Bearer <token>`
- JWT access tokens carry `sub` and `email`; successful parse injects `user_id` into request context
- refresh-token authority lives in PostgreSQL by hashed opaque token, not in the JWT layer

Authorization enforcement:
- application services re-check workspace membership and role from the database
- `workspace_members` is the canonical authority for resource access
- viewers are blocked from document/container mutations in application code
- notification ownership is enforced in repository reads and mark-read paths

Hide-as-not-found behavior:
- once a concrete page or thread has been resolved, foreign-membership access is often remapped to `not_found` to reduce existence leaks
- this pattern is enforced in page and thread visibility checks after resource resolution
- generic workspace membership failures for workspace-scoped endpoints still return the repository/application result directly

Trust boundaries to preserve:
- frontend role checks are advisory only; backend role checks are authoritative
- notification recipient resolution must continue to derive from current membership, not stale payload assumptions
- mention validation must continue to prove mentioned users belong to the current workspace
- SSE connections are authenticated per user and must only expose that user's invalidation stream

Infrastructure-side security controls:
- rate limiting is in-process and keyed by resolved client IP
- optional proxy-header trust requires explicit trusted CIDRs
- security headers and request logging sanitization are transport concerns

# 11. Forbidden Patterns

- Do not treat `notifications`, unread counters, SSE payloads, or API envelopes as source of truth for invitations, memberships, pages, or threads.
- Do not place authorization rules, role checks, or product validation in HTTP handlers.
- Do not add a second parallel thread-notification pattern; extend the existing outbox/projection model or the existing synchronous legacy path deliberately.
- Do not bypass invitation `version` checks or repository row locking.
- Do not mutate `page_comment_threads.anchor_state` or recovered `block_id` ad hoc; use the reevaluation logic so event history stays coherent.
- Do not build new collaboration product work on legacy `page_comments` unless the task is explicitly backward compatibility.
- Do not infer that `mode=all` means a stored thread preference row exists; sparse storage makes row absence the default.
- Do not recompute unread badges by scanning the inbox inside request handlers when the counter table is the intended read model.
- Do not assume outbox projector workers are running in production just because the types exist in the codebase.
- Do not turn foreign-resource `not_found` paths back into `forbidden` without intentionally changing the visibility contract.

# 12. Open Architectural Questions

High-impact architecture questions:
- How should outbox projectors run in deployment: inside `cmd/api`, in a separate worker binary, or via an external scheduler not represented in this repo?
- Should invitation notifications fully migrate to outbox-driven live projection instead of the current create-only synchronous write?
- Should flat-comment notifications stay synchronous and workspace-wide, or be retired or migrated into the newer thread notification model?
- Should thread notification preferences affect recipient resolution and projection, or remain stored-only until a later delivery policy is chosen?
- Should draft save, delete/restore, and revision restore be collapsed into stronger atomic units so anchor reevaluation cannot partially apply?
- Is `anchor_type='page_legacy'` still a real architectural path, or just schema residue now that create-thread accepts only block anchors?

Lower-priority technical cleanup questions:
- Is the currently injected local file-storage dependency a future subsystem or leftover scaffolding that should disappear from the server shape?

# 13. Architectural Debt and Planned Evolution

- Notification architecture is partially migrated:
  - thread create/reply already use transactional outbox writes
  - projector implementations exist
  - shipped binaries still do not run projector workers
- Invitation notifications are split:
  - create path writes a live notification directly
  - update/accept/reject/cancel source transitions do not currently emit live projector work in the shipped runtime
  - reconciliation is the safety net for managed invitation state drift
- Collaboration architecture is split:
  - legacy flat comments remain active
  - thread discussions are the newer model with richer history and mention support
- Thread preference architecture is incomplete:
  - storage and APIs exist
  - delivery logic does not consult preferences yet
- Anchor reevaluation is functionally present but technically weak:
  - reevaluation is synchronous request follow-up
  - it is not part of the same transaction as the triggering content mutation
  - failures can surface after the main write already committed
- Some boundary leakage is intentional but transitional:
  - some page visibility filtering is pushed into SQL to reduce existence leaks
  - this is practical, but it is a partial mix of access filtering into persistence queries
- a local file-storage dependency is still injected into the server shape even though no current endpoint uses it.

# 14. Change Guidance for Future Work

Where new logic usually belongs:
- add request parsing, envelope, and middleware changes in `internal/transport/http`
- add business validation, role checks, sequencing, and cross-repository orchestration in `internal/application`
- add new persistence rules, transactions, and indexes in `internal/repository/postgres` plus migrations
- add stable enums and shared record shapes in `internal/domain`

Changes that require special care:
- anything touching draft save, revision restore, page delete, or trash restore because thread anchor reevaluation is not atomic with the triggering write
- anything touching invitation state transitions because versioning and row-locking are correctness-critical
- anything touching notifications because uniqueness, read-state preservation, unread counters, and reconciliation assumptions are tightly coupled
- anything touching hide-as-not-found behavior because resource existence leakage is intentionally constrained
- anything touching thread recipient resolution because comment and mention delivery correctness depends on current membership filtering

When to prefer extension over a new pattern:
- extend the existing thread/outbox/projection path instead of adding a new notification fanout mechanism
- extend the sparse `thread_notification_preferences` model instead of creating parallel per-thread delivery state
- extend the current repository transaction boundaries rather than moving business logic into handlers

When architecture changes should trigger an ADR:
- introducing a new long-running worker or changing how outbox projection is deployed
- changing what data is source of truth versus projection
- changing consistency guarantees for draft/revision/thread reevaluation flows
- changing access-control visibility semantics
- introducing new infrastructure components beyond PostgreSQL and in-process middleware

When to update `PRD.md` versus `ARCHITECTURE.md`:
- update `PRD.md` when user-visible rules, roles, flows, or product semantics change
- update `ARCHITECTURE.md` when layer ownership, authority boundaries, source-of-truth rules, async behavior, consistency guarantees, or transitional status changes
- update both when a product change also changes the technical contract for ownership or consistency
