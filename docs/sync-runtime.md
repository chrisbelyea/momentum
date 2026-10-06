# External CalDAV synchronization

Momentum keeps the local task database authoritative while synchronizing an
external CalDAV VTODO collection through the provider-neutral reconciliation
engine.

## Configure a backend

Create an `external_caldav` backend through `POST /backends` with a URL and
credentials. `config.calendar_path` may contain the collection URL. If it is
omitted, Momentum discovers the first VTODO-capable calendar on the initial
run. Credentials are encrypted at rest using `MOMENTUM_ENCRYPTION_KEY`.

## Run and inspect synchronization

An authenticated client can trigger one bounded pass:

```http
POST /api/sync/run?backend_id=2
```

The response includes the selected collection and durable checkpoint. Status
can be read without running a pass:

```http
GET /api/sync/status?backend_id=2
```

To enable background synchronization, set `MOMENTUM_SYNC_INTERVAL` to a
positive Go duration (for example `15m`). It is disabled by default so a
single-user installation can choose explicit sync timing. The worker never
logs credentials and serializes runs per backend.

Use a CA-trusted HTTPS CalDAV endpoint in production. `skip_tls_verify` is
limited to loopback development targets and must not be used for a remote
provider.
