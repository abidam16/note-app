# ADR-002: Threads Are The Forward Collaboration Model

## Status
Accepted (retrospective)

## Context
- The repository still ships both legacy `page_comments` and the newer thread model.
- That dual-track state creates a clear risk: future work can accidentally keep both models evolving in parallel.
- The current docs consistently describe threads as the actively evolved model with richer history, anchors, mentions, and notification work.

## Decision
- Use thread endpoints and thread message state as the default backend integration path for new collaboration work.
- Keep legacy `page_comments` only for backward compatibility, existing clients, or explicit migration and retirement work.
- Do not introduce a third collaboration model or a parallel notification path for threaded discussion.

## Consequences
### Positive
- Concentrates new work on the model that already carries anchors, lifecycle history, mentions, workspace inbox views, and notification projection.
- Reduces future ambiguity for Codex planning, review, and endpoint selection.
- Preserves compatibility without freezing the newer thread model.

### Negative
- The system still carries two collaboration surfaces until flat-comment retirement is decided and executed.
- Some user-facing ambiguity remains because flat comments are still live in the current product.

### Follow-on rules
- New collaboration UI and backend feature work should start from thread endpoints, not `page_comments`.
- Changes to legacy flat comments should be explicitly framed as compatibility maintenance or migration support.
- If the product later retires flat comments or restores them as a first-class model, record that with a new ADR.

## Evidence / confidence
- Supported by `ARCHITECTURE.md` sections 4, 11, 13, and 14.
- Supported by `PRD.md` sections 8, 10, 11, and 12.
- Supported by `docs/threaded-discussion-roadmap.md` and `docs/invitation-notification-thread-roadmap.md`.
- Reflected in the richer thread implementation under `internal/application/thread_service.go`, `internal/repository/postgres/thread_repository.go`, and the newer notification projectors, while `internal/application/comment_service.go` remains the simpler legacy path.
- Confidence: High. This is a current-state ADR; the final end-state for flat comments is still an open product question.
