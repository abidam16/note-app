# Task 30 No-Workspace Invitation-First Entry Contract Plan

## 1. Task Summary

Define and document the canonical post-auth entry contract for authenticated users with zero workspace memberships so clients route them to pending invitation review before empty-workspace onboarding, using the existing backend surfaces instead of inventing a new bootstrap endpoint in this task.

## 2. Objective

After this task, the frontend-facing contract clearly defines one stable no-workspace entry flow:

- use `GET /api/v1/workspaces` to determine whether the signed-in user already has memberships
- if none exist, use `GET /api/v1/my/invitations?status=pending` as the authoritative invitation-review source
- route to empty-workspace onboarding only when both checks show no memberships and no pending invitations

This task should remove contract ambiguity without changing invitation source-of-truth ownership or broadening auth bootstrap behavior.

## 3. Scope

### In Scope

- Define the canonical no-workspace invitation-first entry flow in frontend-facing API docs
- Make explicit that invitation source data, not notifications, is authoritative for accept/reject state
- Define the role of existing surfaces:
  - `GET /api/v1/auth/me` for authenticated identity
  - `GET /api/v1/workspaces` for membership presence
  - `GET /api/v1/my/invitations?status=pending` for invitation-review gating and data
- Document the client routing rule for users with zero memberships
- Document the post-action refresh expectation after accept or reject from the invitation-review entry path
- Update `docs/checkpoint.md` for the completed contract-alignment slice

### Out Of Scope

- Adding a new endpoint
- Adding invitation summary fields to `GET /api/v1/auth/me`
- Changing the response shape of `GET /api/v1/workspaces`
- Changing invitation accept/reject/cancel/update runtime behavior
- Redesigning onboarding UX beyond the entry-routing contract
- Changing notification inbox or SSE ownership rules
- Introducing a new backend-owned bootstrap aggregate model

## 4. Detailed Specification

### 4.1 Canonical Entry Decision

Task scope is limited to documenting the canonical client decision sequence after the user is authenticated:

1. Call `GET /api/v1/workspaces`
2. If the response contains one or more workspaces, proceed with the normal workspace-aware app entry flow
3. If the response is empty, call `GET /api/v1/my/invitations?status=pending`
4. If that response contains one or more pending invitations, route to pending invitation review before empty-workspace onboarding
5. If that response is also empty, route to empty-workspace onboarding

This task does not add a new server decision endpoint. It standardizes how the existing backend contract must be consumed.

### 4.2 Surface Ownership

Document these ownership boundaries explicitly:

- `GET /api/v1/auth/me`
  - remains identity-only
  - must not be documented as the source of invitation-first routing
- `GET /api/v1/workspaces`
  - remains the authoritative membership-presence check
  - empty list means only that the actor has no current memberships
  - empty list does not imply there are no pending invitations
- `GET /api/v1/my/invitations?status=pending`
  - is the authoritative invitation-review source for users with no memberships
  - drives invitation-review routing and action state
- notification inbox and SSE
  - may hint that invitation activity exists
  - must not be documented as the canonical source for no-workspace entry routing

### 4.3 Canonical Client Guidance

The contract docs should make the following guidance explicit:

- clients must not send a no-workspace user directly to empty-workspace onboarding based only on `GET /api/v1/workspaces = []`
- clients should treat pending invitation review as the first follow-up check when workspace membership is empty
- invitation review should use invitation rows from `GET /api/v1/my/invitations?status=pending`, not notification payloads, as the actionable source
- after accepting or rejecting an invitation from this entry path, clients should refresh the relevant authoritative surfaces before deciding the next route:
  - `GET /api/v1/workspaces`
  - `GET /api/v1/my/invitations?status=pending`

### 4.4 Contract Shape Constraint

No new response fields or endpoint shapes are introduced in this task.

The smallest safe implementation is contract clarification only because:

- the PRD requires invitation-first routing
- the architecture keeps invitation records, not bootstrap DTOs or notifications, as the source of truth
- the PRD still leaves open whether a dedicated bootstrap-oriented surface is ultimately desirable

If implementation discovers that the current two-endpoint contract is insufficient to ship the intended frontend flow, stop and route that gap into a new follow-up artifact rather than expanding this task.

### 4.5 Documentation Behavior

After implementation, the frontend-facing contract docs should describe the actual canonical sequence instead of leaving no-workspace routing implicit.

Update:

- `frontend-repo/API_CONTRACT.md`
- `frontend-repo/API_DELTA_NOTIFICATION_INVITATION.md`
- `docs/checkpoint.md`

Recommended documentation placement:

- add frontend-usage notes under `GET /api/v1/workspaces`
- add invitation-review-first notes under `GET /api/v1/my/invitations`
- add one concise cross-reference that `GET /api/v1/auth/me` is not sufficient for no-workspace routing by itself

## 5. Files / Components To Change

### Expected Changes

