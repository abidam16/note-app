# ADR-003: Draft And Revisions Are Separate State Models

## Status
Accepted (retrospective)

## Context
- The product depends on a clear distinction between editable current work and intentional historical checkpoints.
- The main architectural risk is semantic collapse: treating routine draft saves as history creation, or treating restore as history rewriting.
- The current implementation also ties thread anchoring to the current draft, which makes the working-state model especially important.

## Decision
- Keep exactly one mutable current draft per page.
- Keep revisions as immutable checkpoints separate from the draft.
- Keep draft save as an overwrite of current working state only; it does not create a revision.
- Keep revision restore additive: restore updates the current draft and appends a new revision entry instead of rewriting or deleting history.
- Keep trash restore separate from revision history; visibility recovery must not erase prior revisions.

## Consequences
### Positive
- Preserves the repository's core recovery model and keeps user intent legible.
- Prevents future work from collapsing draft, revision, and deleted-state semantics into one blurred state machine.
- Keeps thread anchoring tied to a single current document state rather than a moving history abstraction.

### Negative
- Restore flows remain more complex than a simple pointer swap.
- The current implementation still has non-atomic follow-up work around thread anchor reevaluation after some content mutations.

### Follow-on rules
- Do not turn routine draft saves into implicit revision creation without an explicit product and architecture change.
- Do not mutate or delete historical revisions during restore flows.
- If future work strengthens atomicity around restore and anchor reevaluation, preserve this state separation unless a new ADR changes it.

## Evidence / confidence
- Supported by `PRD.md` sections 1, 2, 6, 7, 8, and 11.
- Supported by `ARCHITECTURE.md` sections 1, 4, 5, 6, 7, and 14.
- Supported by `docs/backend-feature-roadmap.md` and `docs/threaded-discussion-roadmap.md`.
- Reflected in `internal/application/revision_restore.go`, `internal/application/page_service.go`, and the `pages`, `page_drafts`, `revisions`, and `trash_items` persistence model.
- Confidence: High. The intent and implementation are aligned, even though some mutation edges remain technically weak.
