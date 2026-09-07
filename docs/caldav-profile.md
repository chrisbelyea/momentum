# Momentum CalDAV VTODO profile

Momentum exposes one authenticated VTODO collection at `/caldav/tasks`. The
collection is selected with the `backend_id` query parameter (or the user's
default backend) and every resource operation is scoped to the authenticated
user. The collection is online-only; credentials and task data are never
accepted from an unauthenticated request in production.

## Supported methods

The collection supports:

- `OPTIONS`, advertising WebDAV class 1 and the CalDAV calendar-access
  extension.
- `PROPFIND` with `Depth: 0` or `Depth: 1`, returning collection metadata and
  VTODO members as a `207 Multi-Status` XML response.
- `POST` with `text/calendar` to create a VTODO, returning `201 Created`, a
  resource `Location`, and an `ETag`.
- `REPORT` with CalDAV `calendar-query` or `calendar-multiget` bodies. These
  return `207 Multi-Status` XML responses containing `getetag`,
  `getcontenttype`, and `calendar-data`. Missing multiget members are reported
  as `404 Not Found` entries without exposing another user's resources.
- `GET`, `PUT`, and `DELETE` on a VTODO resource. Calendar responses use
  `text/calendar`; JSON remains available for the legacy API compatibility
  path.

Resource `GET` supports `If-None-Match`. `PUT` and `DELETE` support
`If-Match`; a stale validator returns `412 Precondition Failed`. ETags are
content-derived and therefore change whenever the serialized VTODO changes.

## Compatibility boundaries

The current profile intentionally does not advertise or implement `REPORT`
filters beyond collection enumeration and explicit resource multiget. It does
not implement `MKCALENDAR`, free/busy reports, scheduling, WebDAV locks,
server-side sync tokens, or arbitrary calendar components. Clients should use
the VTODO component set returned by `PROPFIND` and must not assume event or
contact support.

External CalDAV discovery and VTODO CRUD are implemented by the outbound
client in `internal/caldav/client.go`. It requires HTTPS, validates the
configured origin for discovered resource URLs, bounds response bodies, and
uses ETag conditionals for updates and deletes.

## Compatibility verification

The repository contains standards-focused `httptest` fixtures covering:

- authenticated collection discovery and `Depth: 0/1` behavior;
- `calendar-query` and `calendar-multiget` XML responses;
- VTODO create, fetch, conditional update, and delete;
- outbound discovery, authenticated VTODO create/fetch/update/delete, ETag
  conditions, and cross-origin href rejection;
- a complete outbound lifecycle matrix in
  `internal/caldav/interop_matrix_test.go`.

The matrix has two explicit local profiles. `strict-rfc-server` requires
`Depth: 1`, XML and iCalendar media types, `If-None-Match: *` for creation,
and matching `If-Match` validators for updates and deletes. It returns relative
collection-member hrefs and a VTODO representation from successful PUTs.
`hosted-provider-shaped-server` returns absolute same-origin hrefs, includes
media-type parameters such as `charset=utf-8`, and exercises a successful
empty `204 No Content` update response. Both profiles require Basic
authentication and run the same discovery, list, create, read, update, and
delete sequence over TLS.

The matrix also verifies that a relative member href is resolved against the
collection request URI, rather than the configured discovery root. This is a
wire-compatibility behavior and is intentionally covered by the strict
profile.

CI additionally runs `scripts/caldav/radicale-interop.sh`, which starts the
pinned Radicale 3.1.8 standards-focused server with TLS and Basic
authentication, creates a VTODO collection, and runs
`TestLiveCalDAVInterop` through the real outbound client. That check proves
the documented profile against Radicale's implementation, while remaining
reproducible and credential-free.

CI also runs `scripts/caldav/nextcloud-interop.sh` against the pinned
`nextcloud:31.0.8-apache` image (manifest digest
`sha256:92bc503ea0c19789f402b0469ecfb8df1ffea81e2bf90a45bba39063a626aa00`).
The job installs the pinned Nextcloud Tasks `0.17.1` release, creates a fresh
task list through `occ`, and runs the same authenticated
discovery/list/create/read/update/delete lifecycle through a local TLS proxy.
This exercises a common provider-compatible VTODO implementation and its
provider-specific `/remote.php/dav/calendars/<user>/` discovery path without
requiring a hosted account or credentials. Tasks `0.17.1` supports Nextcloud
31 through 33.

The local matrix, Radicale run, and Nextcloud-compatible run are reproducible
interoperability evidence, not hosted-provider certification. They do not
prove compatibility with nextcloud.com, Google, Apple, or another hosted
service whose authentication, proxy, and deployment configuration may differ.
Those hosted-service results must be reported separately if credentials and a
maintained test account become available.
