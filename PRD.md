# 1. Product Overview

Observed current product:
- The product is a workspace-based collaborative document system for teams that need one current working version of a page, explicit historical checkpoints, and asynchronous discussion around content.
- Its core unit is a structured page inside a shared workspace. Users organize pages in folders, edit a mutable draft, create immutable revisions on purpose, recover deleted pages, discuss content, and receive user-scoped notifications about relevant activity.
- The implemented product surface in this repository is mainly backend and API behavior. The frontend repository documents intended UI integration but does not yet represent a shipped end-user application.

Who it serves:
- Small teams, internal collaborators, and reviewers who need shared documentation with role-based access, durable history, and asynchronous review rather than real-time co-editing.

Core job:
- Help a team maintain shared documents without losing the distinction between what is being edited now, what was intentionally preserved before, and what collaboration happened around that content.

Core value:
- Users can edit freely in the current draft, preserve checkpoints deliberately, recover from mistakes safely, and discuss specific content without turning the product into a real-time editor.

Likely intended direction:
- A document-first team workspace where organization, revision safety, and asynchronous collaboration are more important than publishing, live editing, or broad external sharing.

# 2. Product Goals

Primary Goals:
- Preserve the distinction between mutable current work and immutable historical checkpoints.
- Make workspace membership and role rules the authoritative boundary for content access, collaboration, and administrative actions.
- Support safe recovery from change through additive revision restore and soft-delete trash restore.
- Support asynchronous review and discussion tied to document content rather than live co-editing.

Secondary Goals:
- Keep content organization simple and legible through folders, page lists, and workspace scoping.
- Make relevant activity visible through personal notifications and unread state.
- Keep discovery lightweight through workspace-scoped search.
- Keep product behavior predictable enough that clients can build confidently against it without inferring hidden rules.

# 3. Product Non-Goals

These are intentionally excluded from the current product scope, not merely unimplemented:
- Real-time multiplayer editing, live cursors, or presence.
- External sharing, public publishing, or guest access outside workspace membership.
- Turning pages into general file storage or media management objects.
- Permanent-delete-first content lifecycle behavior. The current model favors recoverability.
- Synchronous collaboration requirements such as instant co-authoring or live document negotiation.

# 4. User Roles and Actors

- Anonymous visitor: can register, log in, refresh a session, and log out.
- Authenticated user without workspace membership: can create a workspace, review pending invitations addressed to the account email before empty-workspace onboarding, accept or reject those invitations, and view only personal notification data.
- Workspace owner: full content permissions plus workspace rename, invitation create/list/update/cancel, member role management, and all editor capabilities.
- Workspace editor: can create and rename folders, create and update pages, save drafts, create revisions, compare and restore revisions, delete and restore pages from trash, resolve flat comments, and resolve threads.
- Workspace viewer: can read workspace content, list folders, pages, and trash, search, create flat comments, create threads, reply to threads, reopen threads, and read or mark notifications; cannot mutate folders, pages, drafts, revisions, or trash state and cannot resolve flat comments or threads.
- Invited user: an authenticated user whose account email matches an invitation can accept or reject that invitation.

# 5. Core Domains

- Identity, workspace access, and invitations:
  Identity, session lifecycle, workspace membership, roles, and invitations.

- Content system:
  Folders, pages, structured drafts, revision history, revision comparison, and restore behavior.

- Collaboration system:
  Flat comments, anchored discussion threads, mentions, lifecycle actions, and thread-level user preference state.

- Recovery and awareness:
  Trash and restore, workspace search, personal notification inbox, unread state, and notification freshness signals.

# 6. Core User Flows

- Register for an account
Actor: anonymous visitor.
Trigger: submits email, password, and full name.
Expected behavior: account is created with normalized email and a valid password.
Important failure behavior: duplicate email, invalid email, and weak password are rejected.

- Sign in and continue a session
Actor: anonymous visitor or signed-in user with an expiring session.
Trigger: logs in, refreshes a session, or logs out.
Expected behavior: login returns an authenticated session; refresh rotates the refresh token; logout revokes the supplied refresh token.
Important failure behavior: invalid credentials, expired or revoked refresh token, and missing authenticated user record are rejected.

