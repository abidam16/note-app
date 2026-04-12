# Threaded Discussion Backend Roadmap

## Feature Summary
- Goal: deliver the backend for block-anchored threaded discussion on pages.
- Scope of this file: backend work only in this repository.
- Frontend implementation is out of scope for this roadmap and should be tracked in the frontend repository.

## Locked Decisions
- New threaded discussion system, no migration from legacy `page_comments`
- Block anchors only in v1
- All workspace members can create and reply
- `owner|editor` can resolve
- Any member can reopen
- Reply on a resolved thread auto-reopens it
- Separate states:
  - `thread_state`: `open | resolved`
  - `anchor_state`: `active | outdated | missing`
- Missing block after edits or restore becomes `missing`
- Search is page-scoped in v1
- Legacy flat comments remain supported, but are deprecated for new discussion UI

## Current Task
- Task 20: Advanced Anchor And Revision Semantics

## Task List
- `done` Task 1: Feature Spec Baseline
- `done` Task 2: Domain Model Introduction
- `done` Task 3: Database Migration For Threads
- `done` Task 4: Repository Interfaces And Stubs
- `done` Task 5: Create Thread Backend
- `done` Task 6: Thread Detail Backend
- `done` Task 7: Thread List Backend
- `done` Task 8: Reply Backend
- `done` Task 9: Resolve And Reopen Backend
- `done` Task 10: Anchor Evaluator
- `done` Task 11: Trigger Reevaluation On Draft Update
- `done` Task 12: Trigger Reevaluation On Revision Restore
- `done` Task 13: Thread Search Implementation
- `done` Task 14: Notifications For Threads
- `done` Task 15: API Contract And Deprecation Notes
- `done` Task 16: Backend Hardening Pass
- `done` Task 17: Backend Filter And Sort Expansion
- `done` Task 18: Workspace-Level Thread Inbox APIs
- `done` Task 19: Backend Activity Metadata
- `in_progress` Task 20: Advanced Anchor And Revision Semantics

## Last Completed Task
- Task 19: Backend Activity Metadata

## Next Task
- Continue deterministic anchor recovery refinements and revision semantics hardening.

## Backend Tasks
### Task 16: Backend Hardening Pass
- Add missing edge-case coverage for:
  - permissions
  - idempotent resolve and reopen
  - invalid block anchors
  - anchor reevaluation after page delete, trash restore, and revision restore
  - notification side effects and failure propagation
- Review thread endpoint error mapping for consistency.
- Verify route, service, and repository behavior stay aligned with the contract.
- Done when the current thread backend is stable enough to stop changing v1 semantics casually.

### Task 17: Backend Filter And Sort Expansion
- Add optional backend filters that improve thread retrieval without changing the core model:
  - `created_by=me`
  - `has_missing_anchor=true|false`
  - `has_outdated_anchor=true|false`
- Add optional sort modes:
  - `recent_activity`
  - `newest`
  - `oldest`
- Keep page-scoped listing as the default behavior.
- Done when frontend can request alternative filtered views without custom client-side sorting.

### Task 18: Workspace-Level Thread Inbox APIs
- Add backend endpoints for workspace-wide thread discovery across pages.
- Support:
  - workspace-scoped filter and search
  - page summary information in thread list items
  - counts by `thread_state` and `anchor_state`
- Keep the current page-scoped APIs intact.
- Done when the backend can support an inbox or review queue without extra joins in the client.

### Task 19: Backend Activity Metadata
- Add backend support for richer thread lifecycle metadata:
  - resolve note
  - reopen reason
  - timeline events such as `created`, `replied`, `resolved`, `reopened`, `anchor_state_changed`
- Keep write paths explicit and testable.
- Done when auditability and review history are strong enough for collaborative teams.

### Task 20: Advanced Anchor And Revision Semantics
- Improve backend handling for difficult document-history cases:
  - smarter anchor recovery when block ids drift
  - optional re-anchor attempts from quoted snapshots
  - explicit revision linkage where useful
  - clearer behavior when a restore moves threads from `active` to `missing`
- Do this only after profiling the actual need and complexity.
- Done when anchor behavior is strong enough for long-lived documents with heavy revision churn.

## Current State
- Thread endpoints are implemented:
  - `POST /api/v1/pages/{pageID}/threads`
  - `GET /api/v1/pages/{pageID}/threads`
  - `GET /api/v1/threads/{threadID}`
  - `POST /api/v1/threads/{threadID}/replies`
  - `POST /api/v1/threads/{threadID}/resolve`
  - `POST /api/v1/threads/{threadID}/reopen`
