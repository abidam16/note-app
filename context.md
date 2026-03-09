# Project Context

## Latest Session Update
This section supersedes older references below if they differ.

Latest completed roadmap feature:
- 23. In-app notifications

Current next strict roadmap feature:
- None (backend roadmap features 1-23 are complete)

What was completed in this session (post-roadmap hardening):
- Added `GET /api/v1/workspaces` endpoint to list workspaces for the authenticated user only
- Added service method `WorkspaceService.ListWorkspaces`
- Added repository method `WorkspaceRepository.ListByUserID` with SQL join on `workspace_members.user_id`
- Kept workspace listing strictly user-scoped (no cross-user workspace leakage)
- Updated invitation rule: only registered users can be invited by email (`POST /api/v1/workspaces/{workspaceID}/invitations`)
- Added workspace-list coverage in:
  - `internal/application/workspace_service_additional_test.go`
  - `internal/transport/http/server_auth_workspace_test.go`
  - `internal/repository/postgres/user_workspace_refresh_repository_test.go`
- Enforced workspace creation name uniqueness for each authenticated user:
  - `POST /api/v1/workspaces` now rejects duplicate name per actor (case-insensitive, trim-aware)
  - duplicate workspace names now return `422 validation_failed`
  - updated frontend contract note in `frontend-repo/API_CONTRACT.md`

Verification from this session:
- `go test ./internal/application ./internal/transport/http` passed
- `go test ./internal/repository/postgres -run TestDoesNotExist` passed (compile check only)
- Full integration tests still require local PostgreSQL availability
- `go test ./internal/application ./internal/repository/postgres ./internal/transport/http` passed

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
- `GET /api/v1/workspaces`
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
