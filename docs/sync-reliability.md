# Sync reliability controls

The provider-neutral sync runner supports bounded retries, cancellation,
per-backend limits, and structured operation events. A deployment that runs
multiple backends should construct one `sync.BackendLimiter` and share it with
all cycles:

```go
limiter, _ := sync.NewBackendLimiter(2, 250*time.Millisecond)
runner := sync.Runner{
    Policy:  sync.DefaultRetryPolicy,
    Limiter: limiter,
    Observer: func(event sync.SyncEvent) {
        // Forward event.Operation, event.Outcome, event.Attempt, and
        // event.Duration to the deployment's logs or metrics.
    },
}
```

`maxConcurrent` is enforced independently for each backend, so one slow
provider cannot consume another provider's slots. The interval spaces starts
for a backend and waits honor the cycle context; cancellation releases the
slot. Pulls, pushes, and remote deletes all pass through the limiter and emit
an event after every attempt. Pull reads are deliberately not recorded in the
idempotency store, while mutations retain their stable operation keys.

Provider adapters should wrap transient transport failures with
`sync.Retryable`. Permanent validation or authentication failures are emitted
once and are not retried. Retry attempts use the bounded `RetryPolicy`, and a
checkpoint is advanced only by the transactional apply step after a complete
cycle succeeds.

The limiter and observer are process-local controls. A deployment running
multiple Momentum processes must coordinate provider quotas outside the
process, or use one scheduler responsible for each backend.

## Incremental CalDAV pulls

The CalDAV adapter uses the RFC 6578 `sync-collection` REPORT when a provider
supports it. The opaque response token is returned as `PullResult.NextCursor`
and must be persisted with the same transaction as the page's task and entity
changes. A subsequent cycle passes that token back to the adapter. Deleted
resources are reported by href and resolved against the durable mapping before
the planner emits a local deletion, so a partial or incremental page never
causes an unrelated task to be removed.

Providers that reject the REPORT can still be used for an initial import via
the existing Depth-1 PROPFIND listing. That fallback intentionally does not
advance a cursor; subsequent runs retry the standard REPORT. If a provider
returns HTTP 403 or 409 for a non-empty token, the token has expired and the
caller must schedule a fresh import rather than applying an incomplete delta.

These controls and the cursor/resume tests are implemented in the provider-neutral
library. They are not an automatic runtime feature of the v0.2.2 release binary:
the server does not yet construct a sync adapter, persist a runtime cycle, or
schedule external pulls/pushes. Runtime integration and release-binary evidence
are tracked in [#68](https://github.com/chrisbelyea/momentum/issues/68) and
[#144](https://github.com/chrisbelyea/momentum/issues/144). The Radicale and
Nextcloud CI lifecycle jobs certify the outbound adapter separately from the
running server.