- Anchor reevaluation runs after:
  - draft update
  - revision restore
- Notifications exist for:
  - new thread creation
  - new thread reply
- Resolve and reopen do not notify in v1.

## Notes
- Current create-thread validation assumes stable top-level block `id` values in saved draft content.
- `quoted_block_text` is stored from the server-side block snapshot to avoid client drift.
- Legacy `page_comments` remain unchanged in storage and API, but are deprecated for new product flows.
- Current backend verification is limited by local PostgreSQL availability for DB-backed integration tests.
- First Task 16 slice completed:
  - restoring a trashed page now reevaluates thread anchors with the restored draft content
  - reevaluator errors now propagate through `RestoreTrashItem`
- Second Task 16 slice completed:
  - deleting a page now reevaluates thread anchors against an empty document so page-level anchors become `missing`
  - reevaluator errors now propagate through `DeletePage`
- Third Task 16 slice completed:
  - normal thread endpoints now have locked tests proving trashed pages behave as `not_found`
  - covered paths: thread detail, thread list, and thread reply
- Fourth Task 16 slice completed:
  - create, resolve, and reopen now have locked tests proving trashed pages behave as `not_found`
  - resolve and reopen now have explicit HTTP idempotency regression coverage
- Fifth Task 16 slice completed:
  - thread create and reply endpoints now have locked HTTP regression coverage for malformed JSON returning `400 invalid_json`
  - thread list endpoint now has locked HTTP regression coverage for invalid `anchor_state` returning `422 validation_failed`
  - no production code changes were required because handler behavior already matched the contract
- Sixth Task 16 slice completed:
  - thread create, detail, list, reply, resolve, and reopen now have locked non-member permission regression coverage
  - service layer now explicitly verifies non-members receive `forbidden` across the remaining thread operations
  - HTTP layer now explicitly verifies the same permission surface returns `403`
  - no production code changes were required because existing permission checks already matched the contract
- Seventh Task 16 slice completed:
  - thread create and reply now have locked regression coverage for notification failure propagation
  - service layer now explicitly verifies publisher failures are returned after persistence
  - HTTP layer now explicitly verifies those failures map to `500`
  - no production code changes were required because the current notification contract already matched this behavior
- Eighth Task 16 slice completed:
  - repository integration coverage now locks `ErrNotFound` for reply/state updates on missing threads
  - repository integration coverage now locks direct `anchor_state` filtering and confirms filter counts remain page-wide
  - PostgreSQL-backed repository behavior matched the contract without production code changes
- First Task 17 slice completed:
  - page-scoped thread listing now supports `created_by=me`
  - the service resolves `me` to the current actor instead of exposing arbitrary user-id filtering
  - invalid `created_by` values now return `422 validation_failed`
  - service, HTTP, and PostgreSQL-backed repository coverage now lock the filter behavior end to end
- Second Task 17 slice completed:
  - page-scoped thread listing now supports `has_missing_anchor=true|false`
  - `true` keeps only threads whose `anchor_state` is `missing`
  - `false` excludes threads whose `anchor_state` is `missing`
  - invalid `has_missing_anchor` values now return `422 validation_failed`
  - service, HTTP, and PostgreSQL-backed repository coverage now lock the filter behavior end to end
- Third Task 17 slice completed:
  - page-scoped thread listing now supports `has_outdated_anchor=true|false`
  - `true` keeps only threads whose `anchor_state` is `outdated`
  - `false` excludes threads whose `anchor_state` is `outdated`
  - invalid `has_outdated_anchor` values now return `422 validation_failed`
  - service, HTTP, and PostgreSQL-backed repository coverage now lock the filter behavior end to end
- Fourth Task 17 slice completed:
  - page-scoped thread listing now supports `sort=recent_activity|newest|oldest`
  - default behavior remains the current recent-activity ordering
  - `recent_activity` preserves open-first ordering, then `last_activity_at DESC`, then `id ASC`
  - `newest` orders by `created_at DESC`, then `id ASC`
  - `oldest` orders by `created_at ASC`, then `id ASC`
  - invalid `sort` values now return `422 validation_failed`
  - service, HTTP, and PostgreSQL-backed repository coverage now lock the ordering behavior end to end
