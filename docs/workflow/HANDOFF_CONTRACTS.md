# HANDOFF_CONTRACTS.md

Purpose: define the minimum required input/output fields between workflow phases so each skill can hand off cleanly to the next phase with low ambiguity and controlled token usage.

---

## 1. Core Rule

Each phase should pass forward only the **minimum structured payload** the next phase needs.

Do not pass the entire history if a compact handoff is enough.

Every handoff must include:
- artifact type
- artifact status
- decision/routing
- core rationale
- references to upstream artifacts
- open issues that materially affect the next phase

---

## 2. Standard Field Conventions

Use these field meanings consistently.

### `artifact_type`
Examples:
- `BRAINSTORM_OUTPUT`
- `PRD`
- `PRD_DELTA`
- `ADR`
- `ROADMAP`
- `ROADMAP_DELTA`
- `PLAN`
- `PLAN_DELTA`
- `IMPLEMENTATION_SUMMARY`
- `TASK_REVIEW_REPORT`
- `ROADMAP_REVIEW_REPORT`

### `artifact_status`
Examples:
- `DRAFT`
- `APPROVED`
- `UPDATED`
- `REJECTED`
- `DEFERRED`
- `SUPERSEDED`
- `BLOCKED`

### `decision`
The explicit routing or acceptance decision for that phase.

### `source_artifacts`
List of upstream artifacts used as source of truth.

### `open_questions`
Only unresolved questions that materially affect the next phase.

### `constraints`
Only constraints that materially affect the next phase.

### `next_step`
Exactly one immediate next step.

---

## 3. Global Handoff Policy

### Required in every handoff
- `artifact_type`
- `artifact_status`
- `decision`
- `why`
- `source_artifacts`
- `next_step`

### Optional
- `open_questions`
- `constraints`
- `risks`
- `deferred_items`
- `follow_up_needed`

### Do not pass forward unless needed
- full exploratory narrative
- duplicated product background
- repeated roadmap prose
- repeated implementation detail already captured in plan
- low-importance observations

---

## 4. Brainstorm → PRD

Use when brainstorm decides:
- `NEW_PRD`
- `PRD_UPDATE`

### Required output from brainstorm
- `artifact_type: BRAINSTORM_OUTPUT`
- `decision: NEW_PRD | PRD_UPDATE`
- `problem_statement`
- `target_users_or_actors`
- `business_need`
- `product_intent_summary`
- `goals`
- `non_goals`
- `key_flows_or_domains`
- `known_constraints`
- `reason_prd_is_needed`
- `source_artifacts`
- `next_step`

### Consumed by PRD writer
- problem and user context
- product intent
- goals / non-goals
- affected flows/domains
- constraints
- why create vs update

### Not required
- roadmap phases
- plan-level detail
- implementation file lists

---

## 5. Brainstorm → ADR

Use when brainstorm decides:
- `NEW_ADR`
- `ADR_UPDATE` if your practice allows it

### Required output from brainstorm
- `artifact_type: BRAINSTORM_OUTPUT`
- `decision: NEW_ADR | ADR_UPDATE`
- `decision_scope`
- `technical_problem_statement`
- `why_this_is_technical_not_product`
- `decision_drivers`
- `credible_options_if_known`
- `known_constraints`
- `source_artifacts`
- `next_step`

### Consumed by ADR writer
- decision boundary
- technical context
- drivers
- constraints
- why ADR is the correct artifact

### Not required
- full PRD structure
- full roadmap structure
- plan/task detail

---

## 6. Brainstorm → Roadmap

Use when brainstorm decides:
- `NEW_PRODUCT_ROADMAP`
- `PRODUCT_ROADMAP_UPDATE`
- `NEW_INITIATIVE_ROADMAP`
- `INITIATIVE_ROADMAP_UPDATE`

### Required output from brainstorm
- `artifact_type: BRAINSTORM_OUTPUT`
- `decision`
- `initiative_or_product_scope`
- `delivery_objective`
- `why_roadmap_is_needed_now`
- `known_dependencies`
- `known_risks`
- `known_constraints`
- `whether_prd_is_already_sufficient`
- `source_artifacts`
- `next_step`

