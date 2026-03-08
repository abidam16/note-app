# Development Checkpoint

## Latest Update
This section supersedes older references below if they differ.

Current completed feature:
- 23. In-app notifications

Current next strict roadmap feature:
- None (backend roadmap complete through feature 23)

What was completed in this session:
- Added notification schema migration:
  - `migrations/000008_notifications.up.sql`
  - `migrations/000008_notifications.down.sql`
- Added notification domain model:
  - `internal/domain/notification.go`
- Added PostgreSQL notification repository:
  - `internal/repository/postgres/notification_repository.go`
- Added notification application service and event publisher contract:
  - `internal/application/notification_service.go`
  - `internal/application/notification_events.go`
- Integrated notification publishing into existing flows:
  - Invitation events via `internal/application/workspace_service.go`
  - Comment events via `internal/application/comment_service.go`
- Added notification transport wiring and handlers:
  - `internal/transport/http/server.go`
  - `internal/transport/http/handlers.go`
- Wired dependencies in startup:
  - `cmd/api/main.go`
- Added/updated tests:
  - `internal/application/notification_service_test.go`
  - `internal/application/notification_events_test.go`
  - `internal/transport/http/server_test.go`

Implemented endpoints now include:
- `GET /api/v1/notifications`
- `POST /api/v1/notifications/{notificationID}/read`

Implemented notification behavior:
- Notifications are scoped to one user (`user_id`)
- Invitation events create unread notifications for invited users when the invited email exists in `users`
- Comment events create unread notifications for workspace members except the comment author
- Duplicate event notifications are prevented by unique key `(user_id, type, event_id)`
- Mark-read is idempotent via `read_at = COALESCE(read_at, now)`

Verification completed for feature 23:
- `go test ./...` passed
- Note: in this environment, tests required network-enabled module download before passing

Local runtime state after verification:
- API server status: not started by this session
- PostgreSQL container status: unchanged by this session

## Current State
Completed roadmap features:
- 1 through 23 (all backend roadmap features)

Backend status:
- Backend roadmap implementation complete
- No frontend work started
- No additional strict backend feature remains in `docs/backend-feature-roadmap.md`

## Exact Next Step
- Wait for user direction for post-roadmap work (for example: hardening, integration checks, frontend start, or new scoped feature additions)

## Resume Prompt
If resuming in a new session, use this instruction:

"Read `context.md`, `AGENTS.md`, and `docs/checkpoint.md` first. Continue from the current state without repeating completed work. Treat backend roadmap features as complete through feature 23 unless I explicitly add new scope." 
