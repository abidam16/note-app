# Task 1 Invitation State Schema Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce the invitation state-machine schema foundation so invitations can later support update, reject, and cancel, while keeping the current create and accept endpoints working without a breaking API change.

**Architecture:** This task is a compatibility-first schema migration. It adds the new invitation state columns, backfills existing rows, replaces the old pending uniqueness rule with a status-based rule, and updates repository persistence so the current create and accept flows continue to work against the new schema. Public invitation API shape should stay unchanged in this task; the new fields are internal foundation for later tasks.

**Tech Stack:** Go, PostgreSQL, `pgx`, SQL migrations, repository integration tests, Go unit tests

---

## 1. Scope

### In Scope
- Add invitation lifecycle columns to `workspace_invitations`
- Backfill existing rows safely
- Keep the current endpoints functioning:
  - `POST /api/v1/workspaces/{workspaceID}/invitations`
  - `POST /api/v1/workspace-invitations/{invitationID}/accept`
- Update repository SQL to read and write the new schema
- Keep the current public JSON contract stable for this task
- Add tests for migration-compatible behavior and repository correctness

### Out Of Scope
- No new endpoints
- No invitation list endpoints
- No invitation update endpoint
- No reject endpoint
- No cancel endpoint
- No notification or outbox work
- No frontend changes

---

## 2. Detailed Spec

## 2.1 Objective

Replace the current implicit invitation state model:
- `pending` if `accepted_at IS NULL`
- `accepted` if `accepted_at IS NOT NULL`

with an explicit state-machine foundation:
- `pending`
- `accepted`
- `rejected`
- `cancelled`

This task only introduces the schema and persistence foundation. It does not expose the new invitation state in public API responses yet.

## 2.2 Public API Impact

### New Endpoints
- None

### Existing Endpoints In Scope
- `POST /api/v1/workspaces/{workspaceID}/invitations`
- `POST /api/v1/workspace-invitations/{invitationID}/accept`

### Public Request Payload Changes
- None

### Public Response Payload Changes
- None intended for this task

### Public Validation Changes
- None intended at the transport layer

### Public Response Code Changes
- None intended

### Compatibility Rule
The current invitation endpoints must keep their current observable behavior. Internally, repository and domain code may carry the new state fields, but those fields must not become part of the public JSON contract in this task.

## 2.3 Data Model Spec

### Existing Table
- `workspace_invitations`

### New Columns
- `status TEXT`
- `version BIGINT`
- `updated_at TIMESTAMPTZ`
- `responded_by UUID NULL REFERENCES users(id) ON DELETE SET NULL`
- `responded_at TIMESTAMPTZ NULL`
- `cancelled_by UUID NULL REFERENCES users(id) ON DELETE SET NULL`
- `cancelled_at TIMESTAMPTZ NULL`

### Existing Column Kept For Compatibility
- `accepted_at TIMESTAMPTZ`

`accepted_at` stays in place for now so the current accept flow and current API contract can remain stable during the transition. It will be treated as a compatibility field:
- `status = 'accepted'` implies `accepted_at IS NOT NULL`
- any other status implies `accepted_at IS NULL`

### Status Rules
- `pending`
  - `accepted_at` null
  - `responded_by` null
  - `responded_at` null
  - `cancelled_by` null
  - `cancelled_at` null
- `accepted`
  - `accepted_at` non-null
  - `responded_by` non-null
  - `responded_at` non-null
  - `cancelled_by` null
  - `cancelled_at` null
- `rejected`
  - `accepted_at` null
  - `responded_by` non-null
  - `responded_at` non-null
  - `cancelled_by` null
  - `cancelled_at` null
- `cancelled`
  - `accepted_at` null
  - `responded_by` null
  - `responded_at` null
  - `cancelled_by` non-null
  - `cancelled_at` non-null

### Version Rules
- default initial version is `1`
- current create flow writes `version = 1`
- current accept flow increments version from `1` to `2`
- future update/reject/cancel work will reuse the same column

### Updated At Rules
- on create: `updated_at = created_at`
- on accept: `updated_at = accepted_at`
- for backfilled accepted rows: `updated_at = accepted_at`
- for backfilled pending rows: `updated_at = created_at`

### Pending Uniqueness Rule
Replace the current partial uniqueness strategy:
- old: unique `(workspace_id, email)` where `accepted_at IS NULL`

with the new rule:
- unique `(workspace_id, email)` where `status = 'pending'`

This is required because later `rejected` and `cancelled` invitations will also have `accepted_at IS NULL`.

## 2.4 Backfill Rules

For existing rows:
- if `accepted_at IS NULL`
  - `status = 'pending'`
  - `version = 1`
  - `updated_at = created_at`
  - `responded_by = NULL`
  - `responded_at = NULL`
  - `cancelled_by = NULL`
  - `cancelled_at = NULL`
