# Agent Playbooks

## Spec to Issues
- Parse requirements/specification.md and vision.md to extract discrete work items.
- Use Issue Forms (feature/task/docs) with acceptance criteria and references to spec sections.
- Apply labels: `type:*`, `area:*`, `priority:*`; set Phase milestones.
- Add created Issues to the GitHub Project board (Todo) and swimlanes by area.
- Confirm CI requirements and cross-links are present.

## Migration Workflow
- Draft changeset YAML, run `validate` locally, include rollback.
- Open PR with tests/docs and link Issue.

## PR Review Rubric
- Linked Issue present; scope matches.
- Tests and docs updated; CI passing.
- Security and portability verified.

## Issue Sync Workflow
- On spec changes, re-parse the spec and compare against existing Issues.
- Update acceptance criteria and labels; create or close Issues as needed.
- Maintain board states and milestones; notify maintainers on significant scope changes.
