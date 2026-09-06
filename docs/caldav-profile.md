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
  conditions, and cross-origin href rejection.

These fixtures do not claim certification against a named hosted provider.
Before declaring provider interoperability complete, run the same lifecycle
matrix against a real standards server and at least one provider-compatible
implementation; that work remains tracked by issue #67.
