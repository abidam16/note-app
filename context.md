# Project Context

## Latest Session Update
This section supersedes older references below if they differ.

Latest completed roadmap feature:
- 23. In-app notifications

Latest completed post-roadmap backend feature:
- Workspace rename and folder rename
- Folder sibling-name uniqueness enforced for both create and rename
- Workspace and folder page listing

Current next strict roadmap feature:
- None (backend roadmap features 1-23 are complete)

What was completed in this session (post-roadmap hardening):
- Added `PATCH /api/v1/workspaces/{workspaceID}` endpoint for owner-only workspace rename
- Added `PATCH /api/v1/folders/{folderID}` endpoint for owner/editor folder rename
- Added service methods:
  - `WorkspaceService.RenameWorkspace`
  - `FolderService.RenameFolder`
- Added repository methods:
  - `WorkspaceRepository.GetByID`
  - `WorkspaceRepository.UpdateName`
  - `FolderRepository.HasSiblingWithName`
  - `FolderRepository.UpdateName`
- Enforced folder sibling-name uniqueness for both create and rename:
  - sibling scope is `(workspace_id, parent_id)`
  - root folders are siblings of other root folders
  - duplicate comparison is trim-aware and case-insensitive
- Added migration:
  - `000009_folder_sibling_uniqueness`
- Added migration rollout guard:
  - `go run ./cmd/migrate -preflight folder-sibling-uniqueness` reports duplicate sibling folder names before migration `000009`
  - migration `000009` now fails with a clear preflight message if duplicates still exist
- Added page listing endpoint:
  - `GET /api/v1/workspaces/{workspaceID}/pages`
- Added `PageService.ListPages`
- Added `PageRepository.ListByWorkspaceIDAndFolderID`
- Implemented page listing semantics:
  - omitted or blank `folder_id` lists workspace-root pages only
  - `folder_id={folderID}` lists direct pages in that folder only
  - listing excludes soft-deleted pages
  - listing is ordered by `updated_at DESC, id ASC`
- Documented frontend bootstrap pattern:
  - load folders and root pages in parallel
  - load folder pages on demand with `folder_id`
- Added explicit environment workflow support:
  - `go run ./cmd/api -env-file ...`
  - `go run ./cmd/migrate -env-file ...`
  - `.env.local.example`
  - `.env.test.example`
  - `.env.production.example`
  - `docs/environment-workflow.md`
  - single-command wrappers in `scripts/` for dev runtime, QA/test runtime, DB-backed tests, and production compose
- Updated PostgreSQL test DSN resolution:
  - DB-backed tests now prefer `.env.test` before `.env`
  - optional `TEST_ENV_FILE` override supported
- Local Docker PostgreSQL now creates `noteapp_test` on a fresh volume via init script
- Split Compose responsibilities:
  - `docker-compose.yml` is production-only
  - `docker-compose.local-db.yml` is for local dev/test PostgreSQL only
- Updated frontend/backend documentation:
  - `frontend-repo/API_CONTRACT.md`
  - `frontend-repo/CONTEXT.md`
  - `docs/checkpoint.md`
  - `docs/backend-feature-roadmap.md`

Verification from this session:
- `go test ./cmd/migrate` passed
- `go test ./internal/infrastructure/database -run TestFormatFolderSiblingUniquenessConflicts` passed
- `go test ./internal/application ./internal/transport/http` passed
- `go test ./internal/application` passed
- `go test ./internal/transport/http` passed
- `go test ./internal/repository/postgres -run TestDoesNotExist` passed (compile check only)
- `go test ./cmd/api ./cmd/migrate ./internal/infrastructure/config ./internal/testutil/testenv` passed
- `docker compose config` passed
- `docker compose -f docker-compose.local-db.yml config` passed
- `go test ./internal/repository/postgres` was not run because PostgreSQL-backed repository tests are destructive and should use an isolated test DSN

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
- `PATCH /api/v1/workspaces/{workspaceID}`
- `POST /api/v1/workspaces/{workspaceID}/invitations`
- `POST /api/v1/workspace-invitations/{invitationID}/accept`
- `GET /api/v1/workspaces/{workspaceID}/members`
- `PATCH /api/v1/workspaces/{workspaceID}/members/{memberID}/role`
- `POST /api/v1/workspaces/{workspaceID}/folders`
- `GET /api/v1/workspaces/{workspaceID}/folders`
- `PATCH /api/v1/folders/{folderID}`
- `POST /api/v1/workspaces/{workspaceID}/pages`
- `GET /api/v1/workspaces/{workspaceID}/pages`
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
- Production compose is `docker-compose.yml`
- Local dev/test PostgreSQL compose is `docker-compose.local-db.yml`
- Typical local commands:
  - `docker compose -f docker-compose.local-db.yml up -d postgres`
  - `go run ./cmd/migrate -env-file .env.local -direction up`
  - `go run ./cmd/migrate -env-file .env.test -direction up`
  - `go run ./cmd/migrate -env-file .env.production -preflight folder-sibling-uniqueness`
  - `go run ./cmd/migrate -env-file .env.production -direction up`
  - `go test ./...`
  - `go run ./cmd/api -env-file .env.local` on `8082`
  - `go run ./cmd/api -env-file .env.test` on `8081`
  - `./scripts/dev-up.ps1` or `./scripts/dev-up.sh`
  - `./scripts/qa-up.ps1` or `./scripts/qa-up.sh`
  - `./scripts/test-db.ps1` or `./scripts/test-db.sh`
  - `./scripts/test-all.ps1` or `./scripts/test-all.sh`
  - `docker compose --env-file .env.production up -d --build`
  - `./scripts/production-compose-up.ps1` or `./scripts/production-compose-up.sh`
  - see `docs/environment-workflow.md`
