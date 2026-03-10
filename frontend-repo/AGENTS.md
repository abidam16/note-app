# AGENTS.md (for new frontend repository)

## Mission
Build a production-ready frontend that integrates with the existing backend API and reflects the true product model:
- structured document editing
- explicit revision control
- async collaboration (comments + notifications)

## Backend Maturity Assumption
Treat backend as mature and stable for frontend product work:
- Backend roadmap is complete through feature 23
- Search and notification flows were refactored/enhanced and are stable integration targets
- Handoff quality assumption includes high automated test maturity (coverage milestone >90%)

Frontend implication:
- Prioritize product-grade UX and interaction quality
- Use integration tests to lock contracts, not to discover unknown behavior

## Delivery Rules
- Do not redesign backend contracts without explicit approval.
- Treat backend as source of truth for auth, permissions, validation, and state transitions.
- Implement frontend features in vertical slices, but preserve this order:
  1. Auth and session foundation
  2. Workspace/member foundation
  3. Page and draft editing
  4. Revisions (save/list/compare/restore)
  5. Comments
  6. Search
  7. Trash
  8. Notifications
- Complete each slice with UI states + API integration + tests before moving on.

## Integration Contract Rules
- All API success responses are `{ "data": ... }`.
- All API errors are `{ "error": { code, message, request_id } }`.
- Required error handling:
  - `401`: session refresh attempt, then force re-auth on failure
  - `403`: show permission-aware UI message/state
  - `404`: show not-found state with recovery navigation
  - `409`: show conflict message with actionable next step
  - `422`: show field/content validation feedback
- Never assume optimistic writes are final until backend confirms.

## Role-Aware UI Rules
Use workspace membership role to drive UX:
- `owner`: can invite members and change roles
- `editor`: can mutate content and resolve comments
- `viewer`: read-only for content mutations, but can create comments and consume notifications

The frontend must:
- hide/disable forbidden controls
- still handle server `403` safely

## Product-Critical UX Rules
- Keep “Draft autosave” and “Save revision” as clearly separate actions.
- Revisions are immutable and timeline-like.
- Restore operations are destructive to current draft but additive to history; require confirmation.
- Resolved comments remain visible (history preservation).
- Trash restore must preserve revision continuity in user mental model.

Search UX requirements:
- Prevent blank query submits (`422` server validation fallback remains required)
- Show deterministic, workspace-scoped results with clear empty states
- Preserve query state in URL for shareable/reloadable search screens

Notification UX requirements:
- Show unread vs read clearly
- Mark-read interactions must be idempotent-friendly
- Support quick contextual navigation from notification items to related app surfaces

## Frontend Quality Standards
- Use strong typing for all DTOs and domain models.
- Centralize API client, auth interceptor, and error mapping.
- Prefer deterministic state transitions over ad hoc local mutation.
- Implement loading, empty, error, and permission states for every major page.
- Accessibility baseline required:
  - keyboard navigation
  - semantic landmarks
  - contrast compliance
  - proper form labeling and error messaging

## Testing Expectations
Minimum expected coverage per feature slice:
- Unit tests for state management and transformation logic
- Integration tests for API client contracts and error handling
- UI/component tests for key interaction flows
- End-to-end happy path for major user journeys

Critical E2E journeys:
1. Register/login -> create workspace -> invite -> accept -> role update
2. Create folder/page -> autosave draft -> save revision -> compare -> restore
3. Create comment as viewer -> resolve as editor
4. Search pages by query
5. Delete to trash -> restore from trash
6. List notifications -> mark notification as read

## Collaboration and Change Control
- If backend behavior appears inconsistent with this context, verify with reproducible API calls before changing frontend assumptions.
- If a frontend requirement needs backend API change, stop and document:
  - current behavior
  - proposed contract change
  - impact on existing flows
- Do not invent speculative product features without explicit scope approval.

## Definition of Done (frontend feature slice)
A slice is done only when all are true:
- API integration complete
- role-aware UI behavior complete
- loading/empty/error states complete
- tests pass for that slice
- notes updated for implemented behavior and assumptions

## Initial Setup Checklist for a New Agent
1. Read `CONTEXT.md` fully.
2. Read `API_CONTRACT.md` and implement typed DTOs and clients first.
3. Implement auth/session and global error handling.
4. Build role-aware layout/navigation shell.
5. Build feature slices in required order.
6. Add integration + E2E checks per slice before moving on.
