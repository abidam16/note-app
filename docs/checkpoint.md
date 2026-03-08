# Development Checkpoint

## Latest Update
This section supersedes older references below if they differ.

Current completed feature:
- 22. Trash delete and restore

Current next strict roadmap feature:
- 23. In-app notifications

What was completed in this session:
- Added trash operations to page service in `internal/application/page_service.go`
- Added soft-delete and trash persistence methods in `internal/repository/postgres/page_repository.go`
- Added trash model in `internal/domain/page.go`
- Added schema migration `migrations/000007_trash.up.sql`
- Wired trash endpoints into HTTP routes and handlers
- Added tests for trash endpoint flow and page service permissions

Implemented endpoints now include:
- `DELETE /api/v1/pages/{pageID}`
- `GET /api/v1/workspaces/{workspaceID}/trash`
- `POST /api/v1/trash/{trashItemID}/restore`

Implemented trash behavior:
- Delete is soft-delete only; page rows are not physically removed
- Soft delete inserts a trash record linked to the page and workspace
- `viewer` cannot delete or restore (`403`)
- Workspace members can list trash items
- Restore clears page deletion state and marks trash item restored
- Revision history remains intact across delete and restore

Verification completed for feature 22:
- `go test ./...` passed
- Migration `000007_trash` applied successfully to local PostgreSQL
- Live API verification passed with real DB:
  - viewer delete returned `403`
  - owner delete moved page to trash
  - trash listing returned one item
  - restore returned the original page
  - revision history remained available after restore

Local runtime state after verification:
- API server is stopped
- PostgreSQL container is still running

Exact next step:
- Implement feature 23: in-app notifications
- Add notification creation from invitations/comments and unread/read flows
## Current State
This checkpoint captures the backend progress so development can resume without re-discovery.

Completed roadmap features:
- 1. Repository governance files
- 2. Project foundation
- 3. Database and migration foundation
- 4. Error and response standardization
- 5. User registration
- 6. User sign-in
- 7. Token refresh and sign-out
- 8. Workspace creation
- 9. Workspace member invitation
- 10. Workspace role assignment
- 11. Folder creation
- 12. Page creation
- 13. Page rename and move
- 14. Draft persistence
- 15. Structured content validation
- 16. Manual revision save
- 17. Revision history listing
- 18. Two-version diff
- 19. Revision restore

Next strict roadmap feature:
- 20. Page comments

## Implemented Backend Capabilities
Working endpoints already implemented and verified:
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
- `PUT /api/v1/pages/{pageID}/draft`
- `POST /api/v1/pages/{pageID}/revisions`
- `GET /api/v1/pages/{pageID}/revisions`
- `GET /api/v1/pages/{pageID}/revisions/compare?from={id}&to={id}`
- `POST /api/v1/pages/{pageID}/revisions/{revisionID}/restore`

Implemented behavior:
- Email/password registration with password hashing
- JWT access token issuance
- Refresh token persistence and rotation
- Authenticated current-user resolution
- Workspace creation with owner bootstrap
- Invitation flow for workspace membership
- Role updates with last-owner protection
- Folder creation for `owner` and `editor`
- Folder listing for any workspace member
- Viewer blocked from folder creation with `403`
- Page creation for `owner` and `editor`
- Optional folder placement during page creation
- Folder/workspace validation on page creation
- Automatic initial draft creation with empty structured content `[]`
- Page retrieval returns both page metadata and current draft
- Viewer blocked from page creation with `403`
- Page rename via `PATCH /api/v1/pages/{pageID}`
- Page move into a folder via `PATCH /api/v1/pages/{pageID}`
- Page move back to workspace root with `folder_id: null`
- Viewer blocked from page updates with `403`
- Draft overwrite via `PUT /api/v1/pages/{pageID}/draft`
- Draft update allowed for `owner` and `editor` only
- Draft update does not create revisions
- Page retrieval reflects the latest draft content immediately
- Viewer blocked from draft updates with `403`
- Draft content is validated before persistence through a centralized application-layer validator
- Draft documents must be a JSON array of supported blocks
- Supported blocks in v1:
  - `paragraph`
  - `heading`
  - `bullet_list`
  - `numbered_list`
  - `task_list`
  - `quote`
  - `code_block`
  - `table`
  - `image`
- Supported marks in v1:
  - `bold`
  - `italic`
  - `inline_code`
  - `link`
