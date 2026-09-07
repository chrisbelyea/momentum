# Post-Phase-1 Roadmap

This document records the product decisions and delivery order for work after
the verified `v0.2.0-rc3` Phase-1 server release. It is intentionally a
planning document: an item listed here is not implemented unless its linked
GitHub issue and release validation say so.

## Product decisions

1. The authenticated server and PWA remain the supported cross-platform client
   for Phase 1. The Phase-1 release does not include a native client.
2. The first native clients are Windows 11 and Linux desktop. Windows uses a
   native desktop framework selected during [#131][131]; Linux targets GTK and
   the supported glibc baseline selected during [#132][132].
3. macOS and iOS/iPadOS are deferred until the Windows/Linux client contract
   has shipped and been validated. They are not Phase-1 acceptance criteria.
4. The provider order is Microsoft To Do / Graph first, Google Tasks second,
   and hosted Nextcloud certification as a separate validation milestone. iCloud,
   Trello, Jira, GitHub, and hosted SaaS capabilities are unapproved and remain
   outside the roadmap until explicitly reviewed.
5. Native clients consume a versioned HTTPS API and use OS credential stores;
   they do not access the server database or provider secrets directly.

## Ordered milestones

| Order | Milestone | Tracking | State |
| --- | --- | --- | --- |
| 1 | Shared native API/versioning and credential contract | [#130][130] | Design required before client work |
| 2 | Windows native client | [#131][131] | Approved; blocked on #130 |
| 3 | Linux native client | [#132][132] | Approved; blocked on #130 |
| 4 | Hosted Nextcloud Tasks certification | [#133][133] | Approved validation milestone |
| 5 | Microsoft To Do / Graph adapter | [#135][135] | First new provider; blocked on #130 |
| 6 | Google Tasks adapter | [#134][134] | Second new provider; blocked on #130 |
| 7 | Cross-backend transfer and metadata-loss policy | [#136][136] | Starts after two real adapters exist |

Each implementation issue must define its security model, interoperability
criteria, release validation, and explicit non-goals before implementation
begins. New provider work must not silently expand the Phase-1 release.

## Shared native-client contract

The normative design is in [`native-client-contract.md`](native-client-contract.md).
In summary:

- Native clients use a versioned HTTPS JSON boundary, capability discovery, and
  stable error/concurrency semantics; they do not depend on internal Go or SQL
  types.
- The existing browser routes remain compatible with the Phase-1 PWA. A native
  API version is introduced deliberately and is not inferred from unversioned
  browser routes.
- Momentum session/refresh credentials and OAuth flow state belong in the
  platform keychain. Provider refresh tokens needed by server-side sync remain
  in the server's encrypted credential store and are never returned to clients.
- Logout and revocation remove local credentials and invalidate server-side
  sessions/tokens where supported. Credentials must not appear in files, URLs,
  logs, crash reports, or analytics.

## Transfer and metadata-loss gate

The current release has one production sync adapter (CalDAV). Therefore no
cross-provider transfer claim is made yet. Once two approved adapters exist,
[#136][136] will define and test the durable two-step transfer protocol: create
and map the destination first, delete the source only after confirmed success,
and preserve unsupported fields in canonical extension/loss evidence with a
user-visible warning before destructive transfer.

## Explicitly out of scope

Hosted Momentum SaaS, automatic production certificate provisioning, silent
provider conversion, and unapproved iCloud/Trello/Jira/GitHub integrations are
not part of Phase-1 or this approved post-Phase-1 order.

[130]: https://github.com/chrisbelyea/momentum/issues/130
[131]: https://github.com/chrisbelyea/momentum/issues/131
[132]: https://github.com/chrisbelyea/momentum/issues/132
[133]: https://github.com/chrisbelyea/momentum/issues/133
[134]: https://github.com/chrisbelyea/momentum/issues/134
[135]: https://github.com/chrisbelyea/momentum/issues/135
[136]: https://github.com/chrisbelyea/momentum/issues/136
