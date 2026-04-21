# Invitation Notification Payload Contract Violation

Date: 2026-04-19

Status: Confirmed backend contract violation

Audience: Backend team

## Summary

The frontend notification surfaces cannot render `Accept` / `Reject` actions for invitation notifications because the backend is returning invitation notification rows with an empty `payload` object.

This violates the documented notification contract for invitation rows and breaks both:

- notification preview popover
- full notification inbox

The separate invitee invitation endpoint still returns valid pending invitation data, so the issue is isolated to notification projection/serialization rather than invitation storage itself.

## User Impact

Affected user flow:

- invitee receives workspace invitation
- invitee opens notification preview or full inbox
- invitation row appears, but no actionable controls are available

Current result:

- user cannot accept or reject from notifications
- user must fall back to `My invitations`

This is a UX regression against the intended notification-driven invitation flow.

## Expected Contract

Per `API_CONTRACT.md`, invitation notification rows returned by both:

- `GET /api/v1/notifications`
- `POST /api/v1/notifications/{notificationID}/read`

must include invitation action payload fields such as:

```json
{
  "payload": {
    "invitation_id": "uuid",
    "workspace_id": "uuid",
    "email": "invitee@example.com",
    "role": "editor",
    "status": "pending",
    "version": 3,
    "can_accept": true,
    "can_reject": true
  }
}
```

Relevant contract references:

- `API_CONTRACT.md:1403` `GET /api/v1/notifications`
- `API_CONTRACT.md:1434` invitation notification payload fields
- `API_CONTRACT.md:1472` `POST /api/v1/notifications/{notificationID}/read`
- `API_CONTRACT.md:1496` invitation notification payload fields in mark-read response

## Observed Live Behavior

Observed on 2026-04-19 from the active frontend browser session.

### 1. Notification list response

Request:

`GET /api/v1/notifications?status=all&type=all&limit=10`

Observed invitation notification row:

```json
{
  "id": "3d03c82a-c13e-4f4b-9a71-847eb1d96a2d",
  "workspace_id": "d9bbeaf5-3d6f-499b-aab6-e7444e6cdea9",
  "type": "invitation",
  "actor_id": "a30d2b66-7263-47c8-82e4-af0bba01a37c",
  "actor": {
    "id": "a30d2b66-7263-47c8-82e4-af0bba01a37c",
    "email": "rawr@yopmail.com",
    "full_name": "rawr"
  },
  "title": "Workspace invitation",
  "content": "You have a new workspace invitation",
  "is_read": true,
  "read_at": "2026-04-18T19:40:47.272412+07:00",
  "actionable": true,
  "action_kind": "invitation_response",
  "resource_type": "invitation",
  "resource_id": "23e617d5-850a-40cc-912d-ca16a01d8340",
  "payload": {},
  "created_at": "2026-04-18T19:37:15.92929+07:00",
  "updated_at": "2026-04-18T19:40:47.272412+07:00"
}
```

Problem:

- `payload` is empty
- row is marked `actionable: true`
- row does not include the fields required to actually perform the action

### 2. Mark-read response

Request:

`POST /api/v1/notifications/3d03c82a-c13e-4f4b-9a71-847eb1d96a2d/read`

Observed response:

```json
{
  "id": "3d03c82a-c13e-4f4b-9a71-847eb1d96a2d",
  "workspace_id": "d9bbeaf5-3d6f-499b-aab6-e7444e6cdea9",
  "type": "invitation",
  "actor_id": "a30d2b66-7263-47c8-82e4-af0bba01a37c",
  "actor": {
    "id": "a30d2b66-7263-47c8-82e4-af0bba01a37c",
    "email": "rawr@yopmail.com",
    "full_name": "rawr"
  },
  "title": "Workspace invitation",
  "content": "You have a new workspace invitation",
  "is_read": true,
  "read_at": "2026-04-18T19:40:47.272412+07:00",
  "actionable": true,
  "action_kind": "invitation_response",
  "resource_type": "invitation",
  "resource_id": "23e617d5-850a-40cc-912d-ca16a01d8340",
  "payload": {},
  "created_at": "2026-04-18T19:37:15.92929+07:00",
  "updated_at": "2026-04-18T19:40:47.272412+07:00"
}
```

