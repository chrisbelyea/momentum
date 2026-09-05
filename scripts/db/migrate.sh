#!/usr/bin/env bash
# Apply the canonical SQLite schema. Runtime upgrades are performed by the Go
# binary's InitializeSchema compatibility migrator.
set -euo pipefail
DB_PATH="${DB_PATH:-momentum.db}"
exec sqlite3 "${DB_PATH}" < "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/internal/db/schema/init.sql"
