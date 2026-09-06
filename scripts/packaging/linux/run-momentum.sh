#!/usr/bin/env bash
# Launch Momentum using per-user, writable Linux paths.
#
# Usage: ./run-momentum.sh [path/to/momentum-server]
# Optional environment: MOMENTUM_DATA_DIR, MOMENTUM_BIN, PORT,
# MOMENTUM_ENCRYPTION_KEY, and MOMENTUM_DEV_MODE.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_DATA_DIR="${XDG_CONFIG_HOME:-${HOME}/.config}/Momentum"
DATA_DIR="${MOMENTUM_DATA_DIR:-${DEFAULT_DATA_DIR}}"
BINARY="${MOMENTUM_BIN:-${1:-${SCRIPT_DIR}/momentum-server}}"

if [[ ! -x "${BINARY}" ]]; then
  echo "Momentum binary is not executable: ${BINARY}" >&2
  echo "Set MOMENTUM_BIN or pass the binary path as the first argument." >&2
  exit 1
fi

umask 077
mkdir -p "${DATA_DIR}"
export DB_PATH="${DB_PATH:-${DATA_DIR}/momentum.db}"
export PORT="${PORT:-8080}"

exec "${BINARY}"
