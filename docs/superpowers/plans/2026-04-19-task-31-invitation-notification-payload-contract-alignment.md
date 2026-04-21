# Task 31 Invitation Notification Payload Contract Alignment Plan

## 1. Task Summary

Align invitation notification rows with the documented inbox contract so `GET /api/v1/notifications` and `POST /api/v1/notifications/{notificationID}/read` return populated invitation payload fields for actionable invitation rows.

## 2. Objective

After this task, invitation notification rows produced by the legacy direct create path, projector replay, and reconciliation all converge on the same payload shape and actionability semantics, while `workspace_invitations` remains the canonical source of truth for invitation state and mutation authority.

## 3. Scope

### In Scope

- Fix invitation notification payload population for invitation rows returned by:
  - `GET /api/v1/notifications`
  - `POST /api/v1/notifications/{notificationID}/read`
- Align the legacy direct invitation notification write in `NotificationService.NotifyInvitationCreated`
- Preserve and reuse the existing invitation live-notification semantics:
  - one live row per invitation
  - read-state preservation across live updates
  - unread-counter correctness
- Converge direct create, projector, and reconciliation paths on one invitation notification payload contract
- Add regression coverage for:
  - direct create-path payload content
  - projector payload and terminal-state mapping
  - repository live-row upsert payload persistence
  - inbox list and mark-read endpoint payload visibility
- Update frontend-facing contract docs to describe the implemented runtime truth
- Update `docs/checkpoint.md` after implementation

### Out Of Scope

- Changing invitation accept or reject endpoint behavior
- Replacing `GET /api/v1/my/invitations` as the canonical invitation review surface
- Adding new notification action kinds beyond `invitation_response`
- Redesigning inbox UX, notification grouping, or client routing behavior
- Migrating invitation delivery fully to always-on worker processing
- Broader notification architecture changes outside invitation payload alignment

## 4. Detailed Specification

### 4.1 Contract Boundary

Task scope is limited to invitation notification rows in the existing notification inbox contract.

For invitation rows, both:

- `GET /api/v1/notifications`
- `POST /api/v1/notifications/{notificationID}/read`

must return `payload` with these minimum fields:

```json
{
  "invitation_id": "uuid",
  "workspace_id": "uuid",
  "email": "invitee@example.com",
  "role": "viewer",
  "status": "pending",
  "version": 1,
  "can_accept": true,
  "can_reject": true
}
```

### 4.2 Source-Of-Truth Rule

This task must preserve the existing product and architecture boundary:

- `workspace_invitations` remains the authority for invitation lifecycle and optimistic-concurrency version
- notification rows remain a derived convenience surface
- inline inbox accept or reject actions continue to call invitation endpoints using payload-derived inputs

Do not shift invitation authority into the notification row or notification API envelope.

### 4.3 Preferred Fix Shape

Use the smallest fix that removes payload drift at the notification mapping layer, not an endpoint-only patch.

Preferred implementation direction:

- reuse or extract one shared invitation-notification builder inside `internal/application`
- have the direct create path build invitation notifications from the same mapping rules already used by projector or reconciliation logic
- keep repository list and mark-read paths as pass-through reads of the stored inbox row shape

Do not solve this by hydrating invitation payload only inside HTTP handlers or only during response serialization. That would leave stored rows, replay, and reconciliation behavior inconsistent.

### 4.4 Actionability Semantics

The task must preserve these rules:

- pending invitation notifications:
  - `actionable = true`
  - `action_kind = invitation_response`
  - `payload.can_accept = true`
  - `payload.can_reject = true`
- accepted, rejected, and cancelled invitation notifications:
  - `actionable = false`
  - `action_kind = null`
  - `payload.can_accept = false`
  - `payload.can_reject = false`

`actionable`, `action_kind`, and payload booleans must never contradict each other.

### 4.5 Identity And Read-State Preservation

The task must preserve existing live invitation notification behavior:

- one row per `(user_id, invitation_id)` identity
- repeated projector or reconciliation runs remain idempotent
- existing read state stays preserved when the live invitation row updates from pending to terminal state
- unread counters must not increment again on invitation live-row updates

