# Authentication API contract

Momentum uses one local identity per self-hosted instance in Phase 1. Passwords
are hashed with Argon2id; clients never receive a password or session token in a
JSON response. The server must run over HTTPS in production. Browser mutations
must include a same-origin `Origin` (or omit it for a non-browser client).

## First account

`POST /auth/register` is available only while the database has no
password-backed account. On a fresh database, or on a pre-authentication
database containing the single password-less compatibility user, registration
adopts that local identity so its existing backend and tasks remain available.
The
JSON body is:

```json
{"email":"you@example.com","password":"at-least-12-characters"}
```

On success the response is `201 Created`, sets the `momentum_session` cookie,
and returns `{"user_id": <id>}` (the ID is database-generated). Registration
creates the private `Local tasks` backend when the adopted identity does not
already have one. Invalid input returns `400`; a cross-origin mutation returns
`403`; once an account exists registration returns `403` with `registration
closed`.

The browser equivalent is the rendered `/register` page. It redirects to the
requested safe relative return URL after creating the session.

## Login

`POST /auth/login` accepts the same JSON fields. A successful response is `200
OK`, sets a fresh session cookie, and returns `{"user_id": <id>}`. Invalid
credentials return `401`; malformed input returns `400`; a cross-origin
mutation returns `403`.

The browser equivalent is `/login`. Unauthenticated HTML navigation redirects
to `/login?return=<relative-path>`; external URLs are discarded rather than
used as open redirects.

## Logout and sessions

`POST /auth/logout` revokes the current session and returns `204 No Content`.
It clears the cookie with `Secure`, `HttpOnly`, and `SameSite=Strict` attributes.
Expired or revoked sessions are rejected with `401 authentication required` on
protected routes. Sessions expire after 24 hours; cleanup removes expired rows.

The browser equivalent is the same-origin `/logout` form, which redirects to
`/login` after revocation.

Protected HTML, task, backend, and synchronization routes require the cookie.
`GET /health` is the unauthenticated readiness endpoint. Do not log cookie
values, passwords, or backend credentials. Production deployments must use
trusted TLS certificate/key files and set `MOMENTUM_ENCRYPTION_KEY`; the
self-signed development certificate and `MOMENTUM_DEV_MODE` are for local
development and tests only.
