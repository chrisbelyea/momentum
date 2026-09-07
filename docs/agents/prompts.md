# Agent Prompts

## Implement the Next Issue (start here)

Copy the prompt for the next open issue and paste it into the Copilot chat at
[github.com/copilot](https://github.com/copilot), **or** open the issue in
GitHub and assign it directly to Copilot via the _Assignees_ field.

### Issue #11 — TLS-only defaults & setup docs (do this first)
```
Implement Issue #11 (TLS-only defaults & setup docs) in the chrisbelyea/momentum repository.

Acceptance criteria:
- The Go server (cmd/server/main.go) rejects plain HTTP connections and only serves HTTPS/TLS 1.3+.
- A helper script or Make target generates a self-signed certificate for local development.
- A new docs/tls-setup.md documents dev (self-signed) and production (Let's Encrypt / reverse proxy) TLS setup.
- The server README is updated to reflect the TLS requirement and link to docs/tls-setup.md.
- All existing Go tests continue to pass; add at least one test that verifies HTTP is rejected.

Follow the conventions in .github/AGENTS.md: branch name feature/tls-only-defaults, include rollback-safe changes, update docs alongside code.
```

### Issue #9 — Web kanban scaffold (drag-and-drop)
```
Implement Issue #9 (Web kanban scaffold with drag-and-drop) in the chrisbelyea/momentum repository.

Acceptance criteria:
- The Go server serves an HTML kanban board at GET / with three columns: Todo (NEEDS-ACTION), In Progress (IN-PROCESS), Done (COMPLETED).
- Task cards are rendered via Go html/template, one card per task fetched from the existing /caldav/tasks API.
- Dragging a card between columns uses the native HTML5 Drag-and-Drop API (no external library). On drop, an htmx request sends a PUT /caldav/tasks/{id} to update the task status; the board reflects the new state without a full-page reload.
- Keyboard accessibility: cards must also be moveable via a status select/button as a fallback (no JS required).
- Add at least one integration test verifying the board renders tasks and status update round-trips.
- Update cmd/server/README.md with the new UI endpoint.

Follow the conventions in .github/AGENTS.md: branch name feature/web-kanban-scaffold, prefer htmx, keep JS minimal.
```

### Issue #10 — List view with filter/sort
```
Implement Issue #10 (List view with filter/sort) in the chrisbelyea/momentum repository.

Acceptance criteria:
- A list view is served at GET /list showing all tasks in a table/list layout.
- Filters: status, due date range, tags. Sort: by due date (asc/desc), by status, by title.
- Filter and sort state is reflected in the URL query string so views are shareable/bookmarkable.
- Filters are consistent with the kanban board (same tasks, same statuses).
- Use htmx for filter/sort interactions without full-page reloads.
- Add tests for the filter and sort logic.
- Update cmd/server/README.md with the new /list endpoint.

Follow the conventions in .github/AGENTS.md: branch name feature/list-view-filter-sort.
```

### Issue #8 — External CalDAV connection configuration
```
Implement Issue #8 (External CalDAV connection configuration) in the chrisbelyea/momentum repository.

Acceptance criteria:
- Users can configure an external CalDAV backend via URL + username/password or client certificate.
- The server validates the connection on save (performs a PROPFIND and returns success/error).
- Credentials are stored encrypted at rest in the existing backends/credentials tables; never in plaintext.
- A settings UI page (or API endpoint) allows adding, listing, and removing backend configurations.
- Add unit tests for connection validation and credential encryption/decryption.
- Update docs/design-overview.md and cmd/server/README.md with the new capability.

Follow the conventions in .github/AGENTS.md: branch name feature/external-caldav-connection, use OS keychain on clients, encrypt at rest on server.
```

### Issue #12 — Credential storage policy (OS keychains)
```
Implement Issue #12 (Credential storage policy — OS keychains) in the chrisbelyea/momentum repository.

Acceptance criteria:
- A new doc docs/credential-storage.md documents the OS keychain mechanism for each platform:
  Windows (Windows Credential Manager / DPAPI), macOS (Keychain), Linux (Secret Service / libsecret).
- The document includes code or pseudocode examples for reading/writing secrets on each platform.
- The document is referenced from docs/design-overview.md (Security section) and .github/AGENTS.md.
- If any client-side Go code exists that handles credentials, update it to use the documented approach.

Follow the conventions in .github/AGENTS.md: branch name docs/credential-storage-policy.
```

### Issue #15 — Release packaging plan
```
Implement Issue #15 (Release packaging plan) in the chrisbelyea/momentum repository.

Acceptance criteria:
- A new doc docs/packaging.md documents the steps to produce each release artifact:
  Linux single-executable + systemd unit file, Windows single-executable + service installer, PWA build.
- scripts/build/ contains shell scripts (or a Makefile) that produce the Linux and Windows binaries locally with `go build`.
- A CI workflow job (in .github/workflows/release.yml or ci.yml) builds and uploads the Linux binary on pushes to main.
- The doc explains how to set up a self-hosted instance end-to-end (binary + DB migration + TLS).
- Update README.md and cmd/server/README.md to link to docs/packaging.md.

Follow the conventions in .github/AGENTS.md: branch name feature/release-packaging.
```

---

## Generic / Maintenance Prompts

### Issue Triage
"Review the specification and create Issues with acceptance criteria for the items below. Use labels: type:feature/task/docs/infra and area:*."

### Migration Authoring
"Update the canonical SQLite SQL changelog under internal/db/schema/changelog, regenerate internal/db/schema/init.sql with scripts/db/generate-init-sql.sh, and add or update migration/upgrade tests. Do not create Liquibase or independent DDL sources."

### PR Review
"Verify linked Issue, tests, docs updates, and CI. Comment on security, portability, and simplicity."

### Create Issues from Spec (Phase 1)
"Create GitHub Issues from requirements/specification.md for Phase 1. Use the Feature/Task/Docs Issue Forms. Apply labels: type:feature/task/docs, area:web/backend/sync/security/ci, priority:P1. Add each to the Project board in Todo, assign Phase 1 milestone, and include acceptance criteria referencing the spec sections. Create sub-issues where appropriate to break down complex issues into smaller, more manageable tasks."

### Sync Issues with Updated Spec
"Review changes in requirements/specification.md and sync existing Issues. Update acceptance criteria, create new Issues as needed, and adjust Project board states and milestones."

### Create Sub-Issues
"Break Issue #<number> (<title>) into smaller sub-issues for <list key components>. Create those as GitHub Issues linked to #<number>."

### Implement Issue (generic)
"Implement Issue #<number> (<title>). Verify acceptance criteria, run tests/validation locally, update docs, and open a PR when done."

### Review PR Against Issue
"Review PR #<number> against Issue #<number>. Verify acceptance criteria are met, tests cover edge cases, security requirements (TLS, secrets) are followed, and docs are updated."
