# Invitation, Notification, And Thread Roadmap

## Summary
This roadmap defines backend-only work for invitations, notifications, and threaded comment notifications.

The goal is to make the collaboration inbox reliable, clear, and safe under concurrency. Invitation notifications use one live notification per invitation. Comment and mention notifications are append-only and go only to relevant users.

This roadmap does not extend legacy flat `page_comments`. New product work should build on thread endpoints and thread messages. Legacy flat comments remain supported only for backward compatibility.

Original roadmap scope for Tasks 1 through 28 is complete and tracked in `docs/checkpoint.md`. This roadmap now also carries the next follow-on slice required by the 2026-04-18 PRD update for invitation eligibility and no-workspace invitation-first entry behavior.

## Diagnosis
The original initiative started from three core gaps:
- invitation state is too small for update, reject, and cancel flows
- notifications are too small for actor, title, content, action state, and future types
- comment and thread notifications are produced synchronously in request paths, which is fragile under retries, spikes, and races

The current backend now also has invitation-policy drift against the updated PRD:
- invite create still allows unregistered target emails
- self-invite is not enforced as a distinct invalid create case
- invalid create cases are not all distinguishable to clients
- no-workspace post-auth entry still depends on the client coordinating workspace and invitation calls explicitly

The core design problem is not only API shape. It is consistency. Invitation rows, thread messages, notification rows, and unread counts must stay aligned even when multiple actors act at the same time.

## Guiding Policies
- Keep authoritative state in source tables:
  - invitations in invitation tables
  - thread messages in thread tables
  - notifications as a projection, not the source of truth
- Use the current stack:
  - Go
  - PostgreSQL
  - `pgx`
  - explicit SQL repositories
  - no new broker for the first version
- Use transactional outbox for notification production.
- Use conditional state transitions with `version` for invitation races.
- Keep one notification row per invitation.
- Keep comment and mention notifications append-only.
- Notify only relevant users.
- Keep list endpoints cursor-based and bounded.
- Keep mark-read idempotent.

## Locked Product Decisions
- Invitation target email is immutable after create.
- Invitation creation is allowed only for already registered accounts.
- Workspace owners may not invite their own account email.
- A pending invitation may update its role in place.
- If the inviter wants a different target user, they must cancel and create a new invitation.
- Invitation status is:
  - `pending`
  - `accepted`
  - `rejected`
  - `cancelled`
- Invitation notification policy:
  - one live notification per invitation
  - the same notification row is updated as invitation state changes
  - only `pending` invitation notifications are actionable
- Invite-create invalid cases for unregistered email, self-invite, existing member, and duplicate pending invite must remain distinguishable to clients.
- If an authenticated user has no workspace memberships but does have pending invitations, invitation review takes precedence over empty-workspace onboarding.
- Comment notification recipients are the relevant users only:
  - thread creator
  - prior repliers
  - explicit mention targets
  - exclude the acting user
- Mention notifications are separate from comment notifications.
- New collaboration work uses thread endpoints, not legacy flat comments.

## Status Model
- `not_started`
- `in_progress`
- `done`
- `blocked`

## Current Task
- `not_started` Task 29: invitation eligibility and no-workspace entry alignment

## Phases
- Phase 1: MVP foundation and invitation lifecycle
- Phase 2: inbox projection and read models
- Phase 3: relevant-user comment notifications
- Phase 4: mention support
- Phase 5: advanced control, real-time delivery, and recovery
- Phase 6: invitation eligibility and no-workspace entry alignment

## Tasks

Tasks 1 through 28 are preserved as the historical delivery record for the original roadmap scope. Their authoritative completion state lives in `docs/checkpoint.md`, not in the inline status labels below.

### Phase 1: MVP Foundation And Invitation Lifecycle