- Create a workspace
Actor: authenticated user.
Trigger: submits a workspace name.
Expected behavior: a workspace is created and the creator becomes owner immediately.
Important failure behavior: blank name and duplicate normalized workspace name within that user's workspace set are rejected.

- Invite a collaborator
Actor: workspace owner.
Trigger: submits an invitation email and role.
Expected behavior: a pending invitation is created for that workspace and starts a versioned invitation lifecycle.
Important failure behavior: non-owner access is blocked; inviting an unregistered email, inviting the actor's own email, inviting an existing member, and duplicate pending invitations are blocked; these failure reasons remain distinguishable to clients.

- Manage a pending invitation
Actor: workspace owner.
Trigger: updates the role on a pending invitation, lists invitations, or cancels a pending invitation.
Expected behavior: the invitation remains the source of truth for pre-membership access and changes state through explicit versioned actions.
Important failure behavior: stale invitation version requests fail with conflict; non-pending invitations cannot be updated or cancelled.

- Respond to an invitation
Actor: invited authenticated user.
Trigger: opens personal invitation list and accepts or rejects an invitation.
Expected behavior: only an account whose email matches the invitation target can respond; acceptance creates membership atomically and advances invitation state.
Important failure behavior: foreign invitations are hidden as not found; stale version and non-pending invitations fail with conflict.

- Organize workspace content
Actor: owner or editor for mutations; any member for reads.
Trigger: creates folders, renames folders, creates pages, renames pages, moves pages, or lists pages in workspace root or one folder.
Expected behavior: folders can be nested; pages can live at workspace root or directly in one folder; page listing is non-recursive.
Important failure behavior: viewers cannot mutate containers or pages; parent and target folders must belong to the same workspace; duplicate sibling folder names are rejected.

- Edit the current draft
Actor: owner or editor.
Trigger: saves page draft content.
Expected behavior: the page's current draft is overwritten with the new structured document content and remains the live editable version.
Important failure behavior: viewers cannot save drafts; invalid structured content is rejected; draft content without stable block ids required for thread anchoring is rejected.

- Create a revision checkpoint
Actor: owner or editor.
Trigger: explicitly saves a revision.
Expected behavior: the current draft is copied into an immutable revision, optionally labeled or noted.
Important failure behavior: viewers cannot create revisions; page access rules still apply.

- Compare revisions
Actor: any workspace member with page access.
Trigger: selects two revisions on the same page.
Expected behavior: the product returns a page-scoped comparison of two historical checkpoints.
Important failure behavior: revisions from another page are rejected.

- Restore an older revision
Actor: owner or editor.
Trigger: restores a revision to become the current page content.
Expected behavior: the current draft is replaced with that revision's content and the restore creates a new revision entry instead of deleting history.
Important failure behavior: viewers cannot restore; revision/page mismatch is rejected.

- Create and resolve flat page comments
Actor: any member can create; owner or editor can resolve.
Trigger: adds a page-level comment or resolves one.
Expected behavior: comments remain historical records when resolved.
Important failure behavior: blank comment bodies are rejected; viewers cannot resolve.

- Start and continue anchored thread discussion
Actor: any workspace member for create, reply, and reopen; owner or editor for resolve.
Trigger: creates a block-anchored thread, replies to a thread, resolves a thread, or reopens a thread.
Expected behavior: thread state and anchor state are preserved; replying to a resolved thread auto-reopens it; mentions are explicit selected users, not inferred from text.
Important failure behavior: blank bodies are rejected; thread anchors must point to the current draft; non-members are denied or hidden depending on the access path.

- Delete a page to trash and restore it
Actor: owner or editor for delete and restore; any member for trash reads.
Trigger: deletes a page, lists trash, previews a trashed page, or restores it.
Expected behavior: delete is soft delete; trashed pages disappear from normal page and thread surfaces; restore returns the page without erasing revision history.
Important failure behavior: viewers cannot delete or restore; missing trash items return not found.

- Search workspace content
Actor: any workspace member.
Trigger: submits a workspace search query.
Expected behavior: search returns workspace-scoped page matches by title and draft body.
Important failure behavior: blank queries are rejected.

