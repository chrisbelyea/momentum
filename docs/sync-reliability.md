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

These controls are the reliability slice currently implemented for [#68](https://github.com/chrisbelyea/momentum/issues/68).
The CalDAV adapter still performs a complete collection listing for each pull;
provider-native incremental cursor execution and live-provider interrupted-sync
evidence remain open acceptance work.
