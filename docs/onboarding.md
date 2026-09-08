# First-run onboarding

Momentum is a self-hosted HTTPS application. On a new database, open
`https://localhost:8443/` in a browser. The first navigation redirects to the
account page, where you create the instance's first account. The password must
contain at least 12 characters. No default password is created.

After registration Momentum signs you in and creates the private `Local tasks`
backend. The board supports task creation, editing, status changes, and
deletion. Use **Sign out** in the board or list navigation to revoke the
session; return to `/` to sign in again.

Registration is intentionally first-account-only in Phase 1. This prevents a
packaged server from becoming an open account-registration service. Additional
accounts and invitations require a separately designed administration flow.
The JSON endpoints (`/auth/register`, `/auth/login`, and `/auth/logout`) remain
available for compatible clients, but browser users should use the rendered
pages. Their request, response, cookie, status, and first-account rules are
defined in the [authentication API contract](auth-api.md).

The development certificate is self-signed. Accept the browser's explicit
warning for local use, or configure a trusted certificate as described in
[TLS setup](tls-setup.md). Production installs must set
`MOMENTUM_ENCRYPTION_KEY`; the packaged Linux and Windows launchers provide
the required persistent data and encryption-key locations.
