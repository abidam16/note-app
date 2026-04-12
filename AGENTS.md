# Development Agent Rules

This repository is developed feature-by-feature with strict sequencing and explicit boundaries.

## Delivery Order
- Implement one feature at a time with no overlapping core feature work.
- Confirm the current feature scope before coding if requirements are ambiguous.

## Engineering Standards
- Prefer official Go documentation and official documentation for adopted dependencies.
- Use clean architecture boundaries: `transport`, `application`, `domain`, `repository`, `infrastructure`.
- Follow clean code practices: small focused functions, explicit naming, no dead code, no hidden side effects.
- Use DRY, YAGNI, SOLID, and TDD principles while developing a feature
- Apply Go best practices: context-aware I/O, structured errors, table-driven tests where appropriate, and dependency injection through interfaces only when it reduces coupling.
- Use database migrations for every schema change. Never edit production schema manually.
- Always consider performance : time and memory complexity, I/O bound, race condition.
- Innovate inside the accepted product scope only. Do not introduce speculative features or architecture.

## Feature Workflow
- Each feature must define: objective, API contract, data model impact, validation rules, tests, and acceptance criteria before implementation.
- Never develop multiple core features in parallel.
- Stop and ask questions if a requirement changes domain behavior, data ownership, auth rules, or revision semantics.

## Communication
- Always use the most suitable available skill to solve the problem. Aim to give the most high quality code, document, or explanation.
- Always aim for token efficiency when do or implement a work/task but still maintaining the quality of the work/task. For the analysis or research use latest model with high accuracy and extra high reasoning effort. For the simple task or code use cheaper model or the mini version (if any) delegated to sub-agent (sub-agent drivent development). You have to maintain high standard quality of the code and document. When you delegate the task/work to lower model or lower reasoning effort, always review and give a feedback if something is not right or can be improved.
- Always analyze, understand, and plan the best strategy to implement the requirement to code. Aim for high quality of code fulfilling the function, non-functional, and security.
- Always : Ask something if it's not clear. Innovate if any other better way.
- Once you identified the problem/task definition, create a plan how you execute your idea and why you use that approach.
- If the new problem affect the PRD or next plan, update the PRD and plan a strategy for the next task.
- If the new problem or next task is too big for one implementation phase, analyze a clear roadmap to identify step-by-step feasible task, then write it.
- After each completed feature, explain what changed, why it changed, how it was verified, and which assumptions were used.
- Keep implementation notes concrete and tied to the current feature only.
- Always update checkpoint.md after each completed feature and API_CONTRACT.md if it affects the endpoint behavior.
- Always write a plan using writing-plan skills for each task in roadmap execution.
- When execute a plan and if the subagent-driven development can produce better result, implement that using cheaper model with sufficient skill to produce high quality code or document.
- Always review code so it not only pass the test but also high quality of code.
- Always review all written document so it produce high quality document with clear and concise document with sufficient context and detail that understanded other human or AI agent.