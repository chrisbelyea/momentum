# Progressive Web App scope

Phase 1 supports online use of the authenticated Momentum web UI. The manifest
is served by the release binary and includes branded 192px and 512px icons.
Repository and release-binary checks verify the manifest, icons, and
same-origin static-asset service worker. Browser installability is not claimed
without browser-level verification. Task data is never cached by a service
worker, and offline mutation is intentionally deferred until a conflict-safe
queue exists.

Supported browsers are current Chromium, Firefox, and Safari releases on
Windows, Linux, and macOS. A user must reconnect before creating, editing,
deleting, or changing task status. This avoids stale authenticated data and
prevents offline writes from bypassing server-side authorization and ETag
checks. Offline mutation, background sync, push notifications, and
browser-installability automation remain deferred scope; [issue #70](https://github.com/chrisbelyea/momentum/issues/70)
records the decision and implementation evidence. They must not be inferred
from the current manifest alone.
