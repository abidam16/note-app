# Development Agent Rules

This repository is developed feature-by-feature with strict sequencing and explicit boundaries.

## Delivery Order
- Implement one feature at a time with no overlapping core feature work.
- Finish backend work before starting frontend work.
- Follow the roadmap in `docs/backend-feature-roadmap.md` as the canonical implementation order.
- Confirm the current feature scope before coding if requirements are ambiguous.

## Engineering Standards
- Prefer official Go documentation and official documentation for adopted dependencies.
- Use clean architecture boundaries: `transport`, `application`, `domain`, `repository`, `infrastructure`.
- Follow clean code practices: small focused functions, explicit naming, no dead code, no hidden side effects.
- Apply Go best practices: context-aware I/O, structured errors, table-driven tests where appropriate, and dependency injection through interfaces only when it reduces coupling.
- Use database migrations for every schema change. Never edit production schema manually.
- Innovate inside the accepted product scope only. Do not introduce speculative features or architecture.

## Feature Workflow
- Each feature must define: objective, API contract, data model impact, validation rules, tests, and acceptance criteria before implementation.
- Never develop multiple core features in parallel.
- Stop and ask questions if a requirement changes domain behavior, data ownership, auth rules, or revision semantics.

## Communication
- After each completed feature, explain what changed, why it changed, how it was verified, and which assumptions were used.
- Keep implementation notes concrete and tied to the current feature only.