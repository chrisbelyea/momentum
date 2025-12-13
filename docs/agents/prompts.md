# Agent Prompts

## Issue Triage
"Review the specification and create Issues with acceptance criteria for the items below. Use labels: type:feature/task/docs/infra and area:*.")

## Migration Authoring
"Create Liquibase changesets for the described schema changes. Provide forward and rollback, and ensure validate passes."

## PR Review
"Verify linked Issue, tests, docs updates, and CI. Comment on security, portability, and simplicity."

## Create Issues from Spec (Phase 1)
"Create GitHub Issues from requirements/specification.md for Phase 1. Use the Feature/Task/Docs Issue Forms. Apply labels: type:feature/task/docs, area:web/backend/sync/security/ci, priority:P1. Add each to the Project board in Todo, assign Phase 1 milestone, and include acceptance criteria referencing the spec sections. Create sub-issues where appropriate to break down complex issues into smaller, more manageable tasks."

## Sync Issues with Updated Spec
"Review changes in requirements/specification.md and sync existing Issues. Update acceptance criteria, create new Issues as needed, and adjust Project board states and milestones."

## Create Sub-Issues
"Break Issue #<number> (<title>) into smaller sub-issues for <list key components>. Create those as GitHub Issues linked to #<number>."

## Implement Issue
"Implement Issue #<number> (<title>). Verify acceptance criteria, run tests/validation locally, update docs, and open a PR when done."

## Review PR Against Issue
"Review PR #<number> against Issue #<number>. Verify acceptance criteria are met, tests cover edge cases, security requirements (TLS, secrets) are followed, and docs are updated."
