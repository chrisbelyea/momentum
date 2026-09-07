#!/usr/bin/env bash
# Run the outbound CalDAV lifecycle against a pinned Nextcloud Tasks server.
#
# The CI job supplies the Nextcloud service container. This script installs the
# provider's Tasks app, creates a fresh task list, and runs the same outbound
# lifecycle used for a real external backend. A local TLS proxy is used because
# Momentum deliberately requires HTTPS for external CalDAV connections.

set -euo pipefail

NEXTCLOUD_BASE_URL="${NEXTCLOUD_BASE_URL:-http://127.0.0.1:8080}"
NEXTCLOUD_USER="${NEXTCLOUD_USER:-alice}"
NEXTCLOUD_PASSWORD="${NEXTCLOUD_PASSWORD:-secret}"
NEXTCLOUD_CALENDAR_URI="${NEXTCLOUD_CALENDAR_URI:-momentum-interop}"
NEXTCLOUD_CONTAINER_ID="${NEXTCLOUD_CONTAINER_ID:-}"
PROXY_PORT="${MOMENTUM_NEXTCLOUD_PROXY_PORT:-18443}"
TASKS_VERSION="0.17.1"
TASKS_SHA256="23941d35abe0bfbe054ae5f0156373ffef8b398f59f2435984429fda4d75f0bf"
TEMP_DIR="$(mktemp -d)"
PROXY_CONTAINER_ID=""

cleanup() {
	if [[ -n "${PROXY_CONTAINER_ID}" ]]; then
		docker rm -f "${PROXY_CONTAINER_ID}" >/dev/null 2>&1 || true
	fi
	rm -rf "${TEMP_DIR}"
}
trap cleanup EXIT

if [[ -z "${NEXTCLOUD_CONTAINER_ID}" ]]; then
	NEXTCLOUD_CONTAINER_ID="$(docker ps --format '{{.ID}} {{.Image}}' | awk '$2 ~ /^nextcloud:/ {print $1; exit}')"
fi
if [[ -z "${NEXTCLOUD_CONTAINER_ID}" ]]; then
	echo 'Nextcloud service container was not found' >&2
	docker ps -a >&2
	exit 1
fi

status_url="${NEXTCLOUD_BASE_URL}/status.php"
installed=false
for _ in $(seq 1 90); do
	if status="$(curl -fsS "${status_url}" 2>/dev/null)" && grep -Eq '"installed"[[:space:]]*:[[:space:]]*true' <<<"${status}"; then
		installed=true
		break
	fi
	sleep 2
done
if [[ "${installed}" != true ]]; then
	echo 'Nextcloud did not finish its initial installation' >&2
	docker logs "${NEXTCLOUD_CONTAINER_ID}" >&2 || true
	exit 1
fi

occ() {
	docker exec --user www-data "${NEXTCLOUD_CONTAINER_ID}" php occ "$@"
}

# The Tasks app is the provider component that makes VTODO collections
# available. Install the exact app release that is compatible with the pinned
# server image, rather than allowing the app store's current version to drift.
tasks_archive="${TEMP_DIR}/tasks.tar.gz"
curl -fsSL "https://github.com/nextcloud/tasks/releases/download/v${TASKS_VERSION}/tasks.tar.gz" -o "${tasks_archive}"
echo "${TASKS_SHA256}  ${tasks_archive}" | sha256sum -c -
docker cp "${tasks_archive}" "${NEXTCLOUD_CONTAINER_ID}:/tmp/tasks.tar.gz"
docker exec "${NEXTCLOUD_CONTAINER_ID}" sh -ec '
	rm -rf /tmp/tasks-install /var/www/html/custom_apps/tasks
	mkdir -p /tmp/tasks-install /var/www/html/custom_apps
	tar -xzf /tmp/tasks.tar.gz -C /tmp/tasks-install
	mv /tmp/tasks-install/tasks /var/www/html/custom_apps/tasks
	chown -R www-data:www-data /var/www/html/custom_apps/tasks
'
occ app:enable tasks
occ dav:create-calendar "${NEXTCLOUD_USER}" "${NEXTCLOUD_CALENDAR_URI}"

# Momentum's external-client policy requires HTTPS, including for provider
# validation. Terminate TLS locally and forward to the HTTP-only test service.
openssl req -x509 -newkey rsa:2048 -nodes \
	-keyout "${TEMP_DIR}/server.key" -out "${TEMP_DIR}/server.crt" \
	-subj '/CN=127.0.0.1' -addext 'subjectAltName=IP:127.0.0.1' -days 1 \
	>/dev/null 2>&1
cat >"${TEMP_DIR}/nginx.conf" <<EOF
events {}
http {
  access_log /dev/stdout;
  error_log /dev/stderr;
  server {
    listen ${PROXY_PORT} ssl;
    server_name 127.0.0.1;
    ssl_certificate /etc/nginx/server.crt;
    ssl_certificate_key /etc/nginx/server.key;
    location / {
      proxy_pass ${NEXTCLOUD_BASE_URL};
      proxy_set_header Host 127.0.0.1;
      proxy_set_header X-Forwarded-Proto https;
      proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    }
  }
}
EOF

PROXY_CONTAINER_ID="$(docker run -d --network host \
	-v "${TEMP_DIR}/nginx.conf:/etc/nginx/nginx.conf:ro" \
	-v "${TEMP_DIR}/server.crt:/etc/nginx/server.crt:ro" \
	-v "${TEMP_DIR}/server.key:/etc/nginx/server.key:ro" \
	nginx:1.27.1-alpine@sha256:a5127daff3d6f4606be3100a252419bfa84fd6ee5cd74d0feaca1a5068f97dcf)"
for _ in $(seq 1 30); do
	if curl -kfsS "https://127.0.0.1:${PROXY_PORT}/status.php" >/dev/null; then
		break
	fi
	sleep 1
done
if ! curl -kfsS "https://127.0.0.1:${PROXY_PORT}/status.php" >/dev/null; then
	echo 'Nextcloud TLS proxy did not become ready' >&2
	docker logs "${PROXY_CONTAINER_ID}" >&2 || true
	exit 1
fi

MOMENTUM_CALDAV_INTEROP_URL="https://127.0.0.1:${PROXY_PORT}/remote.php/dav/calendars/${NEXTCLOUD_USER}/" \
	MOMENTUM_CALDAV_INTEROP_USER="${NEXTCLOUD_USER}" \
	MOMENTUM_CALDAV_INTEROP_PASSWORD="${NEXTCLOUD_PASSWORD}" \
	MOMENTUM_CALDAV_INTEROP_SKIP_TLS_VERIFY=1 \
	go test ./internal/caldav -run '^TestLiveCalDAVInterop$' -count=1