Do not reset `is_read`, overwrite an existing `read_at`, or create duplicate invitation rows while fixing payload shape.

### 4.6 Documentation Behavior

After implementation:

- `frontend-repo/API_CONTRACT.md` must continue to document invitation notification payloads as canonical inbox response fields
- `frontend-repo/API_DELTA_NOTIFICATION_INVITATION.md` must explicitly reflect that notification payloads are usable for inline invitation actions, while `My invitations` remains canonical for invitation-review authority
- `docs/checkpoint.md` must record Task 31 completion and the aligned runtime behavior

## 5. Files / Components To Change

### Expected Changes

- `internal/application/notification_service.go`
- `internal/application/notification_service_test.go`
- `internal/application/notification_service_additional_test.go`
- `internal/application/invitation_notification_projector.go`
- `internal/application/invitation_notification_projector_test.go`
- `internal/application/notification_reconciliation.go`
- `internal/application/notification_reconciliation_test.go`
- `internal/repository/postgres/content_repository_test.go`
- `internal/repository/postgres/notification_replay_idempotency_test.go`
- `internal/transport/http/server_test.go`
- `frontend-repo/API_CONTRACT.md`
- `frontend-repo/API_DELTA_NOTIFICATION_INVITATION.md`
- `docs/checkpoint.md`

### Must Not Change

- `internal/application/workspace_service.go`
  - unless implementation proves the invite-create call site needs a minimal signature-compatible adjustment
- invitation accept, reject, cancel, or update business rules
- repository mark-read semantics unrelated to payload preservation
- SSE stream behavior
- roadmap, PRD, or ADR content unless a contradiction is discovered

## 6. Validation And Test

### Validation

Verify all of the following:

- create-path invitation notifications no longer persist `payload = {}`
- inbox list returns populated invitation payload fields for pending invitation rows
- mark-read returns the same populated invitation payload fields
- accepted, rejected, and cancelled invitation rows remain non-actionable and carry terminal payload booleans
- invitation live-row updates keep one-row identity and preserve pre-existing read state
- replay and reconciliation do not reintroduce empty invitation payloads
- docs describe the runtime truth precisely:
  - notifications are usable as a convenience action surface
  - `My invitations` remains the canonical invitation-review surface

### Tests

Add or update application tests in:

- `internal/application/notification_service_test.go`
- `internal/application/notification_service_additional_test.go`
- `internal/application/invitation_notification_projector_test.go`
- `internal/application/notification_reconciliation_test.go`

Required application coverage:

- `NotifyInvitationCreated` writes a populated invitation payload instead of `{}` for pending invitations
- projector mapping returns the documented payload fields for pending and terminal invitation states
- reconciliation-generated invitation notifications use the same payload contract
- helper reuse, if introduced, keeps payload and actionability rules consistent across paths

Add or update repository integration tests in:

- `internal/repository/postgres/content_repository_test.go`
- `internal/repository/postgres/notification_replay_idempotency_test.go`

Required repository coverage:

- live invitation upsert stores the full documented payload fields
- mark-read returns the same stored invitation payload unchanged except for read-state fields
- replay or repeat live-upsert preserves read-state and does not regress payload shape

Add or update HTTP tests in:

- `internal/transport/http/server_test.go`

Required HTTP coverage:

- list endpoint returns populated invitation payload fields
- mark-read endpoint returns populated invitation payload fields
- pending invitation notification row is actionable and includes `can_accept = true` and `can_reject = true`
- terminal invitation notification row is non-actionable and includes `can_accept = false` and `can_reject = false`

Run at minimum:

```powershell
go test ./internal/application -run "TestNotificationService|TestInvitationNotificationProjector|TestNotificationReconciliationServiceRun|TestNotificationProjectionConcurrencyInvitationReplayPreservesReadState" -count=1
go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestNotificationReplayIdempotencyInvitationLiveReadState|TestNotificationReconciliationRepositoryIntegration" -count=1
go test ./internal/transport/http -run "TestNotificationEndpoints" -count=1
```

If the implementation introduces a shared helper used by multiple application files, extend only the minimum focused tests needed to prove all three paths now agree.

