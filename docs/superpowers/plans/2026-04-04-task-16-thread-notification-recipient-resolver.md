# Task 16 Thread Notification Recipient Resolver Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace workspace-wide thread notification fanout with a focused resolver that selects only relevant recipients for thread create and reply events.

**Architecture:** This task adds a small application-layer recipient resolver for thread notifications. The resolver accepts workspace context, the acting user, thread detail, and optional explicit mention user ids, then returns a deterministic deduped recipient list filtered to active workspace members. The current synchronous thread notification path in `NotificationService` will use this resolver immediately, so public thread endpoints keep the same request and response contracts while their notification side effect changes from workspace fanout to relevant-user delivery.

**Tech Stack:** Go, application services, pure unit tests, existing membership reader interfaces, existing notification repository interfaces, Markdown API docs

---

## 1. Scope

### In Scope
- Add a reusable thread notification recipient resolver
- Define the relevant-user rule for thread events:
  - thread creator
  - prior repliers
  - explicit mention targets
  - exclude the acting user
  - exclude users without workspace membership
  - dedupe recipients
- Make recipient resolution deterministic for stable tests
- Update the current synchronous thread notification path to use the resolver
- Keep thread create and reply endpoint contracts unchanged
- Add resolver and notification-service tests
- Update API docs and checkpoint to describe the new notification behavior

### Out Of Scope
- No new public endpoint
- No request payload changes
- No response payload changes
- No outbox integration yet
- No mention persistence yet
- No mention notification projector yet
- No legacy flat-comment recipient change in this task
- No unread-count logic changes

---

## 2. Detailed Spec

## 2.1 Objective

Thread notifications currently fan out to every workspace member except the actor. That is noisy and does not match the product decision for relevant-user notifications.

This task introduces one shared resolver so later work can reuse the same recipient policy in:
- the current synchronous notification path
- future outbox producers
- future mention-aware thread events

## 2.2 Public Endpoint Impact

### New Endpoints
- None

### Existing Endpoints With Behavior Change
- `POST /api/v1/pages/{pageID}/threads`
- `POST /api/v1/threads/{threadID}/replies`

### Request Payload Changes
- None in this task

### Response Payload Changes
- None in this task

### Response Code Changes
- None in this task

### Public Contract Rule
The HTTP request and response contracts for thread create and thread reply stay exactly the same. Only the internal notification recipient selection changes.

## 2.3 Internal Resolver Contract

This task should introduce one focused internal resolver interface and input type.

Recommended interface:

```go
type ThreadNotificationRecipientResolver interface {
    ResolveRecipients(ctx context.Context, input ResolveThreadNotificationRecipientsInput) ([]string, error)
}
```

Recommended input type:

```go
type ResolveThreadNotificationRecipientsInput struct {
    WorkspaceID             string
    ActorID                 string
    Detail                  domain.PageCommentThreadDetail
    ExplicitMentionUserIDs  []string
}
```

### Internal Output
- ordered `[]string` of recipient user ids

### Internal Output Rules
- each returned user id appears at most once
- output order is deterministic
- empty result is valid

## 2.4 Validation Rules

### Public Validation
- No new public validation rules
- Existing thread create and reply validation rules remain unchanged

### Internal Resolver Validation
1. `WorkspaceID` must be non-empty
2. `ActorID` must be non-empty
3. `Detail.Thread.ID` must be non-empty
4. `Detail.Thread.CreatedBy` should be allowed to be empty only if the source data is already broken; the resolver should ignore blank candidate ids rather than panic
5. `ExplicitMentionUserIDs` may be empty
6. duplicate or blank mention ids must not produce duplicate recipients

If required fields such as `WorkspaceID`, `ActorID`, or `Detail.Thread.ID` are empty:
- return `domain.ErrValidation`

If workspace membership lookup fails:
- propagate the error

## 2.5 Recipient Rules

### Candidate Sources
Build recipient candidates from:
1. `detail.Thread.CreatedBy`
2. every `message.CreatedBy` from `detail.Messages`, in message order
3. `ExplicitMentionUserIDs`, in input order