- Text-bearing blocks accept either plain `text` or inline `children`, but not both at once
- Inline `children` only accept `text` nodes with supported marks
- Invalid block types, invalid inline nesting, malformed link URLs, and malformed image metadata are rejected with `422`
- Manual revision save exists as an explicit action through `POST /api/v1/pages/{pageID}/revisions`
- Revision save is allowed for `owner` and `editor` only
- Revision save reads from the current validated draft rather than from arbitrary request content
- Revision save persists immutable revision content with optional `label` and `note`
- Revision save does not mutate the current draft content
- Revision metadata currently includes: `id`, `page_id`, `label`, `note`, `created_by`, `created_at`
- Revision history is available through `GET /api/v1/pages/{pageID}/revisions`
- Revision history is readable by any workspace member who can access the page, including `viewer`
- Revision history is scoped to one page and returned in chronological ascending order
- Revision history responses intentionally omit revision content and return metadata only
- Two-version comparison is available through `GET /api/v1/pages/{pageID}/revisions/compare?from={id}&to={id}`
- Comparison requires both revisions to belong to the requested page
- Comparison is readable by any workspace member who can access the page
- Comparison returns deterministic block-level statuses: `unchanged`, `modified`, `added`, `removed`
- Modified text-bearing blocks include a word-level inline diff with `equal`, `added`, and `removed` chunks
- Revision restore is available through `POST /api/v1/pages/{pageID}/revisions/{revisionID}/restore`
- Restore requires the revision to belong to the requested page
- Restore is allowed for `owner` and `editor` only
- Restore overwrites the current draft with the restored revision content
- Restore creates a new revision event instead of deleting or collapsing prior history
- Restore responses return the restored draft plus the newly created revision summary
- The first diff implementation is intentionally simple and readable, not optimized for every edit pattern yet
- Validation is centralized in the application layer so revision-save, diff, and restore features reuse the same document rules

## Important Files
Core repo governance:
- `AGENTS.md`
- `context.md`
- `docs/backend-feature-roadmap.md`
- `docs/checkpoint.md`

Backend bootstrap and infrastructure:
- `cmd/api/main.go`
- `cmd/migrate/main.go`
- `internal/infrastructure/config/config.go`
- `internal/infrastructure/database/postgres.go`
- `docker-compose.yml`
- `.env.example`

Implemented domain/application slices:
- `internal/application/auth_service.go`
- `internal/application/workspace_service.go`
- `internal/application/folder_service.go`
- `internal/application/page_service.go`
- `internal/application/document_validator.go`
- `internal/application/revision_service.go`
- `internal/application/revision_diff.go`
- `internal/application/revision_restore.go`
- `internal/domain/user.go`
- `internal/domain/workspace.go`
- `internal/domain/folder.go`
- `internal/domain/page.go`
- `internal/domain/revision.go`
- `internal/domain/revision_diff.go`

HTTP/API layer:
- `internal/transport/http/server.go`
- `internal/transport/http/handlers.go`
- `internal/transport/http/response.go`
- `internal/transport/http/middleware/middleware.go`

Persistence and schema:
- `internal/repository/postgres/user_repository.go`
- `internal/repository/postgres/refresh_token_repository.go`
- `internal/repository/postgres/workspace_repository.go`
- `internal/repository/postgres/folder_repository.go`
- `internal/repository/postgres/page_repository.go`
- `internal/repository/postgres/revision_repository.go`
- `migrations/000001_init.up.sql`
- `migrations/000002_folders.up.sql`
- `migrations/000003_pages.up.sql`
- `migrations/000004_revisions.up.sql`

Tests already present:
- `internal/application/auth_service_test.go`
- `internal/application/workspace_service_test.go`
- `internal/application/folder_service_test.go`
- `internal/application/page_service_test.go`
- `internal/application/document_validator_test.go`
- `internal/application/revision_service_test.go`
- `internal/infrastructure/auth/token_test.go`
- `internal/infrastructure/config/config_test.go`
- `internal/transport/http/server_test.go`

## Verification Completed
Automated verification:
- `go test ./...` passed after revision restore implementation

Database verification completed:
- Local PostgreSQL container is running via Docker Compose
- Migrations `000001_init`, `000002_folders`, `000003_pages`, and `000004_revisions` applied successfully
- Verified schema tables include:
  - `users`
  - `refresh_tokens`
  - `workspaces`
  - `workspace_members`
  - `workspace_invitations`
  - `folders`
  - `pages`
  - `page_drafts`
  - `revisions`
  - `schema_migrations`

