# Sync status and conflict recovery

Momentum retains both canonical snapshots whenever a two-way synchronization
cycle detects a manual conflict. The authenticated recovery API is available
from the embedded server:

```text
GET   /api/sync/conflicts?status=open&backend_id=<id>
GET   /api/sync/conflicts/<id>
PATCH /api/sync/conflicts/<id>
```

The list endpoint defaults to `status=open`; use `status=all` to include
resolved and dismissed records. Every response includes the backend and task
identifiers, policy/status, timestamps, and both `local_snapshot` and
`remote_snapshot` fields. Snapshots are retained after resolution for audit
and recovery evidence.

Resolve a conflict by sending one of these JSON bodies:

```json
{"resolution":"remote"}
{"resolution":"local"}
{"resolution":"dismissed"}
```

`remote` atomically applies the retained provider snapshot to the local task,
updates the sync mapping, and closes the conflict. `local` preserves the local
task so the next sync can push it. `dismissed` records an explicit operator
acknowledgement without choosing either snapshot. A conflict can only be
resolved once, and backend ownership is checked against the authenticated
session before any evidence is returned or changed.

This is the Phase 1 recovery path. Automatic field-level merging, scheduled
background synchronization, and provider-specific conflict policies remain
outside this release and are tracked in issue #68.
