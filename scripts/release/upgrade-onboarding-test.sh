#!/usr/bin/env bash
# Verify first-account onboarding against a database that already contains
# the compatibility local user/backend and an existing task.
set -euo pipefail

BINARY="${1:?Usage: $(basename "$0") <path-to-momentum-server-binary>}"
BINARY_ABS="$(cd "$(dirname "$BINARY")" && pwd)/$(basename "$BINARY")"
PORT="${MOMENTUM_UPGRADE_ONBOARDING_PORT:-18460}"
BASE_URL="https://127.0.0.1:${PORT}"
TEMP_DIR="$(mktemp -d)"
DB_PATH="${TEMP_DIR}/momentum.db"
COOKIE_JAR="${TEMP_DIR}/cookies.txt"
LOG_FILE="${TEMP_DIR}/server.log"
SERVER_PID=""

cleanup() {
	if [[ -n "${SERVER_PID}" ]]; then
		kill "${SERVER_PID}" 2>/dev/null || true
		wait "${SERVER_PID}" 2>/dev/null || true
	fi
	if [[ -f "${LOG_FILE}" ]]; then
		cat "${LOG_FILE}" >&2 || true
	fi
	rm -rf "${TEMP_DIR}"
}
trap cleanup EXIT

[[ -x "${BINARY_ABS}" ]] || { echo "binary is not executable: ${BINARY_ABS}" >&2; exit 1; }

# Use Python's standard-library SQLite driver so this release gate does not
# depend on a separately installed sqlite3 CLI.
python3 - "${DB_PATH}" <<'PY'
import pathlib
import sqlite3
import sys

path = pathlib.Path(sys.argv[1])
connection = sqlite3.connect(path)
schema = pathlib.Path("internal/db/schema/init.sql").read_text(encoding="utf-8")
connection.executescript(schema)
connection.execute(
    "INSERT INTO tasks(backend_id,uid,title,status,dtstamp,created_at) VALUES(1,?,?,?,?,?)",
    ("upgrade-existing-task", "Existing pre-onboarding task", "NEEDS-ACTION", "2026-09-08T12:00:00Z", "2026-09-08T12:00:00Z"),
)
connection.commit()
connection.close()
PY

(
	cd "${TEMP_DIR}"
	env DB_PATH="${DB_PATH}" PORT="${PORT}" MOMENTUM_DEV_MODE=1 \
		MOMENTUM_ENCRYPTION_KEY='upgrade-onboarding-e2e-key' \
		"${BINARY_ABS}" >"${LOG_FILE}" 2>&1
) &
SERVER_PID=$!

for _ in $(seq 1 30); do
	if curl -k -fsS --max-time 5 "${BASE_URL}/health" >/dev/null 2>&1; then
		break
	fi
	sleep 1
done
curl -k -fsS --max-time 5 "${BASE_URL}/health" >/dev/null || {
	echo 'release binary did not become healthy' >&2
	exit 1
}

AUTH_EMAIL="upgrade-${RANDOM}@example.invalid"
AUTH_BODY="$(curl -k -fsS --max-time 30 -c "${COOKIE_JAR}" -X POST "${BASE_URL}/auth/register" \
	-H 'Content-Type: application/json' \
	-d "{\"email\":\"${AUTH_EMAIL}\",\"password\":\"upgrade-password-123\"}")"
grep -q '"user_id":1' <<<"${AUTH_BODY}" || {
	echo "first account did not adopt compatibility user: ${AUTH_BODY}" >&2
	exit 1
}

BACKENDS="$(curl -k -fsS --max-time 30 -b "${COOKIE_JAR}" "${BASE_URL}/backends")"
grep -q '"id":1' <<<"${BACKENDS}" || { echo "compatibility backend missing: ${BACKENDS}" >&2; exit 1; }
if grep -q '"id":2' <<<"${BACKENDS}"; then
	echo "onboarding created a duplicate backend: ${BACKENDS}" >&2
	exit 1
fi

TASKS="$(curl -k -fsS --max-time 30 -b "${COOKIE_JAR}" "${BASE_URL}/caldav/tasks?backend_id=1")"
grep -q 'Existing pre-onboarding task' <<<"${TASKS}" || {
	echo "existing task was not visible after onboarding: ${TASKS}" >&2
	exit 1
}

CREATE_CODE="$(curl -k -sS --max-time 30 -o /dev/null -w '%{http_code}' -b "${COOKIE_JAR}" \
	-X POST "${BASE_URL}/api/tasks" -H 'Content-Type: application/json' \
	-d '{"backend_id":1,"title":"Post-onboarding task","status":"NEEDS-ACTION"}')"
[[ "${CREATE_CODE}" == 201 ]] || { echo "post-onboarding task creation returned ${CREATE_CODE}" >&2; exit 1; }

echo 'Upgrade onboarding release-binary E2E passed: adopted user/backend, preserved task, created new task'
