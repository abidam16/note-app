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
  "invited_by": "uuid",
  "accepted_at": "2026-03-08T10:00:00Z",
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

### 2.11 Notification
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "workspace_id": "uuid",
  "type": "invitation|comment",
  "event_id": "uuid",
  "message": "New comment on a page in your workspace",
  "created_at": "2026-03-08T10:00:00Z",
  "read_at": "2026-03-08T11:00:00Z"
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

## 3.3 Workspaces and Members

### GET `/api/v1/workspaces`
- Auth: yes
- Response `200`: `Workspace[]`
- Security and scope:
    - Returns only workspaces where the authenticated user is a member.
    - Must not include workspaces from other users.
- Frontend usage:
    - Call this after login/refresh to load persisted workspace context.

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
- Validation:
    - `email` must be valid format
    - invitee email must already belong to a registered user
    - unregistered email returns `422 validation_failed`

### POST `/api/v1/workspace-invitations/{invitationID}/accept`
- Auth: yes
- Response `200`: `WorkspaceMember`

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
        ]
      }
    ]
  }
}
```

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

### POST `/api/v1/pages/{pageID}/comments`
- Auth: yes (workspace member, including `viewer`)
- Request:
```json
{ "body": "Please verify this section" }
```
- Response `201`: `PageComment`

### GET `/api/v1/pages/{pageID}/comments`
- Auth: yes (workspace member)
- Response `200`: `PageComment[]`

### POST `/api/v1/comments/{commentID}/resolve`
- Auth: yes (`owner|editor`)
- Response `200`: updated `PageComment`

## 3.8 Search

### GET `/api/v1/workspaces/{workspaceID}/search?q={query}`
- Auth: yes (workspace member)
- Query parameter:
    - `q` required and non-blank
- Response `200`:
```json
{ "data": [ { "id": "uuid", "workspace_id": "uuid", "folder_id": "uuid", "title": "Architecture", "updated_at": "..." } ] }
```

## 3.9 Trash

### GET `/api/v1/workspaces/{workspaceID}/trash`
- Auth: yes (workspace member)
- Response `200`: `TrashItem[]`

### POST `/api/v1/trash/{trashItemID}/restore`
- Auth: yes (`owner|editor`)
- Response `200`: restored `Page`

## 3.10 Notifications

### GET `/api/v1/notifications`
- Auth: yes
- Response `200`: `Notification[]` sorted newest-first

### POST `/api/v1/notifications/{notificationID}/read`
- Auth: yes (notification owner)
- Response `200`: updated `Notification` with `read_at`

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
