# Task 29 Invite-Create Eligibility And Error-Outcome Alignment Plan

## 1. Task Summary

Align `POST /api/v1/workspaces/{workspaceID}/invitations` with the updated PRD so invite creation allows only registered accounts, rejects self-invite explicitly, preserves existing-member and duplicate-pending rejection, and returns client-distinguishable invalid outcomes.

## 2. Objective

After this task, invite-create behavior, tests, and frontend-facing contract docs all agree on the PRD target for invite eligibility and invalid-case outcomes, without pulling in the separate no-workspace bootstrap/onboarding task.

## 3. Scope

### In Scope

- Update the application-layer invite-create validation path in `WorkspaceService.InviteMember`
- Reject unregistered invite target email
- Reject actor self-email
- Preserve existing rejection for current workspace member and duplicate pending invite
- Introduce transport-visible, client-distinguishable outcomes for:
  - unregistered email
  - self email
  - existing workspace member
  - duplicate pending invite
- Update endpoint-focused service and HTTP tests
- Update frontend-facing contract docs so they describe the new runtime truth instead of current/target drift
- Update `docs/checkpoint.md` for the completed slice

### Out Of Scope

- Task 30 no-workspace invitation-first entry contract or bootstrap changes
- Outbound invitation delivery or email sending
- Ownership-transfer or invitation-role policy changes
- Invitation list, update, accept, reject, or cancel endpoint behavior
- Notification projector, inbox, SSE, or unread-count changes
- Broad error-taxonomy cleanup outside invite-create

## 4. Detailed Specification

### 4.1 Endpoint Boundary

Task scope is limited to:

- `POST /api/v1/workspaces/{workspaceID}/invitations`

Existing request and success response shape stay intact:

```json
{ "email": "invitee@example.com", "role": "viewer" }
```

Successful responses still return `201` with a pending `WorkspaceInvitation` whose:

- `status = pending`
- `version = 1`
- `updated_at = created_at`

### 4.2 Validation Rules

Invite-create must enforce these rules in order:

1. Actor must be authenticated
2. Actor must be a workspace member
3. Actor must be an `owner`
4. `role` must be valid
5. `email` must be valid format and normalized before checks
6. If normalized target email equals the authenticated actor's email, reject as self-invite
7. If no registered user exists for the normalized email, reject as unregistered target
8. If the registered user is already a workspace member, reject as existing member
9. If a pending invitation already exists for the same `(workspace_id, normalized_email)`, reject as duplicate pending invite

### 4.3 Distinguishable Error Outcomes

This task assumes the smallest implementation that satisfies the PRD:

- keep business-rule invalid cases in the `409` family to avoid broad status churn
- make them client-distinguishable via explicit API error codes and stable messages

Recommended outcome contract:

- self invite:
  - status: `409`
  - code: `invitation_self_email`
- unregistered target:
  - status: `409`
  - code: `invitation_target_unregistered`
- existing workspace member:
  - status: `409`
  - code: `invitation_existing_member`
- duplicate pending invite:
  - status: `409`
  - code: `invitation_duplicate_pending`

If implementation finds a materially better fit within existing patterns, it may choose different conflict-code names, but the final contract must still provide four stable, client-distinguishable outcomes and update docs/tests accordingly.

### 4.4 Error-Layer Design Constraint

Follow the existing layer boundaries:

- domain/application decides which invalid condition occurred
- transport maps that condition into the public API error code/message

Do not move product validation into HTTP handlers.

The expected implementation pattern is:

- add invite-create-specific domain sentinel errors
- return those sentinels from `WorkspaceService.InviteMember`
- map them in transport without changing unrelated endpoint behavior

### 4.5 Data / Repository Behavior

No schema change is expected.

Repository use should remain bounded to existing capabilities:

- `users.GetByEmail`
- `workspaces.GetMembershipByUserID`
- `workspaces.GetActiveInvitationByEmail`
- `workspaces.CreateInvitation`

Do not change invitation status/version semantics in this task.

### 4.6 Documentation Behavior

After implementation, the contract docs must stop describing current-versus-target drift for invite-create and instead describe the actual runtime truth.

Update:

- `frontend-repo/API_CONTRACT.md`
- `frontend-repo/API_DELTA_NOTIFICATION_INVITATION.md`

Task 30 notes about invitation-first entry may remain, but no new bootstrap contract should be invented here.

## 5. Files / Components To Change

### Expected Changes

- `internal/domain/errors.go`
- `internal/application/workspace_service.go`
- `internal/application/workspace_service_additional_test.go`
- `internal/application/notification_events_test.go`
- `internal/transport/http/server.go`
- `internal/transport/http/server_auth_workspace_test.go`
- `frontend-repo/API_CONTRACT.md`
- `frontend-repo/API_DELTA_NOTIFICATION_INVITATION.md`
- `docs/checkpoint.md`

### Must Not Change

- `internal/repository/postgres/workspace_repository.go`
  - unless implementation proves a missing read helper blocks the task
- invitation accept/reject/cancel/update behavior
- notification projector or outbox code
- auth bootstrap endpoints such as `/api/v1/auth/me`
- roadmap or PRD content, unless a contradiction is discovered during implementation

