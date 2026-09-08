#!/usr/bin/env bash
# Exercise the running release binary's authenticated external CalDAV workflow
# against a real, pinned Radicale server.
#
# Usage:
#   scripts/release/runtime-sync-radicale.sh <path-to-momentum-server-binary>
#
# The test deliberately uses the public HTTP APIs and a persistent SQLite file.
# It therefore catches wiring, encryption, checkpoint, and restart regressions
# that provider-only unit tests cannot see.

set -euo pipefail

BINARY="${1:?Usage: $(basename "$0") <path-to-momentum-server-binary>}"
BINARY_ABS="$(cd "$(dirname "$BINARY")" && pwd)/$(basename "$BINARY")"
MOMENTUM_PORT="${MOMENTUM_RUNTIME_SYNC_PORT:-18454}"
RADICALE_PORT="${MOMENTUM_RADICALE_PORT:-15233}"
MOMENTUM_BASE="https://127.0.0.1:${MOMENTUM_PORT}"
RADICALE_BASE="https://127.0.0.1:${RADICALE_PORT}/alice"
TEMP_DIR="$(mktemp -d)"
DB_PATH="${TEMP_DIR}/momentum.db"
RADICALE_PID=""
MOMENTUM_PID=""
COOKIE_JAR="${TEMP_DIR}/cookies.txt"
PASS_COUNT=0

cleanup() {
	if [[ -n "${MOMENTUM_PID}" ]]; then
		kill "${MOMENTUM_PID}" 2>/dev/null || true
		wait "${MOMENTUM_PID}" 2>/dev/null || true
	fi
	if [[ -n "${RADICALE_PID}" ]]; then
		kill "${RADICALE_PID}" 2>/dev/null || true
		wait "${RADICALE_PID}" 2>/dev/null || true
	fi
	if [[ -f "${TEMP_DIR}/momentum.log" ]]; then
		cat "${TEMP_DIR}/momentum.log" >&2 || true
	fi
	if [[ -f "${TEMP_DIR}/radicale.log" ]]; then
		cat "${TEMP_DIR}/radicale.log" >&2 || true
	fi
	rm -rf "${TEMP_DIR}"
}
trap cleanup EXIT

die() {
	echo "[FAIL] $*" >&2
	exit 1
}

pass() {
	PASS_COUNT=$((PASS_COUNT + 1))
	echo "[PASS] $*"
}

# Read one dotted JSON path from stdin. Numeric path components select array
# entries, which keeps this harness independent of jq.
json_field() {
	local path="$1"
	python3 -c '
import json, sys
value = json.load(sys.stdin)
for part in sys.argv[1].split("."):
    value = value[int(part)] if isinstance(value, list) else value[part]
if value is None:
    raise SystemExit(1)
print(value)
' "${path}"
}

task_field_by_uid() {
	local uid="$1"
	local field="$2"
	python3 -c '
import json, sys
tasks = json.load(sys.stdin)
uid, field = sys.argv[1:]
for task in tasks:
    if task.get("uid") == uid:
        value = task.get(field)
        if value is None:
            raise SystemExit(1)
        print(value)
        break
else:
    raise SystemExit(1)
' "${uid}" "${field}"
}

assert_http() {
	local expected="$1"
	local body_file="$2"
	shift 2
	local code
	code="$(curl -k -sS --max-time 30 -o "${body_file}" -w '%{http_code}' "$@" || true)"
	if [[ "${code}" != "${expected}" ]]; then
		echo "Response body:" >&2
		cat "${body_file}" >&2 || true
		die "HTTP ${code}, expected ${expected}, for curl request"
	fi
}

wait_for_url() {
	local url="$1"
	shift
	local auth_args=("$@")
	for _ in $(seq 1 30); do
		if curl -k -fsS --max-time 5 "${auth_args[@]}" "${url}" >/dev/null 2>&1; then
			return 0
		fi
		sleep 1
	done
	return 1
}

start_momentum() {
	(
		cd "${TEMP_DIR}"
		exec env DB_PATH="${DB_PATH}" PORT="${MOMENTUM_PORT}" \
		MOMENTUM_DEV_MODE=1 MOMENTUM_ENCRYPTION_KEY='runtime-radicale-e2e-key' \
		"${BINARY_ABS}" >"${TEMP_DIR}/momentum.log" 2>&1
	) &
	MOMENTUM_PID=$!
	if ! wait_for_url "${MOMENTUM_BASE}/health"; then
		die "Momentum release binary did not become healthy"
	fi
}

