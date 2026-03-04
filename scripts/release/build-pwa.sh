#!/usr/bin/env bash
# Package the Momentum PWA web assets into a distributable archive under dist/web/.
#
# The Momentum web frontend is served by the Go server binary; this script
# collects the templates and static assets (manifest.json, icons, service worker)
# into dist/web/ and produces a compressed tarball for deployment alongside the
# server binary.
#
# Usage:
#   ./scripts/release/build-pwa.sh
#
# Environment variables:
#   VERSION   Override the version string (default: git describe)
#
# Output:
#   dist/web/           — unpacked web assets tree
#   dist/web.tar.gz     — compressed archive of dist/web/
#   dist/web.tar.gz.sha256

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DIST_DIR="${REPO_ROOT}/dist/web"

VERSION="${VERSION:-$(git -C "${REPO_ROOT}" describe --tags --always --dirty 2>/dev/null || echo "dev")}"

echo "==> Packaging Momentum PWA assets ${VERSION}..."

rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

# Copy Go templates (rendered server-side)
cp -r "${REPO_ROOT}/web/templates" "${DIST_DIR}/"

# Copy static assets (manifest.json, icons, service worker)
if [[ -d "${REPO_ROOT}/web/static" ]]; then
  cp -r "${REPO_ROOT}/web/static" "${DIST_DIR}/"
fi

echo "==> Creating archive..."
ARCHIVE="${REPO_ROOT}/dist/web.tar.gz"
tar -czf "${ARCHIVE}" -C "${REPO_ROOT}/dist" web

echo "==> Generating checksum..."
(cd "${REPO_ROOT}/dist" && sha256sum "web.tar.gz" > "web.tar.gz.sha256")

echo ""
echo "Artifacts:"
ls -lh "${REPO_ROOT}/dist/web.tar.gz" "${REPO_ROOT}/dist/web.tar.gz.sha256"
echo ""
echo "Web asset tree:"
find "${DIST_DIR}" -type f | sort
