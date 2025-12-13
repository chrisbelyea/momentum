# Copilot Agents Operating Manual

## Purpose
Agents assist by turning the spec into Issues, scaffolding code, writing migrations, and reviewing PRs with human oversight.

## Guardrails
- Do not commit secrets or modify deployment credentials.
- Follow [requirements/specification.md](../requirements/specification.md) and [docs/design-overview.md](../docs/design-overview.md).
- Prefer standards-first solutions (CalDAV VTODO, TLS-only, secure storage).

## Conventions
- Branch names: `feature/<slug>`, `fix/<slug>`, `chore/<slug>`.
- Commits: concise subject, reference Issue (e.g., `#123`).
- Tests: include unit/integration where applicable.
- Docs: update spec/design/README when behavior changes.
- Apply appropriate labels to Issues/PRs (`type:*`, `area:*`, `priority:*`).
- Apply appropriate milestones to issues and pull requests. A pull request's milestone should match that of the issue it is implementing.
- Apply appropriate project board columns to issues and pull requests. A pull request's project board column should match that of the issue it is implementing.

## Workflows
- Spec → Issues (Agent-standardized):
	- Parse requirements/specification.md and vision.md to identify discrete work items.
	- Use Issue Forms (feature/bug/task) to create Issues with acceptance criteria.
	- Apply labels: `type:*`, `area:*`, `priority:*` and add cards to the GitHub Project board in `Todo`.
	- Set Phase milestones (Phase 1–3) based on the roadmap.
	- Cross-link spec sections in the Issue body for traceability.
- PR Review: ensure linked Issue, tests, docs, CI passing; enforce security and portability constraints.
- Liquibase: validate changeLogs in CI; include rollback where feasible; block merges on validation failures.

## Escalation
- If blocked by missing information or unsafe operations, open a `type:task` Issue labeled `blocked` and request human input.

## Agent Prompts (Canonical)
(refer to docs/agents/prompts.md for additional details)
- Create Issues from Spec:
	"Create GitHub Issues from requirements/specification.md for Phase 1. Use the Feature Issue Form, apply labels (type:feature, area:web/backend/sync, priority:P1), add cards to the Project board (Todo), and include acceptance criteria referencing spec sections."
- Sync Issues with Updated Spec:
	"Review changes in requirements/specification.md and update existing Issues accordingly. Add new Issues where needed, adjust acceptance criteria, and update Project board states."
- PR Creation:
	"Open a PR for the completed Issue #<id>, scaffold tests/docs, and ensure CI workflows pass (including Liquibase validate)."