stop_momentum() {
	if [[ -z "${MOMENTUM_PID}" ]]; then
		return
	fi
	kill "${MOMENTUM_PID}" 2>/dev/null || true
	for _ in $(seq 1 10); do
		if ! kill -0 "${MOMENTUM_PID}" 2>/dev/null; then
			MOMENTUM_PID=""
			return
		fi
		sleep 1
	done
	kill -9 "${MOMENTUM_PID}" 2>/dev/null || true
	wait "${MOMENTUM_PID}" 2>/dev/null || true
	MOMENTUM_PID=""
}

put_remote() {
	local uid="$1"
	local summary="$2"
	local etag="${3:-}"
	local body="${TEMP_DIR}/${uid}.ics"
	local headers="${TEMP_DIR}/${uid}.headers"
	cat >"${body}" <<EOF
BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Momentum//Runtime E2E//EN
BEGIN:VTODO
UID:${uid}
DTSTAMP:20260908T120000Z
LAST-MODIFIED:20260908T120000Z
SUMMARY:${summary}
STATUS:NEEDS-ACTION
END:VTODO
END:VCALENDAR
EOF
	local args=(-k -sS --max-time 30 -u alice:secret -X PUT -H 'Content-Type: text/calendar; charset=utf-8' --data-binary "@${body}" -D "${headers}" -o /dev/null -w '%{http_code}')
	if [[ -n "${etag}" ]]; then
		args+=(-H "If-Match: ${etag}")
	else
		args+=(-H 'If-None-Match: *')
	fi
	local code
	code="$(curl "${args[@]}" "${RADICALE_BASE}/tasks/${uid}.ics" || true)"
	if [[ "${code}" != "201" && "${code}" != "204" && "${code}" != "200" ]]; then
		cat "${TEMP_DIR}/radicale.log" >&2 || true
		die "Radicale PUT ${uid} returned HTTP ${code}"
	fi
	sed -n 's/^ETag:[[:space:]]*//Ip' "${headers}" | tr -d '\r' | tail -n 1
}

echo "Momentum runtime sync release-binary E2E"
echo "Binary: ${BINARY_ABS}"
echo "Momentum: ${MOMENTUM_BASE}; Radicale: ${RADICALE_BASE}"

[[ -x "${BINARY_ABS}" ]] || die "release binary is not executable: ${BINARY_ABS}"
mkdir -p "${TEMP_DIR}/collections"
openssl req -x509 -newkey rsa:2048 -nodes \
	-keyout "${TEMP_DIR}/server.key" -out "${TEMP_DIR}/server.crt" \
	-subj '/CN=127.0.0.1' -addext 'subjectAltName=IP:127.0.0.1' -days 1 \
	>/dev/null 2>&1
printf 'alice:secret\n' >"${TEMP_DIR}/users"
printf '[server]\nhosts = 127.0.0.1:%s\nssl = True\ncertificate = %s\nkey = %s\n\n[auth]\ntype = htpasswd\nhtpasswd_filename = %s\nhtpasswd_encryption = plain\n\n[rights]\ntype = owner_only\n\n[storage]\ntype = multifilesystem\nfilesystem_folder = %s\n\n[logging]\nlevel = warning\n' \
	"${RADICALE_PORT}" "${TEMP_DIR}/server.crt" "${TEMP_DIR}/server.key" \
	"${TEMP_DIR}/users" "${TEMP_DIR}/collections" >"${TEMP_DIR}/radicale.conf"
radicale --config "${TEMP_DIR}/radicale.conf" >"${TEMP_DIR}/radicale.log" 2>&1 &
RADICALE_PID=$!
wait_for_url "${RADICALE_BASE}/" -u alice:secret -X PROPFIND -H 'Depth: 0' || die "Radicale did not become ready"

MKCALENDAR_CODE="$(curl -k -sS --max-time 30 -u alice:secret -X MKCALENDAR \
	-H 'Content-Type: application/xml' \
	--data '<c:mkcalendar xmlns:c="urn:ietf:params:xml:ns:caldav"><d:set xmlns:d="DAV:"><d:prop><d:displayname>Momentum Runtime E2E</d:displayname><c:supported-calendar-component-set><c:comp name="VTODO"/></c:supported-calendar-component-set></d:prop></d:set></c:mkcalendar>' \
	-o /dev/null -w '%{http_code}' "${RADICALE_BASE}/tasks/" || true)"
[[ "${MKCALENDAR_CODE}" == "201" || "${MKCALENDAR_CODE}" == "207" || "${MKCALENDAR_CODE}" == "405" ]] || die "Radicale MKCALENDAR returned HTTP ${MKCALENDAR_CODE}"