- Review and clear notifications
Actor: authenticated user.
Trigger: opens the notification inbox, marks items read, batch-marks items read, checks unread count, or opens the notification stream.
Expected behavior: notifications are personal inbox items; unread state is backend-owned; SSE provides freshness signals while REST remains the source of truth.
Important failure behavior: users cannot read or mark other users' notifications.

# 7. Product Rules

- Workspace membership is required for all workspace content access.
- Role enforcement is authoritative on the server. UI assumptions must not override backend permission rules.
- At least one owner must remain in every workspace.
- Only owners can manage invitations and member roles.
- Workspace invitations are explicit stateful records with `pending`, `accepted`, `rejected`, and `cancelled` states.
- Only pending invitations can be updated, accepted, rejected, or cancelled.
- Invitation acceptance is bound to the authenticated user's email, not a user-selected email value.
- Invitation creation is allowed only for already registered accounts.
- Owners cannot invite their own account email into a workspace.
- Invitation create must reject an email that already belongs to a current workspace member.
- Invitation create must reject a duplicate pending invite for the same workspace and target email.
- Invitation create failures for unregistered email, self-invite, existing member, and duplicate pending invite must remain distinguishable to clients so the UI can guide the next action correctly.
- If an authenticated user has no workspace memberships but does have pending invitations, invitation review is the primary post-auth entry path before empty-workspace onboarding.
- Folder names must be unique among siblings within the same workspace and parent scope, using trim-aware, case-insensitive comparison.
- Every page always has one current draft.
- Draft save overwrites current page state and does not create a revision.
- Revisions are immutable once created.
- Revision restore is additive: it updates the current draft and creates a new revision entry rather than rewriting history.
- Page delete is soft delete only.
- Trashed pages are excluded from normal page, page list, and thread access paths until restored.
- Search scope is one workspace at a time.
- Flat comments are page-level records and remain visible after resolution.
- Anchored threads are block-level discussion records whose anchor state follows the current page content and may become `active`, `outdated`, or `missing`.
- Replying to a resolved thread reopens it automatically.
- Notifications are personal inbox records with backend-owned unread and read state.

Current collaboration authority model:
- Viewers can create flat comments.
- Viewers can create threads, reply to threads, and reopen threads.
- Only owners and editors can resolve flat comments or threads.
- This is observed current behavior. It is stable enough to plan against, but its long-term intent remains an open product question.

Invitation targeting policy:
- Invitations target registered accounts only.
- Owners cannot invite their own account email.
- Owners can currently invite directly into any of the three roles: `owner`, `editor`, or `viewer`.
- Invite-create failure cases for unregistered email, self-invite, existing member, and duplicate pending invite should remain product-visible as distinct outcomes rather than one generic failure bucket.
- The current backend implementation still drifts from part of this policy; see current behavior and target behavior sections below.

# 8. Current Behavior by Domain

- Identity, workspace access, and invitations:
  Observed behavior: email/password registration and login are implemented. Passwords must be at least eight characters and include uppercase, lowercase, and numeric characters. Login returns access and refresh tokens, refresh rotates the refresh token, and logout revokes the supplied refresh token. Users can create multiple workspaces and list only workspaces where they are members.
  Observed behavior: workspace rename is owner-only. Member listing is available to any workspace member. Owners can change member roles, but there is no member-removal endpoint.
  Observed behavior: invitations are first-class records with explicit status and optimistic-concurrency versioning. Owners can create, list, update role on, and cancel pending invitations. Invitees can list their own invitations across workspaces and accept or reject only invitations matching their account email.
  Observed drift: invite creation still allows unregistered target emails, does not enforce self-invite as a separate product rule, and currently collapses duplicate-pending and existing-member invite failures into generic conflict responses rather than distinct client-visible outcomes.
  Observed behavior: the product already exposes a dedicated cross-workspace invitation review surface for signed-in users without memberships, but pending invitation presence is not bundled into the basic auth bootstrap or workspace list response.
  Current limitation: no password reset, email verification, non-email/password auth flow, or outbound invitation-delivery flow is present in this product surface.