## 7. Review Checklist

- The task changes invitation notification payload alignment only and does not alter invitation authority.
- Direct create, projector, and reconciliation all use one consistent invitation notification payload contract.
- Pending invitation rows are actionable and terminal invitation rows are not.
- List and mark-read responses expose the same invitation payload shape.
- Existing read-state preservation and one-live-row semantics remain intact.
- No endpoint-only payload patching was used to hide underlying stored-row drift.
- Docs match the implemented runtime behavior and preserve the convenience-vs-authority distinction.
- Verification commands were run and their scope matches the touched paths.

## 8. Trade-Offs And Risks

- The architecture still has split invitation delivery paths. This task should align their payload mapping, not try to retire that split in the same slice.
- If helper extraction is too broad, the implementation can drift into an unnecessary notification refactor. Keep reuse narrow and invitation-specific.
- If only the direct create path is fixed, reconciliation or replay may still emit a different payload contract later. Coverage must explicitly guard against this.
- If payload content is reconstructed from stale assumptions rather than current invitation state rules, actionability may drift from invitation status. Keep state-derived booleans explicit and tested.

## 9. Future Improvements

- A later roadmap slice can decide whether invitation notifications should fully leave the synchronous create path and rely solely on projector-driven live updates.
- A later UX slice can expand notification actions beyond invitations if the PRD and architecture explicitly choose that direction.
- If the product later wants stronger no-workspace entry guarantees, bootstrap-oriented invitation hints can be revisited separately from this task.

## Plan Handoff

- artifact_type: `PLAN`
- artifact_status: `DRAFT`
- decision: `PROCEED_TO_IMPLEMENTATION`
- task_summary: invitation notification payload contract alignment
- objective: align invitation notification payloads across direct create, projection, replay, and endpoint reads without changing invitation source-of-truth ownership
- in_scope:
  - populated invitation inbox payloads
  - consistent actionability semantics
  - regression coverage across application, repository, and HTTP layers
  - direct docs/checkpoint updates
- out_of_scope:
  - invitation endpoint redesign
  - inbox UX redesign
  - full notification architecture migration
- expected_changes:
  - notification service invitation create path
  - invitation projector and reconciliation mapping reuse
  - repository and endpoint regression tests
  - frontend-facing notification contract docs
- must_not_change:
  - invitation lifecycle authority
  - accept/reject endpoint contracts
  - SSE and unrelated notification types
- validation_requirements:
  - payload correctness for list and mark-read
  - pending vs terminal actionability consistency
  - replay and reconciliation consistency
- test_requirements:
  - `go test ./internal/application -run "TestNotificationService|TestInvitationNotificationProjector|TestNotificationReconciliationServiceRun|TestNotificationProjectionConcurrencyInvitationReplayPreservesReadState" -count=1`
  - `go test ./internal/repository/postgres -run "TestRevisionCommentNotificationRepositoriesIntegration|TestNotificationReplayIdempotencyInvitationLiveReadState|TestNotificationReconciliationRepositoryIntegration" -count=1`
  - `go test ./internal/transport/http -run "TestNotificationEndpoints" -count=1`
- review_checkpoints:
  - no authority drift
  - no endpoint-only patching
  - no read-state regression
- tradeoffs_and_risks:
  - split delivery-path drift
  - over-broad helper extraction
  - replay/reconciliation mismatch
- future_improvements:
  - future migration away from synchronous create path
  - later UX expansion
- source_artifacts:
  - `PRD.md`
  - `ARCHITECTURE.md`
  - `docs/invitation-notification-thread-roadmap.md`
  - `docs/user_report/2026-04-19-invitation-notification-payload-contract-violation.md`
  - `frontend-repo/API_CONTRACT.md`
  - `frontend-repo/API_DELTA_NOTIFICATION_INVITATION.md`
  - `internal/application/notification_service.go`
  - `internal/application/invitation_notification_projector.go`
  - `internal/application/notification_reconciliation.go`
  - `internal/repository/postgres/notification_repository.go`
- next_step: implement this plan as one bounded task