- if `accepted_at IS NOT NULL`
  - `status = 'accepted'`
  - `version = 1`
  - `updated_at = accepted_at`
  - `responded_by = NULL`
  - `responded_at = accepted_at`
  - `cancelled_by = NULL`
  - `cancelled_at = NULL`

Note:
- `responded_by` cannot be backfilled safely for existing accepted rows because historical data does not record it.
- Therefore, do not add a strict database check requiring `responded_by` for accepted rows in this task.
- Application writes from this task onward should set `responded_by` on accept.

## 2.5 Repository Behavior After Task 1

### Create Invitation
Repository create must:
- insert `status = 'pending'`
- insert `version = 1`
- insert `updated_at = created_at`
- keep `accepted_at = NULL`

### Get Active Invitation By Email
Repository active lookup must:
- filter by `status = 'pending'`
- stop using `accepted_at IS NULL` as the active condition

### Get Invitation By ID
Repository read must:
- scan all new columns
- keep compatibility with current service usage

### Accept Invitation
Repository accept must:
- still lock the invitation row
- still create the membership in the same transaction
- set:
  - `status = 'accepted'`
  - `version = current_version + 1`
  - `updated_at = acceptedAt`
  - `accepted_at = acceptedAt`
  - `responded_by = userID`
  - `responded_at = acceptedAt`
- reject non-pending invitations as conflict

## 2.6 Domain Model Spec

Add:
- `type WorkspaceInvitationStatus string`
- status constants:
  - `pending`
  - `accepted`
  - `rejected`
  - `cancelled`

Update `domain.WorkspaceInvitation` to include the new fields, but do not expose them in JSON yet. Use `json:"-"` for the new internal-only fields in this task:
- `Status`
- `Version`
- `UpdatedAt`
- `RespondedBy`
- `RespondedAt`
- `CancelledBy`
- `CancelledAt`

Keep:
- `AcceptedAt` with current JSON tag so existing API responses remain stable

## 2.7 Negative And Positive Cases

### Public HTTP Cases
No new HTTP cases are introduced in this task.

Expected result:
- current invite and accept endpoints behave exactly as before
- current response codes remain unchanged

### Persistence Cases

Positive:
- migration succeeds on empty database
- migration succeeds on a database with pending and accepted invitations
- repository create stores pending invitation with version `1`
- repository active lookup returns only pending invitations
- repository accept transitions pending invitation to accepted and increments version

Negative:
- repository create still returns `ErrConflict` for duplicate pending invitation
- repository accept returns `ErrConflict` when invitation is already terminal
- repository accept returns `ErrNotFound` for missing invitation
- migration should fail if a data corruption case violates a new not-null or enum constraint during rollout

---

## 3. Files To Change

### Create
- `migrations/000019_invitation_state_schema.up.sql`
- `migrations/000019_invitation_state_schema.down.sql`

### Modify
- `internal/domain/workspace.go`
- `internal/repository/postgres/workspace_repository.go`
- `internal/repository/postgres/user_workspace_refresh_repository_test.go`
- `internal/repository/postgres/additional_integration_test.go`
- `internal/repository/postgres/closed_pool_errors_test.go`
- `internal/application/workspace_service_test.go`
- `internal/application/workspace_service_additional_test.go`
- `internal/transport/http/server_auth_workspace_test.go`

### Optional Documentation Updates
- none required for public API contract in this task if response shape remains unchanged
- roadmap does not need modification

### Files Explicitly Not In Scope
- `frontend-repo/API_CONTRACT.md`
- `docs/checkpoint.md`
- `internal/application/notification_service.go`
- `internal/transport/http/handlers.go`

---

## 4. Test Plan

## 4.1 Migration Tests

1. Apply migration on empty database
- Expected:
  - new columns exist
  - new partial unique index on `status = 'pending'` exists

2. Apply migration on database with one pending invitation
- Seed:
  - `accepted_at = NULL`
- Expected:
  - `status = 'pending'`
  - `version = 1`
  - `updated_at = created_at`
  - audit columns null

3. Apply migration on database with one accepted invitation
- Seed:
  - `accepted_at = <timestamp>`
- Expected:
  - `status = 'accepted'`
  - `version = 1`
  - `updated_at = accepted_at`
  - `responded_at = accepted_at`

## 4.2 Repository Integration Tests

4. Create invitation stores pending state
- Expected:
  - `status = pending`
  - `version = 1`
  - `updated_at = created_at`
  - `accepted_at = NULL`

5. Create invitation duplicate pending email returns conflict
- Expected:
  - `domain.ErrConflict`

6. Active invitation lookup returns pending invitation
- Seed:
  - one pending invitation
- Expected:
  - lookup succeeds

