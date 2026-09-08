#!/usr/bin/env bash
# Exercise the packaged Linux install, systemd lifecycle, and task workflow.
# This test requires an active per-user systemd manager (as on a lingering
# user-service install). It is intentionally separate from test-launcher.sh,
# which only validates install-only behavior in an isolated directory.
# Usage: test-systemd-service.sh /path/to/momentum-server
set -euo pipefail

BINARY="${1:?Usage: $(basename "$0") /path/to/momentum-server}"
[[ -f "${BINARY}" && -x "${BINARY}" ]] || {
  echo "binary is not executable: ${BINARY}" >&2
  exit 1
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p "${HOME}/.cache"
TEST_ROOT="$(mktemp -d "${HOME}/.cache/momentum-systemd-test.XXXXXX")"
INSTALL_DIR="${TEST_ROOT}/install"
DATA_DIR="${TEST_ROOT}/custom data"
COOKIE_JAR="${TEST_ROOT}/cookies.txt"
PORT="${MOMENTUM_SYSTEMD_TEST_PORT:-18446}"
SERVICE_NAME="momentum-test-${BASHPID}.service"
BASE_URL="https://127.0.0.1:${PORT}"

export MOMENTUM_INSTALL_DIR="${INSTALL_DIR}"
export MOMENTUM_DATA_DIR="${DATA_DIR}"
export MOMENTUM_SERVICE_NAME="${SERVICE_NAME}"
export PORT

cleanup() {
  # The uninstaller is deliberately used here so a failed test does not leave
  # an enabled service behind. Its purge is limited to this test's directory.
  "${SCRIPT_DIR}/uninstall-user.sh" --purge-data >/dev/null 2>&1 || true
  rm -rf -- "${TEST_ROOT}"
}
trap cleanup EXIT

if ! command -v systemctl >/dev/null 2>&1; then
  echo 'systemctl is required for the Linux systemd lifecycle test' >&2
  exit 1
fi

# systemctl --user talks to the manager through the per-user runtime bus. A
# missing bus means this is not the real user-systemd environment promised by
# the CI job, so fail instead of silently testing only generated files.
if ! systemctl --user show-environment >/dev/null 2>&1; then
  echo 'systemd user manager is unavailable; enable lingering and the user bus before running this test' >&2
  exit 1
fi

wait_for_health() {
  for _ in $(seq 1 30); do
    if curl -kfsS "${BASE_URL}/health" >/dev/null; then
      return 0
    fi
    sleep 1
  done
  systemctl --user status "${SERVICE_NAME}" --no-pager >&2 || true
  return 1
}

json_field() {
  local field="$1" body="$2"
  if command -v jq >/dev/null 2>&1; then
    jq -r ".${field}" <<<"${body}"
  else
    sed -n "s/.*\"${field}\"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p" <<<"${body}"
  fi
}

first_backend_id() {
  local body="$1"
  if command -v jq >/dev/null 2>&1; then
    jq -r '.[0].id // empty' <<<"${body}"
  else
    sed -n 's/.*"id"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' <<<"${body}" | head -1
  fi
}

echo '==> Installing and activating a dedicated user service'
"${SCRIPT_DIR}/install-user.sh" --activate "${BINARY}"
systemctl --user is-active --quiet "${SERVICE_NAME}"
[[ -x "${INSTALL_DIR}/momentum-server" ]] || { echo 'installed binary missing' >&2; exit 1; }
[[ -f "${DATA_DIR}/momentum.env" ]] || { echo 'service environment file missing' >&2; exit 1; }
[[ "$(stat -c '%a' "${DATA_DIR}/encryption.key")" == 600 ]] || {
  echo 'encryption key is not mode 600' >&2
  exit 1
}
wait_for_health

EMAIL="momentum-systemd-${BASHPID}@example.invalid"
PASSWORD='systemd-test-password'
echo '==> Exercising authenticated task CRUD'
REGISTER_RESPONSE="$(curl -kfsS -c "${COOKIE_JAR}" -X POST "${BASE_URL}/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}")"
BACKENDS_RESPONSE="$(curl -kfsS -b "${COOKIE_JAR}" "${BASE_URL}/backends")"
BACKEND_ID="$(first_backend_id "${BACKENDS_RESPONSE}")"
[[ "${REGISTER_RESPONSE}" == *user_id* && -n "${BACKEND_ID}" ]] || {
  echo "registration/backend setup failed: ${REGISTER_RESPONSE} ${BACKENDS_RESPONSE}" >&2
  exit 1
}

CREATE_RESPONSE="$(curl -kfsS -X POST "${BASE_URL}/caldav/tasks" -b "${COOKIE_JAR}" \
  -H 'Content-Type: application/json' \
  -d "{\"backend_id\":${BACKEND_ID},\"title\":\"systemd task\",\"status\":\"NEEDS-ACTION\"}")"
TASK_ID="$(json_field id "${CREATE_RESPONSE}")"
[[ -n "${TASK_ID}" ]] || { echo "task creation failed: ${CREATE_RESPONSE}" >&2; exit 1; }

curl -kfsS -X PUT "${BASE_URL}/caldav/tasks/${TASK_ID}" -b "${COOKIE_JAR}" \
  -H 'Content-Type: application/json' \
  -d "{\"backend_id\":${BACKEND_ID},\"title\":\"persisted systemd task\",\"status\":\"IN-PROCESS\"}" >/dev/null

echo '==> Restarting service and checking persisted session/task state'
systemctl --user restart "${SERVICE_NAME}"
wait_for_health
PERSISTED_RESPONSE="$(curl -kfsS -b "${COOKIE_JAR}" "${BASE_URL}/caldav/tasks/${TASK_ID}")"
[[ "${PERSISTED_RESPONSE}" == *'persisted systemd task'* ]] || {
  echo "task did not persist across restart: ${PERSISTED_RESPONSE}" >&2
  exit 1
}

curl -kfsS -X DELETE "${BASE_URL}/caldav/tasks/${TASK_ID}" -b "${COOKIE_JAR}" >/dev/null
LIST_RESPONSE="$(curl -kfsS -b "${COOKIE_JAR}" "${BASE_URL}/caldav/tasks?backend_id=${BACKEND_ID}")"
[[ "${LIST_RESPONSE}" != *'persisted systemd task'* ]] || {
  echo 'deleted task remains in task list' >&2
  exit 1
}

echo '==> Verifying install-only upgrade and clean uninstall'
systemctl --user stop "${SERVICE_NAME}"
! systemctl --user is-active --quiet "${SERVICE_NAME}"
"${SCRIPT_DIR}/install-user.sh" --install-only "${BINARY}"
[[ -f "${DATA_DIR}/momentum.db" && -s "${DATA_DIR}/encryption.key" ]] || {
  echo 'install-only upgrade did not preserve data' >&2
  exit 1
}
"${SCRIPT_DIR}/install-user.sh" --activate "${BINARY}"
wait_for_health
"${SCRIPT_DIR}/uninstall-user.sh"
[[ ! -e "${SERVICE_DIR:-${XDG_CONFIG_HOME:-${HOME}/.config}/systemd/user}/${SERVICE_NAME}" ]] || {
  echo 'uninstall left service unit behind' >&2
  exit 1
}
[[ ! -e "${INSTALL_DIR}/momentum-server" ]] || { echo 'uninstall left binary behind' >&2; exit 1; }
[[ -f "${DATA_DIR}/momentum.db" ]] || { echo 'uninstall unexpectedly removed data' >&2; exit 1; }

echo 'Linux systemd user-service lifecycle passed: install, health, auth/task CRUD, restart persistence, upgrade, and uninstall.'
