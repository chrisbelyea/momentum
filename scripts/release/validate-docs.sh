#!/usr/bin/env bash
# Validate the release documentation contract without network access.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
required_files=(
  "${ROOT}/LICENSE"
  "${ROOT}/README.md"
  "${ROOT}/docs/operations.md"
  "${ROOT}/docs/release-packaging.md"
  "${ROOT}/docs/tls-setup.md"
  "${ROOT}/docs/database-schema.md"
)
for file in "${required_files[@]}"; do
  test -s "${file}" || { echo "missing or empty release document: ${file}" >&2; exit 1; }
done

grep -q '^MIT License$' "${ROOT}/LICENSE"
grep -q 'v0.2.2' "${ROOT}/docs/release-packaging.md"
# Keep this check tolerant of equivalent wording while still requiring the
# README to explain that first-run development certificates are automatic.
grep -Eqi 'self-signed development certificate.*(automatically|auto-generat)' "${ROOT}/README.md"
grep -q 'MOMENTUM_DEV_CERT_DIR' "${ROOT}/docs/tls-setup.md"
grep -q 'canonical SQLite' "${ROOT}/docs/database-schema.md"
grep -q 'docs/operations.md' "${ROOT}/.goreleaser.yml"
grep -q 'docs/operations.md' "${ROOT}/.github/workflows/release.yml"
grep -q 'LICENSE' "${ROOT}/.goreleaser.yml"
grep -q 'LICENSE' "${ROOT}/.github/workflows/release.yml"

# The packaged server uses IPv4 loopback by default. Keep automated health
# probes on that same address so localhost IPv6-first resolution cannot make
# a healthy service look unavailable on Windows or Linux.
grep -q 'BASE_URL="https://127.0.0.1:${PORT}"' "${ROOT}/scripts/release/integration-test.sh"
grep -q 'https://127.0.0.1:8443/health' "${ROOT}/.github/workflows/ci.yml"
grep -q 'https://127.0.0.1:8443/health' "${ROOT}/.github/workflows/release.yml"

# Keep local quick-start examples on the explicit IPv4 loopback address too.
# A localhost URL can resolve to ::1 first while the packaged default listener
# intentionally binds only 127.0.0.1, making a healthy installation appear
# unavailable on some Windows and Linux hosts.
local_docs=(
  "${ROOT}/README.md"
  "${ROOT}/cmd/server/README.md"
  "${ROOT}/docs/caldav-connection-config.md"
  "${ROOT}/docs/operations.md"
  "${ROOT}/docs/tls-setup.md"
  "${ROOT}/web/README.md"
)
for file in "${local_docs[@]}"; do
  grep -q 'https://127.0.0.1:8443' "${file}"
  if grep -q 'https://localhost:8443' "${file}"; then
    echo "stale localhost quick-start URL found in ${file}" >&2
    exit 1
  fi
done

# These are contributor-facing rules. A positive Liquibase workflow here would
# direct future changes to a schema source that is not used by the release
# binary; a prohibition that names the retired system is valid documentation.
if rg -ni 'Use Liquibase|Liquibase (changeset|changelog|migration).*(must|required|validate)|liquibase --' \
  "${ROOT}/.github/CONTRIBUTING.md" "${ROOT}/.github/PULL_REQUEST_TEMPLATE.md"; then
  echo 'stale Liquibase contributor instruction found' >&2
  exit 1
fi

echo 'Release documentation contract validated.'
