#!/usr/bin/env bash
# Validate the PWA assets shipped by the source tree and an optional archive.
#
# Usage:
#   scripts/release/validate-pwa.sh [path/to/web.tar.gz]
#
# The archive argument is intentionally optional so CI can validate both the
# checked-in source assets and the exact tarball produced by build-pwa.sh.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
ARCHIVE="${1:-}"

contains() {
  local needle="$1"
  local haystack="$2"
  grep -Fq -- "${needle}" <<<"${haystack}"
}

validate_manifest() {
  local label="$1"
  local manifest="$2"
  contains '"name": "Momentum"' "${manifest}" || { echo "${label}: manifest name is not Momentum" >&2; return 1; }
  contains '"start_url": "/"' "${manifest}" || { echo "${label}: manifest start_url must be /" >&2; return 1; }
  contains '"scope": "/"' "${manifest}" || { echo "${label}: manifest scope must be /" >&2; return 1; }
  contains '"display": "standalone"' "${manifest}" || { echo "${label}: manifest display must be standalone" >&2; return 1; }
  contains '"src": "/static/icon-192.png"' "${manifest}" || { echo "${label}: 192px icon missing" >&2; return 1; }
  contains '"sizes": "192x192"' "${manifest}" || { echo "${label}: 192px icon dimensions missing" >&2; return 1; }
  contains '"src": "/static/icon-512.png"' "${manifest}" || { echo "${label}: 512px icon missing" >&2; return 1; }
  contains '"sizes": "512x512"' "${manifest}" || { echo "${label}: 512px icon dimensions missing" >&2; return 1; }
}

validate_worker() {
  local label="$1"
  local worker="$2"
  contains 'request.method !== "GET"' "${worker}" || { echo "${label}: worker must reject non-GET requests" >&2; return 1; }
  contains 'url.origin !== self.location.origin' "${worker}" || { echo "${label}: worker must reject cross-origin requests" >&2; return 1; }
  contains '!url.pathname.startsWith("/static/")' "${worker}" || { echo "${label}: worker must restrict caching to /static/" >&2; return 1; }
  contains 'url.pathname === "/static/sw.js"' "${worker}" || { echo "${label}: worker must not cache itself" >&2; return 1; }
  contains 'self.addEventListener("fetch"' "${worker}" || { echo "${label}: fetch handler missing" >&2; return 1; }
  if contains 'caches.match(event.request)' "${worker}" || contains 'url.pathname.startsWith("/api' "${worker}"; then
    echo "${label}: worker must not cache pages or API responses" >&2
    return 1
  fi
}

validate_templates() {
  local label="$1"
  local index="$2"
  local list="$3"
  for template in "${index}" "${list}"; do
    contains '<link rel="manifest" href="/static/manifest.json">' "${template}" || {
      echo "${label}: template is missing the manifest link" >&2
      return 1
    }
    contains "navigator.serviceWorker.register('/sw.js', {scope: '/'})" "${template}" || {
      echo "${label}: template is missing root-scoped worker registration" >&2
      return 1
    }
  done
}

echo "==> Validating checked-in PWA assets"
for path in \
  "${ROOT}/web/static/manifest.json" \
  "${ROOT}/web/static/sw.js" \
  "${ROOT}/web/static/icon-192.png" \
  "${ROOT}/web/static/icon-512.png" \
  "${ROOT}/web/templates/index.html" \
  "${ROOT}/web/templates/list.html"; do
  test -f "${path}" || { echo "missing PWA asset: ${path}" >&2; exit 1; }
done
validate_manifest source "$(<"${ROOT}/web/static/manifest.json")"
validate_worker source "$(<"${ROOT}/web/static/sw.js")"
validate_templates source "$(<"${ROOT}/web/templates/index.html")" "$(<"${ROOT}/web/templates/list.html")"

if [[ -n "${ARCHIVE}" ]]; then
  if [[ ! "${ARCHIVE}" = /* ]]; then
    ARCHIVE="${ROOT}/${ARCHIVE}"
  fi
  test -f "${ARCHIVE}" || { echo "PWA archive not found: ${ARCHIVE}" >&2; exit 1; }
  echo "==> Validating PWA archive ${ARCHIVE}"
  archive_has() {
    # Do not use grep -q here: with pipefail, grep exiting early makes tar
    # report SIGPIPE and turns an otherwise valid archive into a false failure.
    tar -tzf "${ARCHIVE}" | grep -Fx -- "$1" >/dev/null
  }
  for path in \
    "web/static/manifest.json" \
    "web/static/sw.js" \
    "web/static/icon-192.png" \
    "web/static/icon-512.png" \
    "web/templates/index.html" \
    "web/templates/list.html"; do
    archive_has "${path}" || { echo "archive is missing ${path}" >&2; exit 1; }
  done
  validate_manifest archive "$(tar -xOf "${ARCHIVE}" web/static/manifest.json)"
  validate_worker archive "$(tar -xOf "${ARCHIVE}" web/static/sw.js)"
  validate_templates archive \
    "$(tar -xOf "${ARCHIVE}" web/templates/index.html)" \
    "$(tar -xOf "${ARCHIVE}" web/templates/list.html)"

  checksum="${ARCHIVE%.tar.gz}.tar.gz.sha256"
  if [[ -f "${checksum}" ]] && command -v sha256sum >/dev/null 2>&1; then
    (cd "$(dirname "${ARCHIVE}")" && sha256sum -c "$(basename "${checksum}")")
  fi
fi

echo 'PWA assets validated: manifest, icons, root worker, templates, and cache policy.'
