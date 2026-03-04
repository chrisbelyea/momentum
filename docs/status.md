# Project Status

This document provides an up-to-date assessment of Momentum's implementation progress and outlines the prioritized next steps.

> Last updated: 2026-03-04

---

## Summary

Momentum has a solid foundation in place: repository infrastructure, database schema, CI pipeline, and a bare-bones Go HTTP server. The project is at the **end of its bootstrapping phase** and is ready to begin feature implementation. No user-facing functionality exists yet — there is no web UI, no authentication, and no TLS enforcement.

---

## What Is Complete

### Infrastructure & CI
- GitHub repository setup with branch protections, CODEOWNERS, issue/PR templates, and Copilot agent instructions.
- CI workflow (`ci.yml`) running on every push and PR:
  - Liquibase validation and migration against both SQLite and PostgreSQL.
  - Go build, test (with race detector and coverage), and binary artifact upload.
  - Repository structure validation.
- Liquibase migrations validated and documented.

### Database Schema (Liquibase)
All core tables are defined and pass `liquibase validate` on SQLite and PostgreSQL:

| Table | Purpose |
|---|---|
| `users` | Momentum user accounts |
| `backends` | Per-user task backend configurations |
| `tasks` | Task storage (VTODO fields) |
| `sync_state` | Per-backend incremental sync checkpoints |
| `credentials` | Hashed credentials per user/type |
| `sessions` | Active user sessions with expiry |

Performance indexes on `(backend_id, status)`, `due_at`, credential/session lookups are also in place.

### Go Server Scaffold
A minimal HTTP server (`cmd/server/main.go`) is running with:
- SQLite backend via `database/sql` + `go-sqlite3`.
- `internal/models`: `Task` struct and `TaskRow` with nullable-field parsing.
- `internal/db`: `TaskRepository` with full CRUD (List, Get, Create, Update, Delete).
- `internal/caldav`: HTTP handlers for `GET/POST /caldav/tasks` and `GET/PUT/DELETE /caldav/tasks/{id}`.
- `/health` endpoint.
- Unit tests covering all CRUD operations with an in-memory SQLite database.

### Documentation
- `requirements/vision.md` — product goals, target users, success criteria.
- `requirements/specification.md` — complete functional and non-functional requirements.
- `requirements/feature-list.md` — feature roadmap.
- `docs/design-overview.md` — full architecture, sync orchestration design, conflict resolution policies, and design principles with Mermaid diagrams.
- `docs/vtodo-mapping.md` — canonical VTODO field mapping, round-trip fidelity guarantees, validation rules.
- `cmd/server/README.md` — server build, run, and API reference.

---

## Current Gaps

The following are **not yet implemented** in code:

| Area | Gap |
|---|---|
| **Authentication** | Auth tables exist; no login, session, or middleware code. |
| **TLS** | Server runs plain HTTP. TLS 1.3+ is required by spec; HTTP must be rejected. |
| **Web UI** | No HTML, CSS, or JavaScript exists. No kanban board, no list view. |
| **Sync** | No sync orchestration code. Design exists in `docs/design-overview.md`. |
| **External CalDAV** | No connection management or external CalDAV client code. |
| **VTODO wire format** | API is JSON-only. No iCalendar serialization/deserialization (`pkg/vtodo` is a placeholder). |
| **Credential storage** | No OS keychain integration for clients. |
| **Release packaging** | No single-executable build scripts for Linux/Windows service packaging or PWA build pipeline. |

---

## Open Issues (All P1)

| # | Title | Area |
|---|---|---|
| [#8](https://github.com/chrisbelyea/momentum/issues/8) | External CalDAV connection configuration | sync, security |
| [#9](https://github.com/chrisbelyea/momentum/issues/9) | Web kanban scaffold (drag-and-drop) | web |
| [#10](https://github.com/chrisbelyea/momentum/issues/10) | List view with filter/sort | web |
| [#11](https://github.com/chrisbelyea/momentum/issues/11) | TLS-only defaults & setup docs | security |
| [#12](https://github.com/chrisbelyea/momentum/issues/12) | Credential storage policy (OS keychains) | security, docs |
| [#15](https://github.com/chrisbelyea/momentum/issues/15) | Release packaging plan | ci, docs |

---

## Recommended Next Steps

The following order is recommended to unblock end-to-end user flows as quickly as possible.

### 1. TLS & Authentication (Unblocks everything else)
Without TLS and auth, the server cannot be safely exposed or tested end-to-end.

- **Issue #11** — Enforce TLS 1.3+; reject plain HTTP. Add self-signed cert generation for dev; document prod cert setup (Let's Encrypt / reverse proxy).
- Add auth middleware: session cookie validation, Argon2id password hashing, login/logout endpoints. Tables (`credentials`, `sessions`) already exist.

### 2. Web UI Scaffold — Kanban Board
The primary user-facing feature. Build on the existing server with htmx for progressive enhancement.

- **Issue #9** — Serve HTML via Go templates; implement three-column kanban (Todo / In Progress / Done) backed by the existing task API. Add drag-and-drop to persist status changes.

### 3. Web UI — List View
Complement to the kanban board; reuses existing task API.

- **Issue #10** — List view with filter by status/due/tags and sort. Filters must be consistent with kanban board behavior.

### 4. External CalDAV Connection
Enables the core multi-backend value proposition.

- **Issue #8** — Add backend configuration UI and server-side CalDAV client. Store credentials securely (encrypted at rest, per spec §8).

### 5. Credential Storage Policy
Required before any client ships or external backends are wired up.

- **Issue #12** — Document and implement OS keychain integration (Windows Credential Manager, macOS Keychain, Linux Secret Service) for client-side secrets.

### 6. Release Packaging
Required before any public or self-hosted release.

- **Issue #15** — Document and automate single-executable packaging for Linux (systemd), Windows (service), and PWA build steps. Add CI jobs to produce release artifacts.

---

## Architecture Gaps to Resolve Before Feature Completion

1. **`pkg/vtodo` package** — VTODO serialization/deserialization is referenced in server README as a future placeholder. Needed before external CalDAV sync and proper RFC 4791 compliance.
2. **Backend abstraction layer** — The `backends` table exists, but there is no Go interface defining the backend contract (List, Get, Create, Update, Delete, Sync). This interface should be defined before implementing the external CalDAV backend to keep the architecture extensible.
3. **Sync orchestration** — Designed in `docs/design-overview.md` but has no code. Should be scaffolded after the internal CalDAV server is stable and external CalDAV connections are possible.

---

## References

- [Vision](../requirements/vision.md)
- [Specification](../requirements/specification.md)
- [Feature List](../requirements/feature-list.md)
- [Design Overview](design-overview.md)
- [VTODO Mapping](vtodo-mapping.md)
