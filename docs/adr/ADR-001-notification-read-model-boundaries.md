# ADR-001: Notifications Are Read Models, Not Source Of Truth

## Status
Accepted (retrospective)

## Context
- The repository exposes notification state through projected inbox rows, unread counters, and an SSE stream.
- That shape creates a predictable failure mode: future contributors may start treating those delivery surfaces as business truth for invitations, memberships, pages, or threads.
- The codebase also contains outbox projectors, but no shipped API runtime continuously consumes the outbox. Only reconciliation is wired operationally, so projection lag and repair are part of the current architecture.

## Decision
- Treat source tables such as `workspace_invitations`, `workspace_members`, `page_comment_threads`, and `page_comment_messages` as authoritative business state.
- Treat `notifications` and `notification_unread_counters` as derived read models owned by projection and reconciliation logic.
- Treat `GET /api/v1/notifications` and `GET /api/v1/notifications/unread-count` as the canonical notification reads.
- Treat `GET /api/v1/notifications/stream` as freshness and invalidation only. It is not a durable event log and it does not replace REST refetch.
- Future notification work must extend the existing outbox/projector/reconciliation path deliberately. Do not assume live projector workers are running just because projector code exists.

## Consequences
### Positive
- Protects source-of-truth boundaries and prevents notification rows from becoming accidental business state.
- Keeps SSE simple and tolerant of duplicate or missed invalidations.
- Gives future Codex work a clear rule for where notification correctness comes from and how drift is repaired.

### Negative
- Notification freshness is intentionally weaker than source-table correctness.
- Managed notification rows may lag source state until projection or reconciliation catches up.
- Some legacy notification paths remain split from the newer projected path.

### Follow-on rules
- Do not treat inbox rows, unread counters, or SSE payloads as canonical state for invitations, memberships, pages, or threads.
- Do not add a second notification delivery pattern for thread work without an explicit architectural change.
- Do not add replay-dependent client behavior on top of the current SSE stream without a new ADR.

## Evidence / confidence
- Supported by `ARCHITECTURE.md` sections 2, 5, 6, 7, 9, 11, and 14.
- Supported by `PRD.md` sections 6, 8, and 11.
- Reflected in `internal/application/notification_stream.go`, `internal/repository/postgres/notification_repository.go`, `cmd/api/app.go`, and `cmd/notification-reconcile/main.go`.
- Supported by `docs/invitation-notification-thread-roadmap.md` and `docs/checkpoint.md`.
- Confidence: High. The historical path into this design is only partially recoverable, so this ADR records current load-bearing reality.