### Consumed by roadmap planner
- scope of roadmap
- delivery objective
- dependency/risk signals
- create vs update basis
- product vs initiative roadmap mode

### Not required
- single-task detail
- code file expectations
- full implementation behavior

---

## 7. PRD → Roadmap

Use when PRD is complete enough and the next need is delivery sequencing.

### Required output from PRD writer
- `artifact_type: PRD | PRD_DELTA`
- `artifact_status`
- `decision: PROCEED_TO_ROADMAP | HOLD`
- `product_overview_summary`
- `current_goals`
- `current_non_goals`
- `key_roles_and_flows`
- `product_rules`
- `current_behavior_summary`
- `target_behavior_summary`
- `success_criteria`
- `important_open_questions`
- `roadmap_implications`
- `source_artifacts`
- `next_step`

### Consumed by roadmap planner
- product truth
- delivery implications
- areas requiring staged work
- open questions that may affect sequencing

### Not required
- task-level implementation detail
- low-level design alternatives

---

## 8. ADR → Roadmap

Use when a technical decision is accepted and staged delivery is needed.

### Required output from ADR writer
- `artifact_type: ADR | ADR_DELTA`
- `artifact_status`
- `decision: PROCEED_TO_ROADMAP | PROCEED_TO_PLAN | HOLD`
- `decision_title`
- `context_summary`
- `decision_drivers`
- `chosen_option`
- `key_consequences`
- `scope_and_impact`
- `non_goals_or_not_addressed`
- `downstream_delivery_implications`
- `source_artifacts`
- `next_step`

### Consumed by roadmap planner
- decision and consequences
- scope/impact
- constraints that shape sequencing
- whether roadmap should exist at all

---

## 9. Roadmap → Plan

Use when one roadmap phase or slice is ready to become an executable task.

### Required output from roadmap planner
- `artifact_type: ROADMAP | ROADMAP_DELTA`
- `artifact_status`
- `decision: PROCEED_TO_PLAN`
- `roadmap_mode: PRODUCT | INITIATIVE`
- `selected_phase_or_slice`
- `phase_objective`
- `why_this_slice_is_next`
- `in_scope_for_this_slice`
- `out_of_scope_for_this_slice`
- `dependencies`
- `risks`
- `exit_criteria`
- `plan_handoff_candidates`

### `plan_handoff_candidates` format
For each candidate task:
- `task_name`
- `task_objective`
- `why_it_is_one_task`
- `scope_boundary`
- `expected_components_or_layers`
- `validation_direction`

### Consumed by plan writer
- one selected roadmap slice
- one candidate task
- scope boundary
- validation direction

### Not required
- full roadmap history
- all phases if only one task is being planned

---

## 10. Plan → Implementation

Use when one single-task plan is approved.

### Required output from plan writer
- `artifact_type: PLAN | PLAN_DELTA`
- `artifact_status`
- `decision: PROCEED_TO_IMPLEMENTATION | SPLIT_REQUIRED | HOLD`
- `task_summary`
- `objective`
- `in_scope`
- `out_of_scope`
- `detailed_spec`
- `expected_changes`
- `must_not_change`
- `validation_requirements`
- `test_requirements`
- `review_checkpoints`
- `tradeoffs_and_risks`
- `future_improvements`
- `source_artifacts`
- `next_step`

### Consumed by implement-task
- complete single-task execution contract
- exact scope boundaries
- validation/test obligations
- file/component boundaries

### Hard rule
If `decision != PROCEED_TO_IMPLEMENTATION`, implementation must not start.

---

## 11. Implementation → Review

Use when one approved task has been implemented or implementation was blocked.

### Required output from implement-task
- `artifact_type: IMPLEMENTATION_SUMMARY`
- `artifact_status: COMPLETED | BLOCKED | PARTIAL`
- `decision: READY_FOR_REVIEW | BLOCKED`
- `plan_reference`
- `objective_restatement`
- `scope_followed_summary`
- `files_changed`
- `files_not_changed`
- `implementation_summary`
- `validation_done`
- `tests_run_or_updated`
- `deviations`
- `blockers`
- `remaining_gaps`
- `self_check_result`
- `next_step`

