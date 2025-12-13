# Contributing

## Workflow
- Open an Issue using templates; include acceptance criteria.
- Implement via a PR linked to the Issue.
- Ensure CI passes and docs are updated.
- Request review from CODEOWNERS.

## Branch Protections

The `main` branch is protected with the following requirements:

### Required for Merge
- **CI Status Checks**: All CI workflow jobs must pass before merging
  - Repository structure validation
  - Liquibase installation verification
  - (Future: linting, unit tests, integration tests, security scans)
- **Pull Request Reviews**: At least one approving review from CODEOWNERS
- **Up-to-date Branch**: PRs must be up-to-date with the base branch before merging

### Best Practices
- Keep PRs focused and scoped to a single issue
- Write descriptive commit messages
- Update documentation alongside code changes
- Ensure all CI checks pass locally before pushing
- Address review feedback promptly

## Standards
- Follow spec in requirements/specification.md and design in docs/design-overview.md.
- Security: TLS-only, secure secret handling.
- Tests accompany changes; prefer simple, portable solutions.
