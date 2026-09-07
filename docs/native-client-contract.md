# Native Client API and Credential Contract

This is the shared design contract for the approved post-Phase-1 Windows and
Linux clients. It is a compatibility target, not a claim that those clients or
the versioned routes already ship.

## API boundary and versioning

- Clients communicate only with the Momentum HTTPS server; they never open the
  SQLite database or call provider endpoints directly.
- Native endpoints use an explicit major version (`/api/v1/...`) and a JSON
  media type carrying the same major version. Existing unversioned browser
  routes remain a separate compatibility surface for the Phase-1 PWA.
- `GET /api/v1/capabilities` advertises supported task fields, sync features,
  conflict actions, server limits, and the minimum/maximum API versions. A
  client refuses a server outside its supported range and gives the user an
  actionable upgrade message.
- Responses use stable canonical task/VTODO fields from `docs/vtodo-api.md`;
  additive fields are permitted within a major version. Removing or changing
  field meaning requires a new major version and a migration note.
- Mutations carry the task version in `If-Match`. A stale version returns a
  structured `409` conflict with the authoritative task, never an implicit
  overwrite. Rate limits, retryability, and authentication failures use stable
  machine-readable error codes.
- API requests are HTTPS-only. Clients must validate the server certificate and
  must not silently downgrade TLS or disable verification for non-loopback
  endpoints.

The route names above are reserved by this contract; implementing them is
tracked in the native-client and server API issues rather than assumed here.

## Authentication and credential lifecycle

1. The user authenticates through the server's supported login/device flow over
   TLS. The client stores only the resulting session/refresh material, never
   the user's password after authentication completes.
2. Windows clients use Windows Credential Manager/PasswordVault. Linux clients
   use Secret Service/libsecret where available; the documented Argon2id-derived
   encrypted-file fallback is allowed only for headless/minimal environments.
3. Keychain records are scoped by server origin, account, and credential type.
   A logout or explicit revoke deletes local records and asks the server to
   invalidate the corresponding session/refresh token.
4. OAuth authorization-code state and provider refresh tokens are sensitive.
   Provider refresh tokens used by server-side synchronization stay in the
   server's AES-256-GCM encrypted credential store. Native clients receive
   neither provider secrets nor database encryption keys.
5. Secrets must not appear in URLs, source control, settings exports, logs,
   telemetry, crash reports, or clipboard/debug output. Rotation and revocation
   must leave old tokens unusable where the provider supports it.

## Validation contract

Every native client or provider issue must include:

- contract fixtures for capabilities, task fields, errors, stale writes, and
  logout/revocation;
- a secret-free CI path plus an explicitly isolated credential-backed path when
  interoperability requires a real service;
- release-binary validation against the supported Linux/Windows server archives;
- an upgrade test proving that client-visible data survives a server upgrade;
- documentation of unsupported fields, provider limits, and recovery behavior.

This contract deliberately does not define cross-backend transfer semantics.
Those begin only after two real adapters exist and are governed by issue #136.