1. `not_started` Invitation state schema
   - Goal: replace the current invitation shape with an explicit state machine.
   - Why now: update, accept, reject, and cancel cannot be modeled cleanly with `accepted_at` alone.
   - Endpoint: none
   - Scope:
     - add `status`
     - add `version`
     - add `updated_at`
     - add `responded_by`
     - add `responded_at`
     - add `cancelled_by`
     - add `cancelled_at`
     - preserve immutable `email`
   - Validation:
     - `status` must be one of `pending|accepted|rejected|cancelled`
     - only one pending invitation may exist for `(workspace_id, email)`
     - target email cannot change after create
   - Positive codes: none
   - Negative codes: migration failure blocks deploy

2. `not_started` `POST /api/v1/workspaces/{workspaceID}/invitations`
   - Goal: create a pending invitation with the new state model.
   - Why now: invitation lifecycle starts here.
   - Request:
     - `{ "email": "user@example.com", "role": "viewer|editor|owner" }`
   - Response `201`:
     - `Invitation`
   - Validation:
     - auth required
     - actor must be workspace owner
     - email must be valid and normalized
     - role must be valid
     - target user must not already be a workspace member
     - pending duplicate invitation is not allowed
   - Positive codes:
     - `201`
   - Negative codes:
     - `400 invalid_json`
     - `401 unauthorized`
     - `403 forbidden`
     - `404 not_found`
     - `409 conflict`
     - `422 validation_failed`
   - Notes:
     - successful create must write an outbox event in the same transaction

3. `not_started` `GET /api/v1/workspaces/{workspaceID}/invitations`
   - Goal: let an owner list invitations from the authoritative source table.
   - Why now: invitation management must not depend on notifications.
   - Query:
     - `status=pending|accepted|rejected|cancelled|all`
     - `cursor`
     - `limit`
   - Response `200`:
     - `{ "items": [Invitation], "next_cursor": "opaque", "has_more": true }`
   - Validation:
     - auth required
     - actor must be workspace owner
     - `status` filter must be valid
     - `limit` must be positive and bounded
     - `cursor` must be valid
   - Positive codes:
     - `200`
   - Negative codes:
     - `401 unauthorized`
     - `403 forbidden`
     - `404 not_found`
     - `422 validation_failed`

4. `not_started` `GET /api/v1/my/invitations`
   - Goal: let the invited user list only their own invitations.
   - Why now: the target user needs a canonical source separate from the inbox.
   - Query:
     - `status=pending|accepted|rejected|cancelled|all`
     - `cursor`
     - `limit`
   - Response `200`:
     - `{ "items": [Invitation], "next_cursor": "opaque", "has_more": true }`
   - Validation:
     - auth required
     - only invitations addressed to the authenticated user's email are returned
     - `status` filter must be valid
     - `limit` must be positive and bounded
     - `cursor` must be valid
   - Positive codes:
     - `200`
   - Negative codes:
     - `401 unauthorized`
     - `422 validation_failed`

5. `not_started` `PATCH /api/v1/workspace-invitations/{invitationID}`
   - Goal: update a pending invitation in place.
   - Why now: this is the chosen product behavior for role updates.
   - Request:
     - `{ "role": "viewer|editor|owner", "version": 3 }`
   - Response `200`:
     - `Invitation`
   - Validation:
     - auth required
     - actor must be workspace owner
     - invitation must exist
     - invitation must be `pending`
     - request `version` must equal current row version
     - role must be valid
     - target email cannot be changed
   - Positive codes:
     - `200`
   - Negative codes:
     - `400 invalid_json`
     - `401 unauthorized`
     - `403 forbidden`
     - `404 not_found`
     - `409 conflict`
     - `422 validation_failed`
   - Notes:
     - successful update must write an outbox event in the same transaction

