# ADR-004: Invitations Use Explicit Lifecycle State And Optimistic Concurrency

## Status
Accepted (retrospective)

## Context
- Invitation handling crosses owner actions, invitee actions, and membership creation, so race conditions are easy to introduce.
- Without an explicit lifecycle and concurrency guard, stale updates, double acceptance, and conflicting terminal actions become easy to introduce and hard to debug.
- The current repository already treats invitations as first-class lifecycle records with explicit status and versioning.

## Decision
- Treat `workspace_invitations` as the canonical pre-membership state for invitation workflows.
- Model invitation state explicitly with `pending`, `accepted`, `rejected`, and `cancelled`.
- Allow update, accept, reject, and cancel only from `pending`.
- Require caller-supplied `version` checks for mutation and terminal-transition endpoints.
- Enforce repository row locking and transactional state changes for invitation transitions, especially acceptance with membership creation.

## Consequences
### Positive
- Gives invitation flows deterministic behavior under concurrent owner and invitee actions.
- Makes stale requests fail explicitly instead of silently overwriting newer state.
- Preserves one clear source of truth for pre-membership access and transition history.

### Negative
- Clients must round-trip `version` and handle `409 conflict`.
- Invitation flows are more explicit and verbose than a fire-and-forget model.

### Follow-on rules
- Do not bypass invitation `version` checks in service or repository code.
- Do not split invitation acceptance from membership creation into separate non-atomic writes.
- Do not treat invitation notifications as the authoritative invitation lifecycle.

## Evidence / confidence
- Supported by `PRD.md` sections 6, 7, 8, and 11.
- Supported by `ARCHITECTURE.md` sections 4, 5, 6, 7, 8, 11, and 14.
- Supported by `docs/invitation-notification-thread-roadmap.md` and `docs/checkpoint.md`.
- Reflected in `internal/application/workspace_service.go`, `internal/repository/postgres/workspace_repository.go`, and invitation concurrency tests in `internal/repository/postgres`.
- Confidence: High. The original historical reasoning is only partly documented, but the current behavior is explicit and heavily reinforced by code and tests.
