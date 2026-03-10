# Frontend Product Context (for new frontend repository)

## Purpose
This file gives a new frontend agent complete product and backend-integration context so UI/UX can be implemented correctly without re-discovery.

## Product Summary
Product: workspace-based collaborative document app.
Core value:
- Structured document editing (not plain markdown notes)
- Manual immutable revisions
- Readable compare and safe restore
- Async collaboration through comments and notifications

## Backend Maturity Update (post-refactor/enhancement)
Latest backend status:
- Backend roadmap is complete through feature 23.
- Search and notification flows were refactored/enhanced and are production-ready integration surfaces.
- Test quality target/milestone to assume for this frontend handoff: coverage above 90%.

What this means for frontend:
- Treat backend behavior as stable enough to build UX confidently.
- Prioritize product-quality interaction design over defensive workaround logic.
- Use integration tests for contract locking, not for guessing backend behavior.

## Product Scope (implemented backend)
Implemented domains:
- Auth (register/login/refresh/logout/me)
- Workspace and memberships (`owner`, `editor`, `viewer`)
- Invitations and acceptance
- Folders and pages
- Draft autosave model
- Manual revisions + history + compare + restore
- Page comments (create/list/resolve)
- Workspace search (title + body)
- Trash (soft delete + restore)
- In-app notifications (list + mark read)

## Roles and Permission UX Rules
Roles:
- `owner`
- `editor`
- `viewer`

UI behavior by role:
- `owner`: all editor capabilities + member role management and invitations
- `editor`: create/update folders/pages/drafts/revisions/comments resolve/delete/restore trash
- `viewer`: read content, revisions, compare, search, trash list, create comments, list comments, read/mark notifications; cannot mutate folders/pages/drafts/revisions/trash/resolve comment

Important UX requirement:
- Hide or disable forbidden actions preemptively in UI
- Still handle `403` responses gracefully because authorization is server-side truth

## Draft and Revision Mental Model
- Draft = mutable current state, overwritten on save
- Revision = immutable checkpoint, created explicitly
- Restore = draft becomes old content and a new revision event is created (history remains additive)

Frontend implication:
- Keep “Save revision” separate from autosave draft
- Show revision timeline as metadata list
- Compare view takes two revision IDs
- Restore must confirm action and then refresh draft + history

## Search and Notification Enhancements to Leverage
Search:
- Workspace-scoped query endpoint is stable and supports title/body matching
- Empty query is validation error (`422`), so UI should prevent blank submits
- Search responses are lightweight page result objects optimized for list rendering

Notifications:
- Notification feed is user-scoped
- Events currently include invitation and comment triggers
- Mark-read is idempotent
- Notification model supports unread/read UX without frontend-owned state hacks

## API Envelope Contract
Success envelope:
```json
{ "data": ... }
```

Error envelope:
```json
{
  "error": {
    "code": "...",
    "message": "...",
    "request_id": "..."
  }
}
```

Common error handling:
- `401`: token missing/expired/invalid
- `403`: role forbidden
- `404`: missing resource
- `409`: conflict (duplicate email, invitation conflicts, last-owner protection, etc.)
- `422`: validation failed

## Implemented Backend Endpoints
Health:
- `GET /healthz`

Auth:
- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`

Workspaces and members:
- `POST /api/v1/workspaces`
- `POST /api/v1/workspaces/{workspaceID}/invitations`
- `POST /api/v1/workspace-invitations/{invitationID}/accept`
- `GET /api/v1/workspaces/{workspaceID}/members`
- `PATCH /api/v1/workspaces/{workspaceID}/members/{memberID}/role`

Folders:
- `POST /api/v1/workspaces/{workspaceID}/folders`
- `GET /api/v1/workspaces/{workspaceID}/folders`

Pages and draft:
- `POST /api/v1/workspaces/{workspaceID}/pages`
- `GET /api/v1/pages/{pageID}`
- `PATCH /api/v1/pages/{pageID}`
- `PUT /api/v1/pages/{pageID}/draft`

Revisions:
- `POST /api/v1/pages/{pageID}/revisions`
- `GET /api/v1/pages/{pageID}/revisions`
- `GET /api/v1/pages/{pageID}/revisions/compare?from={id}&to={id}`
- `POST /api/v1/pages/{pageID}/revisions/{revisionID}/restore`

Comments:
- `POST /api/v1/pages/{pageID}/comments`
- `GET /api/v1/pages/{pageID}/comments`
- `POST /api/v1/comments/{commentID}/resolve`

Search:
- `GET /api/v1/workspaces/{workspaceID}/search?q=...`

Trash:
- `DELETE /api/v1/pages/{pageID}`
- `GET /api/v1/workspaces/{workspaceID}/trash`
- `POST /api/v1/trash/{trashItemID}/restore`

Notifications:
- `GET /api/v1/notifications`
- `POST /api/v1/notifications/{notificationID}/read`

## Auth and Session Integration
- Access token is JWT (short-lived)
- Refresh token is persisted + rotated by backend

Recommended frontend session design:
- Store access token in memory
- Store refresh token in secure storage strategy chosen by frontend app policy
- On `401` from protected API, attempt one refresh flow
- Retry original request once after successful refresh
- On refresh failure, clear session and send to sign-in

## Suggested Frontend Information Architecture
Primary areas:
- Authentication (register/login)
- Workspace switcher and creation
- Member/invitation management (owner)
- Folder/page tree and page editor surface
- Revision panel (save/list/compare/restore)
- Comments panel
- Search UI
- Trash panel
- Notification center

## UX Intent and Product Tone
- Professional, document-first, team-oriented
- Clear separation between editing now (draft) vs checkpointing (revision)
- Strong affordances for safety operations (restore revision, restore trash)
- Fast scan patterns for history, compare results, comments, search results, and notifications

## Known Backend Guarantees to Leverage
- Validation and authorization are strict server-side
- Compare output is deterministic and stable for rendering
- Search is workspace-scoped and deterministic in response shape
- Soft-delete preserves revision history
- Resolved comments are historical records, not removed
- Notifications preserve unread/read state in backend

## Frontend Agent Starting Checklist
1. Build a typed API client that unwraps `{data}` and normalizes `{error}`.
2. Implement auth/session with refresh retry logic.
3. Implement role-aware UI guards from membership role.
4. Implement page editor flow with autosave draft and explicit revision save.
5. Add compare/restore UI with confirmation and refresh of state.
6. Add comments, enhanced search UX, trash, and notifications surfaces.
7. Add end-to-end integration tests against this backend.

## Future Enhancement Direction (UI/UX)
Potential enhancements after parity:
- Better compare visualization and change navigation
- Notification grouping, filters, and contextual deep-links
- Rich keyboard workflows for editor/history
- Search result ranking/preview UX improvements
- Activity feed across comments/revisions/trash events
- Design-system hardening and accessibility audit
