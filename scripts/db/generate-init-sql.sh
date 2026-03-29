#!/usr/bin/env bash
# Generate internal/db/schema/init.sql from Liquibase changesets.
#
# Usage:
#   ./scripts/db/generate-init-sql.sh
#
# Prerequisites:
#   - Liquibase 4.27 on PATH (or set LB_BIN env var)
#   - H2 driver available to Liquibase
#
# The generated file is committed to the repository so that the Go binary can
# embed it without requiring Liquibase at runtime.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUTPUT="${REPO_ROOT}/internal/db/schema/init.sql"
LB_BIN="${LB_BIN:-liquibase}"

echo "Generating schema SQL from Liquibase changesets..."
"${LB_BIN}" \
    --changeLogFile="${REPO_ROOT}/liquibase/changelog.xml" \
    --url="jdbc:h2:mem:generate;DB_CLOSE_DELAY=-1" \
    --driver=org.h2.Driver \
    --username=sa \
    --password= \
    updateSQL \
    | grep -v '^--' \
    | grep -v '^$' \
    > "${OUTPUT}"

echo "✓ Schema SQL written to: ${OUTPUT}"
