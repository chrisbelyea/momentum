#!/usr/bin/env bash
# Integration test script for the Momentum server release binary.
#
# Tests complete user workflows including database initialization, task
# creation, and task listing against a freshly started server instance.
#
# Usage:
#   ./scripts/release/integration-test.sh <path-to-momentum-server-binary>
#
# Environment variables:
#   PORT   HTTPS port for the test server (default: 18443, avoids conflict with 8443)

set -euo pipefail

# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------

BINARY="${1:?Usage: $(basename "$0") <path-to-momentum-server-binary>}"
# Resolve to an absolute path so we can cd away from the original directory.
BINARY_ABS="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
BINARY_DIR="$(cd "$(dirname "$1")" && pwd)"
PORT="${PORT:-18443}"
# The packaged server defaults to the explicit IPv4 loopback address. Use the
# same address here so hosts that resolve localhost to ::1 do not probe a
# listener that was intentionally not exposed on IPv6.
BASE_URL="https://127.0.0.1:${PORT}"
SERVER_READY_TIMEOUT=15   # seconds to wait for the server health check
SERVER_SHUTDOWN_WAIT=5    # seconds to wait for graceful shutdown before SIGKILL

# Create a temp directory for the database and server logs.
TEMP_DIR="$(mktemp -d)"
DB_PATH="${TEMP_DIR}/integration-test.db"
LOG_FILE="${TEMP_DIR}/server.log"
COOKIE_JAR="${TEMP_DIR}/cookies.txt"

SERVER_PID=""
TESTS_PASSED=0
TESTS_FAILED=0

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

cleanup() {
  if [ -n "${SERVER_PID:-}" ]; then
    echo "==> Stopping server (PID: ${SERVER_PID})..."
    kill "${SERVER_PID}" 2>/dev/null || true
    for i in $(seq 1 "${SERVER_SHUTDOWN_WAIT}"); do
      sleep 1
      kill -0 "${SERVER_PID}" 2>/dev/null || break
    done
    kill -9 "${SERVER_PID}" 2>/dev/null || true
  fi
  echo "==> Cleaning up temp directory: ${TEMP_DIR}"
  rm -rf "${TEMP_DIR}"
}
trap cleanup EXIT

pass() {
  echo "  [PASS] $1"
  TESTS_PASSED=$((TESTS_PASSED + 1))
}

fail() {
  echo "  [FAIL] $1"
  TESTS_FAILED=$((TESTS_FAILED + 1))
}

# ---------------------------------------------------------------------------
# Start server
# ---------------------------------------------------------------------------

echo "========================================"
echo "  Momentum Integration Tests"
echo "========================================"
echo "Binary:   ${BINARY_ABS}"
echo "Port:     ${PORT}"
echo "Database: ${DB_PATH}"
echo ""

echo "==> Starting server..."
# Run the server from the temporary directory, deliberately outside the
# repository and release archive. Templates and static assets are embedded in
# the binary and must not depend on the process' working directory.
(
  cd "${TEMP_DIR}"
  DB_PATH="${DB_PATH}" PORT="${PORT}" MOMENTUM_DEV_MODE=1 "${BINARY_ABS}" >> "${LOG_FILE}" 2>&1 &
  echo $!
) > "${TEMP_DIR}/server.pid"
SERVER_PID="$(cat "${TEMP_DIR}/server.pid")"
echo "    Server PID: ${SERVER_PID}"

# Poll the health endpoint for up to 15 seconds.
# curl -k skips TLS verification because the server uses an auto-generated
# self-signed development certificate.
echo "==> Waiting for server to be ready..."
READY=0
for i in $(seq 1 "${SERVER_READY_TIMEOUT}"); do
  sleep 1
  echo "    Attempt ${i}/${SERVER_READY_TIMEOUT}..."
  HTTP_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" "${BASE_URL}/health" 2>/dev/null || echo "000")
  if [ "${HTTP_CODE}" = "200" ]; then
    READY=1
    echo "    Server is ready!"
    break
  fi
done

if [ "${READY}" = "0" ]; then
  echo ""
  echo "==> FATAL: Server did not become ready within ${SERVER_READY_TIMEOUT} seconds."
  echo "==> Server logs:"
  cat "${LOG_FILE}"
  exit 1
fi

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

echo ""
echo "==> Running integration tests..."
echo ""

