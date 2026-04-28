# Development Agent Rules

These rules apply to any AI working in this repository, including the main agent, sub-agents, review agents, and planning agents.

This repository is developed feature-by-feature with explicit scope, durable documentation, and architecture-aware changes.

## 1. Source Of Truth Order

Use repository knowledge in this order:

1. `PRD.md`
   - product intent
   - user-visible behavior
   - product rules
   - open product questions

2. `ARCHITECTURE.md`
   - system shape
   - layer boundaries
   - source-of-truth rules
   - consistency, concurrency, and async model
   - forbidden patterns

3. `docs/adr/`
   - accepted architectural decisions and rationale

4. roadmap and plan documents
   - execution sequencing
   - task scope
   - acceptance criteria
   - implementation details for the current work

5. current code, tests, and migrations
   - observed implemented behavior
   - current constraints
   - verification evidence

If a proposed change conflicts with `PRD.md`, `ARCHITECTURE.md`, or an accepted ADR, do not silently proceed. Call out the conflict explicitly.

If durable docs and observed implementation disagree, do not silently normalize either side. State the drift explicitly and determine which of these cases applies:
- preserve current implemented behavior
- align implementation to durable docs
- update durable docs to match accepted reality

If the correct direction is not clear from the task, stop and ask.


## 1.5 Workflow Routing

Use these shared workflow docs as the canonical workflow policy:

- `docs/workflow/ARTIFACT_DECISION_MATRIX.md`
- `docs/workflow/HANDOFF_CONTRACTS.md`
- `docs/workflow/CONCRETE_NEXT_STEP_CONTRACT.md`
- `docs/workflow/NEXT_STEP_TYPES.md`
- `docs/workflow/LIGHTWEIGHT_TASK_MODE.md`
- `docs/workflow/ARTIFACT_CONSISTENCY_REVIEW_CONTRACT.md`

Default to the normal durable-artifact workflow.

Use lightweight mode only when the task clearly passes the eligibility rules in `docs/workflow/LIGHTWEIGHT_TASK_MODE.md`.

If uncertain, do not use lightweight mode.

For non-trivial work, progress phase by phase:

- brainstorm-gate
- PRD when product behavior or product rules need clarification
- ARCHITECTURE when system shape, boundaries, data ownership, runtime flow, or integration rules need clarification
- ADR when one durable technical decision must be recorded
- ROADMAP when staged delivery or sequencing is required
- PLAN for one bounded implementation task
- IMPLEMENT
- REVIEW

Do not create every artifact every time. Choose the next artifact/action required by the current uncertainty.

## 1.6 Skill Usage

Use the repo workflow skills when the task is non-trivial, document-producing, architecture-sensitive, or review-oriented:

- `brainstorm-gate`
- `prd-writer`
- `architecture-writer`
- `adr-writer`
- `roadmap-planner`
- `plan-writer`
- `implement-task`
- `review-phase`

Use each skill for its own phase only. Do not collapse multiple phases into one step unless the decision matrix clearly allows it.

All workflow-phase outputs must end with exactly one `Concrete Next Step` block.

## 2. Delivery Order

- Implement one core feature or one tightly scoped fix at a time.
- Do not overlap unrelated feature work in one change unless the plan explicitly requires it.
- Prefer small, reviewable diffs that preserve existing architecture.
- Allow directly supporting work in the same change only when it is necessary to deliver the scoped task correctly, such as:
  - targeted refactors
  - test harness updates
  - contract adjustments
  - migrations
- Do not add unrelated cleanup, opportunistic refactors, or speculative groundwork.

## 3. Task Classification

Before starting work, classify the task into one of these:

### Lightweight task

Use lightweight mode only when all of these are true:

- one primary objective
- local and low-risk change
- preserves existing product behavior
- preserves existing architecture semantics
- no API contract change
- no schema or migration change
- no permission, role, visibility, or security behavior change
- no consistency, concurrency, idempotency, revision, notification, projection, or async behavior change
- no source-of-truth ownership change
- no ADR-worthy decision
- no roadmap sequencing needed
- validation is small and explicit

If any condition is unclear, use the normal workflow.

### Planned task

Use a full task plan when any of these are true:

- API behavior or payload shape changes
- schema, migration, or persistence semantics change
- permissions, roles, or visibility rules change
- consistency, concurrency, idempotency, or revision behavior change
- notification, projection, or async behavior change
- source-of-truth ownership is affected
- architecture boundaries are affected
- ADR-worthy decision may be involved
- work spans multiple layers with non-trivial coordination

A plan should cover one task only. If the work contains multiple primary objectives or unrelated validation paths, split it into multiple plans.

## 4. Layer Boundaries

Preserve these responsibilities:

- `transport`
  - routing
  - request parsing
  - auth extraction
  - HTTP mapping
  - response envelope shaping
  - must not contain product rules or SQL

- `application`
  - use-case orchestration
  - authorization checks
  - sequencing
  - cross-repository coordination
  - must not contain HTTP-specific behavior or raw SQL

- `domain`
  - stable types
  - enums
  - shared errors
  - lightweight invariants
  - must not perform orchestration or persistence

- `repository`
  - SQL
  - transaction boundaries
  - locking
  - persistence mapping
  - read-model access
  - must not contain HTTP concerns

