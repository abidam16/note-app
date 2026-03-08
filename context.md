# Project Context

## Latest Session Update
This section supersedes older references below if they differ.

Latest completed roadmap feature:
- 22. Trash delete and restore

Current next strict roadmap feature:
- 23. In-app notifications

New product semantics locked by the latest implementation:
- Page deletion is soft-delete in v1
- Soft-deleted pages are represented as trash items
- `viewer` cannot delete or restore pages
- Trash listing is workspace-scoped
- Restoring from trash reactivates the page and preserves revision history

New backend surface added:
- `DELETE /api/v1/pages/{pageID}`
- `GET /api/v1/workspaces/{workspaceID}/trash`
- `POST /api/v1/trash/{trashItemID}/restore`

Files added for feature 22:
- `migrations/000007_trash.up.sql`
- `migrations/000007_trash.down.sql`

Files updated materially for feature 22:
- `internal/domain/page.go`
- `internal/application/page_service.go`
- `internal/application/page_service_test.go`
- `internal/repository/postgres/page_repository.go`
- `internal/transport/http/server.go`
- `internal/transport/http/handlers.go`
- `internal/transport/http/server_test.go`

Verification status for the latest feature:
- automated tests passed with `go test ./...`
- local PostgreSQL migration applied successfully
- live local API verification passed for delete, trash list, restore, and revision retention

Resume from here:
- Do not repeat completed work through feature 22
- Continue backend-only with feature 23 notifications
- Keep updating both `context.md` and `docs/checkpoint.md` after each completed feature or before stopping mid-feature
## Purpose of This File
This file is the durable conversation and product context for this repository.
A fresh Codex session should read this file first, then read `AGENTS.md` and `docs/checkpoint.md`, and continue from the current state without re-discovering requirements.

## Product Idea
This project is a web-first note and document application with two core traits:
- rich structured editing similar in spirit to Confluence
- explicit document versioning and readable history comparison similar in spirit to Git

The product is not meant to be a generic markdown notes app. The core value is:
- teams write structured documents
- teams save meaningful versions manually
- teams compare document revisions clearly
- teams restore older content safely without losing history

## Product Direction and Scope Decisions
These decisions were clarified during the conversation and are currently locked unless the user explicitly changes them:
- Platform: web only
- First implementation phase: backend only
- Auth in v1: email/password
- Social login: later (`Google`, `Microsoft` deferred)
- Product model: workspace-based
- Collaboration in v1: async only, no real-time co-editing yet
- Roles in v1: `owner`, `editor`, `viewer`
- Sharing in v1: no public/external sharing
- Draft model: mutable auto-saved current draft
- Version model: immutable manual revisions
- Restore model: restoring history must preserve the audit trail by creating new state, not deleting old state
- Frontend work is blocked until backend reaches a stable API for core document/version flows

## Core Product Objects
The domain discussed so far is built around these objects:
- `User`
- `Workspace`
- `WorkspaceMember`
- `WorkspaceInvitation`
- `Folder`
- `Page`
- `PageDraft`
- `Revision`
- `Comment` (next planned feature)
- `Notification` (planned later)

## Roadmap Philosophy
The user explicitly does not want speculative implementation. Development must proceed feature-by-feature in strict order.
No overlapping feature development is allowed.
One feature should be completed, verified, and written into `docs/checkpoint.md` before the next one starts.

Current strict roadmap source of truth:
- `docs/backend-feature-roadmap.md`

## Development Governance Rules
The repository includes a root `AGENTS.md` that governs development behavior.
Important rules from the conversation and repo state:
- backend first
- one feature at a time
- no overlapping implementation
- clean architecture boundaries
- clean code and best practices
- use official Go documentation and official dependency documentation as primary references
- explain what was implemented and verified after each feature
- use migrations for schema changes
- stop and ask if requirements change domain behavior, auth rules, ownership rules, or revision semantics
- if the session is close to quota or must stop mid-feature, update `docs/checkpoint.md` and `context.md` immediately with completed and incomplete work

## Technical Architecture Chosen
Backend architecture was already selected and partially implemented.

### Language and Stack
- Language: `Go`
- HTTP: `net/http` + `chi`
- Logging: `slog`
- Config: environment-driven typed config loader
- DB access: `pgx` with explicit SQL repositories
- Migrations: `golang-migrate`
- Auth: `bcrypt` password hashing, JWT access tokens, persisted rotating refresh tokens
- Storage abstraction: local disk adapter for development
- Search plan for v1: PostgreSQL full-text search
- Service shape: modular monolith

### Code Boundaries
Current intended module boundaries:
- `internal/domain`
- `internal/application`
- `internal/transport/http`
- `internal/repository/postgres`
- `internal/infrastructure/auth`
- `internal/infrastructure/storage`

A dedicated revision boundary was introduced at feature 16 instead of overloading `PageService`, because the next roadmap items are revision listing, diff, and restore.
That separation should be preserved.

## Current Product Semantics
The following product semantics have already been decided and should not be changed casually:

### Draft vs Revision
- `PageDraft` is the mutable current working state.
- Drafts are auto-saved and overwritten.
- Draft updates do not create history entries.
- `Revision` is immutable and created explicitly by user action.
- Revision save copies from the current draft, not from arbitrary incoming payload.
- Revision creation does not mutate the current draft.
- Revision metadata currently supports optional `label` and optional `note`.
- Revision history is metadata-only and intentionally omits stored revision content.
- Revision history ordering is chronological ascending for now.
- Revision comparison currently uses a deterministic first-pass diff:
  - block statuses: `unchanged`, `modified`, `added`, `removed`
  - word-level inline chunks: `equal`, `added`, `removed`
