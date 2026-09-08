# Project Status

This is the human-readable mirror of the Momentum GitHub Project. Issue
acceptance criteria and passing CI are authoritative; this page does not turn
planned or Draft work into a completion claim.

> Last audited: 2026-09-08 (main `b9ab696`, stable release `v0.2.2`)

## Current release

`v0.2.2` is the latest published release. Its release workflow passed Linux,
Windows, and macOS artifact, launcher, service, and fresh-database task
workflow checks. It can start on Windows and Linux, but it is not yet the
complete Phase 1 end-user experience: browser onboarding, safe default network
binding, and runtime external synchronization are follow-up work.

## Completed and verified

- #59: canonical SQLite schema/changelog recovery, generated SQL drift checks,
  fresh-install and upgrade workflow tests.
- #62: release validation and cross-platform artifact checks.
- #138: packaged Linux and Windows launcher defaults, persistent encryption-key
  setup, and release smoke coverage.
- The server has authenticated task CRUD, an embedded board/list PWA, TLS-only
  serving, CalDAV VTODO transport code, and durable sync-engine libraries.

## Active release work

The following issues are open and remain In Progress in the GitHub Project:

- #142 — Windows service defaults and LocalSystem ACLs ([PR #152](https://github.com/chrisbelyea/momentum/pull/152)).
- #143 — loopback-only packaged binding ([PR #153](https://github.com/chrisbelyea/momentum/pull/153)).
- #145 — browser first-run onboarding and sign-in ([PR #158](https://github.com/chrisbelyea/momentum/pull/158)).
- #147 — fixed Playwright dependency ([PR #154](https://github.com/chrisbelyea/momentum/pull/154)).
- #149 — browser conflict-reconciliation CI race ([PR #155](https://github.com/chrisbelyea/momentum/pull/155)).
- #151 — Linux user-service lifecycle validation ([PR #156](https://github.com/chrisbelyea/momentum/pull/156)).
- #144/#68 — runtime external CalDAV synchronization ([PR #159](https://github.com/chrisbelyea/momentum/pull/159)).

All listed PRs are Draft until their hosted acceptance checks pass. GitHub
Actions is currently unable to start jobs because the repository account
reports a failed payment or exceeded spending limit.

## Remaining Phase 1 work

The Phase 1 program [#61](https://github.com/chrisbelyea/momentum/issues/61)
remains open. Its acceptance checkboxes are intentionally not marked complete
until the active issues above have linked runtime evidence and a new release
candidate passes the full fresh-install, upgrade, authenticated task, and
external CalDAV workflows.

Additional open P1/P2 work includes:

- #63 — finish the authentication/onboarding acceptance audit.
- #65 — finish the complete browser task-management acceptance audit.
- #69 — reconcile all operational documentation and release procedures.
- #146 — add browser backend setup and validation controls.
- #150 — validate installable PWA behavior on Windows and Linux.

The native-client/provider sequence (#71, #130–#136) is deliberately deferred
until the Phase 1 server/PWA is complete. Those issues are planning milestones,
not evidence that native clients are already shipped.

## Deployment truth

The packaged launchers persist an encryption key and database in writable
per-user locations. Production deployments must provide `MOMENTUM_ENCRYPTION_KEY`
and trusted TLS certificates; development certificates are self-signed and
intended for local use. Follow [release-packaging.md](release-packaging.md)
and [tls-setup.md](tls-setup.md) for supported deployment procedures.

## References

- [Phase 1 recovery program (#61)](https://github.com/chrisbelyea/momentum/issues/61)
- [Post-Phase-1 roadmap](post-phase1-roadmap.md)
- [Server configuration](../cmd/server/README.md)
- [Release packaging](release-packaging.md)
- [TLS setup](tls-setup.md)