### Exclusion Rules
Exclude:
- the acting user
- blank ids
- users who are not current workspace members

### Deduplication Rule
- dedupe by user id
- preserve first occurrence

### Deterministic Order Rule
The resolver should preserve first-seen order from the candidate sources:
1. thread creator first
2. prior message authors in chronological message order
3. explicit mention ids in caller-provided order

This gives stable output without sorting and keeps the policy easy to reason about.

## 2.6 Event-Specific Behavior

### Thread Create
For a newly created thread:
- `detail.Thread.CreatedBy == actorID`
- `detail.Messages` contains one starter message by the actor
- if `ExplicitMentionUserIDs` is empty:
  - recipient list is empty
- if explicit mentions are provided later by mention support work:
  - only those mentioned active members receive comment notifications from this resolver

### Thread Reply
For a reply event:
- thread creator may be a recipient if not the actor
- any prior replier may be a recipient if not the actor
- a newly mentioned user may be a recipient even if they have never replied
- if a user appears in multiple candidate groups, they receive only one notification from this event

## 2.7 Membership Filtering Rule

Use current workspace membership as the authoritative filter:
- call `memberships.ListMembers(ctx, workspaceID)`
- build a set of active member user ids
- keep only candidate ids present in that set

This still reads workspace membership, but it does not fan out notifications to every member. It only validates that chosen recipients still belong to the workspace.

## 2.8 Notification Service Integration

Current methods in scope:
- `NotifyThreadCreated`
- `NotifyThreadReplyCreated`

Required behavior after this task:
- stop calling `notifyPageMembers` for thread events
- resolve recipients through the new resolver
- if recipient list is empty:
  - return `nil`
  - create no notifications
- if recipients exist:
  - create notifications only for those recipients

### Legacy Flat Comment Behavior
- `NotifyCommentCreated` for legacy flat comments stays unchanged in this task
- only thread event recipient logic changes now

## 2.9 Public Positive And Negative Cases

### Positive Cases

1. Create thread with no mentions
- Existing endpoint result: unchanged success code and response payload
- Notification side effect: no comment notifications are created

2. Reply to a thread where another user started the thread
- Existing endpoint result: unchanged success code and response payload
- Notification side effect: thread creator receives one comment notification if still a workspace member

3. Reply to a thread with multiple prior repliers
- Existing endpoint result: unchanged success code and response payload
- Notification side effect: prior distinct participants receive one notification each, excluding the actor

4. Reply where a mentioned user is already a participant
- Existing endpoint result: unchanged success code and response payload
- Notification side effect: user still receives only one notification

### Negative Cases

1. Membership lookup fails while resolving recipients
- Existing endpoint result in the current synchronous path: error propagates and the request fails, unchanged from current failure policy

2. Resolver input is internally invalid
- Existing endpoint result: internal validation error propagates

No new public response codes are introduced in this task.

---

## 3. File Structure And Responsibilities

### Create
- `internal/application/thread_notification_recipient_resolver.go`
  - define the resolver interface, input type, and default implementation
- `internal/application/thread_notification_recipient_resolver_test.go`
  - focused unit tests for recipient resolution rules

### Modify
- `internal/application/notification_service.go`
  - replace workspace-wide thread recipient logic with resolver-driven recipient logic
- `internal/application/notification_service_test.go`
  - update thread notification expectations from workspace-wide fanout to relevant-user fanout
- `internal/application/notification_service_additional_test.go`
  - add error propagation and empty-recipient behavior coverage
- `frontend-repo/API_CONTRACT.md`
  - update thread create/reply behavior notes to describe relevant-user notifications instead of workspace-wide fanout
- `docs/checkpoint.md`

### Modify If Needed For Dependency Injection Clarity
- `internal/application/notification_events.go`
  - only if introducing a small interface or constructor helper improves testability without changing public behavior

### Files Explicitly Not In Scope
- `internal/transport/http/handlers.go`
- `internal/transport/http/server.go`
- `internal/repository/postgres/notification_repository.go`
- `internal/application/thread_service.go`
  - method signatures stay the same in this task