- Revision restore updates the current draft and then creates a new revision event so history remains additive.
- The diff is designed to be readable and predictable first; it is not yet an advanced semantic editor diff.

### Permissions
- `owner` can manage membership roles and invitations.
- `editor` can create and modify content.
- `viewer` can read content, revision history, and revision diffs, but cannot mutate folders, pages, drafts, or revisions.
- Permissions are workspace-scoped in v1.
- No per-page ACLs yet.

### Content Model for Drafts
Feature 15 established a validated structured document format.
Draft content is validated in the application layer before persistence.
The root payload is a JSON array of blocks.

Supported block types in v1:
- `paragraph`
- `heading`
- `bullet_list`
- `numbered_list`
- `task_list`
- `quote`
- `code_block`
- `table`
- `image`

Supported mark types in v1:
- `bold`
- `italic`
- `inline_code`
- `link`

Important validation rules already agreed and implemented:
- text-bearing blocks use either `text` or `children`, but not both
- inline `children` may only contain `text` inline nodes
- unsupported block types are rejected
- malformed nesting is rejected
- `link` marks must have valid `http` or `https` URLs
- `image` blocks must have a non-empty `src`
- invalid structured content returns `422`
- revision save, revision history, diff, and restore all rely on the same validated document model

## What Has Been Implemented
The backend currently has implemented and verified features 1 through 19 from the roadmap.
That means the following are done:
- governance files
- project foundation
- database foundation
- response/error standardization
- registration
- sign-in
- refresh/logout
- workspace creation
- workspace invitation and acceptance
- workspace role updates
- folder creation/listing
- page creation/get
- page rename/move
- draft persistence
- structured content validation
- manual revision save
- revision history listing
- two-version diff
- revision restore

## Implemented HTTP Endpoints
These routes currently exist and have been verified in tests and/or live local API checks:
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

## Current Important Files
Core files to read in a fresh session:
- `AGENTS.md`
- `context.md`
- `docs/checkpoint.md`
- `docs/backend-feature-roadmap.md`

Most relevant implementation files so far:
- `cmd/api/main.go`
- `cmd/migrate/main.go`
- `internal/application/auth_service.go`
- `internal/application/workspace_service.go`
- `internal/application/folder_service.go`
- `internal/application/page_service.go`
- `internal/application/document_validator.go`
- `internal/application/revision_service.go`
- `internal/application/revision_diff.go`
- `internal/application/revision_restore.go`
- `internal/domain/revision.go`
- `internal/domain/revision_diff.go`
- `internal/repository/postgres/user_repository.go`
- `internal/repository/postgres/refresh_token_repository.go`
- `internal/repository/postgres/workspace_repository.go`
- `internal/repository/postgres/folder_repository.go`
- `internal/repository/postgres/page_repository.go`
- `internal/repository/postgres/revision_repository.go`
- `internal/transport/http/server.go`
- `internal/transport/http/handlers.go`
- `migrations/000001_init.up.sql`
- `migrations/000002_folders.up.sql`
- `migrations/000003_pages.up.sql`
- `migrations/000004_revisions.up.sql`

## Local Environment Context
This machine-specific context matters because it affected implementation and verification:
- local PostgreSQL is run through Docker Compose
- `docker-compose.yml` was adjusted to use `postgres:15`
- reason: the machine already had an older PostgreSQL 15 volume and `postgres:17` was incompatible with that volume
- current compose uses a fresh project-specific volume: `note_app_pg15_data`
- local PostgreSQL may still be running unless manually stopped
- the API server is usually stopped after live verification unless otherwise stated in `docs/checkpoint.md`

Local env vars used for API and migration commands:
- `POSTGRES_DSN=postgres://noteapp:noteapp@localhost:5432/noteapp?sslmode=disable`
- `JWT_SECRET=super-secret-token`
- `JWT_ISSUER=note-app`
- `ACCESS_TOKEN_TTL=15m`
- `REFRESH_TOKEN_TTL=168h`
- `LOCAL_STORAGE_PATH=./tmp/storage`

## Verification Standard
The working pattern used in this project is:
- implement one feature
- add or update tests
- run `go test ./...`
- if the feature touches runtime behavior meaningfully, verify it against the live local PostgreSQL-backed API
- update `docs/checkpoint.md`
- update `context.md`

This pattern should continue.

## Current Status at Time of Writing
The latest completed feature is:
- 19. Revision restore

The next strict roadmap feature is:
- 20. Page comments

That next feature should introduce:
- page-level comments only
- create/list/resolve comment endpoints
- viewer ability to create comments
- resolved-state persistence without deleting comment history

## How to Resume in a Fresh Session
A fresh Codex session should do this in order:
1. Read `context.md`
2. Read `AGENTS.md`
3. Read `docs/checkpoint.md`
4. Read `docs/backend-feature-roadmap.md`
5. Do not repeat completed work
6. Continue the next strict roadmap feature only
7. Keep backend-only development
8. Update `context.md` and `docs/checkpoint.md` after the feature is completed or if the session must stop mid-feature

## Recommended Resume Prompt
Use this prompt in a new session:

`Read context.md, AGENTS.md, and docs/checkpoint.md first. Continue from the current backend-only state without repeating completed work. Follow the strict one-feature-at-a-time roadmap in docs/backend-feature-roadmap.md. The current next feature is page comments. If the session must stop or approaches quota limits, update both context.md and docs/checkpoint.md with completed and incomplete work before stopping.`

## Relationship Between This File and Checkpoint
Use the files like this:
- `context.md`: durable project and conversation context
- `docs/checkpoint.md`: exact implementation status, verification state, and immediate next step
- `docs/backend-feature-roadmap.md`: canonical ordered feature list
- `AGENTS.md`: development behavior contract



