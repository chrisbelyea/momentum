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

# Create the production schema artifact; do not maintain development-only DDL.
echo "Creating database schema..."
sqlite3 "$DB_PATH" < internal/db/schema/init.sql

echo "Database schema created successfully!"

# Seed test data
if [ -f "scripts/db/seed_tasks.sql" ]; then
    echo "Seeding test data..."
    sqlite3 "$DB_PATH" < scripts/db/seed_tasks.sql
    echo "Test data seeded successfully!"
fi

echo "Database initialization complete!"
echo "Database location: $DB_PATH"
