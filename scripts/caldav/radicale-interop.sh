#!/usr/bin/env bash
# Run the outbound CalDAV lifecycle against a pinned Radicale server.
#
# The script intentionally owns the server lifecycle so CI and local runs use
# the same standards-focused implementation without requiring a checked-in
# database, certificate, collection, or credential.

set -euo pipefail

PORT="${MOMENTUM_RADICALE_PORT:-15232}"
TEMP_DIR="$(mktemp -d)"
SERVER_PID=""
cleanup() {
	if [[ -n "${SERVER_PID}" ]]; then
		kill "${SERVER_PID}" 2>/dev/null || true
		wait "${SERVER_PID}" 2>/dev/null || true
	fi
	rm -rf "${TEMP_DIR}"
}
trap cleanup EXIT

mkdir -p "${TEMP_DIR}/collections"
openssl req -x509 -newkey rsa:2048 -nodes \
	-keyout "${TEMP_DIR}/server.key" -out "${TEMP_DIR}/server.crt" \
	-subj '/CN=127.0.0.1' -days 1 >/dev/null 2>&1
printf 'alice:secret\n' >"${TEMP_DIR}/users"
printf '[server]\nhosts = 127.0.0.1:%s\nssl = True\ncertificate = %s\nkey = %s\n\n[auth]\ntype = htpasswd\nhtpasswd_filename = %s\nhtpasswd_encryption = plain\n\n[rights]\ntype = owner_only\n\n[storage]\ntype = multifilesystem\nfilesystem_folder = %s\n\n[logging]\nlevel = warning\n' \
	"${PORT}" "${TEMP_DIR}/server.crt" "${TEMP_DIR}/server.key" \
	"${TEMP_DIR}/users" "${TEMP_DIR}/collections" >"${TEMP_DIR}/config"

radicale --config "${TEMP_DIR}/config" >"${TEMP_DIR}/server.log" 2>&1 &
SERVER_PID=$!

for _ in $(seq 1 30); do
	if curl -k -fsS -u alice:secret -X PROPFIND \
		-H 'Depth: 0' -H 'Content-Type: application/xml' \
		-o /dev/null "https://127.0.0.1:${PORT}/alice/"; then
		break
	fi
	sleep 1
done

if ! kill -0 "${SERVER_PID}" 2>/dev/null; then
	echo 'Radicale failed to start:' >&2
	cat "${TEMP_DIR}/server.log" >&2
	exit 1
fi

MKCALENDAR_STATUS="$(curl -k -sS -u alice:secret -X MKCALENDAR \
	-H 'Content-Type: application/xml' \
	--data '<c:mkcalendar xmlns:c="urn:ietf:params:xml:ns:caldav"><d:set xmlns:d="DAV:"><d:prop><d:displayname>Momentum Interop</d:displayname><c:supported-calendar-component-set><c:comp name="VTODO"/></c:supported-calendar-component-set></d:prop></d:set></c:mkcalendar>' \
	-o /dev/null -w '%{http_code}' "https://127.0.0.1:${PORT}/alice/tasks/")"
if [[ "${MKCALENDAR_STATUS}" != "201" && "${MKCALENDAR_STATUS}" != "207" && "${MKCALENDAR_STATUS}" != "405" ]]; then
	echo "Radicale MKCALENDAR returned HTTP ${MKCALENDAR_STATUS}" >&2
	cat "${TEMP_DIR}/server.log" >&2
	exit 1
fi

MOMENTUM_CALDAV_INTEROP_URL="https://127.0.0.1:${PORT}/alice/" \
	MOMENTUM_CALDAV_INTEROP_USER=alice \
	MOMENTUM_CALDAV_INTEROP_PASSWORD=secret \
	MOMENTUM_CALDAV_INTEROP_SKIP_TLS_VERIFY=1 \
	go test ./internal/caldav -run '^TestLiveCalDAVInterop$' -count=1
