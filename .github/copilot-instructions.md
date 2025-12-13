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

## Testing Expectations
- Unit/integration tests for core logic and sync behaviors.
- Liquibase validate must pass in CI for DB changes.

## References
- Vision: ../requirements/vision.md
- Specification: ../requirements/specification.md
- Design Overview: ../docs/design-overview.md