REMOTE_IMPORT_UID='runtime-import@example.invalid'
REMOTE_IMPORT_ETAG="$(put_remote "${REMOTE_IMPORT_UID}" 'Remote import task')"
[[ -n "${REMOTE_IMPORT_ETAG}" ]] || die 'Radicale did not return an ETag for the seeded VTODO'
pass 'seeded a remote VTODO in Radicale'

start_momentum

AUTH_EMAIL="runtime-sync-${RANDOM}@example.invalid"
AUTH_BODY="${TEMP_DIR}/auth.json"
assert_http 201 "${AUTH_BODY}" -c "${COOKIE_JAR}" -X POST "${MOMENTUM_BASE}/auth/register" \
	-H 'Content-Type: application/json' -d "{\"email\":\"${AUTH_EMAIL}\",\"password\":\"runtime-password-123\"}"
pass 'registered an authenticated Momentum user'

BACKEND_JSON="${TEMP_DIR}/backend.json"
assert_http 201 "${BACKEND_JSON}" -b "${COOKIE_JAR}" -c "${COOKIE_JAR}" -X POST "${MOMENTUM_BASE}/backends" \
	-H 'Content-Type: application/json' \
	-d "{\"backend_type\":\"external_caldav\",\"name\":\"Radicale runtime E2E\",\"config\":{\"url\":\"${RADICALE_BASE}/\",\"username\":\"alice\",\"password\":\"secret\",\"skip_tls_verify\":true}}"
BACKEND_ID="$(json_field id <"${BACKEND_JSON}")" || die "external backend response had no id"
pass "created external backend ${BACKEND_ID} through the authenticated API"

run_sync() {
	local output="$1"
	assert_http 200 "${output}" -b "${COOKIE_JAR}" -X POST "${MOMENTUM_BASE}/api/sync/run?backend_id=${BACKEND_ID}"
	grep -q '"status":"complete"' "${output}" || die "sync did not complete: $(cat "${output}")"
}

TASKS_JSON="${TEMP_DIR}/tasks-import.json"
assert_http 200 "${TASKS_JSON}" -b "${COOKIE_JAR}" "${MOMENTUM_BASE}/caldav/tasks?backend_id=${BACKEND_ID}"
run_sync "${TEMP_DIR}/sync-import.json"
assert_http 200 "${TASKS_JSON}" -b "${COOKIE_JAR}" "${MOMENTUM_BASE}/caldav/tasks?backend_id=${BACKEND_ID}"
grep -q 'Remote import task' "${TASKS_JSON}" || die "remote VTODO was not imported: $(cat "${TASKS_JSON}")"
REMOTE_IMPORT_ID="$(task_field_by_uid "${REMOTE_IMPORT_UID}" id <"${TASKS_JSON}")" || die 'imported task ID was not persisted'
pass "imported remote VTODO as local task ${REMOTE_IMPORT_ID}"

LOCAL_PUSH_BODY="${TEMP_DIR}/local-push.json"
assert_http 201 "${LOCAL_PUSH_BODY}" -b "${COOKIE_JAR}" -X POST "${MOMENTUM_BASE}/api/tasks" \
	-H 'Content-Type: application/json' \
	-d "{\"backend_id\":${BACKEND_ID},\"title\":\"Local push task\",\"status\":\"NEEDS-ACTION\"}"
LOCAL_PUSH_UID="$(json_field uid <"${LOCAL_PUSH_BODY}")" || die 'local push response had no UID'
LOCAL_PUSH_ID="$(json_field id <"${LOCAL_PUSH_BODY}")" || die 'local push response had no ID'
run_sync "${TEMP_DIR}/sync-push.json"
REMOTE_PUSH_BODY="${TEMP_DIR}/remote-push.ics"
curl -k -fsS --max-time 30 -u alice:secret "${RADICALE_BASE}/tasks/${LOCAL_PUSH_UID}.ics" >"${REMOTE_PUSH_BODY}" || die 'local task was not pushed to Radicale'
grep -q 'SUMMARY:Local push task' "${REMOTE_PUSH_BODY}" || die 'pushed VTODO had the wrong summary'
pass "pushed local task ${LOCAL_PUSH_ID} to Radicale"

REMOTE_IMPORT_ETAG="$(put_remote "${REMOTE_IMPORT_UID}" 'Remote update task' "${REMOTE_IMPORT_ETAG}")"
run_sync "${TEMP_DIR}/sync-update.json"
assert_http 200 "${TASKS_JSON}" -b "${COOKIE_JAR}" "${MOMENTUM_BASE}/caldav/tasks?backend_id=${BACKEND_ID}"
grep -q 'Remote update task' "${TASKS_JSON}" || die 'remote update was not applied locally'
pass 'applied a remote VTODO update locally'