6. `not_started` `POST /api/v1/workspace-invitations/{invitationID}/accept`
   - Goal: accept a pending invitation safely under concurrency.
   - Why now: terminal invitation transitions need explicit race handling.
   - Request:
     - `{ "version": 3 }`
   - Response `200`:
     - `{ "invitation": Invitation, "membership": WorkspaceMember }`
   - Validation:
     - auth required
     - actor must match invitation target email
     - invitation must be `pending`
     - request `version` must equal current row version
     - target user must not already be a workspace member
   - Positive codes:
     - `200`
   - Negative codes:
     - `400 invalid_json`
     - `401 unauthorized`
     - `403 forbidden`
     - `404 not_found`
     - `409 conflict`
     - `422 validation_failed`
   - Notes:
     - success must update invitation state, create membership, and write one outbox event in one transaction

7. `not_started` `POST /api/v1/workspace-invitations/{invitationID}/reject`
   - Goal: let the target user reject a pending invitation.
   - Why now: explicit rejection removes ambiguity and keeps invitation state complete.
   - Request:
     - `{ "version": 3 }`
   - Response `200`:
     - `Invitation`
   - Validation:
     - auth required
     - actor must match invitation target email
     - invitation must be `pending`
     - request `version` must equal current row version
   - Positive codes:
     - `200`
   - Negative codes:
     - `400 invalid_json`
     - `401 unauthorized`
     - `403 forbidden`
     - `404 not_found`
     - `409 conflict`
     - `422 validation_failed`
   - Notes:
     - successful reject must write an outbox event in the same transaction

8. `not_started` `POST /api/v1/workspace-invitations/{invitationID}/cancel`
   - Goal: let the inviter cancel a pending invitation.
   - Why now: this is the clean path when target user should change.
   - Request:
     - `{ "version": 3 }`
   - Response `200`:
     - `Invitation`
   - Validation:
     - auth required
     - actor must be workspace owner
     - invitation must be `pending`
     - request `version` must equal current row version
   - Positive codes:
     - `200`
   - Negative codes:
     - `400 invalid_json`
     - `401 unauthorized`
     - `403 forbidden`
     - `404 not_found`
     - `409 conflict`
     - `422 validation_failed`
   - Notes:
     - successful cancel must write an outbox event in the same transaction

### Phase 2: Inbox Projection And Read Models

9. `not_started` Notification schema v2
   - Goal: replace the current minimal notification shape with a usable inbox model.
   - Why now: invitation and thread notifications need actor, content, actions, and resource metadata.
   - Endpoint: none
   - Scope:
     - add `actor_id`
     - add `title`
     - add `content`
     - add `is_read`
     - keep `read_at`
     - add `actionable`
     - add `action_kind`
     - add `resource_type`
     - add `resource_id`
     - add `payload`
     - add `updated_at`
   - Validation:
     - `type` must be one of `invitation|comment|mention`
     - action and resource enums must stay constrained
     - invitation notification uniqueness must support one live row per invitation
   - Positive codes: none
   - Negative codes: migration failure blocks deploy

10. `not_started` Outbox foundation
   - Goal: make notification production reliable and retryable.
   - Why now: request handlers should not fan out notifications directly.
   - Endpoint: none
   - Scope:
     - add `outbox_events`
     - define event keys and event payload rules
     - support retries
     - support worker claiming with `FOR UPDATE SKIP LOCKED`
     - support dead-letter state
   - Validation:
     - each event must have an idempotency key
     - event processing state must be explicit
   - Positive codes: none
   - Negative codes: migration failure blocks deploy

11. `not_started` Invitation notification projector
   - Goal: project invitation events into one live notification row per invitation.
   - Why now: invitation inbox behavior depends on upsert, not append-only fanout.
   - Endpoint: none
   - Scope:
     - consume invitation outbox events
     - upsert notification by invitation id
     - keep `pending` actionable
     - keep terminal states non-actionable
     - update title, content, payload, and `updated_at`
   - Validation:
     - repeated projector runs must be idempotent
     - unread count must not increase on every invitation update
   - Positive codes: none
   - Negative codes: worker failure handled by retry and dead-letter

