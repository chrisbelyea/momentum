# Progressive Web App scope

Phase 1 supports online use of the authenticated Momentum web UI. The manifest
is served by the release binary, but installability is not claimed yet because
branded icons and browser verification remain outstanding. Task data is never
cached by a service worker, and offline mutation is intentionally deferred
until a conflict-safe queue and recovery UX exist.

Supported browsers are current Chromium, Firefox, and Safari releases on
Windows, Linux, and macOS. A user must reconnect before creating, editing,
deleting, or changing task status. This avoids stale authenticated data and
prevents offline writes from bypassing server-side authorization and ETag
checks. Offline mutation, background sync, push notifications, branded icons,
and browser-installability automation remain tracked enhancements in issue
#70 and must not be inferred from the current manifest alone.
