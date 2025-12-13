# GitHub Project Setup

## Labels
- type:feature, type:bug, type:task, type:docs, type:infra
- area:backend, area:sync, area:web, area:native, area:security, area:ci
- priority:P1, priority:P2, good-first-issue, blocked, design-needed

Bootstrap labels using `scripts/ops/bootstrap-labels.sh` (requires GitHub CLI `gh`).

## Project Board
- Columns: Backlog, Todo, In Progress, In Review, Done, Blocked
- Automation:
  - New issues → Todo
  - Linked PR opened → In Review
  - PR merged → Done
  - Label `blocked` → Blocked column
- Views:
  - Board with swimlanes by `area:*`
  - Table view with `priority`, `type`, `assignee`, `status`
  - Roadmap grouped by Phase milestones

## Workflow
1. Create issues from [requirements/specification.md](../../requirements/specification.md) using templates.
2. Add to Project; prioritize with `priority:*` labels.
3. Implement via PRs linked to issues; CI must pass.
