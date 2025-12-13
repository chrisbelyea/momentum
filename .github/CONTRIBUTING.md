# Contributing

## Workflow
- Open an Issue using templates; include acceptance criteria.
- Implement via a PR linked to the Issue.
- Ensure CI passes and docs are updated.
- Request review from CODEOWNERS.

## Standards
- Follow spec in requirements/specification.md and design in docs/design-overview.md.
- Security: TLS-only, secure secret handling.
- Tests accompany changes; prefer simple, portable solutions.

## Database Migrations
- All database schema changes must use Liquibase migrations (see `liquibase/README.md`).
- Create changesets in `liquibase/changelogs/` and include them in `liquibase/changelog.xml`.
- Validate migrations locally before committing:
  ```bash
  liquibase --defaultsFile=liquibase/liquibase.properties validate
  ```
- **CI automatically validates changesets** on every PR. Invalid changesets will fail the build and block merging.
- Never modify deployed changesets; create new changesets for corrections.
- Test migrations on both SQLite (default) and PostgreSQL (production) when possible.