- `infrastructure`
  - config
  - auth helpers
  - DB and bootstrap integrations
  - runtime wiring

If a change requires breaking one of these boundaries, call it out explicitly as an architecture change.

## 5. Engineering Rules

- Prefer extending existing patterns over introducing new parallel patterns.
- Use migrations for all schema changes. Never assume manual production schema edits.
- Preserve explicit source-of-truth boundaries. Do not treat projections, unread counters, SSE payloads, or response DTOs as canonical business state.
- Preserve documented consistency and concurrency rules.
- Preserve hide-as-not-found visibility behavior where it is part of the current contract.
- Do not introduce speculative abstractions, infrastructure, or generic frameworks.
- Do not convert an explicit repository rule into an implicit transport or application convention.

## 6. Planning Rules

When a plan is required:
- read the relevant `PRD.md`, `ARCHITECTURE.md`, ADR, and current plan sections first
- define the smallest implementation slice that preserves correctness
- include risks, assumptions, and non-goals
- prefer execution steps that can be verified incrementally
- separate required enabling work from optional cleanup

When a plan is not required:
- still state the local goal
- state the files or components likely to change
- state the verification steps before making changes


## 6.5 Execution Gate

Implementation should proceed only from one approved single-task plan when planning is required.
If the plan is missing, too broad, internally inconsistent, or blocked by unresolved upstream decisions, stop and surface the issue instead of guessing.
Do not implement multiple unrelated tasks in one execution step.

## 7. Testing And Verification

Every change must be verified at the right layer.

Minimum expectation:
- code compiles
- relevant tests pass
- changed behavior is covered by the appropriate test layer

Use these principles:
- schema or persistence changes require repository or integration coverage
- endpoint or contract changes require HTTP-level coverage
- concurrency-sensitive behavior requires conflict, idempotency, or sequencing coverage
- notification or read-model changes require source-of-truth and projection correctness checks where relevant
- bug fixes should include a regression test when practical

Do not stop at "tests pass." Also check:
- architecture fit
- source-of-truth correctness
- permission correctness
- consistency and concurrency implications
- documentation impact

Do not claim success without fresh verification evidence. If verification could not be run, say so plainly and state the gap.

## 8. Documentation Update Rules

Update durable docs when behavior or durable decisions change:

- update `PRD.md` when user-visible behavior, product rules, roles, or flows change
- update `ARCHITECTURE.md` when layer ownership, source-of-truth boundaries, async behavior, consistency guarantees, concurrency behavior, or transitional architecture changes
- update or add an ADR when a high-impact architectural decision is introduced, reversed, or materially refined
- update `API_CONTRACT.md` when endpoint behavior or payloads change
- update `checkpoint.md` after completing a feature slice or meaningful milestone that future work will rely on
- create or update roadmap docs when phased delivery structure, sequencing, or initiative scope changes
- create or update plan docs when the execution contract for the current single task changes

Do not update docs mechanically. Update only the documents materially affected by the change.

## 9. Review Standard

A task is not complete until the final output states:

1. what changed
2. why it changed
3. how it was verified
4. non-obvious assumptions that affected implementation
5. which durable docs were updated, if any

Review for:
- business requirement fulfillment
- plan and design alignment
- correctness
- architecture fit
- unnecessary complexity
- regression risk
- verification adequacy
- token-efficient clarity in written artifacts

For review-only tasks, prioritize findings first. Focus on correctness, risk, regressions, missing tests, and architecture violations before summaries or praise.

## 10. Ambiguity And Escalation

Stop and call out ambiguity when the task would change any of these without clear guidance:
- product rules
- data ownership
- auth or role semantics
- revision semantics
- notification semantics
- consistency model
- concurrency model
- forward architecture direction

Do not silently invent certainty in these areas.

If ambiguity is local, reversible, and does not affect the categories above, make the smallest safe assumption, state it explicitly, and proceed.

If ambiguity changes behavior, ownership, authority, or architectural direction, stop and ask.

## 11. Token Efficiency Rules

- Read the smallest relevant set of files first.
- Prefer durable repo docs over repeated prompt re-explanation.
- Cite the specific file or section that drives the decision instead of restating large amounts of context.
- Keep implementation notes concrete and tied to the current task only.
- Do not produce long narrative explanations when concise structured output is enough.
- Prefer one strong plan and one strong review over repeated speculative rewrites.
- Use sub-agents only when the task is clearly separable and benefits from scoped context, such as specialized review or bounded implementation work.
- Sub-agents should receive only the context needed for their slice, not full-session replay.
- Reuse existing conclusions unless new evidence changes them.

## 12. Main-Agent And Sub-Agent Rules

- These rules apply equally to the main agent and any delegated agent.
- A sub-agent must respect the same source-of-truth order, boundary rules, and verification standards as the main agent.
- A sub-agent should stay within its assigned scope. If it discovers a blocking issue outside that scope, it should report it rather than expanding the task on its own.
- A main agent must verify delegated work before claiming completion.
- Do not assume a sub-agent may skip planning, verification, or documentation rules just because its task is narrower.

## 13. Final Output Shape

For implementation work, final summaries should usually include:
- changed files or components
- behavior change
- verification performed
- remaining risks or follow-ups
- documentation updated, if any

Keep final output concise. Include enough context to support review and handoff, but do not turn the summary into a changelog.
