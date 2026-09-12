#!/usr/bin/env bash
# Remove Momentum's per-user systemd service and installed executable.
# The data directory is retained by default so an uninstall is recoverable.
# Usage: ./uninstall-user.sh [--purge-data]
set -euo pipefail

INSTALL_DIR="${MOMENTUM_INSTALL_DIR:-${HOME}/.local/bin}"
DATA_DIR="${MOMENTUM_DATA_DIR:-${XDG_CONFIG_HOME:-${HOME}/.config}/Momentum}"
SERVICE_DIR="${XDG_CONFIG_HOME:-${HOME}/.config}/systemd/user"
SERVICE_NAME="${MOMENTUM_SERVICE_NAME:-momentum.service}"
PURGE_DATA=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --purge-data)
      PURGE_DATA=1
      shift
      ;;
    --help|-h)
      echo "Usage: $(basename "$0") [--purge-data]"
      echo '  --purge-data  Also remove the database, key, certificates, and configuration.'
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 2
      ;;
  esac
done

if [[ ! "${SERVICE_NAME}" =~ ^[A-Za-z0-9_.@-]+\.service$ ]]; then
  echo "Invalid MOMENTUM_SERVICE_NAME: ${SERVICE_NAME}" >&2
  exit 2
fi

if command -v systemctl >/dev/null 2>&1 && systemctl --user show-environment >/dev/null 2>&1; then
  # A missing unit is harmless, but an existing service must be stopped before
  # its files are removed. Do not hide a real lifecycle failure.
  if systemctl --user is-enabled --quiet "${SERVICE_NAME}" 2>/dev/null ||
    systemctl --user is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
    systemctl --user disable --now "${SERVICE_NAME}"
  fi
  systemctl --user daemon-reload
fi

rm -f "${SERVICE_DIR}/${SERVICE_NAME}" "${INSTALL_DIR}/momentum-server"
rmdir "${SERVICE_DIR}" 2>/dev/null || true

if [[ "${PURGE_DATA}" == 1 ]]; then
  rm -rf -- "${DATA_DIR}"
  echo "Removed Momentum service, executable, and data from ${DATA_DIR}."
else
  echo "Removed Momentum service and executable; retained data at ${DATA_DIR}."
fi