- First Task 18 slice completed:
  - added `GET /api/v1/workspaces/{workspaceID}/threads`
  - workspace thread inbox uses the same filters, search, and sort semantics as page thread listing
  - response items now include page summary information alongside each thread
  - workspace counts stay workspace-scoped and do not change with filters
  - search also matches page title in the workspace inbox
  - service, HTTP, and PostgreSQL-backed repository coverage now lock the inbox behavior end to end
- Second Task 18 slice completed:
  - workspace inbox now has explicit HTTP regression coverage for `created_by=me` and page-title search
  - PostgreSQL-backed repository coverage now explicitly locks exclusion of trashed pages from the workspace inbox
  - this slice hardened the new inbox contract without changing its public shape
- First Task 19 slice completed:
  - added `resolve_note` and `reopen_reason` columns through migration `000013_thread_activity_notes`
  - thread resolve now accepts an optional `resolve_note` and persists the trimmed value
  - thread reopen now accepts an optional `reopen_reason` and persists the trimmed value
  - reopen clears `resolve_note`, and resolve clears any stale `reopen_reason`
  - reply-driven auto-reopen also clears stale resolve/reopen note metadata
  - service, HTTP, and PostgreSQL-backed repository coverage now lock the note/reason behavior end to end
- Second Task 19 slice completed:
  - added `page_comment_thread_events` through migration `000014_thread_events`
  - thread detail now returns ordered lifecycle `events` in addition to `thread` and `messages`
  - repository-owned event persistence now records `created`, `replied`, `resolved`, `reopened`, and `anchor_state_changed`
  - reply-driven auto-reopen records both `reopened` and `replied` events
  - page thread lists and workspace inbox payloads remain unchanged; event history is detail-only in v1
  - service, HTTP, and PostgreSQL-backed repository coverage now lock lifecycle event behavior end to end
- Third Task 19 slice completed:
  - added `reason` to thread lifecycle events through migration `000015_thread_event_reasons`
  - `anchor_state_changed` events now persist machine-readable reevaluation reasons:
    - `draft_updated`
    - `page_deleted`
    - `page_restored`
    - `revision_restored`
  - page draft save, page delete, trash restore, and revision restore now propagate explicit reevaluation reasons into thread history
  - service and PostgreSQL-backed repository coverage now lock reason propagation end to end
- Backend pagination hardening slice completed:
  - page thread lists and workspace inbox now support forward-only opaque cursor pagination
  - `limit` is bounded for API list endpoints with default `50` and max `100`
  - cursor resume logic is sort-aware for `recent_activity`, `newest`, and `oldest`
  - list responses now return `next_cursor` and `has_more`
  - PostgreSQL-backed repository coverage now locks cursor pagination behavior for both page and workspace thread lists
- First Task 20 slice completed:
  - anchor reevaluation now attempts exact unique reanchor by `quoted_block_text` when the saved `block_id` disappears
  - if exactly one current block matches the saved `quoted_block_text`, the thread anchor recovers to that new `block_id`
  - if multiple blocks match or no blocks match, recovery is skipped to avoid ambiguous attachment
  - thread state remains driven by the recovered content outcome: unique exact match stays `active`, changed matched block stays `outdated`, and no safe match stays `missing`
  - service and PostgreSQL-backed repository coverage now lock exact reanchor persistence end to end
- Second Task 20 slice completed:
  - anchor reevaluation now falls back to unique exact `quoted_text` recovery when full-block recovery is not possible
  - quoted-text fallback recovers the anchor to the new `block_id` and marks the thread `outdated`
  - thread history now records explicit `anchor_recovered` events with `from_block_id`, `to_block_id`, and reevaluation `reason`
  - PostgreSQL-backed repository persistence now keeps anchor recovery auditable even when `anchor_state` itself does not change
- Third Task 20 slice completed:
  - anchor reevaluation events now support explicit revision linkage through `revision_id`
  - revision-restore-triggered `anchor_state_changed` and `anchor_recovered` events now persist the restored revision id
  - page draft updates, page delete, and trash restore remain revision-less reevaluation contexts
  - service and PostgreSQL-backed repository coverage now lock revision-linked thread history end to end

## vNext Backlog
### Near Term
- unread indicators at the backend level
- thread assignment or mentions
- notification tuning for noisy threads

### Longer Term
- quoted diff previews for outdated threads
- review analytics and turnaround metrics
- optional task-like metadata such as assignee, due date, and priority
- eventual legacy flat comment retirement after an explicit migration plan
