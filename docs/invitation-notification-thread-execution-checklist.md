# Invitation, Notification, And Thread Execution Checklist

## Purpose

This document is the implementation gate for the invitation, notification, and thread roadmap.

Use it before starting any coding work so the team does not:
- start a task before its prerequisites exist
- skip a foundational migration or repository change
- implement verification too early
- overlap core feature work

This checklist does not replace the task plans. It tells you:
- what to implement next
- what can wait
- what must stay blocked

## Current Start Point

The correct next implementation task is:
- Task 1: [2026-04-04-task-1-invitation-state-schema.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-1-invitation-state-schema.md)

Do not start any later task before Task 1 is complete.

## Strict Implementation Order

Implement in this order unless a later plan explicitly says it is only documentation or verification:

1. Task 1: [2026-04-04-task-1-invitation-state-schema.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-1-invitation-state-schema.md)
2. Task 2: [2026-04-04-task-2-post-workspace-invitations.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-2-post-workspace-invitations.md)
3. Task 3: [2026-04-04-task-3-get-workspace-invitations.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-3-get-workspace-invitations.md)
4. Task 4: [2026-04-04-task-4-get-my-invitations.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-4-get-my-invitations.md)
5. Task 5: [2026-04-04-task-5-patch-workspace-invitation.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-5-patch-workspace-invitation.md)
6. Task 6: [2026-04-04-task-6-post-workspace-invitation-accept.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-6-post-workspace-invitation-accept.md)
7. Task 7: [2026-04-04-task-7-post-workspace-invitation-reject.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-7-post-workspace-invitation-reject.md)
8. Task 8: [2026-04-04-task-8-post-workspace-invitation-cancel.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-8-post-workspace-invitation-cancel.md)
9. Task 9: [2026-04-04-task-9-notification-schema-v2.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-9-notification-schema-v2.md)
10. Task 10: [2026-04-04-task-10-outbox-foundation.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-10-outbox-foundation.md)
11. Task 11: [2026-04-04-task-11-invitation-notification-projector.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-11-invitation-notification-projector.md)
12. Task 12: [2026-04-04-task-12-get-notifications-inbox-v2.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-12-get-notifications-inbox-v2.md)
13. Task 13: [2026-04-04-task-13-post-notification-read.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-13-post-notification-read.md)
14. Task 14: [2026-04-04-task-14-get-notification-unread-count.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-14-get-notification-unread-count.md)
15. Task 15: [2026-04-04-task-15-post-notifications-read-batch.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-15-post-notifications-read-batch.md)
16. Task 16: [2026-04-04-task-16-thread-notification-recipient-resolver.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-16-thread-notification-recipient-resolver.md)
17. Task 17: [2026-04-04-task-17-post-page-threads-outbox-integration.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-17-post-page-threads-outbox-integration.md)
18. Task 18: [2026-04-04-task-18-post-thread-replies-outbox-integration.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-18-post-thread-replies-outbox-integration.md)
19. Task 19: [2026-04-04-task-19-comment-notification-projector.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-19-comment-notification-projector.md)
20. Task 20: [2026-04-04-task-20-mention-schema.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-20-mention-schema.md)
21. Task 21: [2026-04-04-task-21-post-page-threads-mention-support.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-21-post-page-threads-mention-support.md)
22. Task 22: [2026-04-04-task-22-post-thread-replies-mention-support.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-22-post-thread-replies-mention-support.md)
23. Task 23: [2026-04-04-task-23-mention-notification-projector.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-23-mention-notification-projector.md)
24. Task 24: [2026-04-04-task-24-get-thread-notification-preference.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-24-get-thread-notification-preference.md)
25. Task 25: [2026-04-04-task-25-put-thread-notification-preference.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-25-put-thread-notification-preference.md)
26. Task 26: [2026-04-04-task-26-get-notifications-stream.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-26-get-notifications-stream.md)
27. Task 27: [2026-04-04-task-27-notification-reconciliation-job.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-27-notification-reconciliation-job.md)
28. Task 28: [2026-04-04-task-28-concurrency-and-load-verification.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-28-concurrency-and-load-verification.md)

