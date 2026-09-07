# momentum
Momentum brings your to-dos, reminders, and CalDAV tasks into one flow — visible as lists or boards. Designed for people who want to see their life in motion.

## Documentation
- **Project Status**: [docs/status.md](docs/status.md) — where we are and what's next
- Vision: [requirements/vision.md](requirements/vision.md)
- Specification: [requirements/specification.md](requirements/specification.md)
- Features: [requirements/feature-list.md](requirements/feature-list.md)
- Design Overview: [docs/design-overview.md](docs/design-overview.md)
- Release Packaging: [docs/release-packaging.md](docs/release-packaging.md)
- VTODO Mapping: [docs/vtodo-mapping.md](docs/vtodo-mapping.md)

## Continuing Development with Copilot

The fastest way to move the project forward is to assign open issues to the **GitHub Copilot coding agent** — it will implement the work, run CI, and open a pull request for your review.

### Step-by-step

1. **Pick an issue** from the [open issues list](https://github.com/chrisbelyea/momentum/issues) (see [docs/status.md](docs/status.md) for the recommended order).
2. **Assign the issue to Copilot** by opening the issue, clicking _Assignees_, and selecting **Copilot**. GitHub will queue the agent.
   _Alternatively_, go to [github.com/copilot](https://github.com/copilot) and paste one of the ready-to-use prompts from [docs/agents/prompts.md](docs/agents/prompts.md).
3. **Wait for the PR** — Copilot will create a branch, implement the change, and open a pull request linked to the issue.
4. **Review the PR** — check that acceptance criteria are met, CI is green, docs are updated, and no secrets are committed. Approve and merge when satisfied.
5. **Repeat** — pick the next issue in priority order and assign again.

### Recommended issue order

| Priority | Issue | Why first |
| --- | --- | --- |
| 1 | [#65 Authenticated web workflow](https://github.com/chrisbelyea/momentum/issues/65) | Remaining review/merge step for the green browser E2E workflow PR |
| 2 | [#69 Documentation accuracy](https://github.com/chrisbelyea/momentum/issues/69) | Refresh this documentation and verify operational instructions |
| 3 | [#71 Post-Phase-1 clients and integrations](https://github.com/chrisbelyea/momentum/issues/71) | Deferred planning work after the Phase 1 release |

Issues [#67](https://github.com/chrisbelyea/momentum/issues/67) and
[#68](https://github.com/chrisbelyea/momentum/issues/68) are complete for their
documented Phase 1 acceptance criteria. Hosted-service certification beyond
the pinned Nextcloud-compatible CI service remains explicitly out of scope.

### Tips
- Use the prompts in [docs/agents/prompts.md](docs/agents/prompts.md) for precise instructions when using the Copilot chat interface.
- After merging a PR, update [docs/status.md](docs/status.md) to reflect the new state.
- If Copilot opens a `blocked` issue, add the missing context in the issue comments and re-assign.

## Running the Server

Momentum requires TLS. Set `TLS_CERT` and `TLS_KEY` before starting the server.
See [docs/tls-setup.md](docs/tls-setup.md) for dev (mkcert / openssl) and production
(Let's Encrypt) certificate setup.

By default, Momentum stores its SQLite database in the current user's writable
OS configuration directory: `~/.config/Momentum/momentum.db` on Linux and
`%AppData%\\Momentum\\momentum.db` on Windows. Set `DB_PATH` to use a specific
database location; an explicit value is used unchanged.

```bash
export TLS_CERT=path/to/cert.pem
export TLS_KEY=path/to/key.pem
./bin/momentum-server          # listens on :8443 by default
```

## CI/CD and Tooling (Overview)
- Database schema: canonical SQL changelog with generated embedded initialization; see [docs/database-schema.md](docs/database-schema.md).
- CI: schema drift validation, unit/integration tests, release-binary workflow checks, and platform lifecycle checks.
- Packaging: single-executable server, embedded PWA assets, Linux/Windows helpers, and native macOS release artifacts.
