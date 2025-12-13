# Momentum Specification

This document defines the complete specification for the Momentum application. It complements the product vision in [requirements/vision.md](requirements/vision.md) and the design overview in [docs/design-overview.md](docs/design-overview.md).

## 1. Scope and Goals
- Provide kanban and list views over tasks from one or more backends.
- Use CalDAV VTODO as the default, portable storage format.
- Support both SaaS and self-hosted deployments.
- Offer native clients (web, Windows, macOS, iOS/iPadOS, Linux) built on shared core logic.

## 2. Definitions and Terminology
- Task: A unit of work, represented canonically as a CalDAV VTODO.
- Backend: A task source (e.g., CalDAV server, iCloud Reminders, Microsoft To-Do).
- Board: Kanban representation of tasks grouped by status columns.
- Card: Visual representation of a task on the board.
- Status Column: Logical grouping (e.g., Todo, In Progress, Done).

## 3. Hosting Models
- SaaS: Hosted Momentum instance with managed authentication, storage, and updates.
- Self-hosted: Single-executable server; runs as a service (systemd on Linux; Windows service).
  - Default DB: SQLite (local installs); optional PostgreSQL.
  - Internal CalDAV server runs locally and does not trigger OS firewall prompts.

## 4. Task Backends
- Default: Internal CalDAV server (VTODOs).
- External CalDAV servers: Connect via URL, credentials, or client certificates.
- Future integrations: iCloud Reminders, Microsoft To-Do/Outlook, Microsoft Planner, Google Tasks, Trello, Jira, GitHub Issues, GitHub Projects.
- Transfer between backends: Move VTODOs across configured sources; preserve metadata (labels/tags, due dates, descriptions).

## 5. Functional Requirements
- Create, update, delete tasks (VTODOs) with fields:
  - Title, Description, Due Date, Priority, Status, Labels/Tags, Subtasks (if supported), Attachments (optional, backend-dependent).
- Kanban View (default):
  - Columns map to task statuses; drag-and-drop moves tasks between statuses.
  - Configurable columns per user/board.
  - Filter/search by label, status, backend, text.
- List View:
  - Sort/filter by status, due date, backend, tags.
- Tagging:
  - Use CalDAV labels; map to native labels for other providers when integrated.
- Multi-backend Unified View:
  - Display tasks from multiple backends in a single board/list.
  - Indicate source for each task; allow backend-specific actions.
- Transfers:
  - Move tasks between backends; handle conflicts, rate limits, and schema differences.

## 6. Non-Functional Requirements
- Security: TLS 1.3+ for all network connections; strong cipher suites only; HTTPS exclusively.
- Privacy: Encrypt credentials/tokens client-side using OS keychain; server stores secrets encrypted at rest.
- Reliability: Idempotent sync operations; conflict detection and resolution.
- Performance: Board interactions remain responsive with 1,000+ tasks; incremental syncing.
- Portability: CalDAV VTODO as canonical; migrations between backends are supported.

## 7. Authentication and Authorization
- Momentum account required; credentials stored in encrypted RDBMS.
- Authentication methods:
  - Username/password (hashed with modern KDF, e.g., Argon2id).
  - OAuth/OIDC (for external services where supported).
  - Client certificates (for CalDAV servers where supported).
- Authorization:
  - Per-user access to backends; sharing/teams as future enhancement.

## 8. Security Requirements
- Enforce HTTPS/TLS-only connections; reject plaintext HTTP.
- Secrets management:
  - Client: Use OS credential stores (Windows Credential Manager, macOS Keychain, Linux Secret Service/gnome-keyring).
  - Server: Encrypt at rest; restrict access via least-privilege policies.
- Hardening:
  - No weak ciphers; regular dependency updates; secure defaults.

## 9. System Design Overview
- Client-server architecture; shared core logic minimizes duplication across clients.
- Server responsibilities:
  - Authentication/authorization.
  - Internal CalDAV server management.
  - Sync orchestration with external backends.
  - Data storage (users, configuration, mappings) in RDBMS.
- Clients:
  - Web app (desktop/mobile; installable PWA; prefer htmx where feasible).
  - Windows 11 (WinUI, Windows App SDK).
  - macOS/iOS/iPadOS (Swift; shared code where practical).
  - Linux desktop (Vala + GTK; packaged as .deb/.rpm/Arch native).
- Mermaid diagram placeholder:
  - See [docs/design-overview.md](docs/design-overview.md) for the architecture diagram.

## 10. Data Model Overview
- Canonical Task (VTODO) fields mapped internally.
- User: account, authentication data, profile.
- Backend Config: type, credentials, connection details, scopes.
- Sync State: per-backend checkpoints, conflict flags.

## 11. Integrations
- CalDAV: Primary; robust VTODO support.
- OAuth-based services: secure token flows; refresh handling.
- Username/password services: secure storage, transport.
- Client certificates: configuration and validation.

## 12. User Flows
- Onboarding:
  - Create account → choose hosting model (SaaS/self-hosted) → default internal CalDAV enabled → optionally add external backends.
- Task Management:
  - Create task → assign status/tags → drag between columns → edit details → complete/archive.
- Backend Transfer:
  - Select task(s) → choose destination backend → transfer → confirm mapping and metadata preservation.

## 13. Deployment
- Server as single executable; systemd unit for Linux; Windows service configuration.
- Configuration via environment variables and config files.
- Database migrations: use a migration tool (see CI/CD section).

## 14. Tooling and CI/CD
- Database migrations: Recommend Liquibase or alternatives (Flyway). Decision: use Liquibase for cross-DB support.
- CI:
  - Linting, unit tests for core logic, integration tests for CalDAV.
  - Build clients and server artifacts; run security scans.
- CD:
  - Automated releases for SaaS; packaged artifacts for self-hosting.

## 15. Release Criteria and Validation
- Acceptance tests: CRUD tasks, kanban interactions, CalDAV sync, backend transfers.
- Performance benchmarks: responsive board with large task sets.
- Security checks: TLS-only, credential storage verification, dependency audits.

## 16. Roadmap Highlights
- Phase 1: Core CalDAV support, web app PWA, internal server.
- Phase 2: Native clients (Windows/macOS/iOS/Linux), transfers.
- Phase 3: External integrations (iCloud, Microsoft, Google, Trello, Jira, GitHub).

## Cross-References
- Vision: [requirements/vision.md](requirements/vision.md)
- Features: [requirements/feature-list.md](requirements/feature-list.md)
- Design: [docs/design-overview.md](docs/design-overview.md)