7. Active invitation lookup ignores accepted invitation
- Seed:
  - one accepted invitation only
- Expected:
  - `domain.ErrNotFound`

8. Get invitation by id scans new fields correctly
- Seed:
  - invitation with explicit new columns
- Expected:
  - struct contains scanned state/version/audit data

9. Accept invitation transitions to accepted
- Expected:
  - membership created
  - invitation status becomes `accepted`
  - version increments to `2`
  - `updated_at = acceptedAt`
  - `accepted_at = acceptedAt`
  - `responded_by = userID`
  - `responded_at = acceptedAt`

10. Accept invitation on already accepted invitation returns conflict
- Expected:
  - `domain.ErrConflict`

11. Accept invitation on missing invitation returns not found
- Expected:
  - `domain.ErrNotFound`

12. Historical accepted invitation does not block new pending invitation after uniqueness rule swap
- Seed:
  - one accepted invitation for same `(workspace_id, email)`
- Action:
  - create new invitation for same `(workspace_id, email)`
- Expected:
  - create succeeds

## 4.3 Service And HTTP Regression Tests

13. Existing invite service tests still pass
- Goal:
  - verify create flow is unchanged at behavior level

14. Existing accept service tests still pass
- Goal:
  - verify accept flow still returns current membership semantics

15. Existing invite HTTP tests still pass
- Goal:
  - verify current response shape and codes do not change

16. Existing accept HTTP tests still pass
- Goal:
  - verify current response shape and codes do not change

---

## 5. Execution Plan

### Task 1: Write migration spec and failing DB-backed tests

**Files:**
- Create: `migrations/000019_invitation_state_schema.up.sql`
- Create: `migrations/000019_invitation_state_schema.down.sql`
- Modify: `internal/repository/postgres/user_workspace_refresh_repository_test.go`
- Modify: `internal/repository/postgres/additional_integration_test.go`

- [ ] **Step 1: Add failing integration tests for new invitation state behavior**

Add DB-backed tests that prove:
- create writes pending state
- active lookup uses pending status
- accept updates state and version
- accepted historical rows do not block new pending invite

- [ ] **Step 2: Run targeted repository tests to confirm they fail**

Run:
```powershell
go test ./internal/repository/postgres -run "TestWorkspaceRepository|TestInvitation" -count=1
```

Expected:
- failures or compile errors because the new schema and scans do not exist yet

- [ ] **Step 3: Write the migration**

Implementation requirements:
- use migration number `000019`
- add new columns
- backfill existing rows
- replace old partial unique index
- keep `accepted_at`

Migration shape:
```sql
ALTER TABLE workspace_invitations
  ADD COLUMN status TEXT,
  ADD COLUMN version BIGINT,
  ADD COLUMN updated_at TIMESTAMPTZ,
  ADD COLUMN responded_by UUID REFERENCES users(id) ON DELETE SET NULL,
  ADD COLUMN responded_at TIMESTAMPTZ,
  ADD COLUMN cancelled_by UUID REFERENCES users(id) ON DELETE SET NULL,
  ADD COLUMN cancelled_at TIMESTAMPTZ;

UPDATE workspace_invitations
SET
  status = CASE WHEN accepted_at IS NULL THEN 'pending' ELSE 'accepted' END,
  version = 1,
  updated_at = COALESCE(accepted_at, created_at),
  responded_at = accepted_at;

ALTER TABLE workspace_invitations
  ALTER COLUMN status SET NOT NULL,
  ALTER COLUMN version SET NOT NULL,
  ALTER COLUMN updated_at SET NOT NULL;
```

Then:
- add `CHECK` for allowed status values
- drop old `workspace_invitations_active_email_idx`
- create new pending-only unique index on `status = 'pending'`

- [ ] **Step 4: Run targeted repository tests again**

Run:
```powershell
go test ./internal/repository/postgres -run "TestWorkspaceRepository|TestInvitation" -count=1
```

Expected:
- migration-related failures remain until repository scans are updated

- [ ] **Step 5: Commit**

```bash
git add migrations/000019_invitation_state_schema.up.sql migrations/000019_invitation_state_schema.down.sql internal/repository/postgres/user_workspace_refresh_repository_test.go internal/repository/postgres/additional_integration_test.go
git commit -m "test: add invitation state schema migration coverage"
```

### Task 2: Add domain state types and repository scan support

**Files:**
- Modify: `internal/domain/workspace.go`
- Modify: `internal/repository/postgres/workspace_repository.go`

- [ ] **Step 1: Add failing compile-level or unit assertions for new domain fields if needed**

Prefer small repository-focused assertions over broad refactors.

- [ ] **Step 2: Add new internal invitation state fields to the domain model**

Required additions:
- `WorkspaceInvitationStatus`
- status constants
- internal-only fields with `json:"-"`

