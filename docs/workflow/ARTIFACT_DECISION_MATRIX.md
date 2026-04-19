# ARTIFACT_DECISION_MATRIX.md

Purpose: define the routing rules between workflow artifacts so each phase ends with one clear next step and minimal ambiguity.

---

## 1. Core Rule

At any point in the workflow, choose the **single next artifact** that best resolves the current uncertainty or execution need.

Do not create multiple new artifacts in one step unless explicitly required.

Preferred workflow:

1. Brainstorm
2. PRD or ADR if needed
3. Roadmap if needed
4. Plan
5. Implementation
6. Review

---

## 2. Artifact Roles

### Brainstorm Output
Used to:
- clarify the problem or opportunity
- test whether the idea is worth pursuing
- decide the next artifact

Brainstorm should not automatically generate every downstream artifact.

### PRD
Used to define:
- product intent
- goals
- non-goals
- users/roles
- flows
- product rules
- current vs target behavior
- success criteria

### ADR
Used to define:
- one meaningful technical/architectural decision
- options considered
- chosen option
- rationale
- consequences

### Roadmap
Used to define:
- staged delivery structure
- sequencing
- milestones/phases
- dependencies
- risks
- exit criteria

### Plan
Used to define:
- one bounded task only
- exact implementation scope
- detailed spec
- validation/tests
- review expectations

### Implementation
Used to:
- execute one approved plan
- stay within scope
- report deviations and blockers

### Review
Used to:
- judge business alignment
- judge plan/design alignment
- judge technical quality
- decide acceptance / revision / blockage

---

## 3. Top-Level Routing Order

When deciding the next artifact, use this order:

1. Reject / defer?
2. Need product intent clarified or changed?
3. Need a technical/architectural decision recorded?
4. Need staged delivery structure?
5. Need one-task execution contract?
6. Need implementation?
7. Need review?

This ordering prevents jumping into roadmap or plan when product or architecture is still unclear.

---

## 4. Brainstorm Routing Rules

## Choose `REJECT_OR_DEFER` when:
- the problem is weak or unclear
- the value is too low
- the signal is too speculative
- the idea does not justify immediate action
- required evidence is missing

## Choose `NEW_PRD` when:
- product intent does not yet exist in durable form
- the idea defines a new product or major new capability
- user-facing behavior, goals, or scope must be established

## Choose `PRD_UPDATE` when:
- product intent exists, but parts of it changed
- goals, non-goals, flows, roles, rules, or success criteria need adjustment
- the change affects product behavior or business intent

## Choose `NEW_ADR` when:
- the next blocking question is a significant technical decision
- the main issue is architectural choice, not product scope
- the decision has lasting downstream impact

## Choose `ADR_UPDATE` only when:
- your ADR practice explicitly allows updates in place for non-historic corrections
- otherwise prefer a new ADR that supersedes the old one

Preferred default:
- create a **new ADR** and mark older ADRs as superseded instead of rewriting history

## Choose `NEW_PRODUCT_ROADMAP` when:
- strategic product direction is already clear enough
- the next need is high-level staged product evolution
- no suitable product roadmap exists yet

## Choose `PRODUCT_ROADMAP_UPDATE` when:
- a product-level roadmap already exists
- strategic sequencing, priorities, or major phases changed

## Choose `NEW_INITIATIVE_ROADMAP` when:
- the work is a distinct initiative
- product intent is already clear enough
- the next need is phased delivery for one feature/refactor/migration/capability
- no suitable initiative roadmap exists yet

## Choose `INITIATIVE_ROADMAP_UPDATE` when:
- an initiative roadmap already exists
- phase structure, sequencing, dependencies, risks, or exit criteria changed

### Brainstorm constraint
Brainstorm must end with **exactly one** final routing decision.

---

## 5. PRD Routing Rules

## Create a PRD when:
- the problem, users, goals, or behavior must be defined for the first time
- the work changes product truth materially

## Update a PRD when:
- product truth already exists
- only part of the product truth changed
- the document remains fundamentally valid

## Do not use PRD when:
- the issue is only technical architecture
- the issue is only delivery sequencing
- the issue is only one-task implementation detail

## After PRD, the next step is usually:
- roadmap creation/update, if delivery sequencing is needed
- ADR, if a major technical choice is still unresolved
- stop, if the PRD is being refined but execution should not proceed yet

---

## 6. ADR Routing Rules

## Create a new ADR when:
- there is one meaningful technical decision
- multiple credible options exist
- the decision has important consequences
- future readers will need the rationale

## Prefer a new ADR over updating an old ADR when:
- the actual decision changed
- the old ADR would become historically misleading if rewritten
- the new decision supersedes the old one

## Do not use ADR when:
- the question is product scope or business behavior
- the question is milestone sequencing
- the question is one-task implementation detail

## After ADR, the next step is usually:
- roadmap, if the decision enables staged delivery
- plan, if the decision directly unlocks one bounded task
- PRD update, if the decision changes product assumptions materially

