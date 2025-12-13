# Copilot Instructions

## Project Context
- Momentum is a kanban-focused task manager using CalDAV VTODOs as default storage; multi-backend, SaaS + self-hosted.

## Implementation Preferences
- Web: prefer htmx for progressive enhancement.
- Server: single-executable, simple config, RDBMS (SQLite default, PostgreSQL optional).
- Security: TLS-only, OS keychains for client secrets, encrypted server storage.

## Style & Structure
- Shared core logic; platform-native clients.
- Keep changes minimal and focused; avoid over-engineering.
- Update documentation alongside code changes.
- Follow consistent naming conventions and file organization as established in the codebase.

## Testing Expectations
- Unit/integration tests for core logic and sync behaviors.
- Liquibase validate must pass in CI for DB changes.
- Test coverage should include edge cases and error conditions.
- Prefer table-driven tests for multiple scenarios.

## Database Migrations
- Use Liquibase for all schema changes.
- Each changeset must include forward migration and rollback where feasible.
- Run `liquibase validate` before committing changes.
- Reference Issue numbers in changeset comments.

## Build & Run (when applicable)
- Server: Single-executable binary; config via environment or config file.
- Development: Use scripts in `/scripts` directory for common tasks.
- Database: SQLite for local dev; PostgreSQL for production testing.

## Common Workflows
- **New Feature**: Create Issue → Branch (`feature/<slug>`) → Implement → Tests → Docs → PR → Review → Merge.
- **Bug Fix**: Create Issue → Branch (`fix/<slug>`) → Fix → Tests → PR → Review → Merge.
- **Database Change**: Draft Liquibase changeset → Validate → Test migrations → PR with rollback plan.
- **Integration**: Follow provider-specific auth patterns; use OS keychain for secrets.

## Security Guidelines
- Never commit secrets, tokens, or credentials.
- Use TLS 1.3+ for all network connections.
- Store credentials in OS keychain (client-side) or encrypted at rest (server-side).
- Validate and sanitize all external inputs.
- Follow least-privilege principles for access control.

## Agent-Specific Guidance
- For Copilot Agents working on this repository, see [AGENTS.md](AGENTS.md) for detailed workflows, prompts, and conventions.

## References
- Vision: ../requirements/vision.md
- Specification: ../requirements/specification.md
- Design Overview: ../docs/design-overview.md
- Contributing Guidelines: CONTRIBUTING.md
- Agent Playbooks: ../docs/agents/playbooks.md