12. `not_started` `GET /api/v1/notifications`
   - Goal: return the user inbox with clear read state and latest invitation state.
   - Why now: frontend needs one canonical inbox API.
   - Query:
     - `status=all|read|unread`
     - `type=invitation|comment|mention|all`
     - `cursor`
     - `limit`
   - Response `200`:
     - `{ "items": [Notification], "unread_count": 12, "next_cursor": "opaque", "has_more": true }`
   - Validation:
     - auth required
     - only the actor's inbox is returned
     - filters must be valid
     - `limit` must be positive and bounded
     - `cursor` must be valid
   - Positive codes:
     - `200`
   - Negative codes:
     - `401 unauthorized`
     - `422 validation_failed`

13. `not_started` `POST /api/v1/notifications/{notificationID}/read`
   - Goal: mark one notification as read idempotently.
   - Why now: per-item read state must be cheap and safe to repeat.
   - Request:
     - none
   - Response `200`:
     - `Notification`
   - Validation:
     - auth required
     - notification must belong to actor
     - repeated mark-read must not double-decrement unread count
   - Positive codes:
     - `200`
   - Negative codes:
     - `401 unauthorized`
     - `404 not_found`

14. `not_started` `GET /api/v1/notifications/unread-count`
   - Goal: return a cheap unread badge value without loading inbox pages.
   - Why now: high-traffic unread badges should not require a full list query.
   - Request:
     - none
   - Response `200`:
     - `{ "unread_count": 12 }`
   - Validation:
     - auth required
     - value must come from a maintained read model or counter, not a costly full scan
   - Positive codes:
     - `200`
   - Negative codes:
     - `401 unauthorized`

15. `not_started` `POST /api/v1/notifications/read`
   - Goal: mark many notifications as read in one request.
   - Why now: inbox UX should not require one request per item.
   - Request:
     - `{ "notification_ids": ["id1", "id2", "id3"] }`
   - Response `200`:
     - `{ "updated_count": 3, "unread_count": 9 }`
   - Validation:
     - auth required
     - all ids must belong to actor
     - batch size must be bounded
     - repeated calls must be idempotent
   - Positive codes:
     - `200`
   - Negative codes:
     - `400 invalid_json`
     - `401 unauthorized`
     - `404 not_found`
     - `422 validation_failed`

### Phase 3: Relevant-User Comment Notifications

16. `not_started` Thread notification recipient resolver
   - Goal: define relevant recipients for comment notifications.
   - Why now: notification policy must be explicit before changing thread write paths.
   - Endpoint: none
   - Scope:
     - recipients are thread creator, prior repliers, and explicit mention targets
     - exclude the acting user
     - dedupe recipients
     - ignore users who no longer have workspace access
   - Validation:
     - no workspace-wide fanout
     - one recipient should never receive the same event twice
   - Positive codes: none
   - Negative codes: none

17. `not_started` `POST /api/v1/pages/{pageID}/threads` notification outbox integration
   - Goal: stop producing comment notifications synchronously when a thread is created.
   - Why now: thread creation must not fail because a notification write failed after persistence.
   - Request:
     - existing create-thread contract
   - Response:
     - existing `201` create-thread response
   - Validation:
     - existing thread validation stays unchanged
     - successful create must write one outbox event in the same transaction
     - direct notification fanout from the request path must be removed
   - Positive codes:
     - existing `201`
   - Negative codes:
     - existing `400`
     - existing `401`
     - existing `403`
     - existing `404`
     - existing `422`

18. `not_started` `POST /api/v1/threads/{threadID}/replies` notification outbox integration
   - Goal: stop producing reply notifications synchronously when a reply is created.
   - Why now: replies have the same reliability problem as thread creation.
   - Request:
     - existing create-reply contract
   - Response:
     - existing `201` create-reply response
   - Validation:
     - existing reply validation stays unchanged
     - successful reply must write one outbox event in the same transaction
     - direct notification fanout from the request path must be removed
   - Positive codes:
     - existing `201`
   - Negative codes:
     - existing `400`
     - existing `401`
     - existing `403`
     - existing `404`
     - existing `422`