### `deviations` format
For each deviation:
- `severity: HIGH | MEDIUM | LOW`
- `what_changed_from_plan`
- `why`
- `impact`
- `whether_plan_update_is_needed`

### Consumed by review phase
- implementation summary
- deviations/blockers
- self-check result
- plan reference
- evidence of validation/tests

---

## 12. Plan → Review (Task Review Inputs)

Task review should not rely on implementation summary alone.

### Required review inputs
- `PLAN`
- `IMPLEMENTATION_SUMMARY`
- relevant code/diff/tests
- relevant roadmap slice if needed
- relevant PRD/ADR context if needed

### Required source-of-truth order
1. `PLAN`
2. roadmap slice
3. PRD/ADR sections if relevant
4. implementation output
5. validation evidence

---

## 13. Roadmap → Review (Roadmap Implementation Review Inputs)

Use for cross-task or initiative-level review.

### Required inputs
- `ROADMAP`
- set of relevant `PLAN` artifacts
- set of relevant `IMPLEMENTATION_SUMMARY` artifacts
- relevant PRD/ADR context
- integration evidence if available

### Required review focus
- roadmap fulfillment
- cross-task gaps
- integration risk
- business fulfillment across the initiative
- sequencing or dependency issues
- follow-up tasks needed

---

## 14. Review Outputs

## Task Review output
### Required fields
- `artifact_type: TASK_REVIEW_REPORT`
- `artifact_status`
- `decision: APPROVED | APPROVED_WITH_MINOR_IMPROVEMENTS | NEEDS_REVISION | BLOCKED`
- `review_scope`
- `source_artifacts`
- `business_alignment_assessment`
- `plan_alignment_assessment`
- `technical_quality_assessment`
- `validation_and_test_assessment`
- `findings`
- `risk_assessment`
- `recommended_next_actions`
- `next_step`

## Roadmap Review output
### Required fields
- `artifact_type: ROADMAP_REVIEW_REPORT`
- `artifact_status`
- `decision: APPROVED | APPROVED_WITH_MINOR_IMPROVEMENTS | NEEDS_REVISION | BLOCKED`
- `review_scope`
- `source_artifacts`
- `roadmap_fulfillment_assessment`
- `business_alignment_assessment`
- `cross_task_alignment_assessment`
- `technical_quality_summary`
- `findings`
- `risk_assessment`
- `recommended_next_actions`
- `next_step`

### `findings` format
For each finding:
- `severity: HIGH | MEDIUM | LOW`
- `category`
- `title`
- `description`
- `why_it_matters`
- `recommended_action`

---

## 15. Minimal Carry-Forward Rules

To control token usage, pass only:

### From brainstorm
- decision
- rationale
- key problem/intent/constraints

### From PRD
- product truth summary
- roadmap implications
- open questions that materially matter

### From ADR
- chosen decision
- drivers
- consequences
- downstream impact

### From roadmap
- selected phase/slice
- why it is next
- task candidate boundary

### From plan
- exact task execution contract

### From implementation
- what changed
- what deviated
- what was validated
- blockers/gaps

### From review
- verdict
- major findings
- next actions

---

## 16. Stop / Escalation Contract

Any phase may stop and escalate instead of handing off forward.

### Required stop fields
- `artifact_type`
- `artifact_status: BLOCKED | DEFERRED`
- `decision`
- `why_forward_progress_should_stop`
- `what_is_missing_or_conflicting`
- `recommended_resolution`
- `next_step`

No stop condition should end without a recommended resolution.

---

## 17. Portability Rule

These handoff contracts are workflow-generic.

They are intended to work across:
- frontend
- backend
- platform/infrastructure
- internal tools
- healthcare systems
- finance systems
- general product applications

Domain-specific additions should be layered separately through:
- `AGENTS.md`
- nested `AGENTS.md`
- domain-specific review checklists
- domain-specific skills

Do not overload these generic handoff contracts with domain-specific regulation content.