## Phase Gates

### Gate 1: Invitation Lifecycle Complete

You may not start notification schema or outbox work until Tasks 1 through 8 are done.

Completion means:
- invitation schema is upgraded
- create, list, update, accept, reject, and cancel all exist
- version conflict behavior is stable

### Gate 2: Notification Foundation Complete

You may not start thread notification migration work until Tasks 9 through 15 are done.

Completion means:
- notification v2 schema exists
- outbox exists
- invitation live projection exists
- inbox read APIs exist
- unread count and mark-read paths are stable

### Gate 3: Thread Comment Notification Path Complete

You may not start mention delivery work until Tasks 16 through 19 are done.

Completion means:
- relevant-user policy is defined
- thread create and reply write to outbox
- comment projector is stable

### Gate 4: Mention Path Complete

You may not start advanced control and recovery work until Tasks 20 through 23 are done.

Completion means:
- explicit mention storage exists
- thread create and reply persist mentions
- mention projector is stable

### Gate 5: Advanced Features Complete

Only after Tasks 24 through 27 are done should you run the final verification slice in Task 28.

This is the recommended order even though some Task 28 checks can technically run earlier.

## What To Ignore Until Later

### Ignore Tasks 9 Through 28 While Implementing Task 1 Through Task 8

Do not start:
- notification schema changes
- outbox work
- projector work
- unread-count work
- thread notification work

Reason:
- the invitation source-of-truth model is still moving

### Ignore Tasks 16 Through 28 While Implementing Task 9 Through Task 15

Do not start:
- relevant-user resolver
- thread outbox work
- comment projector
- mentions
- stream
- reconciliation
- final concurrency suite

Reason:
- inbox and unread-count read model must stabilize first

### Ignore Tasks 20 Through 28 While Implementing Task 16 Through Task 19

Do not start:
- mention persistence
- mention projector
- thread notification preferences
- SSE stream
- reconciliation
- final verification

Reason:
- comment projection is the base thread-notification path

### Ignore Tasks 24 Through 28 While Implementing Task 20 Through Task 23

Do not start:
- thread notification preferences
- SSE stream
- reconciliation
- final verification

Reason:
- mention delivery must stabilize before advanced controls and recovery

### Ignore Task 28 Until The End

Task 28 is a verification gate, not a base feature.

Do not pull it earlier just because:
- some prerequisites already exist
- you want early benchmarks
- you want early concurrency tests

Reason:
- the most useful version of Task 28 validates the final integrated system

## Special Ordering Notes

### Task 12 Before Task 14

Keep Task 12 before Task 14.

Reason:
- Task 12 establishes the inbox v2 DTO and list contract
- Task 14 adds the standalone unread-count endpoint and counter maintenance

### Task 16 Before Task 17 And Task 18

Keep Task 16 first.

Reason:
- it locks the relevant-user policy before outbox-driven thread notification work starts

### Task 24 And Task 25 Before Task 26

Keep the roadmap order.

Reason:
- thread-level controls are part of the advanced notification feature set
- this avoids mixing real-time delivery work with preference semantics in the same implementation window

### Task 27 Before Task 28

Keep reconciliation before the final verification pass.

Reason:
- Task 27 now publishes best-effort invalidation after repair changes
- Task 28 is most valuable after the full notification lifecycle is present

## Implementation Rule Per Task

For every task:

1. Read the task plan again before coding.
2. Verify all listed prerequisites are already implemented.
3. Implement only that task's scope.
4. Run only the verification commands from that task plan.
5. Update:
   - `docs/checkpoint.md`
   - `frontend-repo/API_CONTRACT.md` when the task changes endpoint behavior
6. Stop and review before moving to the next task.

## Immediate Next Step

Start with:
- Task 1: [2026-04-04-task-1-invitation-state-schema.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-1-invitation-state-schema.md)

After Task 1 is complete, the next allowed task is:
- Task 2: [2026-04-04-task-2-post-workspace-invitations.md](/d:/Project/ProjectGoLang/note-app/docs/superpowers/plans/2026-04-04-task-2-post-workspace-invitations.md)