echo "--- Authentication ---"
AUTH_EMAIL="integration-${RANDOM}@example.invalid"
AUTH_BODY="$(curl -k -s -c "${COOKIE_JAR}" -X POST "${BASE_URL}/auth/register" -H "Content-Type: application/json" -d "{\"email\":\"${AUTH_EMAIL}\",\"password\":\"integration-password-123\"}")"
if echo "${AUTH_BODY}" | grep -q 'user_id'; then pass "POST /auth/register creates an authenticated session"; else fail "Registration failed: ${AUTH_BODY}"; fi
BACKENDS_BODY="$(curl -k -s -b "${COOKIE_JAR}" "${BASE_URL}/backends")"
BACKEND_ID="$(printf '%s' "${BACKENDS_BODY}" | sed -n 's/.*"id"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p')"
if [ -n "${BACKEND_ID}" ]; then pass "Authenticated user has a default backend"; else fail "No default backend: ${BACKENDS_BODY}"; fi

# --- Test: Health endpoint ---
echo "--- Health Endpoint ---"
HTTP_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" "${BASE_URL}/health" 2>/dev/null || echo "000")
if [ "${HTTP_CODE}" = "200" ]; then
  pass "/health returns 200 OK"
else
  fail "/health returned ${HTTP_CODE} (expected 200)"
fi

# --- Test: Web UI homepage loads without errors ---
echo "--- Web UI Homepage ---"
HOMEPAGE_BODY="$(mktemp)"
HTTP_CODE=$(curl -k -s -b "${COOKIE_JAR}" -o "${HOMEPAGE_BODY}" -w "%{http_code}" "${BASE_URL}/" 2>/dev/null || echo "000")
if [ "${HTTP_CODE}" = "200" ]; then
  pass "GET / returns 200 OK"
else
  fail "GET / returned ${HTTP_CODE} (expected 200). Body: $(cat "${HOMEPAGE_BODY}")"
fi
rm -f "${HOMEPAGE_BODY}"

# --- Test: PWA shell assets are embedded in the release binary ---
echo "--- PWA Assets ---"
for PWA_PATH in "/static/manifest.json" "/static/icon-192.png" "/static/icon-512.png" "/sw.js"; do
  HTTP_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" -b "${COOKIE_JAR}" "${BASE_URL}${PWA_PATH}" 2>/dev/null || echo "000")
  if [ "${HTTP_CODE}" = "200" ]; then
    pass "GET ${PWA_PATH} returns 200 OK"
  else
    fail "GET ${PWA_PATH} returned ${HTTP_CODE} (expected 200)"
  fi
done

# --- Test: Database file was created ---
echo "--- Database File ---"
if [ -f "${DB_PATH}" ]; then
  pass "Database file created at ${DB_PATH}"
else
  fail "Database file not found at ${DB_PATH}"
fi

# --- Test: Create a task ---
echo "--- Create Task ---"
CREATE_BODY="$(mktemp)"
HTTP_CODE=$(curl -k -s -o "${CREATE_BODY}" -w "%{http_code}" \
  -X POST "${BASE_URL}/caldav/tasks" -b "${COOKIE_JAR}" \
  -H "Content-Type: application/json" \
  -d '{"backend_id":'"${BACKEND_ID}"', "title": "Integration Test Task", "status": "NEEDS-ACTION"}' \
  2>/dev/null || echo "000")
CREATE_RESPONSE="$(cat "${CREATE_BODY}")"
rm -f "${CREATE_BODY}"
if [ "${HTTP_CODE}" = "201" ]; then
  pass "POST /caldav/tasks returns 201 Created"
	TASK_ID="$(printf '%s' "${CREATE_RESPONSE}" | sed -n 's/.*"id"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p')"
	if [ -z "${TASK_ID}" ]; then
		fail "POST response did not contain a task ID: ${CREATE_RESPONSE}"
	fi
else
  fail "POST /caldav/tasks returned ${HTTP_CODE} (expected 201). Body: ${CREATE_RESPONSE}"
fi

