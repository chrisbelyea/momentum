-- Canonical synchronization state, schema version 4.
-- All provider-specific state is keyed by the owning backend and is retained
-- so a failed sync can be resumed and diagnosed without guessing.
CREATE TABLE IF NOT EXISTS sync_checkpoints (
    backend_id INTEGER PRIMARY KEY,
    cursor TEXT,
    status TEXT NOT NULL DEFAULT 'idle',
    last_started_at TIMESTAMP,
    last_completed_at TIMESTAMP,
    last_error TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (backend_id) REFERENCES backends(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS sync_entities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    backend_id INTEGER NOT NULL,
    task_id INTEGER,
    remote_uid TEXT NOT NULL,
    remote_href TEXT,
    remote_etag TEXT,
    remote_sequence INTEGER,
    state TEXT NOT NULL DEFAULT 'active',
    last_pulled_at TIMESTAMP,
    last_pushed_at TIMESTAMP,
    deleted_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (backend_id) REFERENCES backends(id) ON DELETE CASCADE,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sync_entities_backend_uid ON sync_entities(backend_id, remote_uid);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sync_entities_backend_task ON sync_entities(backend_id, task_id) WHERE task_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sync_entities_backend_state ON sync_entities(backend_id, state);

CREATE TABLE IF NOT EXISTS sync_operations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    backend_id INTEGER NOT NULL,
    entity_id INTEGER,
    direction TEXT NOT NULL,
    operation TEXT NOT NULL,
    outcome TEXT NOT NULL,
    error TEXT,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMP,
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    FOREIGN KEY (backend_id) REFERENCES backends(id) ON DELETE CASCADE,
    FOREIGN KEY (entity_id) REFERENCES sync_entities(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_sync_operations_retry ON sync_operations(backend_id, outcome, next_attempt_at);

CREATE TABLE IF NOT EXISTS sync_conflicts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    backend_id INTEGER NOT NULL,
    entity_id INTEGER,
    task_id INTEGER,
    local_snapshot TEXT NOT NULL,
    remote_snapshot TEXT NOT NULL,
    policy TEXT NOT NULL DEFAULT 'manual',
    status TEXT NOT NULL DEFAULT 'open',
    resolution TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_at TIMESTAMP,
    FOREIGN KEY (backend_id) REFERENCES backends(id) ON DELETE CASCADE,
    FOREIGN KEY (entity_id) REFERENCES sync_entities(id) ON DELETE SET NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_sync_conflicts_backend_status ON sync_conflicts(backend_id, status);