## 6. Validation And Test

### Validation

Verify all of the following:

- owner inviting their own email fails with the self-invite outcome
- owner inviting an unregistered email fails with the unregistered-target outcome
- owner inviting an existing workspace member fails with the existing-member outcome
- owner inviting a duplicate pending invitation fails with the duplicate-pending outcome
- owner inviting a registered non-member still succeeds with `201`
- success response still returns pending invitation state fields unchanged
- non-owner, invalid role, invalid email, and invalid JSON behavior do not regress
- docs describe the implemented contract exactly

### Tests

Add or update service tests in:

- `internal/application/workspace_service_additional_test.go`

Required service coverage:

- registered non-member invite succeeds
- self-invite returns the new self sentinel
- unregistered target returns the new unregistered sentinel
- existing member returns the new existing-member sentinel
- duplicate pending invite returns the new duplicate sentinel

Add or update HTTP tests in:

- `internal/transport/http/server_auth_workspace_test.go`

Required HTTP coverage:

- registered non-member invite returns `201`
- self-invite returns the expected `409` error code
- unregistered target returns the expected `409` error code
- existing member returns the expected `409` error code
- duplicate pending invite returns the expected `409` error code
- existing invalid role/email JSON cases still map correctly

Run at minimum:

```powershell
go test ./internal/application -run "TestWorkspaceService|TestNotificationEvents" -count=1
go test ./internal/transport/http -run "Test.*Invite|TestAcceptInvitation" -count=1
```

If invite-create test fakes rely on old generic conflict assumptions, update only the minimum fake behavior needed for this endpoint slice.

## 7. Review Checklist

- The task changes only invite-create behavior and its direct docs/tests.
- `WorkspaceService.InviteMember` enforces registered-only and self-invite rejection.
- Invalid invite-create outcomes are client-distinguishable without relying on handler-local validation.
- Existing member and duplicate pending rules still work.
- Success response shape and invitation lifecycle state semantics did not change.
- No unrelated notification, bootstrap, or invitation-transition behavior was modified.
- Contract docs match the implemented runtime truth.
- Verification commands were run and their scope is appropriate to the change.

## 8. Trade-Offs And Risks

- Distinct unregistered-target outcomes expose registration state to authorized owners. This is now a product requirement, but the change should remain tightly scoped to invite-create and not spread to unrelated auth flows.
- Adding new domain sentinel errors increases API-mapping surface area; keep the additions specific to this endpoint rather than creating a generic error-taxonomy refactor.
- If transport mapping is implemented too generically, other conflict paths may accidentally inherit the new codes. Keep mapping precise and review route-level impact carefully.
- The current docs already mention Task 30 invitation-first entry behavior; do not let that pull onboarding/bootstrap changes into this task.

## 9. Future Improvements

- Task 30 can decide whether no-workspace invitation-first routing needs a dedicated bootstrap surface or should stay based on `GET /api/v1/workspaces` plus `GET /api/v1/my/invitations`.
- A later hardening pass can revisit whether invite-create should distinguish invalid cases with different HTTP statuses instead of shared `409` responses.
- If the product later reconsiders registration-state exposure, the PRD and contract may need another update before changing this endpoint again.

## Plan Handoff

- artifact_type: `PLAN`
- artifact_status: `DRAFT`
- decision: `PROCEED_TO_IMPLEMENTATION`
- task_summary: invite-create eligibility and error-outcome alignment
- objective: align invite-create runtime behavior, tests, and docs with PRD target while keeping scope limited to the create endpoint
- in_scope:
  - registered-only targeting
  - self-invite rejection
  - distinct invite-create conflict outcomes
  - direct docs/tests updates
- out_of_scope:
  - Task 30 bootstrap/onboarding routing changes
  - invitation delivery
  - other invitation lifecycle endpoints
- expected_changes:
  - domain errors
  - `WorkspaceService.InviteMember`
  - invite-create HTTP mapping/tests
  - frontend-facing API contract docs
- must_not_change:
  - repository schema
  - notification architecture
  - accept/reject/cancel/update invitation flows
- validation_requirements:
  - targeted application and HTTP test coverage for all four invalid outcomes
  - docs updated to runtime truth
- test_requirements:
  - `go test ./internal/application -run "TestWorkspaceService|TestNotificationEvents" -count=1`
  - `go test ./internal/transport/http -run "Test.*Invite|TestAcceptInvitation" -count=1`
- review_checkpoints:
  - precise scope
  - transport-visible distinct outcomes
  - no bootstrap creep
- tradeoffs_and_risks:
  - registration-state exposure to owners
  - mapping bleed into unrelated conflicts
- future_improvements:
  - Task 30 bootstrap contract
  - later status-code reconsideration
- source_artifacts:
  - `PRD.md`
  - `docs/invitation-notification-thread-roadmap.md`
  - `frontend-repo/API_CONTRACT.md`
  - `internal/application/workspace_service.go`
  - `internal/transport/http/server.go`
- next_step: implement this plan as one bounded task
