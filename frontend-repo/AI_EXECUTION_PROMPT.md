# Frontend Execution Prompt

You are implementing the invitation, notification, comment/mention, and thread-notification-preference UX for this app.

Read in this order before coding:
1. `frontend-repo/FRONTEND_HANDOFF.md`
2. `frontend-repo/IMPLEMENTATION_SEQUENCE.md`
3. `frontend-repo/API_DELTA_NOTIFICATION_INVITATION.md`
4. `frontend-repo/UI_STATE_MATRIX.md`
5. `frontend-repo/mocks/`
6. `frontend-repo/API_CONTRACT.md`
7. `frontend-repo/CONTEXT.md` if you need extra product context

Treat the handoff pack as the primary guidance and `frontend-repo/API_CONTRACT.md` as the detailed reference. Do not invent backend behavior outside these docs.

REST endpoints are canonical. Use SSE only for freshness and invalidation.

Implementation rules:
- Preserve one live notification card per invitation.
- Treat comment and mention notifications as append-only.
- Use the latest invitation notification `payload.version` when triggering accept or reject from the inbox.
- Surface clear UI for unread/read state and stale `409` invitation conflicts.
- If existing UI patterns conflict with the handoff pack, stop and ask for clarification before diverging.