- `internal/application/comment_service.go`
  - legacy flat-comment notification behavior is unchanged

---

## 4. Test Matrix

## 4.1 Resolver Unit Tests

Add focused tests in:
- `internal/application/thread_notification_recipient_resolver_test.go`

### Positive Cases

1. Create thread with actor as thread creator and only one actor-authored message returns no recipients

2. Reply event returns thread creator when creator is not the actor

3. Reply event returns prior distinct repliers in first-seen order

4. Explicit mention target is included when they are an active workspace member

5. Mentioned user already present as thread creator or prior replier is returned only once

6. Blank mention ids are ignored

7. Non-member mention ids are ignored

8. Recipient order stays deterministic:
- thread creator
- prior message authors
- explicit mentions

### Negative Cases

9. Empty `WorkspaceID` returns `domain.ErrValidation`

10. Empty `ActorID` returns `domain.ErrValidation`

11. Empty `Detail.Thread.ID` returns `domain.ErrValidation`

12. Membership list error propagates

## 4.2 Notification Service Tests

Add or update tests in:
- `internal/application/notification_service_test.go`
- `internal/application/notification_service_additional_test.go`

### Positive Cases

13. `NotifyThreadCreated` with no relevant recipients creates no notifications

14. `NotifyThreadCreated` with future-style explicit mentions would create notifications only for resolved recipients
- this may be covered directly through the resolver if current service signatures do not accept mention ids yet

15. `NotifyThreadReplyCreated` notifies only relevant users, not all workspace members

16. `NotifyThreadReplyCreated` excludes the actor even if the actor previously replied

17. `NotifyThreadReplyCreated` dedupes repeated participants

### Negative Cases

18. Resolver membership lookup failure propagates

19. Notification repository batch create failure propagates after recipient resolution

## 4.3 Documentation Tests

20. `frontend-repo/API_CONTRACT.md` documents:
- thread create notifications no longer fan out to all workspace members
- thread reply notifications go only to relevant users
- relevant users are thread creator, prior repliers, and explicit mention targets
- actor does not receive their own notification

21. `docs/checkpoint.md` records:
- thread notification recipient policy changed to relevant-user delivery
- create-thread with no explicit mentions may produce zero notifications
- reply notifications now target prior participants instead of full workspace fanout

---

## 5. Execution Plan

### Task 1: Write failing resolver tests

**Files:**
- Create: `internal/application/thread_notification_recipient_resolver_test.go`

- [ ] **Step 1: Add failing tests for create-thread empty-recipient behavior**

Cover:
- actor-created thread
- one actor-authored starter message
- empty mentions

- [ ] **Step 2: Add failing tests for reply recipient selection**

Cover:
- thread creator included
- prior repliers included
- actor excluded
- dedupe behavior
- deterministic order

- [ ] **Step 3: Add failing tests for mention and membership filtering**

Cover:
- mentioned active member included
- blank mention ignored
- non-member ignored
- membership lookup error propagation

- [ ] **Step 4: Run targeted application tests**

Run:
```powershell
go test ./internal/application -run "TestThreadNotificationRecipientResolver" -count=1
```

Expected:
- FAIL because the resolver does not exist yet

- [ ] **Step 5: Commit**

```bash
git add internal/application/thread_notification_recipient_resolver_test.go
git commit -m "test: define thread notification recipient resolver behavior"
```

### Task 2: Implement the resolver

**Files:**
- Create: `internal/application/thread_notification_recipient_resolver.go`

- [ ] **Step 1: Define the resolver interface and input type**

Required:
- `ResolveRecipients(ctx, input) ([]string, error)`
- dedicated input struct with workspace, actor, detail, and explicit mention ids

- [ ] **Step 2: Implement validation and membership filtering**

Required behavior:
- validate required fields
- load workspace members
- build active-member set

- [ ] **Step 3: Implement candidate collection, exclusion, dedupe, and stable order**

Required order:
- thread creator
- message authors
- explicit mentions

- [ ] **Step 4: Re-run targeted resolver tests**

