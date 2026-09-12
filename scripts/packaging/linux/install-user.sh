#!/usr/bin/env bash
# Install a Momentum release binary as a per-user Linux systemd service.
# This script never writes outside the invoking user's home directory.
#
# Usage: ./install-user.sh [--install-only] [--activate] /path/to/momentum-server
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_ONLY=0
ACTIVATE=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --install-only)
      INSTALL_ONLY=1
      shift
      ;;
    --activate)
      ACTIVATE=1
      shift
      ;;
    --help|-h)
      echo "Usage: $(basename "$0") [--install-only|--activate] /path/to/momentum-server"
      echo "  --install-only  Install files without contacting the user systemd manager."
      echo "  --activate      Require systemd activation and fail if the service is not active."
      exit 0
      ;;
    --)
      shift
      break
      ;;
    -* )
      echo "Unknown option: $1" >&2
      exit 2
      ;;
    *)
      break
      ;;
  esac
done

if [[ "${INSTALL_ONLY}" == 1 && "${ACTIVATE}" == 1 ]]; then
  echo 'choose only one of --install-only or --activate' >&2
  exit 2
fi
BINARY="${1:?Usage: $(basename "$0") [--install-only|--activate] /path/to/momentum-server}"
if [[ ! -f "${BINARY}" || ! -x "${BINARY}" ]]; then
  echo "Release binary is missing or not executable: ${BINARY}" >&2
  exit 1
fi

INSTALL_DIR="${MOMENTUM_INSTALL_DIR:-${HOME}/.local/bin}"
DATA_DIR="${MOMENTUM_DATA_DIR:-${XDG_CONFIG_HOME:-${HOME}/.config}/Momentum}"
SERVICE_DIR="${XDG_CONFIG_HOME:-${HOME}/.config}/systemd/user"
SERVICE_NAME="${MOMENTUM_SERVICE_NAME:-momentum.service}"
TARGET="${INSTALL_DIR}/momentum-server"
KEY_FILE="${MOMENTUM_ENCRYPTION_KEY_FILE:-${DATA_DIR}/encryption.key}"
ENV_FILE="${DATA_DIR}/momentum.env"

if [[ ! "${SERVICE_NAME}" =~ ^[A-Za-z0-9_.@-]+\.service$ ]]; then
  echo "Invalid MOMENTUM_SERVICE_NAME: ${SERVICE_NAME}" >&2
  exit 2
fi

umask 077
mkdir -p "${INSTALL_DIR}" "${DATA_DIR}" "${SERVICE_DIR}"
key_parent="$(dirname -- "${KEY_FILE}")"
mkdir -p "${key_parent}"
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
	# Unit directives expand %specifiers. Preserve literal percent signs in
	# user-selected paths (%% is systemd's escaped percent spelling).
	value=${value//%/%%}
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
	"${SCRIPT_DIR}/momentum.service" >"${SERVICE_DIR}/${SERVICE_NAME}"
chmod 0644 "${SERVICE_DIR}/${SERVICE_NAME}"

if [[ "${INSTALL_ONLY}" == 1 ]]; then
  echo "Installed ${TARGET}; activation was skipped (--install-only)."
elif command -v systemctl >/dev/null 2>&1; then
  if ! systemctl --user daemon-reload; then
    echo "Installed ${TARGET}, but systemd user-manager reload failed." >&2
    echo "Run: systemctl --user daemon-reload && systemctl --user enable --now ${SERVICE_NAME}" >&2
    exit 1
  fi
  if ! systemctl --user enable --now "${SERVICE_NAME}"; then
    echo "Installed ${TARGET}, but systemd user-service activation failed." >&2
    echo "Run: systemctl --user enable --now ${SERVICE_NAME}" >&2
    exit 1
  fi
  if ! systemctl --user is-active --quiet "${SERVICE_NAME}"; then
    echo "Installed ${TARGET}, but ${SERVICE_NAME} is not active." >&2
    echo "Inspect: systemctl --user status ${SERVICE_NAME}" >&2
    exit 1
  fi
  echo "Momentum is running as the per-user service ${SERVICE_NAME}."
else
  if [[ "${ACTIVATE}" == 1 ]]; then
    echo 'systemctl was not found; --activate cannot be satisfied.' >&2
    exit 1
  fi
  echo "Installed ${TARGET}; systemctl was not found, so activation was skipped. Run scripts/packaging/linux/run-momentum.sh ${TARGET}."
fi
