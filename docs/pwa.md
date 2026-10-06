# Progressive Web App scope

Momentum’s Phase 1 PWA is an installable, online-first web client for Windows
and Linux desktop users running a current Chromium-based browser. Production
installations must use HTTPS with a certificate trusted by the browser;
`https://localhost` is also a secure context for local development. The
release binary serves the manifest, branded 192px and 512px icons, and a
root-scoped service worker. Chromium browser CI runs on both Ubuntu and
Windows and verifies those installability prerequisites, worker activation and
update registration, authenticated-session behavior, and the offline cache
boundary.

The offline behavior is deliberately read-only and privacy-preserving:

- The worker caches only same-origin `/static/` assets (manifest and icons).
- HTML pages, authenticated task/API responses, authentication endpoints,
  CalDAV resources, and mutation requests are never cached.
- When disconnected, the static shell assets remain available, but the
  authenticated board cannot be loaded and task create/edit/delete/status
  operations require reconnecting to the server.

This prevents stale authenticated data and offline writes from bypassing
server-side authorization and ETag checks. Offline task data, background
sync, push notifications, and a conflict-safe mutation queue remain deferred
to [issue #70](https://github.com/chrisbelyea/momentum/issues/70). The browser
test proves the installability prerequisites and service-worker lifecycle; the
browser’s OS install prompt itself remains user-agent UI and is not asserted
by automation.

Firefox and Safari remain supported for normal online web use, but this release
does not claim their platform-specific PWA installation UX without equivalent
browser validation.