Run:
```powershell
go test ./internal/application -run "TestThreadNotificationRecipientResolver" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/thread_notification_recipient_resolver.go internal/application/thread_notification_recipient_resolver_test.go
git commit -m "feat: add thread notification recipient resolver"
```

### Task 3: Wire the resolver into notification service thread methods

**Files:**
- Modify: `internal/application/notification_service.go`
- Modify: `internal/application/notification_service_test.go`
- Modify: `internal/application/notification_service_additional_test.go`

- [ ] **Step 1: Add failing service tests for relevant-user thread notifications**

Cover:
- thread create with no recipients creates no notifications
- thread reply targets only creator and prior repliers
- no workspace-wide fanout remains

- [ ] **Step 2: Update `NotificationService` construction if needed**

Requirement:
- inject the resolver cleanly
- keep constructor ergonomics consistent with current codebase patterns

- [ ] **Step 3: Replace `notifyPageMembers` usage for thread events**

Required behavior:
- `NotifyThreadCreated` uses resolver result instead of full member list
- `NotifyThreadReplyCreated` uses resolver result instead of full member list
- `NotifyCommentCreated` remains unchanged

- [ ] **Step 4: Re-run targeted service tests**

Run:
```powershell
go test ./internal/application -run "TestNotificationService" -count=1
```

Expected:
- PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/notification_service.go internal/application/notification_service_test.go internal/application/notification_service_additional_test.go
git commit -m "feat: use relevant recipients for thread notifications"
```

### Task 4: Update documentation

**Files:**
- Modify: `frontend-repo/API_CONTRACT.md`
- Modify: `docs/checkpoint.md`

- [ ] **Step 1: Update the thread notification behavior notes**

Document:
- create-thread notification recipients are relevant users only
- reply notification recipients are relevant users only
- actor exclusion
- no workspace-wide fanout

- [ ] **Step 2: Update checkpoint**

Record:
- new relevant-user thread notification policy
- reply notifications now use thread participants
- thread-create may notify no one when no relevant recipients exist

- [ ] **Step 3: Commit**

```bash
git add frontend-repo/API_CONTRACT.md docs/checkpoint.md
git commit -m "docs: update thread notification recipient policy"
```

### Task 5: Full verification for Task 16

**Files:**
- Modify if needed: none expected

- [ ] **Step 1: Run the exact verification set**

Run:
```powershell
go test ./internal/application -run "TestThreadNotificationRecipientResolver|TestNotificationService" -count=1
```

Expected:
- PASS

- [ ] **Step 2: Manual behavior sanity check if local server is available**

Verify through existing thread flows:
- create a thread with no relevant recipients and confirm no thread notification rows are created
- reply to a thread with one prior participant and confirm only that participant is notified

- [ ] **Step 3: Commit cleanup if needed**

```bash
git add -A
git commit -m "chore: verify thread notification recipient resolver task"
```

---

## 6. Acceptance Criteria

Task 16 is complete only when all are true:
- a dedicated thread notification recipient resolver exists
- the resolver returns only relevant users:
  - thread creator
  - prior repliers
  - explicit mention targets
- the actor is excluded
- non-members are excluded
- duplicate recipients are removed deterministically
- current synchronous thread notification methods use the resolver instead of workspace-wide fanout
- legacy flat-comment notification behavior is unchanged
- public thread endpoint request and response contracts remain unchanged
- tests cover create-thread empty recipient behavior, reply participant behavior, mention filtering, dedupe, and error propagation
- docs and checkpoint reflect the new recipient policy

## 7. Risks And Guardrails

- Do not reintroduce workspace-wide fanout for thread events.
- Do not change thread endpoint request or response payloads in this task.
- Do not change legacy flat-comment recipient behavior here.
- Do not sort recipients arbitrarily; preserve deterministic first-seen order.
- Do not notify users who no longer belong to the workspace.
- Keep the resolver reusable for future outbox and mention-support tasks.

## 8. Follow-On Tasks

This plan prepares for:
- Task 17 thread-create outbox integration
- Task 18 thread-reply outbox integration
- Task 21 and Task 22 mention-aware thread payloads