Problem:

- same contract violation exists in the canonical mark-read response
- this confirms the issue is not limited to one list endpoint response path

### 3. Invitee invitation endpoint remains correct

Request:

`GET /api/v1/my/invitations?status=pending&limit=10`

Observed response item:

```json
{
  "id": "23e617d5-850a-40cc-912d-ca16a01d8340",
  "workspace_id": "d9bbeaf5-3d6f-499b-aab6-e7444e6cdea9",
  "email": "test@yopmail.com",
  "role": "viewer",
  "invited_by": "a30d2b66-7263-47c8-82e4-af0bba01a37c",
  "created_at": "2026-04-18T19:37:15.913627+07:00",
  "status": "pending",
  "version": 1,
  "updated_at": "2026-04-18T19:37:15.913627+07:00"
}
```

Interpretation:

- invitation itself exists
- invitation is still pending
- invitation version is available
- notification row lost the payload needed to represent the same invitation

## Root Cause Statement

Backend notification projection for invitation rows is emitting:

- `actionable: true`
- `action_kind: "invitation_response"`

but not serializing the invitation action payload required by the API contract.

The frontend cannot safely render or submit invitation actions from notifications without:

- `payload.invitation_id`
- `payload.version`
- `payload.status`
- `payload.can_accept`
- `payload.can_reject`

## Likely Backend Fault Area

Most likely fault locations:

- invitation notification read-model projection
- invitation notification serializer / DTO mapper
- invitation payload hydration during notification list response
- invitation payload hydration during mark-read response

Specific inconsistency to investigate:

- `resource_id` is present and appears to equal the invitation ID
- `payload` is empty even though `actionable` is true

That suggests the notification row still knows which invitation it points to, but the payload mapping step is missing or failing.

## Required Backend Fix

For invitation notification rows, both:

- `GET /api/v1/notifications`
- `POST /api/v1/notifications/{notificationID}/read`

should return a populated invitation payload matching the documented contract.

Minimum required fields:

```json
{
  "payload": {
    "invitation_id": "23e617d5-850a-40cc-912d-ca16a01d8340",
    "workspace_id": "d9bbeaf5-3d6f-499b-aab6-e7444e6cdea9",
    "email": "test@yopmail.com",
    "role": "viewer",
    "status": "pending",
    "version": 1,
    "can_accept": true,
    "can_reject": true
  }
}
```

## Acceptance Criteria

- Invitation notification rows returned by `GET /api/v1/notifications` include a non-empty payload with the documented invitation fields.
- Invitation notification rows returned by `POST /api/v1/notifications/{notificationID}/read` include the same payload shape.
- For pending invitee-owned invitations:
  - `payload.status = "pending"`
  - `payload.invitation_id` is present
  - `payload.version` is present
  - `payload.can_accept` and `payload.can_reject` reflect server authorization
- For terminal invitations:
  - payload still reflects latest invitation state
  - `can_accept` / `can_reject` become false as appropriate
- `actionable` and payload contents remain logically consistent.

## Suggested Backend Verification

1. Create a pending workspace invitation for an existing invitee.
2. Query `GET /api/v1/notifications` as the invitee.
3. Confirm the invitation notification row includes the expected populated payload.
4. Call `POST /api/v1/notifications/{notificationID}/read`.
5. Confirm the returned notification row still includes the populated payload.
6. Accept or reject the invitation and verify subsequent notification reads reflect terminal state.

## Frontend Note

Frontend notification actions currently depend on the invitation payload for safe rendering and mutation inputs. Even when the row is marked `actionable: true`, the frontend cannot execute the action if the payload is empty.
