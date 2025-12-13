# momentum
Momentum brings your to-dos, reminders, and CalDAV tasks into one flow — visible as lists or boards. Designed for people who want to see their life in motion.

## Documentation
- Vision: [requirements/vision.md](requirements/vision.md)
- Specification: [requirements/specification.md](requirements/specification.md)
- Features: [requirements/feature-list.md](requirements/feature-list.md)
- Design Overview: [docs/design-overview.md](docs/design-overview.md)
- VTODO Mapping: [docs/vtodo-mapping.md](docs/vtodo-mapping.md)

## Implementing with Copilot Agents and GitHub
- Use Copilot Agents to turn spec items into actionable tasks:
	- Create GitHub Issues for each feature or section of the spec.
	- Organize work in GitHub Projects (Boards) reflecting Momentum’s kanban columns.
	- Use Pull Requests for implementation with linked Issues; require reviews and CI checks.
- Suggested workflow:
	1. Break down [requirements/specification.md](requirements/specification.md) into scoped Issues with acceptance criteria.
	2. Track progress in a GitHub Project (kanban) with columns (Todo/In Progress/Review/Done).
	3. Open PRs referencing Issues; Copilot Agents can scaffold code, tests, and docs.
	4. Merge via protected branches; CI runs lint/tests and (later) packaging.

## CI/CD and Tooling (Overview)
- Database migrations: Liquibase (recommended) or Flyway.
- Initial CI: linting, unit/integration tests, security checks.
- Packaging plans: single-executable server, PWA build, native client artifacts.
