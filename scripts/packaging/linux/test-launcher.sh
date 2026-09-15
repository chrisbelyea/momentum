#!/usr/bin/env bash
# Execute the packaged Linux launcher against an isolated writable directory.
# Usage: test-launcher.sh /path/to/momentum-server
set -euo pipefail

BINARY="${1:?Usage: $0 /path/to/momentum-server}"
[[ -f "${BINARY}" && -x "${BINARY}" ]] || { echo "binary is not executable: ${BINARY}" >&2; exit 1; }

TEMP_DIR="$(mktemp -d)"
PORT="${MOMENTUM_PACKAGING_TEST_PORT:-18445}"
PID=""
cleanup() {
	if [[ -n "${PID}" ]]; then
		kill "${PID}" 2>/dev/null || true
		wait "${PID}" 2>/dev/null || true
	fi
	rm -rf "${TEMP_DIR}"
}
trap cleanup EXIT

MOMENTUM_DATA_DIR="${TEMP_DIR}/data" \
	MOMENTUM_BIN="${BINARY}" \
	PORT="${PORT}" \
	"$(dirname "${BASH_SOURCE[0]}")/run-momentum.sh" "${BINARY}" \
	>"${TEMP_DIR}/server.log" 2>&1 &
PID=$!

for _ in $(seq 1 20); do
	if curl -kfsS "https://127.0.0.1:${PORT}/health" >/dev/null; then
		if ! command -v ss >/dev/null 2>&1; then
			echo 'ss is required to verify loopback-only binding' >&2
			exit 1
		fi
		listeners="$(ss -ltn | awk 'NR > 1 {print $4}')"
		printf '%s\n' "${listeners}" | grep -Eq "^127\\.0\\.0\\.1:${PORT}$" || {
			echo "launcher did not bind the expected loopback address (listeners: ${listeners})" >&2
			exit 1
		}
		if printf '%s\n' "${listeners}" | grep -Eq "^(0\\.0\\.0\\.0|\\[::\\]):${PORT}$"; then
			echo 'launcher unexpectedly bound a non-loopback wildcard address' >&2
			exit 1
		fi
		[[ -s "${TEMP_DIR}/data/encryption.key" ]] || { echo 'launcher did not persist encryption key' >&2; exit 1; }
		[[ -f "${TEMP_DIR}/data/momentum.db" ]] || { echo 'launcher did not create database' >&2; exit 1; }
		key_mode="$(stat -c '%a' "${TEMP_DIR}/data/encryption.key")"
		[[ "${key_mode}" == "600" ]] || { echo "encryption key mode is ${key_mode}, want 600" >&2; exit 1; }
		install_root="${TEMP_DIR}/install"
		install_data="${TEMP_DIR}/custom data"
		XDG_CONFIG_HOME="${TEMP_DIR}/config" \
			MOMENTUM_INSTALL_DIR="${install_root}/bin" \
			MOMENTUM_DATA_DIR="${install_data}" \
			"$(dirname "${BASH_SOURCE[0]}")/install-user.sh" "${BINARY}" \
			>"${TEMP_DIR}/install.log" 2>&1
		service_file="${TEMP_DIR}/config/systemd/user/momentum.service"
		[[ -x "${install_root}/bin/momentum-server" ]] || { echo 'installer did not install binary' >&2; exit 1; }
		[[ -f "${service_file}" ]] || { echo 'installer did not create user service' >&2; exit 1; }
		grep -Fq 'custom\x20data' "${service_file}" || { echo 'service did not preserve custom data path' >&2; exit 1; }
		if command -v systemd-analyze >/dev/null 2>&1; then
			systemd-analyze verify "${service_file}"
		fi
		echo 'Linux packaged launcher passed first-run health/configuration checks.'
		exit 0
	fi
	sleep 1
done

cat "${TEMP_DIR}/server.log" >&2
exit 1