Expected struct additions:
```go
type WorkspaceInvitationStatus string

const (
    WorkspaceInvitationStatusPending   WorkspaceInvitationStatus = "pending"
    WorkspaceInvitationStatusAccepted  WorkspaceInvitationStatus = "accepted"
    WorkspaceInvitationStatusRejected  WorkspaceInvitationStatus = "rejected"
    WorkspaceInvitationStatusCancelled WorkspaceInvitationStatus = "cancelled"
)
```

- [ ] **Step 3: Update repository SQL to read and write the new columns**

Required updates:
- `CreateInvitation`
- `GetActiveInvitationByEmail`
- `GetInvitationByID`
- `AcceptInvitation`
- `getInvitationForUpdate`

Specific behavior:
- create inserts pending state metadata
- active lookup filters `status = 'pending'`
- accept updates accepted state metadata and increments version

- [ ] **Step 4: Run targeted repository tests**

Run:
```powershell
go test ./internal/repository/postgres -run "TestWorkspaceRepository|TestInvitation" -count=1
```

Expected:
- repository tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/domain/workspace.go internal/repository/postgres/workspace_repository.go
git commit -m "feat: add invitation state persistence foundation"
```

### Task 3: Verify current service and HTTP behavior remain stable

**Files:**
- Modify only if compile fixes are required:
  - `internal/application/workspace_service.go`
  - `internal/application/workspace_service_test.go`
  - `internal/application/workspace_service_additional_test.go`
  - `internal/transport/http/server_auth_workspace_test.go`

- [ ] **Step 1: Run current workspace service and HTTP tests**

Run:
```powershell
go test ./internal/application -run "TestWorkspaceService|TestNotificationEvents" -count=1
go test ./internal/transport/http -run "TestInvite|TestAcceptInvitation" -count=1
```

Expected:
- either pass immediately, or fail due to now-missing assumptions about invitation shape

- [ ] **Step 2: Make compatibility-only fixes**

Allowed changes:
- compile fixes
- test expectation updates for internal-only fields
- repository-driven compatibility adjustments

Not allowed:
- changing public request/response contract
- adding new endpoint behavior early

- [ ] **Step 3: Re-run service and HTTP tests**

Run:
```powershell
go test ./internal/application -run "TestWorkspaceService|TestNotificationEvents" -count=1
go test ./internal/transport/http -run "TestInvite|TestAcceptInvitation" -count=1
```

Expected:
- all targeted tests pass

- [ ] **Step 4: Commit**

```bash
git add internal/application/workspace_service.go internal/application/workspace_service_test.go internal/application/workspace_service_additional_test.go internal/transport/http/server_auth_workspace_test.go
git commit -m "test: preserve invite and accept compatibility on invitation state schema"
```

### Task 4: Full verification for Task 1

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/repository/postgres -count=1
go test ./internal/application -run "TestWorkspaceService|TestNotificationEvents" -count=1
go test ./internal/transport/http -run "TestInvite|TestAcceptInvitation" -count=1
go test ./cmd/migrate -count=1
```

Expected:
- PASS for all commands

- [ ] **Step 2: Manual schema sanity check if local DB is available**

Check:
- pending invite row has status `pending`, version `1`
- accepted invite row has status `accepted`, version `2` after accept
- unique pending index allows historical accepted invite plus one new pending invite

- [ ] **Step 3: Commit verification or cleanup changes if any**

```bash
git add -A
git commit -m "chore: verify invitation state schema task"
```

---

## 6. Acceptance Criteria

Task 1 is complete only when all are true:
- migration `000019` applies cleanly
- existing invitation rows are backfilled correctly
- repository create writes pending-state metadata
- repository active lookup is status-based
- repository accept transitions to accepted state and increments version
- current invite and accept endpoints keep their current public behavior
- targeted repository, application, HTTP, and migrate tests pass

## 7. Risks And Guardrails

- Do not remove `accepted_at` yet. That cleanup belongs in a later task after all invitation endpoints move to the new explicit state model.
- Do not expose new invitation fields in JSON yet. Public contract changes belong in the later invitation endpoint tasks.
- Do not introduce reject or cancel semantics in this task.
- Do not change notification code in this task.

## 8. Follow-On Task Dependency

This plan intentionally prepares for:
- Task 2 `POST /api/v1/workspaces/{workspaceID}/invitations`
- Task 5 `PATCH /api/v1/workspace-invitations/{invitationID}`
- Task 6 `POST /api/v1/workspace-invitations/{invitationID}/accept`
- Task 7 `POST /api/v1/workspace-invitations/{invitationID}/reject`
- Task 8 `POST /api/v1/workspace-invitations/{invitationID}/cancel`

Those tasks should assume Task 1 has already landed and should not rework the database foundation.
