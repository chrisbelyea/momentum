-- Canonical VTODO persistence fields, schema version 3.
CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_uid ON tasks(uid) WHERE uid IS NOT NULL;
