# Momentum Vision

Momentum is a Kanban-focused task management application that unifies tasks across multiple backends, with CalDAV VTODOs as the default, portable standard. It offers both list and kanban views, a SaaS and self-hosted deployment model, and native clients across web and desktop/mobile platforms.

## Goals
- Provide a best-in-class kanban experience for tasks.
- Use standards-first storage via CalDAV VTODOs for portability.
- Support multiple task backends and unified views across sources.
- Offer both SaaS and self-hosted deployments with minimal friction.
- Maintain platform-native client experiences with shared core logic.

## Target Users
- Individuals and teams who prefer kanban views to manage tasks.
- Users who value data portability and vendor neutrality (CalDAV).
- Users who need to unify tasks across multiple services.

## Principles
- Standards-first: CalDAV VTODO is the canonical task format.
- Separation of concerns: a shared core with platform-native UIs.
- Security by default: encrypted at rest and in transit, least-privilege.
- Simplicity: self-hosting is straightforward; single-executable server.
- Extensibility: integrations for popular task providers (e.g., iCloud, Microsoft To-Do, Planner, Google Tasks, Trello, Jira, GitHub).

## Value Proposition
- Unified kanban across services without lock-in.
- Portable, interoperable task storage by default.
- Native experiences with consistent functionality and UX.

## Success Criteria
- Users can manage tasks end-to-end via kanban and list views.
- Tasks are synchronized with CalDAV by default; external sources connect easily.
- Self-hosted installs can be set up in minutes; SaaS offers instant onboarding.
- Integrations and transfers between backends work reliably.

## Out of Scope (Initial Releases)
- Full project management features beyond tasks (e.g., time tracking).
- Complex workflow automation beyond basic status transitions.

## References
- See the specification in [requirements/specification.md](requirements/specification.md).
- See the design overview in [docs/design-overview.md](docs/design-overview.md).