19. `not_started` Comment notification projector
   - Goal: project thread create and reply events into append-only comment notifications.
   - Why now: comment notification delivery should come from the outbox worker.
   - Endpoint: none
   - Scope:
     - consume thread create and reply events
     - resolve relevant recipients
     - create one notification per recipient and event
     - include page, thread, and message ids in payload
   - Validation:
     - unique key should prevent duplicate notifications on retries
     - actor must never receive their own notification
   - Positive codes: none
   - Negative codes: worker failure handled by retry and dead-letter

### Phase 4: Mention Support

20. `not_started` Mention schema
   - Goal: store explicit mentions on thread messages.
   - Why now: mention notifications should come from authoritative data, not free-text parsing at notification time.
   - Endpoint: none
   - Scope:
     - add thread message mention table
     - keep one row per `(message_id, mentioned_user_id)`
   - Validation:
     - duplicates must be prevented by constraint
   - Positive codes: none
   - Negative codes: migration failure blocks deploy

21. `not_started` `POST /api/v1/pages/{pageID}/threads` mention support
   - Goal: allow the first thread message to include mentions.
   - Why now: mentions should work from the first comment, not only from replies.
   - Request extension:
     - `{ ..., "mentions": ["user-id-1", "user-id-2"] }`
   - Response `201`:
     - existing thread detail response
   - Validation:
     - mentioned users must be workspace members
     - duplicate ids must be rejected or normalized consistently
     - mention count must be bounded
   - Positive codes:
     - `201`
   - Negative codes:
     - `400 invalid_json`
     - `401 unauthorized`
     - `403 forbidden`
     - `404 not_found`
     - `422 validation_failed`
   - Notes:
     - successful create must persist mentions and write mention-aware outbox metadata in the same transaction

22. `not_started` `POST /api/v1/threads/{threadID}/replies` mention support
   - Goal: allow reply messages to include mentions.
   - Why now: reply mentions are a core collaboration action.
   - Request extension:
     - `{ "body": "reply", "mentions": ["user-id-1", "user-id-2"] }`
   - Response `201`:
     - existing thread detail response
   - Validation:
     - mentioned users must be workspace members
     - duplicate ids must be rejected or normalized consistently
     - mention count must be bounded
   - Positive codes:
     - `201`
   - Negative codes:
     - `400 invalid_json`
     - `401 unauthorized`
     - `403 forbidden`
     - `404 not_found`
     - `422 validation_failed`
   - Notes:
     - successful reply must persist mentions and write mention-aware outbox metadata in the same transaction

23. `completed` Mention notification projector
   - Goal: create direct mention notifications as a separate stream from comment notifications.
   - Why now: mention urgency and recipient rules differ from comment notifications.
   - Endpoint: none
   - Scope:
     - consume mention metadata from thread events
     - create append-only `mention` notifications for mentioned users only
     - include page, thread, and message ids in payload
   - Validation:
     - unique key must prevent duplicate mention notifications on retries
     - actor must not receive a mention notification for their own action
   - Positive codes: none
   - Negative codes: worker failure handled by retry and dead-letter

### Phase 5: Advanced Control, Real-Time Delivery, And Recovery

24. `not_started` `GET /api/v1/threads/{threadID}/notification-preference`
   - Goal: expose the actor's notification preference for one thread.
   - Why now: relevant-user delivery still needs user control for noisy threads.
   - Response `200`:
     - `{ "thread_id": "uuid", "mode": "all|mentions_only|mute" }`
   - Validation:
     - auth required
     - actor must be a workspace member for the thread
   - Positive codes:
     - `200`
   - Negative codes:
     - `401 unauthorized`
     - `403 forbidden`
     - `404 not_found`

