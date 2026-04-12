# UI State Matrix

This matrix removes ambiguity for invitation, notification, unread, SSE, and thread notification preference UX.
Use `frontend-repo/API_CONTRACT.md` as the source of truth when a state here and the API contract appear to differ.

## Invitation Domain State Matrix

| Surface | Invitation status | Primary actions shown | Secondary UI state | Notes |
| --- | --- | --- | --- | --- |
| Recipient inbox notification | `pending` | `Accept`, `Reject` | Show pending status pill and invite role | Show actions only while the latest fetched invitation is still pending. |
| Recipient inbox notification | `accepted` | None | Show accepted status pill | Hide action buttons. Render as terminal. |
| Recipient inbox notification | `rejected` | None | Show rejected status pill | Hide action buttons. Render as terminal. |
| Recipient inbox notification | `cancelled` | None | Show cancelled status pill | Hide action buttons. Render as terminal. |
| Workspace owner invitation list | `pending` | `Edit role`, `Cancel` | Show pending status pill | Owner-only controls. Hide both controls once the invitation leaves `pending`. |
| Workspace owner invitation list | `accepted`, `rejected`, `cancelled` | None | Show terminal status pill | Do not keep edit/cancel controls on terminal rows. |
| Any invitation action request | `409 conflict` | None | Refresh row state, then re-render | Treat as stale invitation state. Refetch the invitation or inbox row, use the returned `status` and `version`, and disable the old buttons until the refetch completes. |

Rules:
- Action buttons are driven by the latest invitation `status` and `actionable`, not by `is_read`.
- Pending invitations stay actionable until the backend returns a terminal state.
- Terminal invitations never show accept/reject/cancel/edit controls.
- If an invitation action starts from a notification row, use the latest `payload.version` from that row for the accept or reject request.
- If a row was opened from cached data and the action returns `409`, the UI should assume the cached version is stale and refresh before retrying.

## Notification Item Rendering Matrix

| Notification type | Row headline / expectation | Read styling | Action area | UI expectation |
| --- | --- | --- | --- | --- |
| `invitation` + `pending` | Invitation pending response | Unread: stronger weight and accent. Read: subdued. | Show invitation actions while pending | Keep the row actionable until the invitation status changes. |
| `invitation` + terminal status | Invitation resolved | Unread/read styling still applies | None | Show the final invitation status, not the old pending action set. |
| `comment` | New comment on a page/thread | Unread: stronger weight and accent. Read: subdued. | Usually none from inbox row | Present as discussion activity from relevant users. |
| `mention` | You were mentioned | Unread: stronger weight and accent. Read: subdued. | Usually none from inbox row | Present as explicit attention. Do not merge it into the related comment row. |

Unread styling rules:
- Unread rows should be visually heavier, with a clear unread marker and stronger contrast.
- Read rows should remain visible but quieter.
- Read state must not hide invitation actions for still-pending invitations.

Comment vs mention rules:
- `comment` rows and `mention` rows are separate inbox items.
- They may coexist for the same thread message.
- Do not dedupe one away just because the thread/message ids match.
- Use distinct copy or badges so explicit mentions are obvious.

## Inbox Interaction Matrix

| Interaction | Expected behavior | Data source | Failure handling |
| --- | --- | --- | --- |
| Open a notification row | Navigate to the linked resource. Do not mark it read unless the UI explicitly calls the single-read endpoint. | REST inbox item + current route state | If navigation or fetch reveals a changed invitation state, re-render from the latest response. |
| Mark one row read | Set the row to read, then update the unread badge | `POST /api/v1/notifications/{notificationID}/read` | If the request fails, revert the local read state and show an error. |
| Accept or reject from invitation notification | Use the current row `payload.version`, then replace or refetch the row after success | Invitation action endpoint + canonical inbox | On `409`, refetch the row or inbox and render the returned terminal or updated state instead of retrying blindly. |
| Batch mark-read | Send the selected notification ids in one atomic request | `POST /api/v1/notifications/read` | Treat the response as all-or-nothing. If it fails, keep the current unread state and keep the selection visible only if that helps retry. |
| Batch mark-read success | Flip only unread rows to read, then reconcile the badge with the returned `unread_count` | Batch response `updated_count` and `unread_count` | Use `updated_count` as the number of unread-to-read transitions, not the size of the selection. |
| Batch mark-read with already-read rows | Succeed with `updated_count` reflecting only rows that changed | Batch response | Do not treat already-read ids as an error. |
| Batch mark-read with duplicate ids | Do not submit a request | Client validation | Dedupe before sending, or block the action and explain the selection is invalid. |

Batch read rules:
- `POST /api/v1/notifications/read` is atomic.
- Use the response `unread_count` as the authoritative badge value after success.
- If the UI performs an optimistic update, reconcile it with the server response before settling the state.
- Do not invent partial-success UI for this endpoint.
- If product design wants open-equals-read, the UI must explicitly call `POST /api/v1/notifications/{notificationID}/read`.

## SSE Client Behavior Matrix

| SSE event | Badge behavior | Inbox list behavior | Connection behavior |
| --- | --- | --- | --- |
| `snapshot` | Set the unread badge from `snapshot.unread_count` | Do not assume the inbox list is already fresh | Use this as the initial state seed when the stream opens or reconnects. |
| `unread_count` | Update the unread badge only when the value changes | Do not refetch the inbox list just for this event | This is a badge update signal, not a row payload. |
| `inbox_invalidated` | Keep the current badge until REST refetch returns a new value | Refetch the inbox list and reconcile unread count from REST | This is the signal that the list may be stale. Coalesce repeated invalidations if needed. |
| Stream disconnect | Preserve the last known badge until a new `snapshot` arrives | Keep the current list visible, but mark it stale if the product shows that state | Reconnect with a fresh stream. `Last-Event-ID` is ignored, so do not expect replay. |

SSE rules:
- Treat REST inbox and unread-count endpoints as source of truth.
- A fresh `snapshot` seeds state; it does not guarantee the inbox list is current.
- `unread_count` is badge-only.
- `inbox_invalidated` means refetch the list.
- On reconnect, do a fresh fetch instead of waiting for missed events.

## Thread Mention And Preference Matrix

| Mode | UI label | Selected state | Meaning in the UI | Backend note |
| --- | --- | --- | --- | --- |
| `all` | `All activity` | Default when no stored preference exists | Show the full thread notification setting | Backend stores this as no row. |
| `mentions_only` | `Mentions only` | Selected when the stored preference is `mentions_only` | Show the reduced notification setting | Backend currently stores preference state only. |
| `mute` | `Muted` | Selected when the stored preference is `mute` | Show the no-notifications setting | Backend currently stores preference state only. |

Preference rules:
- `GET /api/v1/threads/{threadID}/notification-preference` returns `all` when no preference row exists.
- `PUT /api/v1/threads/{threadID}/notification-preference` saves only the preference state.
- `mode = all` deletes the stored row.
- `mode = mentions_only` and `mode = mute` upsert the stored row.
- The UI must not promise delivery suppression yet. The backend does not change notification delivery behavior based on this setting today.

## Recommended Copy

Use these labels or close equivalents to keep the UI unambiguous:

| Situation | Suggested copy |
| --- | --- |
| Pending invitation row | `Awaiting your response` |
| Stale invitation action | `This invitation changed. Refresh to continue.` |
| Unread badge | `Unread` |
| Mention row | `You were mentioned` |
| Thread muted preference | `Muted for this thread` |