Live API verification completed:
- Auth flow verified end-to-end with real DB
- Workspace creation, invitation, acceptance, member listing, and role update verified end-to-end
- Folder creation and listing verified end-to-end with real DB
- Viewer folder creation attempt returned `403`
- Page creation in a folder verified end-to-end with real DB
- Page retrieval verified end-to-end with real DB
- Raw page payload confirmed draft content is `[]`
- Viewer page creation attempt returned `403`
- Page rename verified end-to-end with real DB
- Page move into a folder verified end-to-end with real DB
- Page move to workspace root verified end-to-end with real DB
- Viewer page update attempt returned `403`
- Draft update verified end-to-end with real DB
- Page retrieval after draft update returned the latest content
- Viewer draft update attempt returned `403`
- Structured valid draft content accepted end-to-end with real DB
- Unsupported block type rejected end-to-end with `422`
- Malformed link mark rejected end-to-end with `422`
- Malformed image block rejected end-to-end with `422`
- Manual revision save accepted end-to-end with real DB
- Revision metadata response included the saved `label` and `note`
- Direct PostgreSQL verification confirmed the revision row exists with expected page and metadata
- Page draft remained retrievable after revision creation
- Revision history listing accepted end-to-end with real DB
- Viewer access to revision history verified end-to-end with real DB
- Revision history ordering verified end-to-end as chronological ascending
- Revision history payload verified to omit `content`
- Two-version compare accepted end-to-end with real DB
- Viewer access to compare revisions verified end-to-end with real DB
- Compare payload verified block statuses `modified`, `unchanged`, and `added`
- Compare payload verified word-level inline diff chunks such as `equal:hello`, `added:brave`, `equal:world`
- Revision restore accepted end-to-end with real DB
- Direct PostgreSQL verification confirmed the page draft content was restored to the old revision value
- Revision history count increased after restore instead of shrinking
- Viewer restore remained blocked by permissions in automated tests

## Local Environment Notes
Important local setup details:
- Docker Compose was adjusted to use `postgres:15`
- Reason: an existing local PostgreSQL 15 volume on the machine was incompatible with `postgres:17`
- Current compose uses a fresh project-specific Docker volume: `note_app_pg15_data`
- PostgreSQL remains running locally unless you stop it manually
- The API server is currently stopped

Useful commands to resume:
- Start PostgreSQL: `docker compose up -d postgres`
- Check DB readiness: `docker exec note-app-postgres pg_isready -U noteapp -d noteapp`
- Run migrations: `go run ./cmd/migrate -direction up`
- Run tests: `go test ./...`
- Start API:
  - PowerShell env needed:
    - `POSTGRES_DSN=postgres://noteapp:noteapp@localhost:5432/noteapp?sslmode=disable`
    - `JWT_SECRET=super-secret-token`
    - `JWT_ISSUER=note-app`
    - `ACCESS_TOKEN_TTL=15m`
    - `REFRESH_TOKEN_TTL=168h`
    - `LOCAL_STORAGE_PATH=./tmp/storage`
  - Then run: `go run ./cmd/api`

## Known Notes
- `.gocache/` and `.gomodcache/` exist locally from earlier execution attempts and are not part of product work.
- `docker-compose.yml` is currently valid for this machine.
- No frontend work has started.
- If a future session is close to quota/session limits, update this file immediately with completed vs incomplete work before stopping.
- If a feature is only partially implemented in a future session, write exactly which files changed and which acceptance checks still failed before stopping.
- From this point forward, keep `context.md` and `docs/checkpoint.md` synchronized naturally after each completed feature.

## Exact Next Step
Resume with roadmap feature 20: `Page comments`.

What should be implemented next:
- Add page-level comments only
- Expose `POST /api/v1/pages/{pageID}/comments`
- Expose `GET /api/v1/pages/{pageID}/comments`
- Expose `POST /api/v1/comments/{commentID}/resolve`
- Allow `viewer` to create comments
- Preserve comment history when comments are resolved
- Add tests for create, list, resolve, viewer comment access, and resolved-state persistence

## Resume Prompt
If resuming in a new session, use this instruction:

"Read `context.md`, `AGENTS.md`, and `docs/checkpoint.md` first. Continue from the current backend-only state without repeating completed work. Follow the strict one-feature-at-a-time roadmap in `docs/backend-feature-roadmap.md`. The current next feature is page comments. If the session must stop or approaches quota limits, update both `context.md` and `docs/checkpoint.md` with completed and incomplete work before stopping."