25. `not_started` `PUT /api/v1/threads/{threadID}/notification-preference`
   - Goal: let the actor change how they receive notifications for one thread.
   - Why now: mute and mention-only controls reduce noise without weakening the core inbox model.
   - Request:
     - `{ "mode": "all|mentions_only|mute" }`
   - Response `200`:
     - `{ "thread_id": "uuid", "mode": "all|mentions_only|mute", "updated_at": "..." }`
   - Validation:
     - auth required
     - actor must be a workspace member for the thread
     - mode must be valid
   - Positive codes:
     - `200`
   - Negative codes:
     - `400 invalid_json`
     - `401 unauthorized`
     - `403 forbidden`
     - `404 not_found`
     - `422 validation_failed`

26. `not_started` `GET /api/v1/notifications/stream`
   - Goal: support near-real-time unread badge and inbox refresh.
   - Why now: polling full inbox pages is wasteful after the read model is stable.
   - Response `200`:
     - SSE stream with unread-count changes and inbox invalidation events
   - Validation:
     - auth required
     - reconnect flow must be documented
     - REST inbox remains the source of truth
   - Positive codes:
     - `200`
   - Negative codes:
     - `401 unauthorized`

27. `not_started` Notification reconciliation job
   - Goal: rebuild or repair notifications from source-of-truth data.
   - Why now: advanced reliability requires an explicit repair path.
   - Endpoint: none
   - Scope:
     - rebuild invitation live notifications
     - rebuild comment and mention append-only notifications
     - rebuild unread counters
     - publish best-effort inbox invalidation after effective repair changes
   - Validation:
     - rebuild must be idempotent
     - rebuild must not create duplicates
     - rebuild output must match source state rules
   - Positive codes: none
   - Negative codes: operational only

28. `not_started` Concurrency and load verification
   - Goal: prove correctness under competing invitation actions and high event volume.
   - Why now: the feature has explicit race-condition and transaction-volume requirements.
   - Endpoint: none
   - Scope:
     - invitation tests for:
       - update vs accept
       - update vs reject
       - cancel vs accept
       - cancel vs reject
     - projector tests for:
       - DB-backed duplicate event delivery
       - duplicate event delivery
       - worker retries
       - unread counter correctness
       - recipient dedupe
     - race-detector verification for in-process concurrency paths
   - Validation:
     - only one terminal invitation transition may win
     - stale version must return `409`
     - projector retries must not duplicate notifications
   - Positive codes: none
   - Negative codes: none

## Execution Order
The execution order below is the historical order for Tasks 1 through 28. The new follow-on planning path starts with Task 29 in Phase 6.

- Start with invitation schema and invitation endpoints.
- Build outbox and invitation projector before broad inbox work.
- Move thread notifications to outbox before adding mentions.
- Add mention support only after the comment notification path is stable.
- Add real-time delivery and reconciliation last.
- Use the detailed execution gate in:
  - `docs/invitation-notification-thread-execution-checklist.md`

## Success Criteria
- Invitation lifecycle supports create, list, update, accept, reject, and cancel.
- Invitation races resolve deterministically with `409` on stale actions.
- Inbox API returns clear read state, unread count, and actionable invitation state.
- Invitation notifications show one live row per invitation.
- Thread comment notifications go only to relevant users.
- Mention notifications are separate, direct, and idempotent.
- Notification production no longer depends on synchronous request-path fanout.
- Unread count remains correct under retries and concurrent reads.
- Invite-create policy matches the PRD target for registered-account-only eligibility and self-invite rejection.
- Frontend-facing contract docs make current implementation drift explicit until backend behavior is aligned.
- Authenticated users with no workspace memberships have a canonical invitation-review-first path before empty-workspace onboarding.

## Follow-On Phase 6: Invitation Eligibility And No-Workspace Entry Alignment

- Objective:
  Align invitation create behavior and the frontend-facing contract with the updated PRD without reopening the completed invitation lifecycle and notification architecture work.
