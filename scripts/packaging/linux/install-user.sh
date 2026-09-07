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
KEY_FILE="${MOMENTUM_ENCRYPTION_KEY_FILE:-${DATA_DIR}/encryption.key}"
ENV_FILE="${DATA_DIR}/momentum.env"

umask 077
mkdir -p "${INSTALL_DIR}" "${DATA_DIR}" "${SERVICE_DIR}"
install -m 0755 "${BINARY}" "${TARGET}"

if [[ -z "${MOMENTUM_ENCRYPTION_KEY:-}" ]]; then
	if [[ -s "${KEY_FILE}" ]]; then
		MOMENTUM_ENCRYPTION_KEY="$(<"${KEY_FILE}")"
	else
		if command -v openssl >/dev/null 2>&1; then
			MOMENTUM_ENCRYPTION_KEY="$(openssl rand -hex 32)"
		else
			MOMENTUM_ENCRYPTION_KEY="$(od -An -N32 -tx1 /dev/urandom | tr -d '[:space:]')"
		fi
		[[ -n "${MOMENTUM_ENCRYPTION_KEY}" ]] || { echo 'could not generate an encryption key' >&2; exit 1; }
		printf '%s\n' "${MOMENTUM_ENCRYPTION_KEY}" >"${KEY_FILE}"
	fi
	chmod 600 "${KEY_FILE}"
fi

quote_env_value() {
	local value="$1"
	value=${value//\\/\\\\}
	value=${value//\"/\\\"}
	printf '"%s"' "${value}"
}

{
	printf 'DB_PATH=%s\n' "$(quote_env_value "${DATA_DIR}/momentum.db")"
	printf 'PORT=%s\n' "$(quote_env_value "${PORT:-8443}")"
	printf 'MOMENTUM_ENCRYPTION_KEY=%s\n' "$(quote_env_value "${MOMENTUM_ENCRYPTION_KEY}")"
	printf 'MOMENTUM_DEV_CERT_DIR=%s\n' "$(quote_env_value "${DATA_DIR}/dev-certs")"
} >"${ENV_FILE}"
chmod 600 "${ENV_FILE}"

escape_unit_path() {
	local value="$1"
	value=${value//\\/\\x5c}
	value=${value// /\\x20}
	value=${value//$'\t'/\\x09}
	value=${value//\"/\\x22}
	printf '%s' "${value}"
}

escape_sed_value() {
	printf '%s' "$1" | sed 's/[\\&|]/\\&/g'
}
service_env_file="$(escape_sed_value "$(escape_unit_path "${ENV_FILE}")")"
service_data_dir="$(escape_sed_value "$(escape_unit_path "${DATA_DIR}")")"
service_binary="$(escape_sed_value "$(escape_unit_path "${TARGET}")")"
sed -e "s|@MOMENTUM_ENV_FILE@|${service_env_file}|g" \
	-e "s|@MOMENTUM_DATA_DIR@|${service_data_dir}|g" \
	-e "s|@MOMENTUM_BINARY@|${service_binary}|g" \
	"${SCRIPT_DIR}/momentum.service" >"${SERVICE_DIR}/momentum.service"
chmod 0644 "${SERVICE_DIR}/momentum.service"

if command -v systemctl >/dev/null 2>&1; then
  if systemctl --user daemon-reload && systemctl --user enable --now momentum.service; then
    echo "Momentum is running as the per-user service momentum.service."
  else
    echo "Installed ${TARGET}, but the user systemd service could not be started." >&2
    echo "Run: systemctl --user daemon-reload && systemctl --user enable --now momentum.service" >&2
  fi
else
  echo "Installed ${TARGET}; systemctl was not found. Run scripts/packaging/linux/run-momentum.sh ${TARGET}."
fi