---

## 7. Roadmap Routing Rules

## Choose `PRODUCT_ROADMAP` when:
- the document is strategic
- it tracks product evolution across multiple initiatives
- it acts as a lightweight index of active/completed/deferred initiative streams

## Choose `INITIATIVE_ROADMAP` when:
- the roadmap is for one specific feature, refactor, migration, capability, or major improvement
- this roadmap is the main bridge into planning

Preferred default for real execution:
- use **initiative roadmap**

## Create a new roadmap when:
- the work is a new initiative
- the objective is materially different
- the work deserves its own phased delivery structure
- merging into an existing roadmap would reduce clarity

## Update an existing roadmap when:
- the work is part of the same initiative
- the same roadmap remains structurally valid
- only phase order, scope, risk, dependency, or exit criteria changed

## Do not use roadmap when:
- product intent is still unclear
- a major technical decision is still unresolved
- the next work is just one bounded task with no need for phased structure

## After roadmap, the next step is:
- generate one or more **single-task plans**

---

## 8. Plan Routing Rules

## Create a new plan when:
- one bounded task is ready for execution
- no suitable plan exists yet

## Update an existing plan when:
- the same task still exists
- only details, scope boundaries, files, tests, or validation changed
- the plan remains a single coherent task

## Split instead of update when:
- the plan has more than one primary objective
- the scope crosses multiple unrelated behaviors
- the review criteria are not singular
- the work would naturally produce multiple independent commit themes
- the validation paths are separate enough to deserve distinct tasks

### Hard rule
One plan document must cover **one task only**.

## After plan, the next step is:
- implementation of that one task

---

## 9. Implementation Routing Rules

## Proceed to implementation when:
- one approved plan exists
- scope is bounded
- expected files/components are known
- validation/test expectations are clear

## Do not proceed to implementation when:
- the plan is internally inconsistent
- the task is too broad
- product intent is still unresolved
- a blocking technical decision is still missing

## During implementation:
- follow the plan
- do not silently expand scope
- do not silently deviate
- stop and report blockers if safe implementation is not possible

## After implementation, the next step is:
- review

---

## 10. Review Routing Rules

## Choose `TASK_REVIEW` when:
- reviewing one implementation against one `PLAN.md`

## Choose `ROADMAP_IMPLEMENTATION_REVIEW` when:
- reviewing the combined implementation status of multiple tasks under one roadmap
- checking cross-task gaps, integration issues, or roadmap fulfillment

## Review must assess:
- business alignment
- plan/design alignment
- technical quality
- tests/validation
- integration impact
- risks
- next actions

## Review output status
Use one:
- `APPROVED`
- `APPROVED_WITH_MINOR_IMPROVEMENTS`
- `NEEDS_REVISION`
- `BLOCKED`

---

## 11. Create vs Update Summary

### Create new artifact when:
- there is no suitable existing artifact
- the scope/objective is materially new
- reusing an old artifact would reduce clarity
- historical continuity matters

### Update existing artifact when:
- the existing artifact still represents the same underlying object
- only part of it changed
- updating preserves clarity better than creating a new one

### Prefer new over update when:
- the change would rewrite history in a confusing way
- the objective materially changed
- the old artifact should remain as historical record

---

## 12. Escalation Rules

Stop and escalate instead of proceeding when:
- the chosen artifact is still unclear after analysis
- two artifact types seem equally necessary but one depends on the other
- product intent and technical decision are both unresolved
- the current phase would need to invent upstream decisions
- the scope is too broad for the target artifact

Use this escalation order:
1. clarify product intent first
2. clarify technical decision second
3. sequence delivery third
4. define one-task execution fourth

---

## 13. Minimum Next-Step Requirement

Every phase must end with:

- `Current Decision`
- `Why this is the correct artifact`
- `What is explicitly not next`
- `Immediate Next Step`

No phase should end with only analysis and no routing.

---

## 14. Default Workflow Patterns

### Pattern A: New product or major feature
Brainstorm → PRD → Roadmap → Plan → Implementation → Review

### Pattern B: Existing product change affecting behavior
Brainstorm → PRD Update → Roadmap Update or New Initiative Roadmap → Plan → Implementation → Review

### Pattern C: Pure technical architecture decision
Brainstorm → ADR → Roadmap or Plan → Implementation → Review

### Pattern D: Existing initiative change
Brainstorm → Initiative Roadmap Update → Plan → Implementation → Review

### Pattern E: Weak or premature idea
Brainstorm → Reject / Defer

---

## 15. Portability Rule

This matrix is workflow-generic.

It should work across:
- frontend
- backend
- platform/infrastructure
- finance systems
- healthcare systems
- internal tools
- general product applications

Domain-specific rules should be layered separately through:
- `AGENTS.md`
- nested `AGENTS.md`
- domain-specific checklists
- domain-specific skills

Do not overload this matrix with domain-specific regulations.