- Why this phase exists now:
  The original roadmap delivered the lifecycle, notification, and concurrency model, but the updated PRD now requires stricter invite eligibility plus a clearer no-workspace entry rule than the current backend exposes cleanly.
- Key outcomes:
  - invite create no longer accepts unregistered target emails
  - self-invite is rejected explicitly
  - invalid invite-create cases remain distinguishable to clients
  - the frontend has a canonical contract path for invitation review before empty-workspace onboarding
- In scope:
  - invitation create eligibility and error-outcome alignment
  - frontend-facing API contract updates for current drift and target behavior
  - no-workspace invitation-review entry contract clarification using invitation source data
- Out of scope:
  - outbound email invitation delivery
  - ownership-transfer product changes
  - replacing invitation source data with notification-derived authority
- Dependencies:
  - Tasks 1 through 28 are already complete
  - PRD invitation policy update dated 2026-04-18
- Risks:
  - registered-account-only invite validation can reintroduce account-existence leakage if the error model is designed carelessly
  - the no-workspace entry rule can drift again if bootstrap and invitation surfaces remain loosely coordinated
- Exit criteria:
  - create-invitation behavior and docs align with the PRD target
  - contract docs explicitly separate current backend behavior from target behavior until code lands
  - the next implementation work can be split into bounded single-task plans

### Follow-On Planning Candidates

29. `not_started` Invite-create eligibility and error-outcome alignment
   - Goal: align `POST /api/v1/workspaces/{workspaceID}/invitations` with registered-account-only targeting and explicit invalid-case outcomes.
   - Why now: this endpoint is the main remaining product drift after the original invitation lifecycle work.
   - In scope:
     - reject unregistered target email
     - reject actor self-email
     - preserve existing member and duplicate pending rejection rules
     - expose client-distinguishable outcomes for unregistered, self, existing-member, and duplicate-pending failures
   - Out of scope:
     - invitation delivery by email
     - invitation role policy changes beyond the existing role set
   - Dependencies:
     - completed invitation lifecycle tasks
     - updated frontend-facing API docs
   - Risks:
     - careless error design could leak registration state more broadly than intended
   - Exit criteria:
     - docs, tests, and endpoint behavior agree on the allowed invite targets and invalid cases

30. `not_started` No-workspace invitation-first entry contract
   - Goal: give clients a canonical way to route authenticated users with zero memberships into pending invitation review before empty-workspace onboarding.
   - Why now: the product rule is now explicit, but current client behavior still has to infer it from multiple surfaces.
   - In scope:
     - define the canonical invitation-review source for no-workspace users
     - document whether the existing `GET /api/v1/my/invitations` plus workspace bootstrap is sufficient or whether a dedicated bootstrap contract is needed
     - preserve invitation source data as the authority for accept/reject state
   - Out of scope:
     - redesigning the inbox model
     - general onboarding redesign unrelated to invitations
   - Dependencies:
     - invitation source data and `GET /api/v1/my/invitations` remain available
   - Risks:
     - ambiguous ownership between auth bootstrap and invitation surfaces can create repeated frontend drift
   - Exit criteria:
     - frontend contract documents a stable invitation-review-first flow for users with no memberships

## Decision
- `INITIATIVE_ROADMAP_UPDATE`

## Why This Decision
- The initiative already exists and the original invitation-notification scope remains the right execution container.
- The new work is a follow-on product-alignment slice, not a separate initiative.
- The next need is sequencing and planning for bounded implementation, not another PRD or ADR.

## What Planning Should Do Next
- Create a single-task plan for follow-on candidate 29 first.
- Keep candidate 30 separate unless candidate 29 proves a bootstrap contract change is required.

## What Is Explicitly Deferred Or Not Next
- Outbound invitation delivery is not next.
- Ownership-transfer policy is not next.
- Reworking the completed notification architecture is not next.

## Immediate Next Step
- Proceed to create a single-task plan for Task 29: invite-create eligibility and error-outcome alignment.
