# API Delta: Notifications, Invitations, and Thread Mentions

This doc covers only backend additions and changed behavior relevant to the frontend enhancement. It is not a full API contract.

## 1. Invitations

- `POST /api/v1/workspaces/{workspaceID}/invitations`
  - Auth: `owner`
  - Request: `{ "email": string, "role": "owner|editor|viewer" }`
  - Response used by UI: `id`, `workspace_id`, `email`, `role`, `status`, `version`, `invited_by`, `created_at`, `updated_at`, `accepted_at`
  - UI notes: creates a pending invitation immediately; `status` always starts as `pending`, `version` starts at `1`, and `updated_at` matches `created_at`.
  - Errors / edge cases:
    - invalid email or role returns `422 validation_failed`
    - inviting the actor's own email returns `409 invitation_self_email`
    - inviting an unregistered email returns `409 invitation_target_unregistered`
    - inviting an existing workspace member returns `409 invitation_existing_member`
    - duplicate pending invite returns `409 invitation_duplicate_pending`

- `GET /api/v1/workspaces/{workspaceID}/invitations`
  - Auth: `owner`
  - Query: `status?=pending|accepted|rejected|cancelled|all`, `limit?`, `cursor?`
  - Response used by UI: `items[]` with the same invitation fields as above, plus `next_cursor`, `has_more`
  - UI notes: workspace-only owner view, sorted newest-first, paginated forward only.
  - Errors / edge cases: invalid `status`, `limit`, or `cursor` returns `422`; empty results return `items: []` and `has_more: false`.

- `GET /api/v1/my/invitations`
  - Auth: yes
  - Query: `status?=pending|accepted|rejected|cancelled|all`, `limit?`, `cursor?`
  - Response used by UI: `items[]` with invitation fields, plus `next_cursor`, `has_more`
  - UI notes: this is the signed-in user's cross-workspace invitation inbox; filtering and pagination match the workspace list.
  - UI notes: for signed-in users with zero workspace memberships, `GET /api/v1/my/invitations?status=pending` is the authoritative invitation-review source before empty-workspace onboarding.
  - UI notes: canonical entry sequence for a no-workspace user:
    1. call `GET /api/v1/workspaces`
    2. if the workspace list is empty, call `GET /api/v1/my/invitations?status=pending`
    3. if pending invitations exist, route to invitation review first
    4. if no pending invitations exist, continue to empty-workspace onboarding
  - UI notes: invitation rows from this endpoint, not notification payloads or SSE events, should drive accept/reject actions from the no-workspace entry path.
  - UI notes: after accepting or rejecting from this entry path, refresh `GET /api/v1/workspaces` and `GET /api/v1/my/invitations?status=pending` before deciding the next route.
  - Errors / edge cases: invalid `status`, `limit`, or `cursor` returns `422`; actor lookup failure returns `401`.

- `PATCH /api/v1/workspace-invitations/{invitationID}`
  - Auth: `owner`
  - Request: `{ "role": "owner|editor|viewer", "version": number }`
  - Response used by UI: updated invitation fields, especially `role`, `status`, `version`, `updated_at`
  - UI notes: only pending invitations can be edited; `version` is required for optimistic concurrency.
  - Errors / edge cases: stale `version` or non-pending invitation returns `409`; invalid `role` or `version` returns `422`.

- `POST /api/v1/workspace-invitations/{invitationID}/accept`
  - Auth: yes
  - Request: `{ "version": number }`
  - Response used by UI: `{ invitation, membership }`
  - UI notes: acceptance is atomic; use the returned invitation state and membership role to update workspace context immediately. If this action is triggered from the inbox, send the latest `payload.version` from that invitation notification.
  - Errors / edge cases: foreign invitations are hidden as `404`; stale `version`, non-pending status, or already-member conflicts return `409`.

- `POST /api/v1/workspace-invitations/{invitationID}/reject`
  - Auth: yes
  - Request: `{ "version": number }`
  - Response used by UI: rejected invitation fields, especially `status`, `version`, `responded_by`, `responded_at`
  - UI notes: rejection only updates the invitation; it does not create membership. If this action is triggered from the inbox, send the latest `payload.version` from that invitation notification.
  - Errors / edge cases: foreign invitations are hidden as `404`; stale `version` or non-pending status returns `409`.

- `POST /api/v1/workspace-invitations/{invitationID}/cancel`
  - Auth: `owner`
  - Request: `{ "version": number }`
  - Response used by UI: cancelled invitation fields, especially `status`, `version`, `cancelled_by`, `cancelled_at`
  - UI notes: use this to move a pending invite into its terminal cancelled state. If the UI is currently filtered to pending invitations only, the row should disappear after refresh.
  - Errors / edge cases: stale `version` or non-pending status returns `409`; invalid `version` returns `422`.

## 2. Notification Inbox

- `GET /api/v1/notifications`
  - Auth: yes
  - Query: `status?=all|read|unread`, `type?=all|invitation|comment|mention`, `limit?`, `cursor?`
  - Response used by UI: `items[]` with `id`, `type`, `title`, `content`, `is_read`, `read_at`, `actionable`, `action_kind`, `resource_type`, `resource_id`, `payload`, `created_at`, `updated_at`, plus `unread_count`, `next_cursor`, `has_more`
  - UI notes: this is the canonical inbox list; `unread_count` is the user total, not the filtered subset.
  - Errors / edge cases: invalid `status`, `type`, `limit`, or `cursor` returns `422`; `actor` metadata may be `null`.

