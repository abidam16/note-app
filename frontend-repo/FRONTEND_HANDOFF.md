# Frontend Handoff: Invitations, Notifications, Comments, and Thread Preferences

**Read this first.** This is the entry point for the frontend agent working on invitation, notification, comment/mention, and thread-notification-preference UX.

## Reading Order

Use this order and treat it as strict:

1. This handoff.
2. `frontend-repo/IMPLEMENTATION_SEQUENCE.md`.
3. The API delta doc for the current frontend task.
4. The UI state matrix.
5. The mocks.
6. `frontend-repo/API_CONTRACT.md`.
7. `frontend-repo/CONTEXT.md` if extra product background is needed.

If any source conflicts, follow the higher-priority source above and do not infer missing behavior from older frontend code.

If a human is dispatching another frontend AI, use `frontend-repo/AI_EXECUTION_PROMPT.md` as the paste-ready launch prompt.

## What Changed

The backend is no longer using the older, simplified mental model. The frontend now needs to support:

- invitation center and invitation listing driven by explicit invitation state
- inbox rendering with canonical read/unread data
- unread badge updates from canonical unread count plus SSE freshness
- comment and mention notifications as separate collaboration signals
- thread notification preference read/write UI
- thread mention UI hooks in create/reply flows

REST remains canonical. SSE is freshness and invalidation only.

## Old Assumptions That Are Now Wrong

- An invitation is not just a pending row plus accept time. It now has explicit state, versioning, and update semantics.
- Invitation notifications are not append-only. There is one live notification per invitation, updated as the invitation changes.
- Comment notifications are not synchronous request-path side effects. They are projected asynchronously.
- Mention notifications are not merged into comment notifications. They are a separate append-only notification stream.
- Thread notifications are not for the full workspace. They follow the relevant-user policy.
- Thread preference UI should not assume delivery changes beyond what the docs explicitly state.
- The frontend must not infer server behavior outside the docs.

## Source Of Truth

Follow these rules in this order:

- This handoff defines the frontend task boundary.
- The API delta doc defines the current contract changes.
- The UI state matrix defines UI states, empty states, loading states, and transitions.
- The mocks define expected presentation and interaction shape.
- `frontend-repo/API_CONTRACT.md` is the canonical endpoint and payload reference.

Do not reverse this order.

## Relevant-User Notification Policy

Thread comment notifications go only to relevant users:

- thread creator
- prior repliers
- explicit mention targets
- never the acting user
- never users without workspace access

Mentions are separate notifications and can coexist with comment notifications for the same message.

## Scope Boundaries

Build now:

- invitation center/listing UI
- inbox list and unread badge integration
- SSE-backed freshness/invalidation handling for inbox and badge refresh
- thread mention UI hooks for create and reply flows
- thread notification preference UI for `all`, `mentions_only`, and `mute`

Do not assume yet:

- delivery filtering semantics beyond the documented preference storage behavior
- any new notification types
- any server behavior not described in the docs
- workspace-wide thread fanout
- legacy flat comments as the primary collaboration path

## Frontend Surfaces To Update

- Invitation center and invitation listing
- Inbox
- Unread badge
- SSE freshness handling
- Thread mention UI hooks
- Thread notification preference UI

## Implementation Notes

- Use the REST inbox and unread-count endpoints as canonical state.
- Use SSE only to trigger refetch or invalidation.
- Treat invitation rows as versioned state, not static records.
- When acting on an invitation from the inbox, use the invitation `payload.version` from the latest fetched row.
- Treat notification rows as user-scoped inbox items.
- Treat thread message mentions as explicit user selection, not free-text parsing.