- `frontend-repo/API_CONTRACT.md`
- `frontend-repo/API_DELTA_NOTIFICATION_INVITATION.md`
- `docs/checkpoint.md`

### Must Not Change

- `internal/application/*`
- `internal/transport/http/*`
- `internal/repository/postgres/*`
- `PRD.md`
- `ARCHITECTURE.md`
- invitation endpoint payload shapes
- auth/session payload shapes

If execution reveals that code changes or contract-shape changes are required, stop and create a new follow-up plan instead of stretching Task 30.

## 6. Validation And Test

### Validation

Verify all of the following:

- the docs define one unambiguous no-workspace entry sequence
- the sequence preserves the PRD rule that pending invitation review takes precedence over empty-workspace onboarding
- `GET /api/v1/workspaces` is documented as membership presence only, not invitation absence
- `GET /api/v1/my/invitations?status=pending` is documented as the authoritative invitation-review source for zero-membership users
- notification inbox and SSE remain documented as convenience/freshness surfaces rather than the routing authority
- no new endpoint or response field is implied anywhere in the updated docs
- docs remain consistent with the existing implemented backend behavior

### Tests

No runtime code change is expected in this task.

Required verification is a documentation consistency pass against:

- `PRD.md`
- `ARCHITECTURE.md`
- `internal/transport/http/server.go`
- the current invitation and workspace handler surface definitions

If implementation chooses to add tests anyway, keep them limited to documentation-adjacent assertions or examples and do not broaden the task into backend behavior changes.

## 7. Review Checklist

- The task stays contract-only and does not introduce bootstrap runtime changes.
- The documented route for zero-membership users is explicit and single-path.
- Invitation source data remains the authority for invitation-review actions.
- Notifications are not promoted to source-of-truth status.
- `GET /api/v1/auth/me` is not overloaded with new routing semantics.
- The docs do not imply that `GET /api/v1/workspaces = []` is sufficient to trigger onboarding.
- `docs/checkpoint.md` captures the contract decision clearly enough for later frontend or backend work.

## 8. Trade-Offs And Risks

- Keeping the contract as a two-endpoint sequence preserves current backend shapes and source-of-truth boundaries, but it leaves some client coordination burden in place.
- A future product decision may still prefer a dedicated bootstrap-oriented surface. This task intentionally does not settle that architectural direction.
- If the docs become too implicit again, frontend implementations may regress into treating empty workspace membership as equivalent to no invitations. The wording needs to be direct.
- Because this task is contract-only, it improves clarity rather than reducing round trips. Performance or latency concerns are deferred unless they become a demonstrated problem.

## 9. Future Improvements

- If the product later decides the two-endpoint pattern is too brittle, create a separate plan for a dedicated bootstrap-oriented contract change.
- A later frontend implementation task can codify the exact route transitions and retry behavior around accept/reject from the no-workspace invitation-review entry path.
- If shipped clients need a lower-latency bootstrap, revisit whether an aggregated session/workspace entry contract is justified without weakening source-of-truth rules.

## Plan Handoff

- artifact_type: `PLAN`
- artifact_status: `DRAFT`
- decision: `PROCEED_TO_IMPLEMENTATION`
- task_summary: no-workspace invitation-first entry contract
- objective: codify a canonical two-endpoint no-workspace entry flow using existing workspace and invitation surfaces without adding a new bootstrap contract
- in_scope:
  - no-workspace routing contract clarification
  - invitation-first entry guidance
  - source-of-truth clarification for invitation review
  - checkpoint update
- out_of_scope:
  - new endpoint or new bootstrap field
  - onboarding redesign
  - invitation runtime behavior changes
- expected_changes:
  - frontend-facing API contract docs
  - checkpoint note
- must_not_change:
  - backend runtime code
  - endpoint payload shapes
  - PRD and architecture docs
- validation_requirements:
  - docs align with PRD and current backend surfaces
  - zero-membership routing sequence is explicit and unambiguous
- test_requirements:
  - documentation consistency pass against current server routes and PRD
- source_artifacts:
  - `PRD.md`
  - `ARCHITECTURE.md`
  - `docs/invitation-notification-thread-roadmap.md`
  - `frontend-repo/API_CONTRACT.md`
  - `frontend-repo/API_DELTA_NOTIFICATION_INVITATION.md`
  - `internal/transport/http/server.go`
- next_step:
  - implement this plan as one bounded documentation task

## Decision

- `PROCEED_TO_IMPLEMENTATION`

## Why This Decision

- The roadmap already isolates Task 30 as a separate follow-on item.
- The PRD clearly requires invitation-first routing, but does not yet justify inventing a new bootstrap contract in this slice.
- The smallest safe task is to remove contract ambiguity using existing backend surfaces while preserving current source-of-truth boundaries.

## Plan Status

- `NEW_PLAN`

## Immediate Next Step

- Implement this plan as one bounded documentation task.

## Continuation Prompt

- Proceed to implement this plan as one bounded documentation task.
