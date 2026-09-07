# Canonical task and transport contract

Momentum has one canonical task model. A transport adapter converts that model
to or from a provider representation; transport-specific identifiers and
validators must not replace the canonical UID.

## Canonical identity and fields

- `uid` is the stable VTODO `UID` and is generated once if a local task does
  not provide one. The SQLite integer `id` is local storage identity only.
- `title`, `description`, `status`, `priority`, `due_at`, `start_at`,
  `completed_at`, `percent_complete`, `tags_json`, `related_to_json`, `url`,
  `location`, and `extra_json` represent the supported VTODO fields.
- `due_date_only` and `start_date_only` preserve whether `DUE` and `DTSTART`
  arrived as `VALUE=DATE` rather than date-time values.
- `dtstamp`, `last_modified`, and `sequence` are revision metadata used for
  round trips and synchronization. Provider `ETag` and href values belong in
  sync mappings, not in the task itself.
- Unknown VTODO properties are retained in `extra_json` as name, parameters,
  and value so an adapter can round-trip provider extensions without treating
  them as first-class Momentum fields.

## JSON API versus iCalendar transport

The authenticated `/caldav/tasks` endpoints accept and return JSON for browser
and legacy clients. JSON uses the database field names shown above and exposes
the canonical UID alongside the local integer ID. It is an application
transport, not a second task schema.

When the request `Content-Type` or `Accept` includes `text/calendar`, the same
resource endpoints use RFC 5545 `VCALENDAR`/`VTODO` bodies. The CalDAV profile
and content-type behavior are documented in [`caldav-profile.md`](caldav-profile.md).

Adapters must preserve supported fields, date-only flags, UID, revision
metadata, and `extra_json` extensions. A provider that cannot represent a
field must document that limitation and retain the original value in the
canonical task or extension store rather than silently dropping it.

## Validation and compatibility

`pkg/vtodo` validates required UID/DTSTAMP/SUMMARY fields, status and numeric
ranges, RFC 5545 escaping/folding, date and timezone forms, and malformed
components. Provider fixtures in `pkg/vtodo/testdata` cover valid Google- and
Apple-style extensions plus malformed input. Real-provider interoperability is
validated by the standards-focused Radicale and provider-compatible Nextcloud
Tasks CI jobs linked from [#67](https://github.com/chrisbelyea/momentum/issues/67).
This document does not claim certification for hosted services such as
nextcloud.com, Google, or Apple.