- Content system:
  Observed behavior: folders support parent-child nesting. Folder create and rename enforce sibling-unique names.
  Observed behavior: pages can be created at workspace root or inside one folder. Workspace page listing returns either root pages or direct children of one folder and is non-recursive.
  Current limitation: page titles have no observed uniqueness rule.
  Observed behavior: page drafts use a validated structured JSON document format rather than raw markdown. Page creation bootstraps an empty draft. Draft saves overwrite current content and update page timestamps.
  Observed behavior: manual revisions copy the current draft with optional label and note. Revision compare returns structured diff data for two revisions on the same page. Revision restore writes old content back into the current draft and creates a new revision entry.
  Inferred intent: the product treats the draft as the working state and revisions as deliberate checkpoints rather than autosaved history.

- Collaboration system:
  Observed behavior: legacy flat page comments are still live. Any workspace member, including viewers, can create them. Owners and editors can resolve them. Resolved comments remain visible as historical records.
  Observed behavior: block-anchored thread creation, listing, detail, reply, resolve, reopen, workspace-wide thread inbox, lifecycle events, explicit mentions, and per-thread notification preference storage are implemented. Any workspace member can create threads, reply, and reopen. Only owners and editors can resolve. Replying to a resolved thread auto-reopens it.
  Observed behavior: thread anchor state is reevaluated after draft save, page delete, trash restore, and revision restore.
  Documented preference: newer UI work is documented as preferring thread endpoints over legacy flat comments.
  Current limitation: both collaboration models remain active.

- Recovery and awareness:
  Observed behavior: workspace search matches page title and extracted draft body and rejects blank queries.
  Observed behavior: page deletion is soft delete into trash. Members can list trash and inspect deleted page content. Owners and editors can restore a trashed page. Revision history survives delete and restore.
  Observed behavior: personal notification inbox, unread count, single-read, batch-read, and SSE freshness stream are implemented.
  Observed behavior: invitation notifications behave as one live row per invitation state.
  Observed behavior: thread-based comment and mention notifications are projected asynchronously and are scoped to relevant recipients rather than the full workspace.
  Observed behavior: SSE sends snapshot, unread-count changes, and inbox-invalidated events, but REST remains the source of truth.
  Current limitation: per-thread notification preferences currently store and return `all`, `mentions_only`, or `mute`, but observed current behavior does not apply those preferences to delivery yet.

# 9. Target Behavior / Desired Product Direction

- Identity, workspace access, and invitations:
  Target behavior: owners may invite only already registered accounts that are not already workspace members, are not the actor's own account email, and do not already have a pending invitation for that workspace.
  Target behavior: invite-create failures for unregistered email, self-invite, existing member, and duplicate pending invite are distinguishable to clients so the UI can provide specific guidance instead of one generic conflict state.
  Target behavior: if a signed-in user has no workspace memberships but does have pending invitations, the product routes them to pending invitation review before empty-workspace onboarding.
  Target behavior: pending invitation review uses invitation source data as the authority for acceptance and rejection state, while notifications remain a convenience surface rather than the canonical invitation state.

# 10. Current Product Validation Confidence

- Highest confidence:
  Permission boundaries, state transitions, and API-visible business behavior are strongly evidenced by the implemented backend, tests, and supporting product docs.

- Moderate confidence:
  End-to-end user experience across multiple surfaces is less certain because the repository does not yet contain a shipped frontend. Current confidence is therefore higher for product rules and service behavior than for final interaction design.

- Practical implication:
  Future planning should treat backend-visible behavior as the strongest source of observed current behavior, while treating this PRD's target-behavior sections as the intended product direction wherever implementation drift is explicitly called out.

# 11. Known Product Problems

User-facing product problems:
- Two discussion systems coexist: legacy flat comments and newer anchored threads. This creates overlapping collaboration models and inconsistent user expectations.
- Thread notification preferences expose user-facing modes that do not yet change actual notification delivery behavior.
- The backend currently still allows invitation creation for unregistered emails and does not expose distinct invite-create failure outcomes for every product-defined invalid case, so documented product intent and implementation have drifted.
- No-workspace post-auth routing still depends on the client coordinating workspace and invitation surfaces explicitly; the intended invitation-review-first experience is not yet guaranteed by a single bootstrap response.

