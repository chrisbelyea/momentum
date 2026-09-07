# Project Status

This document provides an up-to-date assessment of Momentum's implementation progress and outlines the prioritized next steps.

> Last updated: 2026-09-06 (main `1b95e2f`)

---

## Summary

Momentum now has a runnable authenticated server for Windows and Linux. Main includes canonical schema upgrades, secure sessions and ownership checks, embedded web assets, TLS-only serving, CalDAV VTODO JSON/iCalendar task resources, external collection discovery and CRUD, a standards-focused Radicale interoperability check, executable sync planning/cycles with transactional apply and conflict recovery, per-backend sync limits and operation events, release-binary workflow tests, and native launch/service packaging. Phase 1 is not complete: richer web editing and browser-level workflow coverage remain open in [#65](https://github.com/chrisbelyea/momentum/issues/65), hosted-provider certification remains open in [#67](https://github.com/chrisbelyea/momentum/issues/67), and incremental provider cursor execution plus live-provider two-way/interrupted-sync evidence remain open in [#68](https://github.com/chrisbelyea/momentum/issues/68).

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

### Go Server and release workflow
A Go HTTP server (`cmd/server/main.go`) is running with:
- SQLite backend via `database/sql` + `go-sqlite3`.
- `internal/models`: `Task` struct and `TaskRow` with nullable-field parsing.
- `internal/db`: `TaskRepository` with full CRUD (List, Get, Create, Update, Delete).
- `internal/caldav`: HTTP handlers for `GET/POST /caldav/tasks` and `GET/PUT/DELETE /caldav/tasks/{id}`.
- `/health` endpoint and authenticated registration/login/logout.
- Release integration validation exercises registration, authenticated task CRUD/persistence/deletion, PWA assets, and graceful shutdown (16 checks); Windows SCM lifecycle runs on `windows-latest` in CI.
- Unit tests covering all CRUD operations with an in-memory SQLite database.

### Documentation
- `requirements/vision.md` — product goals, target users, success criteria.
- `requirements/specification.md` — complete functional and non-functional requirements.
- `requirements/feature-list.md` — feature roadmap.
- `docs/design-overview.md` — full architecture, sync orchestration design, conflict resolution policies, and design principles with Mermaid diagrams.
- `docs/vtodo-mapping.md` — canonical VTODO field mapping, round-trip fidelity guarantees, validation rules.
- `cmd/server/README.md` — server build, run, and API reference.

---

## Current implementation boundaries

The following table distinguishes shipped behavior from remaining limitations
and verification gaps:

| Area | Gap |
|---|---|
| **Authentication** | Core registration/login/logout and session ownership are implemented; release-binary checks cover the authenticated task workflow, while richer web-workflow coverage remains in [#65](https://github.com/chrisbelyea/momentum/issues/65). |
| **TLS** | TLS 1.3+ is enforced; development certificates are explicit and production encryption keys are required. |
| **Web UI** | Board/list views and accessible create/edit/delete/status/backend controls exist; richer field editing, browser-level E2E coverage, and failure/concurrency reconciliation remain in [#65](https://github.com/chrisbelyea/momentum/issues/65). |
| **Sync** | Provider-neutral reconciliation cycles, retry/idempotency, a CalDAV adapter, transactional local application, authenticated conflict listing/detail/recovery, per-backend concurrency/rate limits, and operation events are implemented; incremental provider cursor execution and live-provider two-way/interrupted-sync evidence remain in [#68](https://github.com/chrisbelyea/momentum/issues/68). |
| **External CalDAV** | Authenticated collection discovery, VTODO CRUD, REPORT lifecycle, a sync adapter, deterministic local compatibility profiles, and a green Radicale 3.1.8 TLS/Basic-auth lifecycle check are implemented; hosted-provider certification and provider-specific authentication/discovery evidence remain in [#67](https://github.com/chrisbelyea/momentum/issues/67). |
| **VTODO wire format** | RFC 5545 parser/serializer, canonical persistence, date-only fidelity, provider extension preservation, and fixture coverage are implemented. The JSON/iCalendar boundary is documented in `docs/vtodo-api.md`. |
| **Credential storage** | No OS keychain integration for clients. |
| **Release packaging** | Native Linux/Windows launch and service helpers are included in archives, and published prerelease `v0.2.0-rc2` has passed the downloaded Linux amd64 16-check workflow; a stable release remains gated on the open Phase 1 acceptance work. |

---

## Current tracked work (GitHub is authoritative)

See the [Phase 1 recovery program](https://github.com/chrisbelyea/momentum/issues/61) and its current open work: [#65 web workflow](https://github.com/chrisbelyea/momentum/issues/65), [#67 CalDAV interoperability](https://github.com/chrisbelyea/momentum/issues/67), and [#68 synchronization](https://github.com/chrisbelyea/momentum/issues/68). [#69 documentation](https://github.com/chrisbelyea/momentum/issues/69) tracks this status refresh; [#71 post-Phase-1 clients and integrations](https://github.com/chrisbelyea/momentum/issues/71) is intentionally deferred. Completed child issues and platform work are recorded in GitHub, including #59, #62–#64, #66, #70, and #74–#78.

---

## Historical bootstrap notes

The following bootstrap recommendations are retained for historical context
only; their original gaps are superseded by the current tracked issues above
and they are not a current work queue.

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

1. **Provider certification and sync operations** — `pkg/vtodo`, the CalDAV adapter, transactional local action application, conflict/recovery API, per-backend reliability controls, and a Radicale standards-server run are implemented; remaining work is hosted-provider certification in [#67](https://github.com/chrisbelyea/momentum/issues/67) plus incremental provider cursor execution and live-provider two-way/interrupted-sync evidence in [#68](https://github.com/chrisbelyea/momentum/issues/68).
2. **Backend abstraction layer** — The provider-neutral `internal/sync.Adapter` interface and capability model now separate canonical tasks from CalDAV transport. A broader application-level backend CRUD interface remains future architecture work and is not required for the current release.
3. **Sync orchestration** — Provider-neutral `Runner`/`RunCycle` orchestration and transactional application now exist. Scheduled background synchronization and provider-native cursor integration remain open work in [#68](https://github.com/chrisbelyea/momentum/issues/68).

---

## References

- [Vision](../requirements/vision.md)
- [Specification](../requirements/specification.md)
- [Feature List](../requirements/feature-list.md)
- [Design Overview](design-overview.md)
- [VTODO Mapping](vtodo-mapping.md)