DELETE_CODE="$(curl -k -sS --max-time 30 -u alice:secret -X DELETE -H "If-Match: ${REMOTE_IMPORT_ETAG}" -o /dev/null -w '%{http_code}' "${RADICALE_BASE}/tasks/${REMOTE_IMPORT_UID}.ics" || true)"
[[ "${DELETE_CODE}" == "204" || "${DELETE_CODE}" == "200" ]] || die "Radicale DELETE returned HTTP ${DELETE_CODE}"
run_sync "${TEMP_DIR}/sync-delete.json"
assert_http 200 "${TASKS_JSON}" -b "${COOKIE_JAR}" "${MOMENTUM_BASE}/caldav/tasks?backend_id=${BACKEND_ID}"
if grep -q "${REMOTE_IMPORT_UID}" "${TASKS_JSON}"; then
	die 'remote deletion was not applied locally'
fi
pass 'applied a remote VTODO deletion locally'

STATUS_BEFORE="${TEMP_DIR}/status-before.json"
assert_http 200 "${STATUS_BEFORE}" -b "${COOKIE_JAR}" "${MOMENTUM_BASE}/api/sync/status?backend_id=${BACKEND_ID}"
CURSOR_BEFORE="$(json_field cursor <"${STATUS_BEFORE}")" || die 'sync checkpoint cursor was not persisted'
stop_momentum
start_momentum
STATUS_AFTER="${TEMP_DIR}/status-after.json"
assert_http 200 "${STATUS_AFTER}" -b "${COOKIE_JAR}" "${MOMENTUM_BASE}/api/sync/status?backend_id=${BACKEND_ID}"
CURSOR_AFTER="$(json_field cursor <"${STATUS_AFTER}")" || die 'sync checkpoint was lost after restart'
[[ "${CURSOR_BEFORE}" == "${CURSOR_AFTER}" ]] || die 'sync checkpoint changed during restart'
pass 'reloaded the durable sync checkpoint after a release-binary restart'
run_sync "${TEMP_DIR}/sync-after-restart.json"

# Create a controlled two-sided edit. The local and remote updates happen
# after the same checkpoint, so the runtime must retain a conflict instead of
# silently choosing one side.
LOCAL_CONFLICT_BODY="${TEMP_DIR}/local-conflict.json"
assert_http 200 "${LOCAL_CONFLICT_BODY}" -b "${COOKIE_JAR}" -X PUT "${MOMENTUM_BASE}/api/tasks/${LOCAL_PUSH_ID}" \
	-H 'Content-Type: application/json' -d '{"title":"Local conflict edit"}'
REMOTE_PUSH_ETAG="$(curl -k -sS --max-time 30 -u alice:secret -D "${TEMP_DIR}/local-push-current.headers" -o /dev/null -w '' "${RADICALE_BASE}/tasks/${LOCAL_PUSH_UID}.ics" >/dev/null 2>&1; sed -n 's/^ETag:[[:space:]]*//Ip' "${TEMP_DIR}/local-push-current.headers" | tr -d '\r' | tail -n 1)"
REMOTE_PUSH_ETAG="$(put_remote "${LOCAL_PUSH_UID}" 'Remote conflict edit' "${REMOTE_PUSH_ETAG}")"
run_sync "${TEMP_DIR}/sync-conflict.json"
CONFLICTS_JSON="${TEMP_DIR}/conflicts.json"
assert_http 200 "${CONFLICTS_JSON}" -b "${COOKIE_JAR}" "${MOMENTUM_BASE}/api/sync/conflicts?status=open&backend_id=${BACKEND_ID}"
CONFLICT_ID="$(json_field 0.id <"${CONFLICTS_JSON}")" || die "runtime did not list a conflict: $(cat "${CONFLICTS_JSON}")"
RESOLVED_JSON="${TEMP_DIR}/resolved.json"
assert_http 200 "${RESOLVED_JSON}" -b "${COOKIE_JAR}" -X PATCH "${MOMENTUM_BASE}/api/sync/conflicts/${CONFLICT_ID}" \
	-H 'Content-Type: application/json' -d '{"resolution":"remote"}'
grep -q '"status":"resolved"' "${RESOLVED_JSON}" || die "conflict ${CONFLICT_ID} was not resolved: $(cat "${RESOLVED_JSON}")"
assert_http 200 "${TASKS_JSON}" -b "${COOKIE_JAR}" "${MOMENTUM_BASE}/caldav/tasks?backend_id=${BACKEND_ID}"
grep -q 'Remote conflict edit' "${TASKS_JSON}" || die 'remote conflict resolution was not applied to the task'
pass "listed and resolved sync conflict ${CONFLICT_ID}"

echo "Runtime sync release-binary E2E passed (${PASS_COUNT} checks)"
