-- Momentum database schema
-- Generated from Liquibase changesets in liquibase/changelogs/
-- Run scripts/db/generate-init-sql.sh to regenerate from Liquibase

-- Changeset 001-create-users-table
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) PRIMARY KEY NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Changeset 001-create-backends-table
CREATE TABLE IF NOT EXISTS backends (
    id VARCHAR(36) PRIMARY KEY NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    backend_type VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    config_encrypted TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_backends_user_id ON backends(user_id);

-- Changeset 001-create-tasks-table
CREATE TABLE IF NOT EXISTS tasks (
    id VARCHAR(36) PRIMARY KEY NOT NULL,
    backend_id VARCHAR(36) NOT NULL,
    external_id VARCHAR(255),
    title VARCHAR(500) NOT NULL,
    description TEXT,
    status VARCHAR(50) NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    due_date TIMESTAMP,
    vtodo_data TEXT,
    sync_status VARCHAR(50) NOT NULL DEFAULT 'synced',
    last_synced_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (backend_id) REFERENCES backends(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_tasks_backend_id ON tasks(backend_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_due_date ON tasks(due_date);
