#!/usr/bin/env bash
# Launch Momentum using per-user, writable Linux paths.
#
# Usage: ./run-momentum.sh [path/to/momentum-server]
# Optional environment: MOMENTUM_DATA_DIR, MOMENTUM_BIN, PORT,
# MOMENTUM_ENCRYPTION_KEY, MOMENTUM_ENCRYPTION_KEY_FILE, and
# MOMENTUM_DEV_MODE.
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
export PORT="${PORT:-8443}"
export MOMENTUM_DEV_CERT_DIR="${MOMENTUM_DEV_CERT_DIR:-${DATA_DIR}/dev-certs}"

# A per-user installation gets a stable encryption key on first launch. The
# file is created with mode 0600 under the user-only data directory; callers
# that manage secrets themselves can provide MOMENTUM_ENCRYPTION_KEY instead.
if [[ -z "${MOMENTUM_ENCRYPTION_KEY:-}" ]]; then
	KEY_FILE="${MOMENTUM_ENCRYPTION_KEY_FILE:-${DATA_DIR}/encryption.key}"
	if [[ -s "${KEY_FILE}" ]]; then
		MOMENTUM_ENCRYPTION_KEY="$(<"${KEY_FILE}")"
	else
		mkdir -p "$(dirname "${KEY_FILE}")"
		if command -v openssl >/dev/null 2>&1; then
			MOMENTUM_ENCRYPTION_KEY="$(openssl rand -hex 32)"
		else
			MOMENTUM_ENCRYPTION_KEY="$(od -An -N32 -tx1 /dev/urandom | tr -d '[:space:]')"
		fi
		[[ -n "${MOMENTUM_ENCRYPTION_KEY}" ]] || { echo 'could not generate an encryption key' >&2; exit 1; }
		printf '%s\n' "${MOMENTUM_ENCRYPTION_KEY}" >"${KEY_FILE}"
	fi
	chmod 600 "${KEY_FILE}"
	export MOMENTUM_ENCRYPTION_KEY
fi

exec "${BINARY}"
