# Momentum Agent Guide

This repository is developed through GitHub Issues, the Momentum Project, tested pull requests, and verified releases. `requirements/vision.md` and `requirements/specification.md` define product intent; GitHub is authoritative for current execution state.

## Operating model

- Hermes is the coordinator. It reconciles requirements, Issues, Project status, pull requests, CI, and releases.
- Codex and other coding agents are bounded implementation workers. They may investigate, edit, test, and commit inside an assigned branch or worktree, but they do not decide that an issue is complete, merge a pull request, publish a release, or mutate Hermes Kanban state.
- Before starting work, read the complete live issue and comments, inspect linked pull requests, and search for duplicate or overlapping open work. Prefer repairing an existing branch or pull request over creating a duplicate.
- Use an isolated Git branch/worktree for every issue. Never develop directly on `main`, and never let two agents share a writable worktree.
- Keep one coherent issue per pull request. Link it with `Closes #N` only when the full acceptance criteria are satisfied; otherwise use `Refs #N` and state what remains.

## Definition of done

An issue is complete only when all of the following are true:

1. Its current acceptance criteria are met by the code and documentation on the pull-request branch.
2. Regression tests demonstrate the changed behavior where practical.
3. Relevant local checks pass.
4. Required GitHub Actions checks pass on the exact pull-request head SHA. A queued, skipped, cancelled, billing-blocked, or unrelated run is not green evidence.
5. The diff has been reviewed for security, portability, data migration, and backward compatibility.
6. The pull request is merged and the Issue and Momentum Project reflect the resulting state.

A release is complete only after the release workflow validates the published commit's supported Windows and Linux artifacts and GitHub reports the release as non-draft. Never claim release success from a tag, draft release, or locally cross-compiled binary alone.

## Required workflow

1. Read `requirements/vision.md`, `requirements/specification.md`, `docs/status.md`, and the relevant design or operations documents.
2. Read the live Issue body and all comments with `gh issue view <N> --comments`.
3. Sweep for existing work with `gh pr list --state all --search`, using the issue number and relevant keywords.
4. Reproduce the gap against current `origin/main`; check history when behavior may be intentional.
5. Create or reuse an issue-specific branch/worktree based on current `origin/main`.
6. Make the smallest complete change, including tests and truthful documentation.
7. Run the applicable checks below. Independently verify checks after delegated-agent work.
8. Push and open or update the linked pull request. Read it back to verify base, head, files, and linkage.
9. Inspect live CI and fix failures attributable to the change. Do not repeatedly rerun deterministic failures.
10. Merge only when the definition of done is met, then verify the merged state and update tracking.

## Verification commands

Run the checks applicable to the changed area:

```bash
scripts/db/generate-init-sql.sh --check
go vet ./...
go test -race ./...
go build -trimpath -o bin/momentum-server ./cmd/server
```

For browser behavior:

```bash
npm ci
npx playwright install chromium
npm run test:e2e
```

Linux packaging changes must exercise `scripts/packaging/linux/test-launcher.sh`. Windows packaging and service changes require the corresponding `windows-latest` GitHub Actions jobs; do not fake Windows by patching runtime or platform detection on Linux. Release changes must preserve the staged draft-build, platform smoke/integration validation, and publish-last contract in `.github/workflows/release.yml`.

## Project invariants

- CalDAV VTODO is the canonical portable task representation.
- Preserve unknown/provider-specific VTODO fields and date-only semantics during round trips.
- Sync operations remain idempotent, resumable, conflict-aware, and fail safely.
- Network serving is HTTPS-only with TLS 1.3+; packaged servers bind to loopback by default unless the user explicitly configures otherwise.
- Passwords use the established password hashing flow. Server-side external-service credentials remain encrypted at rest. Never read, print, commit, or request secrets.
- `internal/db/schema/changelog/` is the canonical SQLite schema source. Regenerate `internal/db/schema/init.sql` with `scripts/db/generate-init-sql.sh`; do not add Liquibase or an independent DDL source.
- Preserve existing user data during schema, path, onboarding, and packaging changes. Add fresh-install and upgrade coverage when those paths change.
- Keep `docs/status.md`, release documentation, Issues, Project fields, and release claims aligned with live evidence rather than intended behavior.

## Scope and safety

- Avoid unrelated refactors and dependency upgrades unless required by the assigned issue.
- Do not force-push shared branches, rewrite published history, bypass branch protection, disable tests, or weaken release gates.
- Do not merge or publish while GitHub Actions is unavailable. Record infrastructure failures separately from product failures and continue only with work that can be honestly verified.
- Commits and pull requests should be small enough to review and revert.
