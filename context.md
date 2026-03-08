# Project Context

## Latest Session Update
This section supersedes older references below if they differ.

Latest completed roadmap feature:
- 23. In-app notifications

Current next strict roadmap feature:
- None (backend roadmap features 1-23 are complete)

What was completed in this session (feature 23):
- Added notification domain model in `internal/domain/notification.go`
- Added notification repository in `internal/repository/postgres/notification_repository.go`
- Added notification application service in `internal/application/notification_service.go`
- Added notification event publisher contract in `internal/application/notification_events.go`
- Wired invitation and comment event publishing:
  - `internal/application/workspace_service.go`
  - `internal/application/comment_service.go`
- Added notification HTTP endpoints:
  - `GET /api/v1/notifications`
  - `POST /api/v1/notifications/{notificationID}/read`
- Wired notification service in `cmd/api/main.go` and `internal/transport/http/server.go`
- Added migration:
  - `migrations/000008_notifications.up.sql`
  - `migrations/000008_notifications.down.sql`
- Added tests:
  - `internal/application/notification_service_test.go`
  - `internal/application/notification_events_test.go`
  - Notification endpoint coverage in `internal/transport/http/server_test.go`

Notification semantics now implemented:
- Notifications are user-scoped and stored in `notifications`
- Invitation events create unread notifications for the invited user when the invited email already belongs to a registered user
- Comment events create unread notifications for workspace members except the comment author
- Notification creation is idempotent by `(user_id, type, event_id)`
- Read action is idempotent and sets `read_at` once

Verification from this session:
- `go test ./...` passed
- Note: test run required network access to fetch Go modules in this environment

Resume from here:
- Do not repeat completed work through feature 23
- Keep backend-only until the user explicitly starts frontend work
- Keep `context.md` and `docs/checkpoint.md` synchronized on every feature completion or mid-feature stop

## Purpose of This File
This file is the durable project context for fresh sessions.
A new session should read this file first, then `AGENTS.md` and `docs/checkpoint.md`.

## Locked Product Decisions
- Platform: web only
- Backend-first delivery
- Auth: email/password in v1
- Workspace model with roles: `owner`, `editor`, `viewer`
- Async collaboration in v1
- Mutable draft + immutable manual revisions
- Restore keeps additive revision history

## Roadmap State
Canonical roadmap: `docs/backend-feature-roadmap.md`

Backend roadmap status:
- Features 1-23 complete
- No remaining strict backend roadmap item

## Implemented Backend Endpoints
- `GET /healthz`
- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`
- `POST /api/v1/workspaces`
- `POST /api/v1/workspaces/{workspaceID}/invitations`
- `POST /api/v1/workspace-invitations/{invitationID}/accept`
- `GET /api/v1/workspaces/{workspaceID}/members`
- `PATCH /api/v1/workspaces/{workspaceID}/members/{memberID}/role`
- `POST /api/v1/workspaces/{workspaceID}/folders`
- `GET /api/v1/workspaces/{workspaceID}/folders`
- `POST /api/v1/workspaces/{workspaceID}/pages`
- `GET /api/v1/pages/{pageID}`
- `PATCH /api/v1/pages/{pageID}`
- `DELETE /api/v1/pages/{pageID}`
- `PUT /api/v1/pages/{pageID}/draft`
- `POST /api/v1/pages/{pageID}/revisions`
- `GET /api/v1/pages/{pageID}/revisions`
- `GET /api/v1/pages/{pageID}/revisions/compare?from={id}&to={id}`
- `POST /api/v1/pages/{pageID}/revisions/{revisionID}/restore`
- `POST /api/v1/pages/{pageID}/comments`
- `GET /api/v1/pages/{pageID}/comments`
- `POST /api/v1/comments/{commentID}/resolve`
- `GET /api/v1/workspaces/{workspaceID}/search?q=...`
- `GET /api/v1/workspaces/{workspaceID}/trash`
- `POST /api/v1/trash/{trashItemID}/restore`
- `GET /api/v1/notifications`
- `POST /api/v1/notifications/{notificationID}/read`

## Local Environment Notes
- Docker Compose uses `postgres:15` for local compatibility on this machine
- Typical local commands:
  - `docker compose up -d postgres`
  - `go run ./cmd/migrate -direction up`
  - `go test ./...`
  - `go run ./cmd/api`