- `POST /api/v1/notifications/{notificationID}/read`
  - Auth: notification owner
  - Response used by UI: same inbox item DTO, with `is_read: true` and `read_at` set
  - UI notes: opening a row does not mark it read by itself. Use this endpoint only when the product explicitly decides to mark one notification read, such as after opening a detail view or after an invitation action succeeds.
  - Errors / edge cases: malformed, missing, or foreign notification IDs return `404`; repeated reads are idempotent.

- `POST /api/v1/notifications/read`
  - Auth: yes
  - Request: `{ "notification_ids": string[] }`
  - Response used by UI: `{ updated_count, unread_count }`
  - UI notes: batch-mark notifications read; `updated_count` counts only unread-to-read transitions.
  - Errors / edge cases: duplicate IDs return `422`; missing or foreign IDs return `404`; the batch is all-or-nothing.

## 3. Unread Count and Mark-Read Flows

- `GET /api/v1/notifications/unread-count`
  - Auth: yes
  - Response used by UI: `{ unread_count }`
  - UI notes: use for header badges and quick refresh after read actions; returns `0` when the user has no counter row yet.
  - Errors / edge cases: missing or invalid auth returns `401`.

- Mark-read flow impact
  - The inbox list, single-read endpoint, batch-read endpoint, and unread-count endpoint now share one counter-backed source of truth.
  - UI should refresh the badge from `GET /api/v1/notifications/unread-count` or SSE, not infer the count locally from visible rows.

## 4. SSE Stream

- `GET /api/v1/notifications/stream`
  - Auth: yes
  - Transport: `text/event-stream`
  - Events used by UI:
    - `snapshot`: initial `{ unread_count, sent_at }`
    - `unread_count`: `{ unread_count, sent_at }`
    - `inbox_invalidated`: `{ reason: "notifications_changed", sent_at }`
  - UI notes: open this once after auth, keep the connection alive, and use it to refresh badge/inbox state when the server says counts or inbox state changed.
  - Errors / edge cases: no replay of missed events in v1; heartbeat comments arrive every 25s; if the stream cannot open, the server returns `500` before headers.

## 5. Thread Create/Reply Mention Support

- `POST /api/v1/pages/{pageID}/threads`
  - Auth: workspace member
  - Mention behavior only: request may include `mentions?: string[] | null`
  - Response used by UI: unchanged thread-create response; mentions are not added to the response shape
  - UI notes: mention IDs are trimmed, deduped, limited to 20 unique IDs, and validated against current workspace members; self-mentions are allowed.
  - Errors / edge cases: blank mention IDs, invalid member IDs, or too many unique IDs return `422`; mention handling is transactional with thread creation.

- `POST /api/v1/threads/{threadID}/replies`
  - Auth: workspace member of the thread page
  - Mention behavior only: request may include `mentions?: string[] | null`
  - Response used by UI: unchanged thread-detail response
  - UI notes: same mention rules as create-thread apply; replying to a resolved thread auto-reopens it before the response returns.
  - Errors / edge cases: invalid mention IDs or too many unique IDs return `422`; mention changes do not alter the reply response payload.

## 6. Thread Notification Preferences

- `GET /api/v1/threads/{threadID}/notification-preference`
  - Auth: workspace member of the thread page
  - Response used by UI: `{ thread_id, mode }`
  - UI notes: returns the user's stored per-thread mode, or `all` when no preference exists.
  - Errors / edge cases: non-members, inaccessible threads, and trashed-page threads are hidden as `404`.

- `PUT /api/v1/threads/{threadID}/notification-preference`
  - Auth: workspace member of the thread page
  - Request: `{ "mode": "all|mentions_only|mute" }`
  - Response used by UI: `{ thread_id, mode, updated_at }`
  - UI notes: `mode = all` deletes the stored row and restores the default; `mentions_only` and `mute` persist sparse preference state.
  - Errors / edge cases: blank or invalid mode returns `422`; hidden threads still return `404`.

## Frontend-impacting behavior changes

- No-workspace signed-in entry now has a canonical two-endpoint contract: check `GET /api/v1/workspaces` first, then `GET /api/v1/my/invitations?status=pending` before empty-workspace onboarding.
- Notification inbox rows and SSE remain convenience/freshness surfaces; they are not the canonical routing authority for invitation-first no-workspace entry.
- Invitation responses now carry live state fields such as `status`, `version`, `accepted_at`, `responded_at`, and `cancelled_at`; the UI should stop assuming invitation state is implicit.
- Notification inbox data is now the canonical source for read state, while unread badges should come from the unread-count endpoint or SSE, not from local inbox math.
- SSE is now part of the notification UX, so inbox and badge refresh should react to stream events instead of polling only.
- Thread replies can now carry mention targets, but the public thread response shape did not change, so mention UI must rely on request intent and side effects rather than response fields.
- Thread notification preferences are now per-thread and sparse; `all` means "no stored override," not a separate persisted preference value.
