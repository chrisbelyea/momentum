# Copilot Instructions

## Project Context
Momentum is a kanban-focused task manager (Go server + future PWA/native clients) that uses CalDAV VTODOs as its default storage backend. It supports multiple backends and targets both SaaS and self-hosted deployments.

- **Language / runtime**: Go 1.24 (module `github.com/chrisbelyea/momentum`)
- **Key dependencies**: `github.com/mattn/go-sqlite3` (requires CGO), `github.com/google/uuid`
- **Database**: SQLite (local dev default), PostgreSQL (production); migrations via Liquibase 4.27
- **Server entry point**: `cmd/server/main.go` — builds to a single executable

## Project Layout
```
cmd/server/          # Server entry point (main.go)
internal/
  backend/           # Backend selection handler
  caldav/            # CalDAV client and HTTP handler
  crypto/            # Encryption helpers
  db/                # SQLite/PostgreSQL repositories
  models/            # Shared domain types (Task, Backend, errors)
  web/               # Web UI handlers and filter logic (htmx/Go templates)
liquibase/
  changelog.xml      # Master changelog
  changelogs/        # Individual changesets (YAML)
  liquibase.properties  # Default SQLite config
scripts/
  db/migrate.sh      # Run DB migrations locally
  ops/bootstrap-labels.sh  # Seed GitHub labels
requirements/        # vision.md, specification.md, feature-list.md
docs/                # design-overview.md, vtodo-mapping.md, agents/
.github/
  copilot-instructions.md  # This file
  AGENTS.md          # Agent operating manual
  CODEOWNERS         # Code ownership
  workflows/ci.yml   # CI: Liquibase validate+update, Go lint+test+build
```

## Build & Test Commands
> CGO is required (sqlite3). Ensure a C compiler (gcc/clang) is available.

```bash
# Download dependencies (always run first after checkout)
go mod download

# Build the server binary
CGO_ENABLED=1 go build -v -o bin/momentum-server ./cmd/server

# Run all tests with race detector
CGO_ENABLED=1 go test -v -race ./...

# Lint / vet
go vet ./...

# Validate Liquibase changesets (requires Liquibase 4.27 on PATH)
liquibase --changeLogFile=liquibase/changelog.xml \
  --url="jdbc:h2:mem:validation" --driver=org.h2.Driver \
  --username=sa --password= validate
```

## CI Overview (`.github/workflows/ci.yml`)
All jobs run on pull requests and pushes to `main`:
- **lint-and-test**: `go vet`, `go test -race ./...`, Liquibase H2 validate, repo structure check
- **build-and-test-go**: `go test -race ./...`, build server binary, upload artifact
- **liquibase-sqlite** / **liquibase-postgresql**: validate + update against real DBs

A PR must pass all CI jobs before it can be merged.

## Implementation Preferences
- Web: prefer htmx for progressive enhancement.
- Server: single-executable, simple config, environment variables or config file.
- Security: TLS-only, OS keychains for client secrets, encrypted server storage.

## Style & Structure
- Shared core logic; platform-native clients.
- Keep changes minimal and focused; avoid over-engineering.
- Update documentation alongside code changes.
- Follow consistent naming conventions and file organization as established in the codebase.

## Testing Expectations
- Unit/integration tests for core logic and sync behaviors.
- Prefer table-driven tests for multiple scenarios.
- Test coverage should include edge cases and error conditions.
- Liquibase validate must pass in CI for any DB changeset.

## Database Migrations
- Use Liquibase for all schema changes; changesets live in `liquibase/changelogs/`.
- Each changeset must include a forward migration and rollback where feasible.
- Reference Issue numbers in changeset comments.
- Never modify a deployed changeset; create a new one for corrections.

## Common Workflows
- **New Feature**: Create Issue → Branch (`feature/<slug>`) → Implement → Tests → Docs → PR (draft) → Mark ready for review → Review → Merge.
- **Bug Fix**: Create Issue → Branch (`fix/<slug>`) → Fix → Tests → PR (draft) → Mark ready for review → Review → Merge.
- **Database Change**: Draft Liquibase changeset → Validate → Test migrations → PR (draft) → Mark ready for review (include rollback plan).
- **Integration**: Follow provider-specific auth patterns; use OS keychain for secrets.
- **PR Status**: Always create PRs in draft status initially. When all work is complete (code, tests, docs updated, CI passing), mark the PR as ready for review to signal it's ready for human review and merge.

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
