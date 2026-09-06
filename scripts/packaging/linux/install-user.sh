#!/usr/bin/env bash
# Install a Momentum release binary as a per-user Linux systemd service.
# This script never writes outside the invoking user's home directory.
#
# Usage: ./install-user.sh /path/to/momentum-server
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY="${1:?Usage: $(basename "$0") /path/to/momentum-server}"
if [[ ! -f "${BINARY}" || ! -x "${BINARY}" ]]; then
  echo "Release binary is missing or not executable: ${BINARY}" >&2
  exit 1
fi

INSTALL_DIR="${MOMENTUM_INSTALL_DIR:-${HOME}/.local/bin}"
DATA_DIR="${MOMENTUM_DATA_DIR:-${XDG_CONFIG_HOME:-${HOME}/.config}/Momentum}"
SERVICE_DIR="${XDG_CONFIG_HOME:-${HOME}/.config}/systemd/user"
TARGET="${INSTALL_DIR}/momentum-server"

umask 077
mkdir -p "${INSTALL_DIR}" "${DATA_DIR}" "${SERVICE_DIR}"
install -m 0755 "${BINARY}" "${TARGET}"
install -m 0644 "${SCRIPT_DIR}/momentum.service" "${SERVICE_DIR}/momentum.service"

if command -v systemctl >/dev/null 2>&1; then
  systemctl --user daemon-reload
  systemctl --user enable --now momentum.service
  echo "Momentum is running as the per-user service momentum.service."
else
  echo "Installed ${TARGET}; systemctl was not found. Run scripts/packaging/linux/run-momentum.sh ${TARGET}."
fi
