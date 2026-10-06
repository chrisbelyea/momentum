-- Durable provider-operation completion keys, schema version 6.
-- The key is globally unique because the provider-neutral Runner receives only
-- the stable operation key. Runtime keys include the backend ID, so this still
-- provides per-backend isolation while allowing the generic idempotency store
-- interface to remain backend agnostic.
CREATE TABLE IF NOT EXISTS sync_operation_keys (
    operation_key TEXT PRIMARY KEY,
    backend_id INTEGER NOT NULL,
    completed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (backend_id) REFERENCES backends(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_sync_operation_keys_backend ON sync_operation_keys(backend_id);
