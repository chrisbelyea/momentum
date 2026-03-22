#!/usr/bin/env bash
# Build the Momentum server single-executable and stage it under dist/server/.
#
# Usage:
#   ./scripts/release/build-server.sh
#
# Environment variables:
#   VERSION   Override the version string (default: git describe)
#   GOOS      Target OS  (default: current OS)
#   GOARCH    Target arch (default: current arch)
#
# Output:
#   dist/server/momentum-server[.exe]
#   dist/server/momentum-server[.exe].sha256

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DIST_DIR="${REPO_ROOT}/dist/server"

VERSION="${VERSION:-$(git -C "${REPO_ROOT}" describe --tags --always --dirty 2>/dev/null || echo "dev")}"
GOOS="${GOOS:-$(go env GOOS)}"
GOARCH="${GOARCH:-$(go env GOARCH)}"

BIN_NAME="momentum-server"
if [[ "${GOOS}" == "windows" ]]; then
  BIN_NAME="${BIN_NAME}.exe"
fi

echo "==> Building Momentum server ${VERSION} (${GOOS}/${GOARCH})..."

# When cross-compiling with CGO (required for go-sqlite3), the CC env var must
# point to the appropriate cross-compiler for the target platform.
CURRENT_GOOS="$(go env GOOS)"
CURRENT_GOARCH="$(go env GOARCH)"
if [[ "${GOOS}" != "${CURRENT_GOOS}" || "${GOARCH}" != "${CURRENT_GOARCH}" ]]; then
  if [[ -z "${CC:-}" ]]; then
    echo "WARNING: Cross-compiling to ${GOOS}/${GOARCH} from ${CURRENT_GOOS}/${CURRENT_GOARCH}." >&2
    echo "         go-sqlite3 requires CGO. Set the CC env var to the cross-compiler binary," >&2
    echo "         e.g. CC=aarch64-linux-gnu-gcc for linux/arm64." >&2
    echo "         Continuing — the build will fail if no suitable cross-compiler is found." >&2
  fi
fi

mkdir -p "${DIST_DIR}"
cd "${REPO_ROOT}"

CGO_ENABLED=1 GOOS="${GOOS}" GOARCH="${GOARCH}" go build \
  -ldflags "-s -w -X main.version=${VERSION}" \
  -o "${DIST_DIR}/${BIN_NAME}" \
  ./cmd/server

echo "==> Generating checksum..."
# sha256sum is standard on Linux; macOS ships shasum/openssl instead.
sha256_file() {
  local file="$1"
  if command -v sha256sum &>/dev/null; then
    sha256sum "${file}"
  elif command -v shasum &>/dev/null; then
    shasum -a 256 "${file}"
  else
    openssl dgst -sha256 "${file}" | awk -v fname="${file}" '{print $NF"  "fname}'
  fi
}
(cd "${DIST_DIR}" && sha256_file "${BIN_NAME}" > "${BIN_NAME}.sha256")

echo ""
echo "Artifacts:"
ls -lh "${DIST_DIR}/"
echo ""
echo "Checksum:"
cat "${DIST_DIR}/${BIN_NAME}.sha256"