# --- Test: Update and persist the created task ---
echo "--- Update Task ---"
if [ -n "${TASK_ID:-}" ]; then
  UPDATE_BODY="$(mktemp)"
  HTTP_CODE=$(curl -k -s -o "${UPDATE_BODY}" -w "%{http_code}" \
    -X PUT "${BASE_URL}/caldav/tasks/${TASK_ID}" -b "${COOKIE_JAR}" \
    -H "Content-Type: application/json" \
    -d '{"backend_id":'"${BACKEND_ID}"', "title": "Updated Integration Task", "status": "IN-PROCESS"}' \
    2>/dev/null || echo "000")
  UPDATE_RESPONSE="$(cat "${UPDATE_BODY}")"
  rm -f "${UPDATE_BODY}"
  if [ "${HTTP_CODE}" = "200" ]; then
    pass "PUT /caldav/tasks/${TASK_ID} returns 200 OK"
  else
    fail "PUT /caldav/tasks/${TASK_ID} returned ${HTTP_CODE}. Body: ${UPDATE_RESPONSE}"
  fi

  PERSIST_BODY="$(mktemp)"
  HTTP_CODE=$(curl -k -s -o "${PERSIST_BODY}" -w "%{http_code}" \
    -b "${COOKIE_JAR}" "${BASE_URL}/caldav/tasks/${TASK_ID}" 2>/dev/null || echo "000")
  PERSIST_RESPONSE="$(cat "${PERSIST_BODY}")"
  rm -f "${PERSIST_BODY}"
  if [ "${HTTP_CODE}" = "200" ] && echo "${PERSIST_RESPONSE}" | grep -q 'Updated Integration Task'; then
    pass "Updated task persists and is readable"
  else
    fail "Updated task was not persisted. HTTP ${HTTP_CODE}; Body: ${PERSIST_RESPONSE}"
  fi
fi

# --- Test: Delete the created task ---
echo "--- Delete Task ---"
if [ -n "${TASK_ID:-}" ]; then
  HTTP_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" -X DELETE \
    -b "${COOKIE_JAR}" "${BASE_URL}/caldav/tasks/${TASK_ID}" 2>/dev/null || echo "000")
  if [ "${HTTP_CODE}" = "204" ]; then
    pass "DELETE /caldav/tasks/${TASK_ID} returns 204 No Content"
  else
    fail "DELETE /caldav/tasks/${TASK_ID} returned ${HTTP_CODE}"
  fi
fi

# --- Test: List tasks ---
echo "--- List Tasks ---"
LIST_BODY="$(mktemp)"
HTTP_CODE=$(curl -k -s -o "${LIST_BODY}" -w "%{http_code}" \
  -b "${COOKIE_JAR}" "${BASE_URL}/caldav/tasks?backend_id=${BACKEND_ID}" \
  2>/dev/null || echo "000")
LIST_RESPONSE="$(cat "${LIST_BODY}")"
rm -f "${LIST_BODY}"
if [ "${HTTP_CODE}" = "200" ]; then
  pass "GET /caldav/tasks returns 200 OK for authenticated user"
  if ! echo "${LIST_RESPONSE}" | grep -q "Updated Integration Task"; then
    pass "Deleted task is absent from task list"
  else
    fail "Deleted task remains in task list. Body: ${LIST_RESPONSE}"
  fi
else
  fail "GET /caldav/tasks returned ${HTTP_CODE} (expected 200). Body: ${LIST_RESPONSE}"
fi

# --- Test: graceful shutdown ---
echo "--- Graceful Shutdown ---"
if [ -n "${SERVER_PID:-}" ]; then
  kill "${SERVER_PID}" 2>/dev/null || true
  SHUTDOWN_OK=0
  for _ in $(seq 1 "${SERVER_SHUTDOWN_WAIT}"); do
    sleep 1
    if ! kill -0 "${SERVER_PID}" 2>/dev/null; then SHUTDOWN_OK=1; break; fi
  done
  if [ "${SHUTDOWN_OK}" = "1" ]; then
    pass "Server exits cleanly after SIGTERM"
    SERVER_PID=""
  else
    fail "Server did not exit gracefully after SIGTERM"
  fi
fi

# ---------------------------------------------------------------------------
# Results
# ---------------------------------------------------------------------------

echo ""
echo "========================================"
echo "  Results: ${TESTS_PASSED} passed, ${TESTS_FAILED} failed"
echo "========================================"
echo ""

if [ "${TESTS_FAILED}" -gt 0 ]; then
  echo "==> INTEGRATION TESTS FAILED"
  echo ""
  echo "==> Server logs:"
  cat "${LOG_FILE}"
  exit 1
else
  echo "==> INTEGRATION TESTS PASSED"
fi
