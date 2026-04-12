# Frontend Implementation Sequence

Use this file to decide what to build first.

This is not a product spec. It is an execution-order guide so a frontend AI does not start with the wrong surface.

## Goal

Build the frontend enhancement in vertical slices that reduce risk early:

1. typed data and API integration first
2. invitation lifecycle next
3. inbox and unread behavior after that
4. SSE freshness only after REST flows are stable
5. thread mention and preference UI last

## Read Before Coding

Read in this order:

1. `frontend-repo/FRONTEND_HANDOFF.md`
2. `frontend-repo/IMPLEMENTATION_SEQUENCE.md`
3. `frontend-repo/API_DELTA_NOTIFICATION_INVITATION.md`
4. `frontend-repo/UI_STATE_MATRIX.md`
5. `frontend-repo/mocks/`
6. `frontend-repo/API_CONTRACT.md`

## Recommended Build Order

### Phase 1: Data And API Client

Build first:

- typed invitation models
- typed inbox notification models
- typed unread-count model
- typed thread notification preference models
- typed SSE event payloads
- API client wrappers for the new and changed endpoints

Why first:

- every later surface depends on these shapes
- this is the easiest place to lock correct backend assumptions
- it reduces UI rework caused by bad client typing

Do not build yet:

- final visual polish
- SSE connection management
- optimistic invitation conflict handling

### Phase 2: Invitation Lists And Actions

Build next:

- owner workspace invitation list
- current-user invitation list
- owner update and cancel flows
- invitee accept and reject flows

Required behavior:

- use server `status` and `version`
- hide terminal action buttons
- handle `409` by refetching and re-rendering
- if the actor accepts successfully, update workspace context from returned membership

Why before inbox:

- invitation state is the most stateful and conflict-prone part of the feature
- it is easier to validate invitation actions in a dedicated surface before reusing them inside notification rows

### Phase 3: Inbox And Unread Badge

Build after invitation flows are stable:

- inbox list UI
- read/unread rendering
- single mark-read
- batch mark-read
- unread badge based on canonical unread count

Required behavior:

- treat `GET /notifications` as the canonical inbox source
- treat `GET /notifications/unread-count` as the cheap badge source
- do not derive total unread count from visible filtered rows
- do not merge comment and mention rows
- keep one live invitation notification card per invitation

Why after invitation lists:

- the inbox reuses invitation action rules
- invitation notification rendering is much clearer after the dedicated invitation flow exists

### Phase 4: SSE Freshness

Build only after REST inbox and unread-count behavior is correct:

- one authenticated SSE connection
- `snapshot` handling
- `unread_count` handling
- `inbox_invalidated` handling
- reconnect behavior

Required behavior:

- REST remains source of truth
- `snapshot` seeds initial badge state
- `unread_count` updates badge only
- `inbox_invalidated` triggers inbox refetch
- do not assume missed-event replay in v1

Why not earlier:

- SSE adds complexity but does not define canonical state
- implementing it too early often hides REST bugs instead of exposing them

### Phase 5: Thread Mention Hooks And Notification Preferences

Build last:

- mention user selection in create-thread flow
- mention user selection in reply flow
- thread notification preference read UI
- thread notification preference write UI

Required behavior:

- send explicit mention user IDs only
- do not parse free-text `@mentions` unless the product already has a stable mapper
- treat preference values as stored UI state only
- do not promise delivery suppression yet for `mentions_only` or `mute`

Why last:

- these surfaces depend on the notification mental model already being clear
- preference persistence exists, but backend delivery semantics are intentionally limited today

## What To Ignore Until The End

Ignore these until Phases 1 through 4 are stable:

- grouped notifications
- notification dedupe heuristics in UI
- speculative delivery filtering for `mute`
- workspace-wide comment broadcast assumptions
- legacy flat comment UI as the primary collaboration flow

## Acceptance Check Per Phase

### Phase 1 done when:

- all new endpoint payloads are typed
- API client wrappers exist
- mock payloads deserialize cleanly into the chosen frontend types

### Phase 2 done when:

- owner and invitee flows work from dedicated invitation surfaces
- `409` stale-version handling is visible and correct
- terminal invitations no longer show actions

### Phase 3 done when:

- inbox renders invitation, comment, and mention rows distinctly
- unread badge uses canonical unread-count data
- single and batch mark-read flows reconcile with server values

### Phase 4 done when:

- SSE reconnect is stable
- badge and inbox refresh rules follow the UI state matrix
- the app still behaves correctly when SSE is unavailable

### Phase 5 done when:

- thread create/reply can submit `mentions`
- thread preference can read and write `all|mentions_only|mute`
- the UI does not imply unsupported backend delivery behavior

## If Time Is Limited

Use this reduced order:

1. Phase 1
2. Phase 2
3. Phase 3

Ship that first.

Then add:

4. Phase 4
5. Phase 5

## Final Rule

If the existing frontend codebase patterns conflict with this sequence, preserve the existing architecture but keep the feature order above.
