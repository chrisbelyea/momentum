#!/bin/bash
# Development database initialization script

set -e

DB_PATH="${DB_PATH:-momentum_dev.db}"

echo "Initializing development database: $DB_PATH"

# Remove old database if it exists
if [ -f "$DB_PATH" ]; then
    echo "Removing existing database..."
    rm "$DB_PATH"
fi

# Create database schema
echo "Creating database schema..."
sqlite3 "$DB_PATH" << 'EOF'
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE backends (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    user_id INTEGER NOT NULL,
    backend_type VARCHAR(64) NOT NULL,
    name VARCHAR(128) NOT NULL,
    config_json TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    backend_id INTEGER NOT NULL,
    external_id VARCHAR(255),
    title VARCHAR(512) NOT NULL,
    description TEXT,
    status VARCHAR(64) NOT NULL,
    priority INTEGER,
    due_at TIMESTAMP,
    tags_json TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP,
    FOREIGN KEY (backend_id) REFERENCES backends(id) ON DELETE CASCADE
);
EOF

echo "Database schema created successfully!"

# Seed test data
if [ -f "scripts/db/seed_tasks.sql" ]; then
    echo "Seeding test data..."
    sqlite3 "$DB_PATH" < scripts/db/seed_tasks.sql
    echo "Test data seeded successfully!"
fi

echo "Database initialization complete!"
echo "Database location: $DB_PATH"
