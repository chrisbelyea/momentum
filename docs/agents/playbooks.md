# Agent Playbooks

## Spec to Issues
- Parse requirements/specification.md and vision.md to extract discrete work items.
- Use Issue Forms (feature/task/docs) with acceptance criteria and references to spec sections.
- Apply labels: `type:*`, `area:*`, `priority:*`; set Phase milestones.
- Add created Issues to the GitHub Project board (Todo) and swimlanes by area.
- Confirm CI requirements and cross-links are present.

## Migration Workflow
- Update the canonical SQL changelog under `internal/db/schema/changelog/`.
- Regenerate `internal/db/schema/init.sql` with
  `scripts/db/generate-init-sql.sh` and run its `--check` mode.
- Add fresh-install and upgrade tests, then open a PR with docs and linked
  Issue evidence. Do not add Liquibase or independent DDL sources.

## PR Review Rubric
- Linked Issue present; scope matches.
- Tests and docs updated; CI passing.
- Security and portability verified.

## Issue Sync Workflow
- On spec changes, re-parse the spec and compare against existing Issues.
- Update acceptance criteria and labels; create or close Issues as needed.
- Maintain board states and milestones; notify maintainers on significant scope changes.