Product debt:
- The repository does not yet demonstrate the full end-user product experience in a shipped frontend, so some API-valid flows may still be unproven as coherent UI journeys.
- Workspace administration is incomplete for mature team lifecycle needs: there is role change but no member removal, no workspace deletion, and no explicit ownership transfer flow beyond role changes or inviting another owner.

Product ambiguity:
- It is not yet settled whether flat comments are legacy compatibility only or a long-term parallel collaboration model.
- It is not yet settled whether thread notification preferences are merely stored settings for future use or part of the intended near-term collaboration model.

# 12. Success Criteria

Product Success Criteria:
- A change preserves the distinction between current draft, immutable revision history, and soft-deleted state. It remains clear which user action changes which state, and those states do not collapse into one another.
- A change preserves role boundaries. Owners remain the only actors for invitation and member administration, viewers remain unable to mutate document or container state, and exceptions are explicit rather than accidental.
- A change that touches restore behavior preserves additive history. Restoring a revision or a trashed page does not silently discard recoverable product history.
- A change that touches collaboration preserves a clear model. It either maintains the current flat-comment-plus-thread split intentionally or explicitly narrows it; it does not blur responsibilities between the two models by accident.
- A change that touches notifications preserves recipient correctness. Notifications remain personal, unread state remains backend-owned, and activity does not spread beyond the intended recipient set.
- A change that touches search, lists, or inbox surfaces preserves scope boundaries. Workspace-scoped behavior stays workspace-scoped, and personal inbox behavior stays user-scoped.
- A change that touches invitations preserves the registered-user-only targeting rule, blocks self-invite and invalid membership states clearly, and keeps invite-create failure outcomes explicit enough for the client to route and message users correctly.
- A change that touches signed-in bootstrap or onboarding preserves the rule that pending invitation review takes precedence over empty-workspace onboarding for users with no memberships.

Change Governance Criteria:
- A change that adds or changes product behavior also updates the product document so future work does not need to rediscover intent from code alone.
- New product documentation keeps a clear separation between observed behavior, intended direction, unresolved questions, and deferred work.
- When a change relies on behavior that is still ambiguous, the ambiguity is recorded explicitly rather than replaced with invented certainty.

# 13. Open Product Questions

High-Impact Questions:
- Should legacy flat page comments remain a supported end-user feature, or should the product formally converge on threads as the primary discussion model?
- Should thread notification preference modes eventually change delivery behavior, and if so, how should `mentions_only` interact with relevant-user comment notifications?
- Is inviting someone directly as `owner` an intentional product rule, or should ownership require a later promotion step?
- Should viewers intentionally be allowed to create threads, reply to threads, and reopen threads, or is that broader than the intended review authority?

Lower-Priority Questions:
- Should page titles be allowed to duplicate within the same folder or workspace, or is a future uniqueness rule expected?
- Should pending invitation presence eventually be exposed in a bootstrap-oriented surface such as auth/session context, or should the client continue to query dedicated invitation surfaces explicitly?
- Should notification items become actionable beyond invitations, such as deep-links or inline thread actions, or remain informational rows?
- Should workspace-global search eventually include thread content directly, or remain page-only with thread discovery handled elsewhere?

# 14. Deferred Product Directions

Nearer-Term Deferred Directions:
- Shipped web frontend implementing the documented backend flows.
- Retirement or controlled migration path for legacy flat comments.
- Enforcement of thread notification preferences in delivery logic.
- Additional account lifecycle features such as password reset, email verification, and alternative sign-in methods.
- Richer notification UX such as grouping, deep-links, and workspace-aware filtering.

Longer-Term Possibilities:
- Replayable or more stateful real-time notification delivery beyond freshness and invalidation signals.
- Broader workspace administration features such as member removal, workspace deletion, and clearer ownership transfer flows.
- Richer content workflows such as recursive tree operations, folder move/delete, and stronger organization controls.
- More advanced collaboration features such as real-time editing, live cursors, or presence.
