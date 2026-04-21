# API_CONTRACT.md

Backend base URL (local default):
- `http://localhost:8080`

API version prefix:
- `/api/v1`

## 1. Global Conventions

### 1.1 Content Type
- JSON requests must use `Content-Type: application/json`.
- Responses are JSON except `204 No Content`.

### 1.2 Auth
Protected endpoints require:
- `Authorization: Bearer <access_token>`

Public endpoints:
- `GET /healthz`
- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`

### 1.3 Success Envelope
Most success responses:
```json
{ "data": ... }
```

Exception:
- `204` has no body.

### 1.4 Error Envelope
```json
{
  "error": {
    "code": "validation_failed|invalid_credentials|unauthorized|forbidden|not_found|conflict|internal_error",
    "message": "human readable message",
    "request_id": "..."
  }
}
```

### 1.5 Timestamps
- RFC3339 UTC format (example: `2026-03-08T10:30:00Z`).

### 1.6 Common Status Codes
- `200` OK
- `201` Created
- `204` No Content
- `400` malformed JSON
- `401` unauthorized / token expired
- `403` forbidden
- `404` not found
- `409` conflict
- `422` validation failed

## 2. Domain Models

### 2.1 User
```json
{
  "id": "uuid",
  "email": "user@example.com",
  "full_name": "User Name",
  "created_at": "2026-03-08T10:00:00Z",
  "updated_at": "2026-03-08T10:00:00Z"
}
```

### 2.2 Workspace
```json
{
  "id": "uuid",
  "name": "Product",
  "created_at": "2026-03-08T10:00:00Z",
  "updated_at": "2026-03-08T10:00:00Z"
}
```

### 2.3 WorkspaceMember
```json
{
  "id": "uuid",
  "workspace_id": "uuid",
  "user_id": "uuid",
  "role": "owner|editor|viewer",
  "created_at": "2026-03-08T10:00:00Z",
  "user": {
    "id": "uuid",
    "email": "user@example.com",
    "full_name": "User Name",
    "created_at": "2026-03-08T10:00:00Z",
    "updated_at": "2026-03-08T10:00:00Z"
  }
}
```

### 2.4 WorkspaceInvitation
```json
{
  "id": "uuid",
  "workspace_id": "uuid",
  "email": "invitee@example.com",
  "role": "owner|editor|viewer",
  "status": "pending|accepted|rejected|cancelled",
  "version": 3,
  "invited_by": "uuid",
  "updated_at": "2026-03-08T10:00:00Z",
  "accepted_at": "2026-03-08T10:00:00Z",
  "responded_by": "uuid",
  "responded_at": "2026-03-08T10:00:00Z",
  "cancelled_by": "uuid",
  "cancelled_at": "2026-03-08T10:00:00Z",
  "created_at": "2026-03-08T10:00:00Z"
}
```

### 2.5 Folder
```json
{
  "id": "uuid",
  "workspace_id": "uuid",
  "parent_id": "uuid",
  "name": "Engineering",
  "created_at": "2026-03-08T10:00:00Z",
  "updated_at": "2026-03-08T10:00:00Z"
}
```

### 2.6 Page
```json
{
  "id": "uuid",
  "workspace_id": "uuid",
  "folder_id": "uuid",
  "title": "Architecture",
  "created_by": "uuid",
  "created_at": "2026-03-08T10:00:00Z",
  "updated_at": "2026-03-08T10:00:00Z"
}
```

### 2.6a PageSummary
```json
{
  "id": "uuid",
  "workspace_id": "uuid",
  "folder_id": "uuid",
  "title": "Architecture",
  "updated_at": "2026-03-08T10:00:00Z"
}
```

### 2.7 PageDraft
```json
{
  "page_id": "uuid",
  "content": [],
  "last_edited_by": "uuid",
  "created_at": "2026-03-08T10:00:00Z",
  "updated_at": "2026-03-08T10:00:00Z"
}
```

### 2.8 RevisionSummary
```json
{
  "id": "uuid",
  "page_id": "uuid",
  "label": "v1",
  "note": "Checkpoint before refactor",
  "created_by": "uuid",
  "created_at": "2026-03-08T10:00:00Z"
}
```

### 2.9 PageComment
```json
{
  "id": "uuid",
  "page_id": "uuid",
  "body": "Please verify this section",
  "created_by": "uuid",
  "created_at": "2026-03-08T10:00:00Z",
  "resolved_by": "uuid",
  "resolved_at": "2026-03-08T11:00:00Z"
}
```

### 2.10 TrashItem
```json
{
  "id": "uuid",
  "workspace_id": "uuid",
  "page_id": "uuid",
  "page_title": "Doc title",
  "deleted_by": "uuid",
  "deleted_at": "2026-03-08T10:00:00Z"
}
```

### 2.11 NotificationInboxItem
```json
{
  "id": "uuid",
  "workspace_id": "uuid",
  "type": "invitation|comment|mention",
  "actor_id": "uuid",
  "actor": {
    "id": "uuid",
    "email": "owner@example.com",
    "full_name": "Owner"
  },
  "title": "Workspace invitation",
  "content": "You have a new workspace invitation",
  "is_read": false,
  "read_at": "2026-03-08T11:00:00Z",
  "actionable": true,
  "action_kind": "invitation_response",
  "resource_type": "invitation|thread_message",
  "resource_id": "uuid",
  "payload": {},
  "created_at": "2026-03-08T10:00:00Z",
  "updated_at": "2026-03-08T10:00:00Z"
}
```
- This is the canonical DTO shape returned by `GET /api/v1/notifications` and `POST /api/v1/notifications/{notificationID}/read`.
- It is an API response model, not a raw storage-row dump.

### 2.12 PageCommentThreadAnchor
```json
{
  "type": "block|page_legacy",
  "block_id": "uuid-or-null",
  "quoted_text": "optional matched selection snapshot",
  "quoted_block_text": "current or captured block text snapshot"
}
```

### 2.13 PageCommentThread
```json
{
  "id": "uuid",
  "page_id": "uuid",
  "anchor": {
    "type": "block",
    "block_id": "uuid",
    "quoted_text": "hello",
    "quoted_block_text": "hello world"
  },
  "thread_state": "open|resolved",
  "anchor_state": "active|outdated|missing",
  "created_by": "uuid",
  "created_at": "2026-03-19T08:00:00Z",
  "resolved_by": "uuid",
  "resolved_at": "2026-03-19T09:00:00Z",
  "resolve_note": "Fixed in latest revision",
  "reopened_by": "uuid",
  "reopened_at": "2026-03-19T09:30:00Z",
  "reopen_reason": "Follow-up requested",
  "last_activity_at": "2026-03-19T09:30:00Z",
  "reply_count": 2
}
```
- `anchor_state` semantics in backend:
    - `active`: anchored block still exists and exact block text matches `anchor.quoted_block_text`
    - `outdated`: anchored block still exists but block text changed
    - `missing`: anchored block can no longer be found

### 2.14 PageCommentThreadMessage
```json
{
  "id": "uuid",
  "thread_id": "uuid",
  "body": "Please revise this line",
  "created_by": "uuid",
  "created_at": "2026-03-19T08:00:00Z"
}
```

### 2.15 PageCommentThreadEvent
```json
{
  "id": "uuid",
  "thread_id": "uuid",
  "type": "created|replied|resolved|reopened|anchor_state_changed|anchor_recovered",
  "actor_id": "uuid",
  "message_id": "uuid",
  "revision_id": "uuid",
  "from_thread_state": "resolved",
  "to_thread_state": "open",
  "from_anchor_state": "active",
  "to_anchor_state": "missing",
  "from_block_id": "uuid",
  "to_block_id": "uuid",
  "reason": "draft_updated|page_deleted|page_restored|revision_restored",
  "note": "Follow-up requested",
  "created_at": "2026-03-19T09:30:00Z"
}
```
- `reason` is currently used for system-generated `anchor_state_changed` events.
- `note` is currently used for user-supplied resolve/reopen metadata.
- `from_block_id` and `to_block_id` are populated for `anchor_recovered` events.
- `revision_id` is populated for anchor reevaluation events triggered by revision restore.

### 2.16 PageCommentThreadDetail
```json
{
  "thread": { "id": "uuid", "page_id": "uuid", "anchor": { "type": "block", "block_id": "uuid", "quoted_text": "hello", "quoted_block_text": "hello world" }, "thread_state": "open", "anchor_state": "active", "created_by": "uuid", "created_at": "2026-03-19T08:00:00Z", "resolve_note": null, "reopen_reason": null, "last_activity_at": "2026-03-19T08:00:00Z", "reply_count": 1 },
  "messages": [
    { "id": "uuid", "thread_id": "uuid", "body": "Please revise this line", "created_by": "uuid", "created_at": "2026-03-19T08:00:00Z" }
  ],
  "events": [
    { "id": "uuid", "thread_id": "uuid", "type": "created", "actor_id": "uuid", "message_id": "uuid", "created_at": "2026-03-19T08:00:00Z" }
  ]
}
```

### 2.17 PageCommentThreadFilterCounts
```json
{
  "open": 4,
  "resolved": 2,
  "active": 3,
  "outdated": 1,
  "missing": 2
}
```

### 2.18 PageCommentThreadList
```json
{
  "threads": [
    {
      "id": "uuid",
      "page_id": "uuid",
      "anchor": {
        "type": "block",
        "block_id": "uuid",
        "quoted_text": "hello",
        "quoted_block_text": "hello world"
      },
      "thread_state": "open",
      "anchor_state": "active",
      "created_by": "uuid",
      "created_at": "2026-03-19T08:00:00Z",
      "last_activity_at": "2026-03-19T08:15:00Z",
      "reply_count": 2
    }
  ],
  "counts": {
    "open": 4,
    "resolved": 2,
    "active": 3,
    "outdated": 1,
    "missing": 2
  }
}
```

## 3. Endpoint Contract

## 3.1 Health

### GET `/healthz`
- Auth: no
- Response `200`:
```json
{ "data": { "status": "ok" } }
```

## 3.2 Auth

### POST `/api/v1/auth/register`
- Auth: no
- Request:
```json
{
  "email": "user@example.com",
  "password": "Password1",
  "full_name": "User Name"
}
```
- Response `201`: `User`

### POST `/api/v1/auth/login`
- Auth: no
- Request:
```json
{ "email": "user@example.com", "password": "Password1" }
```
- Response `200`:
```json
{
  "data": {
    "user": { "id": "uuid", "email": "user@example.com", "full_name": "User Name", "created_at": "...", "updated_at": "..." },
    "tokens": {
      "access_token": "jwt",
      "access_token_expires_at": "...",
      "refresh_token": "opaque",
      "refresh_token_expires_at": "..."
    }
  }
}
```

### POST `/api/v1/auth/refresh`
- Auth: no
- Request:
```json
{ "refresh_token": "opaque" }
```
- Response `200`: same shape as login response

### POST `/api/v1/auth/logout`
- Auth: no
- Request:
```json
{ "refresh_token": "opaque" }
```
- Response `204`

### GET `/api/v1/auth/me`
- Auth: yes
- Response `200`: `User`
- Frontend usage:
    - Use this for authenticated identity only.
    - Do not treat this response as sufficient to decide no-workspace invitation-first routing.

## 3.3 Workspaces and Members

### GET `/api/v1/workspaces`
- Auth: yes
- Response `200`: `Workspace[]`
- Security and scope:
    - Returns only workspaces where the authenticated user is a member.
    - Must not include workspaces from other users.
- Frontend usage:
    - Call this after login/refresh to load persisted workspace context.
    - This is the authoritative membership-presence check for signed-in entry.
    - If the response is non-empty, continue the normal workspace-aware app entry flow.
    - If the response is empty, do not send the user straight to empty-workspace onboarding yet; follow with `GET /api/v1/my/invitations?status=pending`.
    - An empty workspace list means only that the actor has no current memberships. It does not imply there are no pending invitations.

### POST `/api/v1/workspaces`
- Auth: yes
- Request:
```json
{ "name": "Product" }
```
- Response `201`:
```json
{
  "data": {
    "workspace": { "id": "uuid", "name": "Product", "created_at": "...", "updated_at": "..." },
    "membership": { "id": "uuid", "workspace_id": "uuid", "user_id": "uuid", "role": "owner", "created_at": "..." }
  }
}
```
- Validation:
    - `workspace` must be unique
    - if workspace already exist, returns `422 validation_failed`

### PATCH `/api/v1/workspaces/{workspaceID}`
- Auth: yes (`owner`)
- Request:
```json
{ "name": "Platform" }
```
- Response `200`: updated `Workspace`
- Validation:
    - `name` is required after trim
    - duplicate workspace name for the acting user returns `422 validation_failed`
    - duplicate comparison is trim-aware and case-insensitive

### POST `/api/v1/workspaces/{workspaceID}/invitations`
- Auth: yes (`owner`)
- Request:
```json
{ "email": "invitee@example.com", "role": "viewer" }
```
- Response `201`: `WorkspaceInvitation`
- Response body:
```json
{
  "data": {
    "id": "uuid",
    "workspace_id": "uuid",
    "email": "invitee@example.com",
    "role": "viewer",
    "status": "pending",
    "version": 1,
    "invited_by": "uuid",
    "created_at": "2026-04-04T08:00:00Z",
    "updated_at": "2026-04-04T08:00:00Z",
    "accepted_at": null
  }
}
```
- Validation:
    - `email` must be valid format
    - `role` must be one of `owner|editor|viewer`
    - invitee email is normalized before duplicate checks and persistence
    - target email must already belong to a registered account
    - actor cannot invite their own account email
    - successful create always returns:
      - `status = pending`
      - `version = 1`
      - `updated_at = created_at`
- Error codes:
    - `409 invitation_self_email` when the normalized target email matches the authenticated actor email
    - `409 invitation_target_unregistered` when no registered account exists for the normalized target email
    - `409 invitation_existing_member` when the registered target is already a member of the workspace
    - `409 invitation_duplicate_pending` when a pending invite already exists for the same workspace and normalized email
    - `422 validation_failed` for invalid email or invalid role

### GET `/api/v1/workspaces/{workspaceID}/invitations`
- Auth: yes (`owner`)
- Query parameters:
    - `status` optional: `pending|accepted|rejected|cancelled|all`, default `all`
    - `limit` optional: positive integer, default `50`, max `100`
    - `cursor` optional: opaque pagination cursor returned by a previous response
- Response `200`:
```json
{
  "data": {
    "items": [
      {
        "id": "uuid",
        "workspace_id": "uuid",
        "email": "invitee@example.com",
        "role": "viewer",
        "status": "pending",
        "version": 1,
        "invited_by": "uuid",
        "created_at": "2026-04-04T08:00:00Z",
        "updated_at": "2026-04-04T08:00:00Z",
        "accepted_at": null
      }
    ],
    "next_cursor": "opaque-cursor",
    "has_more": true
  }
}
```
- Validation:
    - only owners may list workspace invitations
    - invalid `status` returns `422 validation_failed`
    - invalid `limit` returns `422 validation_failed`
    - invalid `cursor` returns `422 validation_failed`
    - empty result set returns `200` with `items=[]`, `has_more=false`
- Behavior:
    - results are ordered by `created_at DESC, id DESC`
    - `status=all` applies no status filter
    - `next_cursor` is omitted on the final page
    - `has_more=true` means another request can continue with `cursor=<next_cursor>`

### GET `/api/v1/my/invitations`
- Auth: yes
- Query parameters:
    - `status` optional: `pending|accepted|rejected|cancelled|all`, default `all`
    - `limit` optional: positive integer, default `50`, max `100`
    - `cursor` optional: opaque pagination cursor returned by a previous response
- Response `200`:
```json
{
  "data": {
    "items": [
      {
        "id": "uuid",
        "workspace_id": "uuid",
        "email": "member@example.com",
        "role": "viewer",
        "status": "pending",
        "version": 1,
        "invited_by": "uuid",
        "created_at": "2026-04-04T08:00:00Z",
        "updated_at": "2026-04-04T08:00:00Z",
        "accepted_at": null
      }
    ],
    "next_cursor": "opaque-cursor",
    "has_more": true
  }
}
```
- Validation:
    - actor must resolve to an existing user record
    - results are scoped by the authenticated user’s canonical email, not by a client-supplied email
    - invalid `status` returns `422 validation_failed`
    - invalid `limit` returns `422 validation_failed`
    - invalid `cursor` returns `422 validation_failed`
    - unknown actor user record returns `401 unauthorized`
- Behavior:
    - returns matching invitations across all workspaces
    - results are ordered by `created_at DESC, id DESC`
    - `status=all` applies no status filter
    - empty result set returns `200` with `items=[]`, `has_more=false`
    - `next_cursor` is omitted on the final page
- Frontend usage:
    - For authenticated users with zero workspace memberships, `GET /api/v1/my/invitations?status=pending` is the authoritative invitation-review source.
    - Canonical no-workspace entry flow:
      1. call `GET /api/v1/workspaces`
      2. if the workspace list is empty, call `GET /api/v1/my/invitations?status=pending`
      3. if pending invitations exist, route to pending invitation review before empty-workspace onboarding
      4. if no pending invitations exist, continue to empty-workspace onboarding
    - Invitation rows from this endpoint, not notification payloads, should drive accept/reject actions and pending-invitation review state.
    - After accepting or rejecting from this entry path, refresh both:
      - `GET /api/v1/workspaces`
      - `GET /api/v1/my/invitations?status=pending`

### PATCH `/api/v1/workspace-invitations/{invitationID}`
- Auth: yes (`owner`)
- Request:
```json
{ "role": "editor", "version": 3 }
```
- Response `200`: updated `WorkspaceInvitation`
- Response body:
```json
{
  "data": {
    "id": "uuid",
    "workspace_id": "uuid",
    "email": "invitee@example.com",
    "role": "editor",
    "status": "pending",
    "version": 4,
    "invited_by": "uuid",
    "created_at": "2026-04-04T08:00:00Z",
    "updated_at": "2026-04-04T09:00:00Z",
    "accepted_at": null
  }
}
```
- Validation:
    - `role` is required and must be one of `owner|editor|viewer`
    - `version` is required and must be a positive integer
    - request body must be a single JSON object with no unknown fields
    - invitation must exist
    - actor must be an owner of the invitation workspace
    - only `pending` invitations can be updated
    - request `version` must match the current invitation version
- Behavior:
    - workspace is derived from the invitation row, not from the URL
    - `email` is immutable and cannot be supplied in this endpoint
    - when `role` changes:
      - `version` increments by `1`
      - `updated_at` changes
    - when `role` is unchanged and `version` matches:
      - returns `200`
      - invitation is unchanged
      - `version` does not increment
      - `updated_at` does not change
- Error codes:
    - `400 invalid_json` for malformed JSON, multiple JSON values, or unknown fields
    - `401 unauthorized` for missing or invalid auth
    - `403 forbidden` for non-owner actors in the invitation workspace
    - `404 not_found` when the invitation does not exist
    - `409 conflict` for stale version or non-pending invitation
    - `422 validation_failed` for invalid role or invalid version

### POST `/api/v1/workspace-invitations/{invitationID}/accept`
- Auth: yes
- Request:
```json
{ "version": 3 }
```
- Response `200`: accepted invitation plus created membership
- Response body:
```json
{
  "data": {
    "invitation": {
      "id": "uuid",
      "workspace_id": "uuid",
      "email": "invitee@example.com",
      "role": "editor",
      "status": "accepted",
      "version": 4,
      "invited_by": "uuid",
      "created_at": "2026-04-04T08:00:00Z",
      "updated_at": "2026-04-04T09:00:00Z",
      "accepted_at": "2026-04-04T09:00:00Z",
      "responded_by": "uuid",
      "responded_at": "2026-04-04T09:00:00Z",
      "cancelled_by": null,
      "cancelled_at": null
    },
    "membership": {
      "id": "uuid",
      "workspace_id": "uuid",
      "user_id": "uuid",
      "role": "editor",
      "created_at": "2026-04-04T09:00:00Z"
    }
  }
}
```
- Validation:
    - request body must be a single JSON object with no unknown fields
    - `version` is required and must be a positive integer
    - actor must resolve to an existing user record
    - invitation must exist
    - actor email must match the invitation target email
    - only `pending` invitations can be accepted
    - request `version` must match the current invitation version
    - user must not already be a member of the workspace
- Behavior:
    - foreign invitations are hidden as `404 not_found`
    - successful accept is atomic:
      - membership is created
      - invitation becomes `accepted`
    - successful accept returns:
      - accepted invitation
      - created membership
- Error codes:
    - `400 invalid_json` for malformed JSON, empty body, multiple JSON values, or unknown fields
    - `401 unauthorized` for missing or invalid auth, or missing actor user record
    - `404 not_found` when the invitation does not exist or does not belong to the authenticated email
    - `409 conflict` for stale version, non-pending invitation, or existing workspace membership
    - `422 validation_failed` for missing or invalid version

### POST `/api/v1/workspace-invitations/{invitationID}/reject`
- Auth: yes
- Request:
```json
{ "version": 3 }
```
- Response `200`: rejected `WorkspaceInvitation`
- Response body:
```json
{
  "data": {
    "id": "uuid",
    "workspace_id": "uuid",
    "email": "invitee@example.com",
    "role": "editor",
    "status": "rejected",
    "version": 4,
    "invited_by": "uuid",
    "created_at": "2026-04-04T08:00:00Z",
    "updated_at": "2026-04-04T09:00:00Z",
    "accepted_at": null,
    "responded_by": "uuid",
    "responded_at": "2026-04-04T09:00:00Z",
    "cancelled_by": null,
    "cancelled_at": null
  }
}
```
- Validation:
    - request body must be a single JSON object with no unknown fields
    - `version` is required and must be a positive integer
    - actor must resolve to an existing user record
    - invitation must exist
    - actor email must match the invitation target email
    - only `pending` invitations can be rejected
    - request `version` must match the current invitation version
- Behavior:
    - foreign invitations are hidden as `404 not_found`
    - successful reject updates only the invitation row
    - successful reject returns:
      - `status = rejected`
      - `responded_by`
      - `responded_at`
    - reject does not create workspace membership
- Error codes:
    - `400 invalid_json` for malformed JSON, empty body, multiple JSON values, or unknown fields
    - `401 unauthorized` for missing or invalid auth, or missing actor user record
    - `404 not_found` when the invitation does not exist or does not belong to the authenticated email
    - `409 conflict` for stale version or non-pending invitation
    - `422 validation_failed` for missing or invalid version

### POST `/api/v1/workspace-invitations/{invitationID}/cancel`
- Auth: yes (`owner`)
- Request:
```json
{ "version": 3 }
```
- Response `200`: cancelled `WorkspaceInvitation`
- Response body:
```json
{
  "data": {
    "id": "uuid",
    "workspace_id": "uuid",
    "email": "invitee@example.com",
    "role": "editor",
    "status": "cancelled",
    "version": 4,
    "invited_by": "uuid",
    "created_at": "2026-04-04T08:00:00Z",
    "updated_at": "2026-04-04T09:00:00Z",
    "accepted_at": null,
    "responded_by": null,
    "responded_at": null,
    "cancelled_by": "uuid",
    "cancelled_at": "2026-04-04T09:00:00Z"
  }
}
```
- Validation:
    - request body must be a single JSON object with no unknown fields
    - `version` is required and must be a positive integer
    - actor must resolve to an existing user record
    - invitation must exist
    - actor must be a member of the invitation workspace to access it
    - actor must have `owner` role in the invitation workspace
    - only `pending` invitations can be cancelled
    - request `version` must match the current invitation version
- Behavior:
    - non-members see `404 not_found`
    - same-workspace non-owner members receive `403 forbidden`
    - any current owner may cancel; cancellation is not limited to `invited_by`
    - successful cancel updates only the invitation row
    - successful cancel returns:
      - `status = cancelled`
      - `cancelled_by`
      - `cancelled_at`
    - cancel does not create or remove workspace membership
- Error codes:
    - `400 invalid_json` for malformed JSON, empty body, multiple JSON values, or unknown fields
    - `401 unauthorized` for missing or invalid auth, or missing actor user record
    - `403 forbidden` for same-workspace non-owner members
    - `404 not_found` when the invitation does not exist or the actor is not a workspace member
    - `409 conflict` for stale version or non-pending invitation
    - `422 validation_failed` for missing or invalid version

### GET `/api/v1/workspaces/{workspaceID}/members`
- Auth: yes (workspace member)
- Response `200`: `WorkspaceMember[]`

### PATCH `/api/v1/workspaces/{workspaceID}/members/{memberID}/role`
- Auth: yes (`owner`)
- Request:
```json
{ "role": "editor" }
```
- Response `200`: updated `WorkspaceMember`

## 3.4 Folders

### POST `/api/v1/workspaces/{workspaceID}/folders`
- Auth: yes (`owner|editor`)
- Request:
```json
{ "name": "Engineering", "parent_id": "uuid-or-null" }
```
- Response `201`: `Folder`
- Validation:
    - `name` is required after trim
    - sibling folder names must be unique within the same `(workspace_id, parent_id)` scope
    - root folders are siblings of other root folders
    - duplicate comparison is trim-aware and case-insensitive
    - same folder name is allowed under different parents

### GET `/api/v1/workspaces/{workspaceID}/folders`
- Auth: yes (workspace member)
- Response `200`: `Folder[]`

### PATCH `/api/v1/folders/{folderID}`
- Auth: yes (`owner|editor`)
- Request:
```json
{ "name": "Platform" }
```
- Response `200`: updated `Folder`
- Validation:
    - `name` is required after trim
    - rename only updates `name`; moving folders is not supported by this endpoint
    - sibling folder names must be unique within the same `(workspace_id, parent_id)` scope
    - duplicate comparison is trim-aware and case-insensitive
    - same folder name is allowed under different parents

## 3.5 Pages and Drafts

### GET `/api/v1/workspaces/{workspaceID}/pages`
- Auth: yes (workspace member)
- Query parameters:
    - `folder_id` optional
    - omitted or blank => list workspace-root pages only (`folder_id IS NULL`)
    - non-blank => list direct pages in that folder only
- Response `200`: `PageSummary[]`
- Validation and behavior:
    - if `folder_id` is provided, the folder must exist
    - folder from another workspace returns `422 validation_failed`
    - missing folder returns `404`
    - response excludes soft-deleted pages
    - response is ordered by `updated_at DESC, id ASC`
- Frontend usage:
    - initial sidebar/tree bootstrap should fetch:
      - `GET /api/v1/workspaces/{workspaceID}/folders`
      - `GET /api/v1/workspaces/{workspaceID}/pages`
    - fetch folder pages lazily with `GET /api/v1/workspaces/{workspaceID}/pages?folder_id={folderID}`

### POST `/api/v1/workspaces/{workspaceID}/pages`
- Auth: yes (`owner|editor`)
- Request:
```json
{ "title": "Architecture", "folder_id": "uuid-or-null" }
```
- Response `201`:
```json
{
  "data": {
    "page": { "id": "uuid", "workspace_id": "uuid", "folder_id": "uuid", "title": "Architecture", "created_by": "uuid", "created_at": "...", "updated_at": "..." },
    "draft": { "page_id": "uuid", "content": [], "last_edited_by": "uuid", "created_at": "...", "updated_at": "..." }
  }
}
```

### GET `/api/v1/pages/{pageID}`
- Auth: yes (workspace member)
- Response `200`: same shape as create page (`page` + `draft`)
- Behavior:
    - only returns active pages
    - trashed pages are not readable through this endpoint

### PATCH `/api/v1/pages/{pageID}`
- Auth: yes (`owner|editor`)
- Request supports partial updates:
```json
{ "title": "New Title" }
```
```json
{ "folder_id": "uuid" }
```
```json
{ "folder_id": null }
```
- `folder_id` semantics:
    - omitted: keep current folder
    - `null`: move to workspace root
    - string: move to that folder
- Response `200`: updated `Page`

### PUT `/api/v1/pages/{pageID}/draft`
- Auth: yes (`owner|editor`)
- Request:
```json
{ "content": [ { "type": "paragraph", "children": [ { "type": "text", "text": "hello" } ] } ] }
```
- Response `200`: updated `PageDraft`
- Validation and behavior:
    - each saved top-level block must include a stable non-empty `id`
    - draft save rejects blocks without `id` with `422 validation_failed`
    - this requirement keeps thread anchors aligned with persisted draft blocks

### DELETE `/api/v1/pages/{pageID}`
- Auth: yes (`owner|editor`)
- Behavior: soft-delete page to trash
- Response `204`

## 3.6 Revisions

### POST `/api/v1/pages/{pageID}/revisions`
- Auth: yes (`owner|editor`)
- Request:
```json
{ "label": "v1", "note": "Before major edits" }
```
- Response `201`: `RevisionSummary`

### GET `/api/v1/pages/{pageID}/revisions`
- Auth: yes (workspace member)
- Response `200`: `RevisionSummary[]`

### GET `/api/v1/pages/{pageID}/revisions/compare?from={id}&to={id}`
- Auth: yes (workspace member)
- Response `200`:
```json
{
  "data": {
    "page_id": "uuid",
    "from_revision_id": "uuid",
    "to_revision_id": "uuid",
    "blocks": [
      {
        "index": 0,
        "status": "unchanged|modified|added|removed",
        "from": { "type": "paragraph", "text": "hello world" },
        "to": { "type": "paragraph", "text": "hello brave world" },
        "inline_diff": [
          { "operation": "equal", "text": "hello" },
          { "operation": "added", "text": "brave" },
          { "operation": "equal", "text": "world" }
        ],
        "lines": [
          {
            "operation": "removed",
            "from_line_number": 1,
            "text": "hello world",
            "chunks": [
              { "operation": "equal", "text": "hello " },
              { "operation": "removed", "text": "world" }
            ]
          },
          {
            "operation": "added",
            "to_line_number": 1,
            "text": "hello brave world",
            "chunks": [
              { "operation": "equal", "text": "hello " },
              { "operation": "added", "text": "brave " },
              { "operation": "equal", "text": "world" }
            ]
          }
        ]
      }
    ]
  }
}
```
- `blocks[].lines` is the primary Git-style rendering payload:
    - `operation=context` means the same source line matched on both sides
    - `operation=removed` means the line exists only in the `from` revision
    - `operation=added` means the line exists only in the `to` revision
    - `from_line_number` and `to_line_number` stay aligned across insertions and deletions, so removing line 3 causes old line 4 to align with new line 3
- `blocks[].inline_diff` remains available as a whole-block summary for modified blocks.

### POST `/api/v1/pages/{pageID}/revisions/{revisionID}/restore`
- Auth: yes (`owner|editor`)
- Response `200`:
```json
{
  "data": {
    "draft": { "page_id": "uuid", "content": [], "last_edited_by": "uuid", "created_at": "...", "updated_at": "..." },
    "revision": { "id": "uuid", "page_id": "uuid", "label": null, "note": null, "created_by": "uuid", "created_at": "..." }
  }
}
```

## 3.7 Comments

Legacy flat comments are still supported by the backend, but they are deprecated for new collaborative review work.
Use threaded discussion endpoints for new frontend discussion UI.
Flat comments remain page-level only and do not support replies, anchors, `anchor_state`, or thread lifecycle behavior.

### POST `/api/v1/pages/{pageID}/comments`
- Auth: yes (workspace member, including `viewer`)
- Request:
```json
{ "body": "Please verify this section" }
```
- Response `201`: `PageComment`
- Status:
    - deprecated for new discussion UI
    - kept for backward compatibility with existing flat comment flows

### GET `/api/v1/pages/{pageID}/comments`
- Auth: yes (workspace member)
- Response `200`: `PageComment[]`
- Status:
    - deprecated for new discussion UI
    - returns legacy flat comments only

### POST `/api/v1/comments/{commentID}/resolve`
- Auth: yes (`owner|editor`)
- Response `200`: updated `PageComment`
- Status:
    - deprecated for new discussion UI
    - resolves one flat comment item, not a reply thread

## 3.8 Search

### GET `/api/v1/workspaces/{workspaceID}/search?q={query}`
- Auth: yes (workspace member)
- Query parameter:
    - `q` required and non-blank
- Response `200`:
```json
{ "data": [ { "id": "uuid", "workspace_id": "uuid", "folder_id": "uuid", "title": "Architecture", "updated_at": "..." } ] }
```

## 3.9 Threads

### POST `/api/v1/pages/{pageID}/threads`
- Auth: yes (workspace member, including `viewer`)
- Request:
```json
{
  "body": "Please revise this line",
  "mentions": ["user-id-1", "user-id-2"],
  "anchor": {
    "type": "block",
    "block_id": "uuid",
    "quoted_text": "hello",
    "quoted_block_text": "hello world"
  }
}
```
- Response `201`: `PageCommentThreadDetail`
- Validation:
    - `body` is required after trim
    - `mentions` is optional
    - `mentions` may be omitted or `null`
    - when provided, `mentions` must be a JSON array of strings
    - mention entries must be user ids, not emails
    - each mention id is trimmed before validation
    - blank mention ids are rejected
    - duplicate mention ids are deduped in first-seen order
    - at most `20` unique mention ids are allowed
    - every mention id must belong to the thread page workspace
    - self-mentions are allowed
    - `anchor.type` must be `block`
    - `anchor.block_id` must exist in the current page draft
    - `anchor.quoted_text`, if provided, must exist within the anchored block text
    - `anchor.quoted_block_text`, if provided, must match the current anchored block snapshot
- Behavior:
    - backend stores `quoted_block_text` from the current server-side block snapshot
    - initial states are `thread_state=open` and `anchor_state=active`
    - response includes the starter message in `messages[0]`
    - response includes `events` as an array, with the starter `created` event in `events[0]`
    - response shape is unchanged when mentions are supplied
    - backend persists one `page_comment_message_mentions` row per normalized mention id for the starter message
    - successful thread creation records a `thread_created` outbox event transactionally
    - `thread_created` outbox payload includes `mention_user_ids`, which is `[]` when there are no mentions
    - thread-create notification delivery is asynchronous and no longer happens in the request path
    - thread creator does not receive their own notification

### GET `/api/v1/threads/{threadID}`
- Auth: yes (workspace member of the thread page)
- Response `200`: `PageCommentThreadDetail`
- Behavior:
    - `messages` are ordered by `created_at ASC, id ASC`
    - `events` are ordered by `created_at ASC, id ASC`
    - event history is only returned by thread detail; list endpoints do not include `events`
    - `reply_count` reflects current message count for the thread
    - thread anchors may be automatically recovered when the original `block_id` disappears and backend reevaluation finds:
      - an exact unique `anchor.quoted_block_text` match, which keeps the thread `active`
      - or a unique `anchor.quoted_text` match inside one current block, which recovers the anchor as `outdated`
    - successful anchor recovery is recorded as `anchor_recovered` in thread detail events
    - when reevaluation is triggered by revision restore, related thread events also carry the triggering `revision_id`

### GET `/api/v1/pages/{pageID}/threads`
- Auth: yes (workspace member)
- Query parameters:
    - `thread_state` optional: `open|resolved|all`
    - `anchor_state` optional: `active|outdated|missing|all`
    - `created_by` optional: `me`
    - `has_missing_anchor` optional: `true|false`
    - `has_outdated_anchor` optional: `true|false`
    - `sort` optional: `recent_activity|newest|oldest`
    - `limit` optional: positive integer, default `50`, max `100`
    - `cursor` optional: opaque pagination cursor returned by a previous response
    - `q` optional substring search
- Response `200`: `PageCommentThreadList`
- Validation:
    - invalid `thread_state` returns `422 validation_failed`
    - invalid `anchor_state` returns `422 validation_failed`
    - invalid `created_by` returns `422 validation_failed`
    - invalid `has_missing_anchor` returns `422 validation_failed`
    - invalid `has_outdated_anchor` returns `422 validation_failed`
    - invalid `sort` returns `422 validation_failed`
    - invalid `limit` returns `422 validation_failed`
    - invalid `cursor` returns `422 validation_failed`
- Behavior:
    - `counts` are page-scoped totals and do not change with current filters
    - `threads` are filtered by `thread_state`, `anchor_state`, `created_by`, `has_missing_anchor`, `has_outdated_anchor`, and `q`
    - `created_by=me` filters to threads created by the current authenticated user
    - `has_missing_anchor=true` keeps only threads whose `anchor_state` is `missing`
    - `has_missing_anchor=false` excludes threads whose `anchor_state` is `missing`
    - `has_outdated_anchor=true` keeps only threads whose `anchor_state` is `outdated`
    - `has_outdated_anchor=false` excludes threads whose `anchor_state` is `outdated`
    - if a thread's original `block_id` disappears, backend reevaluation may recover the anchor to a new `block_id` when:
      - `quoted_block_text` matches exactly one current block
      - or `quoted_text` matches exactly one current block and no exact full-block recovery is available
    - default ordering matches `sort=recent_activity`
    - `sort=recent_activity` orders open threads first, then `last_activity_at DESC`, then `id ASC`
    - `sort=newest` orders by `created_at DESC`, then `id ASC`
    - `sort=oldest` orders by `created_at ASC`, then `id ASC`
    - pagination is forward-only and sort-aware
    - `next_cursor` is omitted on the final page
    - `has_more=true` means another request can continue with `cursor=<next_cursor>`
    - `q` matches `anchor.quoted_text`, `anchor.quoted_block_text`, and any thread message body
    - default order is:
      - open threads first
      - then `last_activity_at DESC`
      - then `id ASC`
    - search is intentionally page-scoped in v1; there is no workspace-global thread search yet

### GET `/api/v1/workspaces/{workspaceID}/threads`
- Auth: yes (workspace member)
- Query parameters:
    - `thread_state` optional: `open|resolved|all`
    - `anchor_state` optional: `active|outdated|missing|all`
    - `created_by` optional: `me`
    - `has_missing_anchor` optional: `true|false`
    - `has_outdated_anchor` optional: `true|false`
    - `sort` optional: `recent_activity|newest|oldest`
    - `limit` optional: positive integer, default `50`, max `100`
    - `cursor` optional: opaque pagination cursor returned by a previous response
    - `q` optional substring search
- Response `200`:
```json
{
  "threads": [
    {
      "thread": { "id": "uuid", "page_id": "uuid", "anchor": { "type": "block", "block_id": "uuid", "quoted_text": "hello", "quoted_block_text": "hello world" }, "thread_state": "open", "anchor_state": "active", "created_by": "uuid", "created_at": "2026-03-19T08:00:00Z", "last_activity_at": "2026-03-19T08:00:00Z", "reply_count": 1 },
      "page": { "id": "uuid", "workspace_id": "uuid", "folder_id": "uuid", "title": "Architecture", "updated_at": "2026-03-19T08:00:00Z" }
    }
  ],
  "counts": { "open": 1, "resolved": 0, "active": 1, "outdated": 0, "missing": 0 },
  "next_cursor": "opaque-cursor",
  "has_more": true
}
```

### 2.19 ThreadNotificationPreferenceView
```json
{
  "thread_id": "uuid",
  "mode": "all|mentions_only|mute"
}
```
- Validation:
    - invalid `thread_state` returns `422 validation_failed`
    - invalid `anchor_state` returns `422 validation_failed`
    - invalid `created_by` returns `422 validation_failed`
    - invalid `has_missing_anchor` returns `422 validation_failed`
    - invalid `has_outdated_anchor` returns `422 validation_failed`
    - invalid `sort` returns `422 validation_failed`
    - invalid `limit` returns `422 validation_failed`
    - invalid `cursor` returns `422 validation_failed`
- Behavior:
    - `counts` are workspace-scoped totals and do not change with current filters
    - the same thread filters/sort semantics as page thread listing apply here
    - `q` matches page title, `anchor.quoted_text`, `anchor.quoted_block_text`, and any thread message body
    - only threads on active pages are returned; threads for trashed pages are excluded
    - anchor recovery uses the same deterministic rules as page-scoped reevaluation:
      - exact unique `quoted_block_text` match first
      - then unique `quoted_text` match as fallback
    - when reevaluation is triggered by revision restore, related thread events also carry the restored `revision_id`
    - pagination is forward-only and sort-aware

### POST `/api/v1/threads/{threadID}/replies`
- Auth: yes (workspace member of the thread page)
- Request:
```json
{
  "body": "Follow-up reply",
  "mentions": ["user-id-1", "user-id-2"]
}
```
- Response `201`: `PageCommentThreadDetail`
- Validation:
    - `body` is required after trim
    - `mentions` is optional and may be omitted or `null`
    - when present, `mentions` must be a JSON array of strings
    - mention ids are normalized by trimming, rejecting blanks, deduping in first-seen order, and enforcing a maximum of 20 unique ids
    - mention ids must belong to current workspace members of the thread page workspace
    - self-mentions are allowed
- Behavior:
    - appends a new message ordered by `created_at ASC, id ASC`
    - updates `thread.last_activity_at`
    - replying to a resolved thread auto-reopens it
    - auto-reopen clears `resolved_by` and `resolved_at`
    - auto-reopen clears stale `resolve_note` and `reopen_reason`
    - auto-reopen sets `reopened_by` and `reopened_at`
    - response returns the full updated thread detail including the new reply
    - response shape remains unchanged when mentions are supplied
    - reply messages persist one `page_comment_message_mentions` row per normalized mention id
    - successful reply creation records a `thread_reply_created` outbox event transactionally
    - `thread_reply_created` outbox payload includes `mention_user_ids`, which is `[]` when there are no mentions
    - reply notification delivery is asynchronous and no longer happens in the request path
    - reply author does not receive a direct synchronous notification

### POST `/api/v1/threads/{threadID}/resolve`
- Auth: yes (`owner|editor`)
- Request:
```json
{
  "resolve_note": "Fixed in latest revision"
}
```
- Response `200`: `PageCommentThreadDetail`
- Behavior:
    - idempotent
    - sets `thread_state=resolved`
    - sets `resolved_by` and `resolved_at`
    - persists optional trimmed `resolve_note`
    - clears stale `reopen_reason`
    - does not remove existing messages

### POST `/api/v1/threads/{threadID}/reopen`
- Auth: yes (workspace member of the thread page)
- Request:
```json
{
  "reopen_reason": "Follow-up requested"
}
```
- Response `200`: `PageCommentThreadDetail`
- Behavior:
    - idempotent
    - sets `thread_state=open`
    - clears `resolved_by` and `resolved_at`
    - clears stale `resolve_note`
    - sets `reopened_by` and `reopened_at`
    - persists optional trimmed `reopen_reason`

### GET `/api/v1/threads/{threadID}/notification-preference`
- Auth: yes (workspace member of the thread page)
- Request body: none
- Response `200`:
```json
{
  "data": {
    "thread_id": "uuid",
    "mode": "all"
  }
}
```
- Behavior:
    - reuses the existing thread visibility rules, including 404 hiding for non-members and inaccessible or trashed threads
    - returns the actor's stored per-thread notification mode when a preference row exists
    - returns `mode = all` when no preference row exists
    - does not create or update any preference row
    - does not accept query parameters
    - mode values are limited to `all`, `mentions_only`, and `mute`

### PUT `/api/v1/threads/{threadID}/notification-preference`
- Auth: yes (workspace member of the thread page)
- Request:
```json
{
  "mode": "mentions_only"
}
```
- Response `200`:
```json
{
  "data": {
    "thread_id": "uuid",
    "mode": "mentions_only",
    "updated_at": "2026-04-09T04:00:00Z"
  }
}
```
- Validation:
    - request body must be a single JSON object
    - unknown fields return `400 invalid_json`
    - malformed JSON returns `400 invalid_json`
    - wrong JSON types return `400 invalid_json`
    - `mode` is required after trim
    - blank `mode` returns `422 validation_failed`
    - invalid `mode` returns `422 validation_failed`
    - allowed modes are `all`, `mentions_only`, and `mute`
- Behavior:
    - reuses the existing thread visibility rules, including 404 hiding for non-members and inaccessible or trashed threads
    - `mode = all` deletes any stored preference row for the actor and returns the default effective mode `all`
    - `mode = mentions_only` and `mode = mute` upsert a preference row for the actor
    - the endpoint is idempotent in effect
    - `updated_at` is the server-side operation time captured once per request
    - no comment or mention delivery behavior changes yet; this endpoint only stores preference state

### Planned Next Thread Work
- Current backend status:
    - anchor-state evaluation rules are implemented internally
    - draft updates now trigger anchor-state reevaluation
    - revision restore now also triggers anchor-state reevaluation
- page-scoped thread search is finalized for v1
- thread create notification delivery now uses the transactional outbox
- thread reply notification delivery now uses the transactional outbox
- thread resolve and reopen do not send notifications in v1
- resolve/reopen activity note metadata is live
- thread lifecycle event history is live on thread detail
- legacy flat comment deprecation notes are now documented in this contract
- Next frontend task: thread panel skeleton using the shipped thread endpoints

## 3.10 Trash

### GET `/api/v1/workspaces/{workspaceID}/trash`
- Auth: yes (workspace member)
- Response `200`: `TrashItem[]`

### GET `/api/v1/trash/{trashItemID}`
- Auth: yes (workspace member of the trash item's workspace)
- Response `200`:
```json
{
  "data": {
    "trash_item": { "id": "uuid", "workspace_id": "uuid", "page_id": "uuid", "page_title": "Architecture", "deleted_by": "uuid", "deleted_at": "..." },
    "page": { "id": "uuid", "workspace_id": "uuid", "folder_id": "uuid", "title": "Architecture", "created_by": "uuid", "created_at": "...", "updated_at": "..." },
    "draft": { "page_id": "uuid", "content": [], "last_edited_by": "uuid", "created_at": "...", "updated_at": "..." }
  }
}
```
- Behavior:
    - only returns pages that are currently in trash
    - use this endpoint to preview deleted page content before restore

### POST `/api/v1/trash/{trashItemID}/restore`
- Auth: yes (`owner|editor`)
- Response `200`: restored `Page`

## 3.11 Notifications

### GET `/api/v1/notifications`
- Auth: yes
- Query parameters:
    - `status` optional: `all|read|unread`, default `all`
    - `type` optional: `all|invitation|comment|mention`, default `all`
    - `limit` optional: positive integer, default `50`, max `100`
    - `cursor` optional: opaque pagination cursor returned by a previous response
- Response `200`:
```json
{
  "data": {
    "items": [
      {
        "id": "uuid",
        "workspace_id": "uuid",
        "type": "invitation",
        "actor_id": "uuid",
        "actor": {
          "id": "uuid",
          "email": "owner@example.com",
          "full_name": "Owner"
        },
        "title": "Workspace invitation",
        "content": "You have a new workspace invitation",
        "is_read": false,
        "read_at": null,
        "actionable": true,
        "action_kind": "invitation_response",
        "resource_type": "invitation",
        "resource_id": "uuid",
        "payload": {
          "invitation_id": "uuid",
          "workspace_id": "uuid",
          "email": "invitee@example.com",
          "role": "editor",
          "status": "pending",
          "version": 3,
          "can_accept": true,
          "can_reject": true
        },
        "created_at": "2026-04-04T08:00:00Z",
        "updated_at": "2026-04-04T08:00:00Z"
      }
    ],
    "unread_count": 12,
    "next_cursor": "opaque-cursor",
    "has_more": true
  }
}
```
- Validation:
    - invalid `status` returns `422 validation_failed`
    - invalid `type` returns `422 validation_failed`
    - invalid `limit` returns `422 validation_failed`
    - invalid `cursor` returns `422 validation_failed`
- Behavior:
    - returns only the authenticated actor inbox
    - ordered by `created_at DESC, id DESC`
    - `unread_count` is the actor total unread count across the whole inbox, not the current filter subset
    - `next_cursor` is omitted on the final page
    - `actor` is `null` when actor metadata is unavailable
    - invitation rows reflect the latest live invitation state and do not duplicate the same invitation
    - invitation rows include populated payload fields for `invitation_id`, `workspace_id`, `email`, `role`, `status`, `version`, `can_accept`, and `can_reject`
    - invitation notification payloads may be used for inline inbox accept/reject actions, but they do not replace invitation source-of-truth ownership
    - comment rows are projected asynchronously from `thread_created` and `thread_reply_created` outbox events
    - comment rows use `type = comment`, `resource_type = thread_message`, and payload fields for `thread_id`, `message_id`, `page_id`, `workspace_id`, and `event_topic`
    - comment rows are delivered only to relevant users, not to the full workspace
    - mention rows are projected asynchronously from the same `thread_created` and `thread_reply_created` outbox events
    - mention rows use `type = mention`, `resource_type = thread_message`, and payload fields for `thread_id`, `message_id`, `page_id`, `workspace_id`, `event_topic`, and `mention_source = explicit`
    - comment and mention rows may coexist for the same message when a user is both relevant and explicitly mentioned

### POST `/api/v1/notifications/{notificationID}/read`
- Auth: yes (notification owner)
- Response `200`:
```json
{
  "data": {
    "id": "uuid",
    "workspace_id": "uuid",
    "type": "invitation",
    "actor_id": "uuid",
    "actor": {
      "id": "uuid",
      "email": "owner@example.com",
      "full_name": "Owner"
    },
    "title": "Workspace invitation",
    "content": "You have a new workspace invitation",
    "is_read": true,
    "read_at": "2026-04-04T09:00:00Z",
    "actionable": true,
    "action_kind": "invitation_response",
    "resource_type": "invitation",
    "resource_id": "uuid",
    "payload": {
      "invitation_id": "uuid",
      "workspace_id": "uuid",
      "email": "invitee@example.com",
      "role": "editor",
      "status": "pending",
      "version": 3,
      "can_accept": true,
      "can_reject": true
    },
    "created_at": "2026-04-04T08:00:00Z",
    "updated_at": "2026-04-04T09:00:00Z"
  }
}
```
- Validation:
    - malformed `notificationID` returns `404 not_found`
    - missing notification returns `404 not_found`
    - other-user notification returns `404 not_found`
- Behavior:
    - marks the notification read for the authenticated owner only
    - first unread-to-read transition sets `is_read=true`, `read_at`, and `updated_at`
    - repeated mark-read stays `200` and preserves the original `read_at` and `updated_at`
    - returns the same inbox item DTO used by `GET /api/v1/notifications`
    - invitation rows preserve the same populated invitation payload fields returned by inbox listing

### POST `/api/v1/notifications/read`
- Auth: yes
- Request:
```json
{
  "notification_ids": [
    "uuid-1",
    "uuid-2"
  ]
}
```
- Response `200`:
```json
{
  "data": {
    "updated_count": 2,
    "unread_count": 9
  }
}
```
- Validation:
    - request body must be valid JSON
    - request body must contain exactly one object
    - unknown fields return `400 invalid_json`
    - empty body returns `400 invalid_json`
    - missing auth returns `401 unauthorized`
    - invalid auth returns `401 unauthorized`
    - authenticated actor id that no longer resolves to a user returns `401 unauthorized`
    - `notification_ids` is required
    - `notification_ids` must contain between `1` and `100` ids
    - every id must be a valid UUID
    - duplicate ids return `422 validation_failed`
- Behavior:
    - actor must resolve to an existing user record
    - every requested notification must belong to the authenticated owner
    - any missing or foreign id returns `404 not_found`
    - the batch is atomic and all-or-nothing
    - already-read notifications are idempotent and do not increment `updated_count`
    - `updated_count` counts only unread-to-read transitions
    - `unread_count` comes from the unread counter table after the transaction commits
    - repeated requests against already-read rows return `200` with `updated_count = 0`

### GET `/api/v1/notifications/unread-count`
- Auth: yes
- Response `200`:
```json
{
  "data": {
    "unread_count": 12
  }
}
```
- Validation:
    - missing auth returns `401 unauthorized`
    - invalid auth returns `401 unauthorized`
    - authenticated actor id that no longer resolves to a user returns `401 unauthorized`
- Behavior:
    - returns the authenticated actor's total unread notification count across all notification types
    - returns `0` when the actor has no unread notifications
    - returns `0` when the actor does not yet have a counter row
    - does not accept filters, query parameters, or request body
    - unread count is backed by a dedicated per-user counter read model, not by loading inbox rows

### GET `/api/v1/notifications/stream`
- Auth: yes
- Transport: `text/event-stream`
- Response `200`:
  - `Content-Type: text/event-stream`
  - `Cache-Control: no-cache`
  - `Connection: keep-alive`
  - `X-Accel-Buffering: no`
- Behavior:
    - sends one initial `snapshot` event immediately after the stream opens
    - `snapshot` payload:
      - `unread_count`
      - `sent_at`
    - sends `unread_count` events only when the latest unread count differs from the last value sent on that connection
    - `unread_count` payload:
      - `unread_count`
      - `sent_at`
    - sends `inbox_invalidated` events whenever the user's inbox may need refetch
    - `inbox_invalidated` payload:
      - `reason = notifications_changed`
      - `sent_at`
    - sends a heartbeat comment at least every `25s`
    - ignores `Last-Event-ID` in v1 and does not replay missed events
    - returns `500` before headers if the response writer cannot flush or if the stream cannot open
    - closes the stream on later internal failures after headers are sent
    - REST inbox and unread-count endpoints remain the source of truth for canonical state

## 4. Structured Document Validation

`content` must be a JSON array of blocks.

Allowed block `type` values:
- `paragraph`
- `heading` (requires `level: 1..6`)
- `bullet_list` (requires `items`)
- `numbered_list` (requires `items`, optional `start >= 1`)
- `task_list` (requires `items`, each item requires `checked`)
- `quote`
- `code_block` (requires `text`, optional `language`)
- `table` (requires `rows[].cells[]`)
- `image` (requires non-empty `src`, optional `alt`)

Text container rules:
- must define exactly one of `text` or `children`
- `children` supports inline nodes of type `text`

Allowed mark types on inline text:
- `bold`
- `italic`
- `inline_code`
- `link` (requires valid `http` or `https` href)

Validation failures return `422 validation_failed`.

## 5. Frontend Integration Checklist

- Always unwrap `{data}` and normalize `{error}` in one API client layer.
- Implement one-time refresh retry flow for protected requests on `401`.
- Never rely on frontend role checks alone; backend `403` handling is required.
- Handle `PATCH /pages/{pageID}` folder semantics exactly as documented.
- Keep autosave draft and save revision as separate UI intents.
- Do not infer notification creation rules in frontend; consume notification APIs only.
- Thread panel integration should use:
  - `POST /api/v1/pages/{pageID}/threads`
  - `GET /api/v1/pages/{pageID}/threads`
  - `GET /api/v1/threads/{threadID}`
  - `POST /api/v1/threads/{threadID}/replies`
  - `POST /api/v1/threads/{threadID}/resolve`
  - `POST /api/v1/threads/{threadID}/reopen`
- Do not build new product flows on legacy flat comment endpoints unless the work is explicitly scoped as backward-compatibility support.
- Expect `anchor_state` to update after `PUT /api/v1/pages/{pageID}/draft`.
- Expect `anchor_state` to update after `POST /api/v1/pages/{pageID}/revisions/{revisionID}/restore` as well.
